package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Codex is started on demand so a Gemini-only user need not install Codex.
type lazyCodex struct {
	mu          sync.Mutex
	binary, cwd string
	env         []string
	client      *codex
	closed      bool
}

func (l *lazyCodex) get() (*codex, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil, problem(503, "harness_stopped", "Bridge is stopping.")
	}
	if l.client == nil || !l.client.alive() {
		c, err := startCodex(l.binary, l.cwd, l.env)
		if err != nil {
			return nil, problem(503, "codex_unavailable", "Codex를 시작할 수 없습니다. 설치 경로를 확인하거나 Gemini를 선택하세요.")
		}
		l.client = c
	}
	return l.client, nil
}
func (l *lazyCodex) alive() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return !l.closed && (l.client == nil || l.client.alive())
}
func (l *lazyCodex) shutdown() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closed = true
	if l.client != nil {
		l.client.shutdown()
	}
}
func (l *lazyCodex) account(ctx context.Context) (account, error) {
	c, e := l.get()
	if e != nil {
		return account{}, e
	}
	return c.account(ctx)
}
func (l *lazyCodex) models(ctx context.Context) ([]model, error) {
	c, e := l.get()
	if e != nil {
		return nil, e
	}
	return c.models(ctx)
}
func (l *lazyCodex) rpc(ctx context.Context, m string, p, o any) error {
	c, e := l.get()
	if e != nil {
		return e
	}
	return c.rpc(ctx, m, p, o)
}
func (l *lazyCodex) generate(ctx context.Context, r chatRequest, s settings, cwd string, d func(string)) (generationResult, error) {
	c, e := l.get()
	if e != nil {
		return generationResult{}, e
	}
	return c.generate(ctx, r, s, cwd, d)
}

func (b *bridge) current() (backend, string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	p := b.provider
	if p == "" {
		p = "chatgpt"
	}
	return b.api, p
}
func (b *bridge) settingsPath(provider string) string {
	name := "generation-settings.json"
	if provider == "gemini" {
		name = "gemini-generation-settings.json"
	}
	return filepath.Join(b.runtime, name)
}
func (b *bridge) selectProvider(provider string) error {
	api := b.providers[provider]
	if api == nil {
		return bad("Unknown provider.")
	}
	s := settings{Model: "subscription-default"}
	data, err := os.ReadFile(b.settingsPath(provider))
	if err == nil {
		if json.Unmarshal(data, &s) != nil {
			return problem(500, "invalid_settings", "저장된 응답 설정을 읽을 수 없습니다.")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err = atomicWrite(filepath.Join(b.runtime, "provider.json"), marshal(provider)); err != nil {
		return err
	}
	b.mu.Lock()
	b.api, b.provider, b.settings = api, provider, s
	b.metrics = metrics{}
	b.mu.Unlock()
	return nil
}
