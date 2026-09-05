package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fakeGeminiMain() {
	s := bufio.NewScanner(os.Stdin)
	for s.Scan() {
		var m rpcMessage
		_ = json.Unmarshal(s.Bytes(), &m)
		result := any(map[string]any{})
		switch m.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": 1}
		case "authenticate":
			_ = os.WriteFile(filepath.Join(os.Getenv("GEMINI_CLI_HOME"), ".gemini", "oauth_creds.json"), []byte(`{"refresh_token":"fixture"}`), 0600)
		case "session/new":
			result = map[string]any{"sessionId": "s1", "models": map[string]any{"currentModelId": "gemini-test", "availableModels": []any{map[string]string{"modelId": "gemini-test", "name": "Gemini test"}}}}
		case "session/prompt":
			if file := os.Getenv("RISU_GEMINI_MARKER"); file != "" {
				config, _ := os.ReadFile(filepath.Join(os.Getenv("GEMINI_CLI_HOME"), ".gemini", "settings.json"))
				_ = os.WriteFile(file, marshal(map[string]any{"params": m.Params, "config": json.RawMessage(config), "apiKey": os.Getenv("GEMINI_API_KEY"), "system": os.Getenv("GEMINI_SYSTEM_MD"), "root": os.Getenv("GEMINI_CLI_HOME")}), 0600)
			}
			if os.Getenv("RISU_GEMINI_MODE") == "quota" {
				fmt.Println(string(marshal(map[string]any{"jsonrpc": "2.0", "id": m.ID, "error": map[string]any{"code": 429, "message": "private raw error"}})))
				continue
			}
			for _, kind := range []string{"agent_thought_chunk", "agent_message_chunk"} {
				fmt.Println(string(marshal(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": "s1", "update": map[string]any{"sessionUpdate": kind, "content": map[string]string{"type": "text", "text": map[string]string{"agent_thought_chunk": "private thinking", "agent_message_chunk": "안녕하세요"}[kind]}}}})))
			}
			if os.Getenv("RISU_GEMINI_MODE") == "wait" {
				time.Sleep(30 * time.Second)
			}
			result = map[string]any{"stopReason": "end_turn", "_meta": map[string]any{"quota": map[string]any{"token_count": map[string]int{"input_tokens": 12, "output_tokens": 3}}}}
		}
		fmt.Println(string(marshal(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": result})))
	}
}
func testGemini(t *testing.T) *gemini {
	t.Helper()
	t.Setenv("RISU_FAKE_GEMINI", "1")
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	g := newGemini(bin, t.TempDir())
	t.Cleanup(g.shutdown)
	if err = os.WriteFile(filepath.Join(g.home, "oauth_creds.json"), []byte(`{"refresh_token":"fixture"}`), 0600); err != nil {
		t.Fatal(err)
	}
	return g
}
func TestGeminiTransportAndIsolation(t *testing.T) {
	g := testGemini(t)
	marker := filepath.Join(t.TempDir(), "input.json")
	t.Setenv("RISU_GEMINI_MARKER", marker)
	t.Setenv("GEMINI_API_KEY", "must-not-pass")
	var output strings.Builder
	r, err := g.generate(context.Background(), chatRequest{Model: "subscription-default", Messages: []message{{Role: "system", Content: "character"}, {Role: "assistant", Content: "old"}, {Role: "user", Content: "new"}}}, settings{Model: "subscription-default"}, "", func(s string) { output.WriteString(s) })
	if err != nil {
		t.Fatal(err)
	}
	if output.String() != "안녕하세요" || r.Usage["total_tokens"] != 15 || r.Model != "gemini-test" {
		t.Fatalf("%+v %q", r, output.String())
	}
	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	var capture struct {
		APIKey string                                   `json:"apiKey"`
		Root   string                                   `json:"root"`
		Params struct{ Prompt []struct{ Text string } } `json:"params"`
		Config struct {
			Tools       struct{ Core []string }
			HooksConfig struct{ Enabled bool }
		} `json:"config"`
	}
	if json.Unmarshal(data, &capture) != nil || capture.APIKey != "" || capture.Config.Tools.Core == nil || len(capture.Config.Tools.Core) != 0 || capture.Config.HooksConfig.Enabled {
		t.Fatalf("Invalid isolation: %s", data)
	}
	if len(capture.Params.Prompt) != 1 || !strings.Contains(capture.Params.Prompt[0].Text, `"role":"assistant"`) {
		t.Fatal("history lost")
	}
	if _, err = os.Stat(capture.Root); !os.IsNotExist(err) {
		t.Fatal("temporary session persisted")
	}
}
func TestGeminiCancelAndQuota(t *testing.T) {
	for _, mode := range []string{"wait", "quota"} {
		t.Run(mode, func(t *testing.T) {
			g := testGemini(t)
			t.Setenv("RISU_GEMINI_MODE", mode)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			start := time.Now()
			_, err := g.generate(ctx, chatRequest{Model: "subscription-default", Messages: []message{{Role: "user", Content: "hi"}}}, settings{Model: "subscription-default"}, "", func(string) {
				if mode == "wait" {
					cancel()
				}
			})
			status, _ := errorDetails(err)
			want := 429
			if mode == "wait" {
				want = 499
			}
			if status != want || time.Since(start) > 5*time.Second {
				t.Fatalf("status=%d err=%v", status, err)
			}
			if !g.op.TryLock() {
				t.Fatal("operation lock leaked")
			}
			g.op.Unlock()
		})
	}
}
func TestProviderSettingsAndRouting(t *testing.T) {
	b := testBridge(t, &fakeBackend{})
	g := testGemini(t)
	b.providers = map[string]backend{"chatgpt": b.api, "gemini": g}
	if err := atomicWrite(b.settingsPath("chatgpt"), marshal(settings{Model: "subscription-default", Verbosity: "high", Instructions: "original"})); err != nil {
		t.Fatal(err)
	}
	w := request(b, "/internal/provider", `{"provider":"gemini"}`, "", "secret")
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	w = request(b, "/v1/chat/completions", chatBody, "", "secret")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "안녕하세요") {
		t.Fatal(w.Body.String())
	}
	w = request(b, "/internal/settings", `{"model":"subscription-default","effort":"high","verbosity":"","instructions":""}`, "", "secret")
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
	b.claim()
	w = request(b, "/internal/provider", `{"provider":"chatgpt"}`, "", "secret")
	if w.Code != 409 {
		t.Fatal(w.Code)
	}
	b.release()
	w = request(b, "/internal/provider", `{"provider":"chatgpt"}`, "", "secret")
	if w.Code != 200 || b.settings.Instructions != "original" || b.settings.Verbosity != "high" {
		t.Fatal("ChatGPT settings not restored")
	}
	w = request(b, "/internal/provider", `{"provider":"gemini"}`, "tauri://localhost", "secret")
	if w.Code != 403 {
		t.Fatal("remote setup write allowed")
	}
}
func TestGeminiLogin(t *testing.T) {
	g := testGemini(t)
	_ = os.Remove(filepath.Join(g.home, "oauth_creds.json"))
	var result any
	if err := g.rpc(context.Background(), "account/login/start", nil, &result); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		a, _ := g.account(context.Background())
		if !a.LoggingIn {
			if !a.Connected {
				t.Fatalf("%+v", a)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("login did not complete")
}
func TestGeminiRealInitialize(t *testing.T) {
	bin := os.Getenv("RISU_REAL_GEMINI_BIN")
	if bin == "" {
		t.Skip("Optional installed Gemini CLI protocol check; no login or generation")
	}
	g := newGemini(bin, t.TempDir())
	defer g.shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	p, err := g.start(ctx, false, "")
	if err != nil {
		t.Fatal(err)
	}
	p.close()
}
