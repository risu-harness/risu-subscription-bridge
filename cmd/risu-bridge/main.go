package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var version = "0.2.3"

func randomKey() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func isQuota(s string) bool {
	s = strings.ToLower(s)
	return strings.Contains(s, "limit") || strings.Contains(s, "quota") || strings.Contains(s, "usage")
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "브리지:", err)
		os.Exit(1)
	}
}
func run() error {
	debug := flag.Bool("debug", false, "ChatGPT 요청 전문을 audit-latest.json에 저장 (최신 1건)")
	restart := flag.Bool("restart", false, "기존 브리지 종료 후 같은 포트로 재시작")
	showVersion := flag.Bool("version", false, "버전 표시")
	flag.Parse()
	if *showVersion {
		fmt.Println("risu-bridge", version)
		return nil
	}
	if flag.NArg() > 0 {
		return errors.New("지원하지 않는 옵션입니다. --help를 확인하세요.")
	}
	action := os.Getenv("BRIDGE_ACTION")
	if *restart {
		action = "restart"
	}
	if action != "" && !contains([]string{"reuse", "stop", "restart"}, action) {
		return errors.New("BRIDGE_ACTION: reuse, stop, restart")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	data := os.Getenv("BRIDGE_DATA_DIR")
	if data == "" {
		data = filepath.Join(home, ".local/share/risu-subscription-bridge/data")
	}
	data, err = filepath.Abs(data)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(data, 0700); err != nil {
		return err
	}
	if err = os.Chmod(data, 0700); err != nil {
		return err
	}
	port := 8787
	explicit := os.Getenv("BRIDGE_PORT") != ""
	if explicit {
		port, err = strconv.Atoi(os.Getenv("BRIDGE_PORT"))
		if err != nil || port < 1024 || port > 65535 {
			return errors.New("Invalid BRIDGE_PORT")
		}
	}
	tokenBytes, err := os.ReadFile(filepath.Join(data, "bridge-key"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	token := strings.TrimSpace(string(tokenBytes))
	existing := findInstance(data, token, port)
	if existing != 0 {
		if action == "" {
			action = chooseAction()
		}
		if action == "reuse" {
			openSetup(existing, token)
			return nil
		}
		if err = stopInstance(existing, token); err != nil {
			return err
		}
		if action == "stop" {
			fmt.Println("브리지를 종료했습니다.")
			return nil
		}
		port = existing
		explicit = true
	} else if action == "stop" {
		fmt.Println("실행 중인 브리지가 없습니다.")
		return nil
	}
	// Compatible with the previous Node instance lock; held until process exit.
	hash := sha256.Sum256([]byte(data))
	guard, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", 30000+binary.BigEndian.Uint32(hash[:4])%20000))
	if err != nil {
		return errors.New("다른 브리지가 시작 중이거나 인스턴스 잠금이 사용 중입니다. 잠시 후 다시 실행하세요.")
	}
	defer guard.Close()
	go func() {
		for {
			c, e := guard.Accept()
			if e != nil {
				return
			}
			c.Close()
		}
	}()
	if token == "" {
		token = randomKey()
		if err = atomicWrite(filepath.Join(data, "bridge-key"), []byte(token)); err != nil {
			return err
		}
	}
	var listener net.Listener
	for {
		listener, err = net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			break
		}
		if explicit || port >= 8799 {
			return err
		}
		port++
	}
	defer listener.Close()
	codexHome := filepath.Join(data, "codex")
	cwd := filepath.Join(data, "work")
	for _, dir := range []string{codexHome, cwd} {
		if err = os.MkdirAll(dir, 0700); err != nil {
			return err
		}
	}
	disabled := []string{"shell_tool", "unified_exec", "code_mode", "code_mode_host", "js_repl", "apps", "connectors", "plugins", "multi_agent", "collab", "memory_tool", "memories", "remote_control", "browser_use", "computer_use", "image_generation", "view_image", "apply_patch_freeform", "hooks", "codex_hooks", "plugin_hooks", "tool_search", "skill_search"}
	config := "model_provider = \"openai\"\nforced_login_method = \"chatgpt\"\nsandbox_mode = \"read-only\"\napproval_policy = \"never\"\nweb_search = \"disabled\"\nproject_doc_max_bytes = 0\ninclude_environment_context = false\ninclude_apps_instructions = false\ninclude_collaboration_mode_instructions = false\ncli_auth_credentials_store = \"file\"\n[history]\npersistence = \"none\"\n[analytics]\nenabled = false\n[features]\n"
	for _, k := range disabled {
		config += k + " = false\n"
	}
	if err = atomicWrite(filepath.Join(codexHome, "config.toml"), []byte(config)); err != nil {
		return err
	}
	env := []string{}
	for _, e := range os.Environ() {
		key, _, _ := strings.Cut(e, "=")
		if !contains([]string{"CODEX_HOME", "OPENAI_API_KEY", "OPENAI_BASE_URL", "OPENAI_ORG_ID", "OPENAI_ORGANIZATION", "CODEX_API_KEY", "CODEX_ACCESS_TOKEN"}, key) {
			env = append(env, e)
		}
	}
	env = append(env, "CODEX_HOME="+codexHome)
	codexBin := os.Getenv("BRIDGE_CODEX_BIN")
	if codexBin == "" {
		codexBin, err = exec.LookPath("codex")
		if err != nil {
			codexBin = "/Applications/ChatGPT.app/Contents/Resources/codex"
		}
	}
	api := &lazyCodex{binary: codexBin, cwd: cwd, env: env}
	defer api.shutdown()
	geminiBin := os.Getenv("BRIDGE_GEMINI_BIN")
	if geminiBin == "" {
		geminiBin = filepath.Join(filepath.Dir(data), "gemini-cli/bin/gemini")
	}
	geminiAPI := newGemini(geminiBin, filepath.Join(data, "gemini"))
	if os.Getenv("BRIDGE_GEMINI_BIN") == "" {
		geminiAPI.fallbackBinary = "gemini"
	}
	defer geminiAPI.shutdown()
	b := &bridge{debug: *debug, api: api, providers: map[string]backend{"chatgpt": api, "gemini": geminiAPI}, token: token, port: strconv.Itoa(port), runtime: data, cwd: cwd, settings: settings{Model: "subscription-default"}, origins: []string{"https://risuai.xyz", "https://risuai.net", "tauri://localhost", "http://tauri.localhost"}}
	provider := "chatgpt"
	if saved, e := os.ReadFile(filepath.Join(data, "provider.json")); e == nil {
		if json.Unmarshal(saved, &provider) != nil {
			return errors.New("저장된 AI 연결 설정을 읽을 수 없습니다.")
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	if err = b.selectProvider(provider); err != nil {
		return err
	}
	if origins := os.Getenv("BRIDGE_ALLOWED_ORIGINS"); origins != "" {
		b.origins = nil
		for _, s := range strings.Split(origins, ",") {
			s = strings.TrimSpace(s)
			if s != "" && s != "*" {
				b.origins = append(b.origins, s)
			}
		}
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer cancel()
	b.stop = cancel
	server := &http.Server{Handler: b, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, BaseContext: func(net.Listener) context.Context { return ctx }}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	openSetup(port, token)
	select {
	case <-ctx.Done():
		api.shutdown()
		_ = server.Close()
		return nil
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
func openSetup(port int, token string) {
	url := fmt.Sprintf("http://127.0.0.1:%d/#key=%s", port, token)
	fmt.Printf("Risu Bridge %s · ChatGPT / Gemini\n설정 페이지: %s\n이 링크에는 로컬 연결 키가 포함되어 있습니다. 터미널을 열어 두세요.\n", version, url)
	if runtime.GOOS == "darwin" && os.Getenv("BRIDGE_OPEN_BROWSER") != "0" {
		cmd := exec.Command("/usr/bin/open", url)
		if cmd.Start() == nil {
			go cmd.Wait()
		}
	}
}
func findInstance(data, token string, requested int) int {
	if token == "" {
		return 0
	}
	ports := []int{requested}
	for p := 8787; p <= 8799; p++ {
		if p != requested {
			ports = append(ports, p)
		}
	}
	results := make(chan int, len(ports))
	for _, p := range ports {
		go func(p int) {
			client := http.Client{Timeout: 3 * time.Second}
			req, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/internal/status", p), nil)
			req.Header.Set("Authorization", "Bearer "+token)
			resp, e := client.Do(req)
			if e != nil {
				results <- 0
				return
			}
			defer resp.Body.Close()
			var s struct {
				Runtime string `json:"runtime"`
			}
			if resp.StatusCode == 200 && json.NewDecoder(io.LimitReader(resp.Body, 1024*1024)).Decode(&s) == nil && filepath.Clean(s.Runtime) == data {
				results <- p
			} else {
				results <- 0
			}
		}(p)
	}
	found := 0
	for range ports {
		if p := <-results; p != 0 && (found == 0 || p == requested) {
			found = p
		}
	}
	return found
}
func stopInstance(port int, token string) error {
	client := http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest("POST", fmt.Sprintf("http://127.0.0.1:%d/internal/stop", port), strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return errors.New("기존 브리지를 종료하지 못했습니다. 해당 터미널에서 Ctrl+C 후 다시 실행하세요.")
	}
	for i := 0; i < 100; i++ {
		l, e := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", port))
		if e == nil {
			l.Close()
			time.Sleep(100 * time.Millisecond)
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("기존 브리지가 아직 종료되지 않았습니다.")
}
func chooseAction() string {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return "reuse"
	}
	defer tty.Close()
	fmt.Fprint(tty, "\n1) 재사용 (기본값)\n2) 종료\n3) 최신 버전으로 재시작\n종료·재시작 시 진행 중인 응답은 중단됩니다.\n선택 [1/2/3]: ")
	s := bufio.NewScanner(tty)
	if s.Scan() {
		switch strings.TrimSpace(s.Text()) {
		case "2":
			return "stop"
		case "3":
			return "restart"
		}
	}
	return "reuse"
}
