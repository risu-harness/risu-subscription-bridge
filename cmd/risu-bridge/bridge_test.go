package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNormalizeAndStop(t *testing.T) {
	var raw map[string]json.RawMessage
	_ = json.Unmarshal([]byte(`{"messages":[{"role":"system","content":"설정"},{"role":"assistant","name":"서우","content":[{"type":"text","text":"안녕"}]}],"temperature":0.8}`), &raw)
	r, err := normalize(raw)
	if err != nil || len(r.Messages) != 2 || r.Messages[1].Name != "서우" || r.Ignored[0] != "temperature" {
		t.Fatalf("%+v %v", r, err)
	}
	for _, body := range []string{`{}`, `{"messages":[{"role":"tool","content":"x"}]}`, `{"messages":[{"role":"user","content":null}]}`, `{"messages":[{"role":"user","content":[{"type":"image_url"}]}]}`, `{"messages":[{"role":"user","content":"x"}],"stream":null}`, `{"messages":[{"role":"user","content":"x"}],"n":2}`} {
		raw = nil
		_ = json.Unmarshal([]byte(body), &raw)
		if _, err = normalize(raw); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
	f := stopFilter{Stops: []string{"<끝>"}}
	out := f.Push("안녕<", false) + f.Push("끝", false) + f.Push(">무시", false) + f.Push("", true)
	if out != "안녕" || !f.Stopped {
		t.Fatal(out)
	}
	f = stopFilter{Stops: []string{"END"}}
	if f.Push("aEN", false)+f.Push("", true) != "aEN" {
		t.Fatal("lost trailing marker")
	}
}

type fakeBackend struct {
	run func(context.Context, chatRequest, settings, func(string)) (generationResult, error)
}

func (f *fakeBackend) alive() bool                              { return true }
func (f *fakeBackend) account(context.Context) (account, error) { return account{Connected: true}, nil }
func (f *fakeBackend) models(context.Context) ([]model, error) {
	return []model{{ID: "test-model", IsDefault: true}}, nil
}
func (f *fakeBackend) rpc(_ context.Context, _ string, _ any, out any) error {
	if out != nil {
		return json.Unmarshal([]byte(`{"authUrl":"https://auth.openai.com/test"}`), out)
	}
	return nil
}
func (f *fakeBackend) generate(c context.Context, r chatRequest, s settings, _ string, d func(string)) (generationResult, error) {
	if f.run != nil {
		return f.run(c, r, s, d)
	}
	d("hello")
	return generationResult{Model: "test-model", Usage: map[string]int64{"total_tokens": 3}}, nil
}
func testBridge(t *testing.T, f *fakeBackend) *bridge {
	t.Helper()
	return &bridge{api: f, token: "secret", port: "8787", runtime: t.TempDir(), settings: settings{Model: "subscription-default"}, origins: []string{"tauri://localhost"}}
}
func request(b *bridge, path, body, origin, token string) *httptest.ResponseRecorder {
	method := "POST"
	if body == "" {
		method = "GET"
	}
	r := httptest.NewRequest(method, "http://127.0.0.1:8787"+path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	b.ServeHTTP(w, r)
	return w
}

const chatBody = `{"messages":[{"role":"user","content":"hi"}]}`

func TestHTTPContracts(t *testing.T) {
	b := testBridge(t, &fakeBackend{})
	for _, tc := range []struct {
		path, body, origin, key string
		status                  int
	}{{"/healthz", "", "", "", 200}, {"/v1/models", "", "", "", 401}, {"/v1/models", "", "https://evil.example", "secret", 403}, {"/internal/status", "", "tauri://localhost", "secret", 403}, {"/v1/models", "", "tauri://localhost", "secret", 200}, {"/v1/chat/completions", chatBody, "", "secret", 200}, {"/v1/chat/completions", "{", "", "secret", 400}, {"/v1/chat/completions", `{"messages":[]}`, "", "secret", 400}, {"/v1/chat/completions", `{"x":"` + strings.Repeat("a", 2*1024*1024) + `"}`, "", "secret", 413}} {
		w := request(b, tc.path, tc.body, tc.origin, tc.key)
		if w.Code != tc.status {
			t.Fatalf("%s got %d: %s", tc.path, w.Code, w.Body.String())
		}
	}
	r := httptest.NewRequest("GET", "http://evil.example:8787/healthz", nil)
	w := httptest.NewRecorder()
	b.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("host accepted")
	}
	w = request(b, "/v1/chat/completions", `{"messages":[{"role":"user","content":"hi"}],"stream":true,"stream_options":{"include_usage":true}}`, "", "secret")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "[DONE]") || !strings.Contains(w.Body.String(), `"usage"`) {
		t.Fatal(w.Body.String())
	}
}
func TestStopErrorsSettings(t *testing.T) {
	b := testBridge(t, &fakeBackend{run: func(ctx context.Context, _ chatRequest, _ settings, d func(string)) (generationResult, error) {
		d("helloST")
		d("OPprivate")
		return generationResult{}, ctx.Err()
	}})
	w := request(b, "/v1/chat/completions", `{"messages":[{"role":"user","content":"hi"}],"stop":"STOP"}`, "", "secret")
	if w.Code != 200 || strings.Contains(w.Body.String(), "private") || !strings.Contains(w.Body.String(), "hello") {
		t.Fatal(w.Body.String())
	}
	b.api = &fakeBackend{run: func(context.Context, chatRequest, settings, func(string)) (generationResult, error) {
		return generationResult{}, problem(429, "rate_limit", "Wait.")
	}}
	w = request(b, "/v1/chat/completions", `{"messages":[{"role":"user","content":"hi"}],"stream":true}`, "", "secret")
	if w.Code != 429 || strings.Contains(w.Body.String(), "[DONE]") {
		t.Fatal(w.Body.String())
	}
	b.api = &fakeBackend{run: func(_ context.Context, _ chatRequest, _ settings, d func(string)) (generationResult, error) {
		d("partial")
		return generationResult{}, fmt.Errorf("private internal details")
	}}
	w = request(b, "/v1/chat/completions", `{"messages":[{"role":"user","content":"hi"}],"stream":true}`, "", "secret")
	if strings.Contains(w.Body.String(), "[DONE]") || strings.Contains(w.Body.String(), "private internal") {
		t.Fatal(w.Body.String())
	}
	b.api = &fakeBackend{}
	w = request(b, "/internal/settings", `{"model":"subscription-default","effort":"","verbosity":"low","instructions":"한국어"}`, "", "secret")
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	data, err := os.ReadFile(filepath.Join(b.runtime, "generation-settings.json"))
	if err != nil || !strings.Contains(string(data), "한국어") {
		t.Fatal(err)
	}
	w = request(b, "/internal/settings", `{"model":"unknown","effort":"","verbosity":"","instructions":""}`, "", "secret")
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
}
func TestBusyAndCancellation(t *testing.T) {
	entered := make(chan struct{})
	b := testBridge(t, &fakeBackend{run: func(ctx context.Context, _ chatRequest, _ settings, _ func(string)) (generationResult, error) {
		close(entered)
		<-ctx.Done()
		return generationResult{}, ctx.Err()
	}})
	ctx, cancel := context.WithCancel(context.Background())
	r := httptest.NewRequest("POST", "http://127.0.0.1:8787/v1/chat/completions", strings.NewReader(chatBody)).WithContext(ctx)
	r.Header.Set("Authorization", "Bearer secret")
	r.Header.Set("Content-Type", "application/json")
	done := make(chan struct{})
	go func() { b.ServeHTTP(httptest.NewRecorder(), r); close(done) }()
	<-entered
	w := request(b, "/v1/chat/completions", chatBody, "", "secret")
	if w.Code != 429 {
		t.Fatal(w.Code)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancel not propagated")
	}
	if !b.claim() {
		t.Fatal("busy leaked")
	}
	b.release()
}
func TestMain(m *testing.M) {
	if os.Getenv("RISU_FAKE_CODEX") == "1" {
		fakeCodexProcess()
		os.Exit(0)
	}
	os.Exit(m.Run())
}
func fakeCodexProcess() {
	s := bufio.NewScanner(os.Stdin)
	s.Buffer(make([]byte, 65536), 4*1024*1024)
	for s.Scan() {
		var m rpcMessage
		_ = json.Unmarshal(s.Bytes(), &m)
		if len(m.ID) == 0 {
			continue
		}
		var result any = map[string]any{}
		switch m.Method {
		case "account/read":
			result = map[string]any{"account": map[string]string{"type": "chatgpt", "planType": "plus"}}
		case "model/list":
			result = map[string]any{"data": []any{map[string]any{"id": "test-model", "isDefault": true}}}
		case "thread/start":
			var p struct {
				Ephemeral                                 bool
				Sandbox, ApprovalPolicy, BaseInstructions string
			}
			_ = json.Unmarshal(m.Params, &p)
			if !p.Ephemeral || p.Sandbox != "read-only" || p.ApprovalPolicy != "never" || !strings.Contains(p.BaseInstructions, "Never execute commands") {
				os.Exit(8)
			}
			result = map[string]any{"thread": map[string]string{"id": "t1"}}
		case "thread/inject_items":
			if path := os.Getenv("RISU_INPUT_MARKER"); path != "" {
				_ = os.WriteFile(path, m.Params, 0600)
			}
			if os.Getenv("RISU_INJECT_FAIL") == "1" {
				fmt.Println(string(marshal(map[string]any{"id": m.ID, "error": map[string]any{"code": -32602, "message": "unsupported"}})))
				continue
			}
		case "thread/unsubscribe":
			if path := os.Getenv("RISU_CLEANUP_MARKER"); path != "" {
				_ = os.WriteFile(path, []byte("unsubscribed"), 0600)
			}
		case "turn/start":
			if path := os.Getenv("RISU_INPUT_MARKER"); path != "" {
				_ = os.WriteFile(path+".turn", m.Params, 0600)
			}
			fmt.Println(string(marshal(map[string]any{"method": "item/agentMessage/delta", "params": map[string]string{"threadId": "t1", "delta": "안녕"}})))
			fmt.Println(string(marshal(map[string]any{"method": "thread/tokenUsage/updated", "params": map[string]any{"threadId": "t1", "tokenUsage": map[string]any{"last": map[string]int{"inputTokens": 10, "outputTokens": 2, "totalTokens": 12}}}})))
			fmt.Println(string(marshal(map[string]any{"method": "turn/completed", "params": map[string]any{"threadId": "t1", "turn": map[string]string{"id": "u1", "status": "completed"}}})))
			result = map[string]any{"turn": map[string]string{"id": "u1"}}
		}
		fmt.Println(string(marshal(map[string]any{"id": m.ID, "result": result})))
	}
}
func TestCodexSubprocess(t *testing.T) {
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "cleanup")
	c, err := startCodex(bin, t.TempDir(), append(os.Environ(), "RISU_FAKE_CODEX=1", "RISU_CLEANUP_MARKER="+marker))
	if err != nil {
		t.Fatal(err)
	}
	defer c.shutdown()
	text := ""
	result, err := c.generate(context.Background(), chatRequest{Model: "subscription-default", Messages: []message{{Role: "user", Content: "hi"}}}, settings{Model: "subscription-default"}, t.TempDir(), func(s string) { text += s })
	if err != nil || text != "안녕" || result.Usage["total_tokens"] != 12 {
		t.Fatalf("%+v %q %v", result, text, err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("thread was not unsubscribed", err)
	}
	c.shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if c.rpc(ctx, "test", nil, nil) == nil {
		t.Fatal("dead harness accepted")
	}
}
