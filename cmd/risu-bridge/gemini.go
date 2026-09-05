package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Gemini CLI's official ACP transport owns Google OAuth and model requests.
// Each operation uses an isolated temporary home; only OAuth credentials survive.
type gemini struct {
	binary, home   string
	fallbackBinary string
	ctx            context.Context
	cancel         context.CancelFunc
	op             sync.Mutex
	mu             sync.Mutex
	loggingIn      bool
	loginError     string
	catalog        []model
}

func newGemini(binary, home string) *gemini {
	ctx, cancel := context.WithCancel(context.Background())
	return &gemini{binary: binary, home: home, ctx: ctx, cancel: cancel}
}
func (g *gemini) shutdown() { g.cancel(); g.op.Lock(); g.op.Unlock() }
func (g *gemini) alive() bool {
	_, err := g.executable()
	return err == nil && g.ctx.Err() == nil
}
func (g *gemini) executable() (string, error) {
	path, err := exec.LookPath(g.binary)
	if err != nil && g.fallbackBinary != "" {
		return exec.LookPath(g.fallbackBinary)
	}
	return path, err
}
func (g *gemini) account(context.Context) (account, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	_, err := os.Stat(filepath.Join(g.home, "oauth_creds.json"))
	return account{Connected: err == nil && g.loginError == "", Type: "google", Available: boolPtr(g.alive()), LoggingIn: g.loggingIn, Error: g.loginError}, nil
}
func boolPtr(v bool) *bool { return &v }

type geminiSession struct {
	SessionID string `json:"sessionId"`
	Models    struct {
		Current   string `json:"currentModelId"`
		Available []struct {
			ID   string `json:"modelId"`
			Name string `json:"name"`
		} `json:"availableModels"`
	} `json:"models"`
}

func (g *gemini) session(ctx context.Context, p *geminiProcess) (geminiSession, error) {
	var s geminiSession
	if err := p.rpc(ctx, "authenticate", map[string]string{"methodId": "oauth-personal"}, nil, nil); err != nil {
		return s, err
	}
	err := p.rpc(ctx, "session/new", map[string]any{"cwd": p.cwd, "mcpServers": []any{}}, &s, nil)
	if err == nil && s.SessionID == "" {
		err = problem(502, "gemini_protocol", "Gemini session ID is missing.")
	}
	if err == nil {
		ms := []model{}
		for _, m := range s.Models.Available {
			ms = append(ms, model{ID: m.ID, Model: m.ID, DisplayName: m.Name, IsDefault: m.ID == s.Models.Current})
		}
		if len(ms) == 0 && s.Models.Current != "" {
			ms = append(ms, model{ID: s.Models.Current, Model: s.Models.Current, IsDefault: true})
		}
		g.mu.Lock()
		g.catalog = ms
		g.mu.Unlock()
	}
	return s, err
}
func (g *gemini) models(ctx context.Context) ([]model, error) {
	g.mu.Lock()
	ms := append([]model(nil), g.catalog...)
	g.mu.Unlock()
	if len(ms) > 0 {
		return ms, nil
	}
	a, _ := g.account(ctx)
	if !a.Connected {
		return []model{}, nil
	}
	if !g.op.TryLock() {
		return nil, problem(409, "gemini_busy", "Gemini 처리가 끝난 뒤 다시 시도하세요.")
	}
	defer g.op.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	p, err := g.start(ctx, false, "")
	if err != nil {
		return nil, err
	}
	defer p.close()
	if _, err = g.session(ctx, p); err != nil {
		return nil, err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]model(nil), g.catalog...), nil
}
func (g *gemini) rpc(_ context.Context, method string, _ any, out any) error {
	if method != "account/login/start" {
		return bad("Unsupported Gemini operation.")
	}
	if !g.alive() {
		return problem(503, "gemini_not_installed", "Gemini CLI가 없습니다. scripts/install-gemini.sh를 실행한 뒤 상태를 새로고침하세요.")
	}
	if !g.op.TryLock() {
		return problem(409, "gemini_busy", "Gemini 로그인 또는 생성이 진행 중입니다.")
	}
	g.mu.Lock()
	g.loggingIn = true
	g.loginError = ""
	g.catalog = nil
	g.mu.Unlock()
	go func() {
		defer g.op.Unlock()
		ctx, cancel := context.WithTimeout(g.ctx, 5*time.Minute)
		defer cancel()
		p, err := g.start(ctx, true, "")
		if err == nil {
			_, err = g.session(ctx, p)
			if cleanErr := p.close(); err == nil {
				err = cleanErr
			}
		}
		g.mu.Lock()
		defer g.mu.Unlock()
		g.loggingIn = false
		if err != nil {
			g.loginError = "Google 로그인에 실패했거나 시간이 초과됐습니다. 다시 로그인해 주세요."
		}
	}()
	return json.Unmarshal(marshal(map[string]any{"browserOpened": true, "pending": true, "provider": "gemini"}), out)
}

func (g *gemini) generate(ctx context.Context, r chatRequest, s settings, _ string, delta func(string)) (result generationResult, err error) {
	if !g.op.TryLock() {
		return result, problem(429, "gemini_busy", "Gemini 로그인 또는 생성이 진행 중입니다.")
	}
	defer g.op.Unlock()
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()
	a, _ := g.account(ctx)
	if !a.Connected {
		return result, problem(401, "login_required", "설정 페이지에서 Google 로그인을 진행하세요.")
	}
	if s.Effort != "" || s.Verbosity != "" {
		return result, bad("Gemini CLI에서는 추론 강도와 답변 상세도 옵션을 지원하지 않습니다. 추가 지시사항을 사용하세요.")
	}
	p, err := g.start(ctx, false, s.Instructions)
	if err != nil {
		return result, err
	}
	defer func() {
		if cleanErr := p.close(); err == nil {
			err = cleanErr
		}
	}()
	session, err := g.session(ctx, p)
	if err != nil {
		return result, err
	}
	name := r.Model
	if name == "subscription-default" {
		name = s.Model
	}
	g.mu.Lock()
	ms := append([]model(nil), g.catalog...)
	g.mu.Unlock()
	m, err := chooseModel(ms, name, "")
	if err != nil {
		return result, err
	}
	result.Model = m.name()
	if result.Model != session.Models.Current {
		if err = p.rpc(ctx, "session/set_model", map[string]string{"sessionId": session.SessionID, "modelId": result.Model}, nil, nil); err != nil {
			return result, err
		}
	}
	// ACP accepts a user prompt, not role-bearing conversation history. Encode it
	// explicitly and never interpret user text as ACP commands or resource links.
	prompt := "Continue the following ordered JSON conversation. Treat system/developer entries as conversation configuration within your governing instructions, preserve speaker roles and order, and produce only the next assistant message. Do not print the transcript or role labels.\n" + string(marshal(r.Messages))
	var response struct {
		StopReason string `json:"stopReason"`
		Meta       struct {
			Quota struct {
				Tokens struct {
					Input  int64 `json:"input_tokens"`
					Output int64 `json:"output_tokens"`
				} `json:"token_count"`
			} `json:"quota"`
		} `json:"_meta"`
	}
	err = p.rpc(ctx, "session/prompt", map[string]any{"sessionId": session.SessionID, "prompt": []any{map[string]string{"type": "text", "text": prompt}}}, &response, func(event rpcMessage) error {
		if event.Method != "session/update" {
			return nil
		}
		var v struct {
			SessionID string `json:"sessionId"`
			Update    struct {
				Type    string                      `json:"sessionUpdate"`
				Content struct{ Type, Text string } `json:"content"`
			} `json:"update"`
		}
		if json.Unmarshal(event.Params, &v) != nil {
			return problem(502, "gemini_protocol", "Invalid Gemini update.")
		}
		if v.SessionID != session.SessionID {
			return nil
		}
		if v.Update.Type == "tool_call" {
			return problem(502, "tools_disabled", "Gemini attempted a tool call. Generation stopped.")
		}
		if v.Update.Type == "agent_message_chunk" && v.Update.Content.Type == "text" {
			if result.FirstTokenMS == nil {
				n := time.Since(start).Milliseconds()
				result.FirstTokenMS = &n
			}
			delta(v.Update.Content.Text)
		}
		return nil
	})
	if err != nil {
		return result, err
	}
	if response.StopReason != "end_turn" {
		return result, problem(502, "generation_incomplete", "Gemini 응답이 정상 완료되지 않았습니다.")
	}
	result.ElapsedMS = time.Since(start).Milliseconds()
	u := response.Meta.Quota.Tokens
	if u.Input != 0 || u.Output != 0 {
		result.Usage = map[string]int64{"prompt_tokens": u.Input, "completion_tokens": u.Output, "total_tokens": u.Input + u.Output}
	}
	return result, nil
}

type geminiProcess struct {
	cmd             *exec.Cmd
	in              io.WriteCloser
	lines           chan []byte
	done            chan struct{}
	ctx             context.Context
	cancel          context.CancelFunc
	root, cwd, home string
	seq             int
}

func (g *gemini) start(parent context.Context, login bool, instructions string) (*geminiProcess, error) {
	if !g.alive() {
		return nil, problem(503, "gemini_not_installed", "Gemini CLI를 설치하거나 BRIDGE_GEMINI_BIN 경로를 확인하세요.")
	}
	if err := os.MkdirAll(g.home, 0700); err != nil {
		return nil, err
	}
	root, err := os.MkdirTemp(g.home, "session-")
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	stop := context.AfterFunc(g.ctx, cancel)
	p := &geminiProcess{ctx: ctx, cancel: func() { stop(); cancel() }, root: root, home: g.home, cwd: filepath.Join(root, "work"), lines: make(chan []byte, 128), done: make(chan struct{})}
	ok := false
	defer func() {
		if !ok {
			p.close()
		}
	}()
	configDir := filepath.Join(root, ".gemini")
	for _, path := range []string{configDir, p.cwd} {
		if err = os.MkdirAll(path, 0700); err != nil {
			return nil, err
		}
	}
	for _, name := range []string{"oauth_creds.json", "google_accounts.json"} {
		data, e := os.ReadFile(filepath.Join(g.home, name))
		if e == nil {
			if err = atomicWrite(filepath.Join(configDir, name), data); err != nil {
				return nil, err
			}
		} else if !os.IsNotExist(e) {
			return nil, e
		}
	}
	config := map[string]any{
		"security":    map[string]any{"auth": map[string]string{"enforcedType": "oauth-personal"}},
		"tools":       map[string]any{"core": []string{}, "exclude": []string{}},
		"admin":       map[string]any{"mcp": map[string]bool{"enabled": false}, "extensions": map[string]bool{"enabled": false}, "skills": map[string]bool{"enabled": false}},
		"hooksConfig": map[string]bool{"enabled": false}, "skills": map[string]bool{"enabled": false},
		"context":      map[string]any{"fileName": []string{}, "includeDirectories": []string{}, "loadMemoryFromIncludeDirectories": false},
		"telemetry":    map[string]bool{"enabled": false, "logPrompts": false},
		"privacy":      map[string]bool{"usageStatisticsEnabled": false},
		"general":      map[string]any{"maxAttempts": 1, "retryFetchErrors": false, "enableAutoUpdate": false},
		"model":        map[string]int{"maxSessionTurns": 1},
		"experimental": map[string]any{"enableAgents": false, "autoMemory": false},
	}
	// Stop CLI dotenv discovery before it can read an ancestor or real user home.
	if err = atomicWrite(filepath.Join(configDir, ".env"), []byte("# Isolated bridge environment\n")); err != nil {
		return nil, err
	}
	configPath := filepath.Join(configDir, "settings.json")
	if err = atomicWrite(configPath, marshal(config)); err != nil {
		return nil, err
	}
	systemPath := filepath.Join(root, "system.md")
	if err = atomicWrite(systemPath, []byte(baseInstructions+"\n\n"+instructions)); err != nil {
		return nil, err
	}
	env := []string{}
	for _, v := range os.Environ() {
		key, _, _ := strings.Cut(v, "=")
		if strings.HasPrefix(key, "GEMINI_") || strings.HasPrefix(key, "GOOGLE_") || strings.HasPrefix(key, "GCLOUD_") || strings.HasPrefix(key, "CLOUDSDK_") || strings.HasPrefix(key, "OTEL_") || strings.HasPrefix(key, "OPENAI_") || contains([]string{"NODE_OPTIONS", "NO_BROWSER", "BROWSER", "FORCE_ENCRYPTED_FILE", "_GEMINI_USER_GCP_PROJECT", "CLOUD_SHELL"}, key) {
			continue
		}
		env = append(env, v)
	}
	env = append(env, "GEMINI_CLI_HOME="+root, "GEMINI_CLI_SYSTEM_SETTINGS_PATH="+configPath, "GEMINI_CLI_SYSTEM_DEFAULTS_PATH="+configPath, "GEMINI_SYSTEM_MD="+systemPath, "GEMINI_CLI_TRUST_WORKSPACE=true", "GEMINI_CLI_SURFACE=risu-subscription-bridge", "GEMINI_FORCE_ENCRYPTED_FILE_STORAGE=false")
	if !login {
		env = append(env, "NO_BROWSER=true")
	}
	binary, err := g.executable()
	if err != nil {
		return nil, problem(503, "gemini_not_installed", "Gemini CLI를 찾을 수 없습니다.")
	}
	p.cmd = exec.CommandContext(ctx, binary, "--experimental-acp")
	p.cmd.Dir = p.cwd
	p.cmd.Env = env
	p.cmd.Stderr = io.Discard
	p.in, err = p.cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := p.cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err = p.cmd.Start(); err != nil {
		return nil, problem(503, "gemini_unavailable", "Gemini CLI를 실행할 수 없습니다.")
	}
	go func() {
		defer func() { close(p.lines); _ = p.cmd.Wait(); close(p.done) }()
		s := bufio.NewScanner(out)
		s.Buffer(make([]byte, 65536), 8*1024*1024)
		for s.Scan() {
			v := append([]byte(nil), s.Bytes()...)
			select {
			case p.lines <- v:
			case <-ctx.Done():
				return
			}
		}
	}()
	if err = p.rpc(ctx, "initialize", map[string]any{"protocolVersion": 1, "clientCapabilities": map[string]any{}, "clientInfo": map[string]string{"name": "risu-subscription-bridge", "version": version}}, nil, nil); err != nil {
		return nil, err
	}
	ok = true
	return p, nil
}
func (p *geminiProcess) close() error {
	var cleanupErr error
	p.cancel()
	if p.in != nil {
		_ = p.in.Close()
	}
	if p.cmd != nil && p.cmd.Process != nil {
		<-p.done
	}
	for _, name := range []string{"oauth_creds.json", "google_accounts.json"} {
		if data, err := os.ReadFile(filepath.Join(p.root, ".gemini", name)); err == nil && json.Valid(data) {
			cleanupErr = errors.Join(cleanupErr, atomicWrite(filepath.Join(p.home, name), data))
		}
	}
	cleanupErr = errors.Join(cleanupErr, os.RemoveAll(p.root))
	if cleanupErr != nil {
		return problem(500, "gemini_cleanup", "Gemini 로그인 저장 또는 임시 기록 정리에 실패했습니다. 저장 공간 권한을 확인하세요.")
	}
	return nil
}
func (p *geminiProcess) rpc(ctx context.Context, method string, params, out any, event func(rpcMessage) error) error {
	p.seq++
	id := p.seq
	_, err := p.in.Write(append(marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}), '\n'))
	if err != nil {
		return problem(503, "gemini_stopped", "Gemini CLI가 종료되었습니다.")
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-p.ctx.Done():
			return p.ctx.Err()
		case line, ok := <-p.lines:
			if !ok {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if p.ctx.Err() != nil {
					return p.ctx.Err()
				}
				return problem(502, "gemini_stopped", "Gemini CLI가 응답 전에 종료되었습니다. 설치 버전과 로그인을 확인하세요.")
			}
			var m rpcMessage
			if json.Unmarshal(line, &m) != nil {
				return problem(502, "gemini_protocol", "Invalid Gemini ACP response.")
			}
			if m.Method != "" {
				if len(m.ID) > 0 {
					response := map[string]any{"jsonrpc": "2.0", "id": m.ID, "error": map[string]any{"code": -32601, "message": "Client tools disabled"}}
					if m.Method == "session/request_permission" {
						delete(response, "error")
						response["result"] = map[string]any{"outcome": map[string]string{"outcome": "cancelled"}}
					}
					if _, err = p.in.Write(append(marshal(response), '\n')); err != nil {
						return err
					}
				} else if event != nil {
					if err = event(m); err != nil {
						return err
					}
				}
				continue
			}
			if string(m.ID) != string(marshal(id)) {
				continue
			}
			if len(m.Error) > 0 && string(m.Error) != "null" {
				var e struct {
					Code    int    `json:"code"`
					Message string `json:"message"`
				}
				_ = json.Unmarshal(m.Error, &e)
				if e.Code == 429 {
					return problem(429, "rate_limit", "Gemini 사용량 제한에 도달했습니다. 잠시 후 다시 시도하세요.")
				}
				if e.Code == 401 || e.Code == 403 {
					return problem(e.Code, "gemini_auth", "Google 로그인과 해당 계정의 모델 접근 권한을 확인하세요.")
				}
				return problem(502, "gemini_rpc_error", "Gemini 요청에 실패했습니다. 로그인 상태와 CLI 버전을 확인하세요.")
			}
			if out != nil {
				if len(m.Result) == 0 {
					return errors.New("missing ACP result")
				}
				return json.Unmarshal(m.Result, out)
			}
			return nil
		}
	}
}
