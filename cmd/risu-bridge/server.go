package main

import (
	"context"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

//go:embed web/*
var assets embed.FS

type settings struct {
	Model        string `json:"model"`
	Effort       string `json:"effort"`
	Verbosity    string `json:"verbosity"`
	Instructions string `json:"instructions"`
}
type backend interface {
	alive() bool
	account(context.Context) (account, error)
	models(context.Context) ([]model, error)
	rpc(context.Context, string, any, any) error
	generate(context.Context, chatRequest, settings, string, func(string)) (generationResult, error)
}
type metrics struct {
	Requests  int `json:"requests"`
	Completed int `json:"completed"`
	Cancelled int `json:"cancelled"`
	Failed    int `json:"failed"`
	Last      any `json:"last"`
}
type bridge struct {
	api                       backend
	token, port, runtime, cwd string
	origins                   []string
	mu                        sync.Mutex
	busy                      bool
	settings                  settings
	metrics                   metrics
	stop                      func()
}

func (b *bridge) claim() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.busy {
		return false
	}
	b.busy = true
	return true
}
func (b *bridge) release() { b.mu.Lock(); b.busy = false; b.mu.Unlock() }
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(marshal(v))
}
func errorDetails(err error) (int, any) {
	e := &bridgeError{500, "bridge_error", "Internal bridge error."}
	if errors.Is(err, context.Canceled) {
		e = &bridgeError{499, "cancelled", "Cancelled."}
	} else if errors.Is(err, context.DeadlineExceeded) {
		e = &bridgeError{504, "generation_timeout", "Request timed out."}
	} else {
		var known *bridgeError
		if errors.As(err, &known) {
			e = known
		}
	}
	return e.Status, map[string]any{"error": map[string]string{"message": e.Message, "type": e.Code, "code": e.Code}}
}
func readJSON(w http.ResponseWriter, r *http.Request) (map[string]json.RawMessage, error) {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return nil, problem(415, "content_type", "Use application/json.")
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2*1024*1024))
	if err != nil {
		var max *http.MaxBytesError
		if errors.As(err, &max) {
			return nil, problem(413, "request_too_large", "Request exceeds 2 MiB.")
		}
		return nil, err
	}
	var v map[string]json.RawMessage
	if json.Unmarshal(raw, &v) != nil || v == nil {
		return nil, problem(400, "invalid_json", "JSON object required.")
	}
	return v, nil
}
func (b *bridge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := b.serve(w, r); err != nil {
		status, body := errorDetails(err)
		writeJSON(w, status, body)
	}
}
func (b *bridge) serve(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Host != "127.0.0.1:"+b.port && r.Host != "localhost:"+b.port {
		return problem(403, "invalid_host", "Invalid Host.")
	}
	local := []string{"http://127.0.0.1:" + b.port, "http://localhost:" + b.port}
	origin := r.Header.Get("Origin")
	if origin != "" {
		if !contains(local, origin) && !contains(b.origins, origin) {
			return problem(403, "origin_denied", "Origin is not allowed.")
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
	}
	if r.Method == "OPTIONS" {
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Proxy-Risu")
		w.Header().Set("Access-Control-Allow-Private-Network", "true")
		w.WriteHeader(204)
		return nil
	}
	path := r.URL.Path
	if r.Method == "GET" && (path == "/" || path == "/setup.js") {
		file := "web/setup.html"
		contentType := "text/html; charset=utf-8"
		if path == "/setup.js" {
			file = "web/setup.js"
			contentType = "text/javascript; charset=utf-8"
		} else {
			w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'unsafe-inline'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		}
		v, _ := assets.ReadFile(file)
		w.Header().Set("Content-Type", contentType)
		_, err := w.Write(v)
		return err
	}
	if r.Method == "GET" && path == "/healthz" {
		writeJSON(w, 200, map[string]any{"ok": b.api.alive(), "version": version, "implementation": "go"})
		return nil
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+b.token)) != 1 {
		return problem(401, "unauthorized", "Local bridge key required.")
	}
	if strings.HasPrefix(path, "/internal/") && origin != "" && !contains(local, origin) {
		return problem(403, "setup_origin", "Setup must be opened locally.")
	}
	ctx := r.Context()
	switch {
	case r.Method == "GET" && path == "/internal/status":
		a, err := b.api.account(ctx)
		if err != nil {
			return err
		}
		b.mu.Lock()
		v := map[string]any{"account": a, "metrics": b.metrics, "busy": b.busy, "runtime": b.runtime, "adapter": "app-server", "delivery": "token-delta", "implementation": "go", "version": version, "mode": "Risu owns history; fresh ephemeral generation per request", "controlPlane": "App Server for login, models and generation"}
		encoded := marshal(v)
		b.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(encoded)
		return nil
	case r.Method == "GET" && path == "/internal/settings":
		ms, err := b.api.models(ctx)
		if err != nil {
			return err
		}
		b.mu.Lock()
		s := b.settings
		b.mu.Unlock()
		writeJSON(w, 200, map[string]any{"settings": s, "models": ms})
		return nil
	case r.Method == "POST" && path == "/internal/settings":
		if !b.claim() {
			return problem(409, "bridge_busy", "응답이 끝난 뒤 설정을 저장하세요.")
		}
		defer b.release()
		raw, err := readJSON(w, r)
		if err != nil {
			return err
		}
		if len(raw) != 4 {
			return bad("Invalid settings.")
		}
		for _, k := range []string{"model", "effort", "verbosity", "instructions"} {
			var s string
			v, ok := raw[k]
			if !ok || string(v) == "null" || json.Unmarshal(v, &s) != nil {
				return bad("Invalid settings.")
			}
		}
		var s settings
		_ = json.Unmarshal(marshal(raw), &s)
		if !contains([]string{"", "low", "medium", "high"}, s.Verbosity) || len([]rune(s.Instructions)) > 16000 {
			return bad("Invalid settings.")
		}
		ms, err := b.api.models(ctx)
		if err != nil {
			return err
		}
		if _, err = chooseModel(ms, s.Model, s.Effort); err != nil {
			return err
		}
		if err = atomicWrite(filepath.Join(b.runtime, "generation-settings.json"), marshal(s)); err != nil {
			return err
		}
		b.mu.Lock()
		b.settings = s
		b.mu.Unlock()
		writeJSON(w, 200, map[string]any{"settings": s})
		return nil
	case r.Method == "POST" && path == "/internal/login":
		if _, err := readJSON(w, r); err != nil {
			return err
		}
		var result any
		if err := b.api.rpc(ctx, "account/login/start", map[string]string{"type": "chatgpt"}, &result); err != nil {
			return err
		}
		writeJSON(w, 200, result)
		return nil
	case r.Method == "POST" && path == "/internal/stop":
		if _, err := readJSON(w, r); err != nil {
			return err
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
		if b.stop != nil {
			go func() { time.Sleep(50 * time.Millisecond); b.stop() }()
		}
		return nil
	case r.Method == "GET" && path == "/v1/models":
		ms, err := b.api.models(ctx)
		if err != nil {
			return err
		}
		data := []any{map[string]string{"id": "subscription-default", "object": "model", "owned_by": "local-bridge"}}
		for _, m := range ms {
			data = append(data, map[string]string{"id": m.name(), "object": "model", "owned_by": "openai"})
		}
		writeJSON(w, 200, map[string]any{"object": "list", "data": data})
		return nil
	case r.Method == "POST" && path == "/v1/chat/completions":
		return b.chat(w, r)
	default:
		return problem(404, "not_found", "Not found.")
	}
}
func (b *bridge) chat(w http.ResponseWriter, r *http.Request) error {
	raw, err := readJSON(w, r)
	if err != nil {
		return err
	}
	req, err := normalize(raw)
	if err != nil {
		return err
	}
	if !b.claim() {
		return problem(429, "bridge_busy", "One generation at a time. Retry after completion.")
	}
	defer b.release()
	b.mu.Lock()
	b.metrics.Requests++
	s := b.settings
	b.mu.Unlock()
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	id := "chatcmpl-" + randomKey()
	created := time.Now().Unix()
	streaming := false
	var writeErr error
	sse := func(v any) {
		if writeErr != nil {
			return
		}
		var data []byte
		if t, ok := v.(string); ok {
			data = []byte(t)
		} else {
			data = marshal(v)
		}
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(15 * time.Second))
		_, writeErr = w.Write(append(append([]byte("data: "), data...), '\n', '\n'))
		if writeErr == nil {
			writeErr = http.NewResponseController(w).Flush()
		}
		if writeErr != nil {
			cancel()
		}
	}
	chunk := func(delta any, finish any, model string) any {
		return map[string]any{"id": id, "object": "chat.completion.chunk", "created": created, "model": model, "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}}
	}
	begin := func() {
		if !streaming && req.Stream {
			streaming = true
			w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
			w.Header().Set("X-Bridge-Ignored-Parameters", strings.Join(req.Ignored, ","))
			sse(chunk(map[string]string{"role": "assistant", "content": ""}, nil, req.Model))
		}
	}
	var text strings.Builder
	push := func(v string) {
		text.WriteString(v)
		if v != "" && req.Stream {
			begin()
			sse(chunk(map[string]string{"content": v}, nil, req.Model))
		}
	}
	filter := stopFilter{Stops: req.Stop}
	result, err := b.api.generate(ctx, req, s, b.cwd, func(v string) {
		push(filter.Push(v, false))
		if filter.Stopped {
			cancel()
		}
	})
	if err != nil && (!filter.Stopped || r.Context().Err() != nil) {
		b.mu.Lock()
		if ctx.Err() != nil {
			b.metrics.Cancelled++
		} else {
			b.metrics.Failed++
		}
		b.mu.Unlock()
		if streaming {
			_, body := errorDetails(err)
			sse(body)
			return nil
		}
		return err
	}
	if writeErr != nil || r.Context().Err() != nil {
		return nil
	}
	push(filter.Push("", true))
	if result.Model == "" {
		result.Model = req.Model
	}
	b.mu.Lock()
	b.metrics.Completed++
	b.metrics.Last = map[string]any{"model": result.Model, "firstTokenMs": result.FirstTokenMS, "elapsedMs": result.ElapsedMS, "usage": result.Usage, "ignoredParameters": req.Ignored}
	b.mu.Unlock()
	if req.Stream {
		begin()
		sse(chunk(map[string]any{}, "stop", result.Model))
		if req.IncludeUsage && result.Usage != nil {
			sse(map[string]any{"id": id, "object": "chat.completion.chunk", "created": created, "model": result.Model, "choices": []any{}, "usage": result.Usage})
		}
		sse("[DONE]")
	} else {
		v := map[string]any{"id": id, "object": "chat.completion", "created": created, "model": result.Model, "choices": []any{map[string]any{"index": 0, "message": map[string]string{"role": "assistant", "content": text.String()}, "finish_reason": "stop"}}, "bridge": map[string]any{"ignored_parameters": req.Ignored}}
		if result.Usage != nil {
			v["usage"] = result.Usage
		}
		writeJSON(w, 200, v)
	}
	return nil
}
func atomicWrite(path string, b []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".bridge-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
