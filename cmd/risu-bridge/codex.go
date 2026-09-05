package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os/exec"
	"sync"
	"time"
)

type rpcMessage struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  json.RawMessage `json:"error"`
}
type codex struct {
	cmd         *exec.Cmd
	in          io.WriteCloser
	mu, writeMu sync.Mutex
	seq         int
	pending     map[string]chan rpcMessage
	events      map[string]chan rpcMessage
	done        chan struct{}
	once        sync.Once
}

func startCodex(binary, cwd string, env []string) (*codex, error) {
	c := &codex{pending: map[string]chan rpcMessage{}, events: map[string]chan rpcMessage{}, done: make(chan struct{})}
	c.cmd = exec.Command(binary, "app-server", "--listen", "stdio://")
	c.cmd.Dir = cwd
	c.cmd.Env = env
	c.cmd.Stderr = io.Discard
	var err error
	c.in, err = c.cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := c.cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err = c.cmd.Start(); err != nil {
		return nil, err
	}
	go func() {
		s := bufio.NewScanner(out)
		s.Buffer(make([]byte, 65536), 8*1024*1024)
		for s.Scan() {
			var m rpcMessage
			if json.Unmarshal(s.Bytes(), &m) != nil {
				continue
			}
			if len(m.ID) > 0 {
				if m.Method != "" {
					_ = c.send(map[string]any{"id": m.ID, "error": map[string]any{"code": -32601, "message": "Interactive tools are disabled by this client."}})
					continue
				}
				c.mu.Lock()
				ch := c.pending[string(m.ID)]
				c.mu.Unlock()
				if ch != nil {
					select {
					case ch <- m:
					default:
					}
				}
			} else if m.Method != "" {
				var p struct {
					ThreadID string `json:"threadId"`
				}
				_ = json.Unmarshal(m.Params, &p)
				c.mu.Lock()
				ch := c.events[p.ThreadID]
				c.mu.Unlock()
				if ch != nil {
					select {
					case ch <- m:
					case <-c.done:
						return
					default:
						c.shutdown()
						return
					}
				}
			}
		}
		c.shutdown()
	}()
	go func() { _ = c.cmd.Wait(); c.once.Do(func() { close(c.done) }) }()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err = c.rpc(ctx, "initialize", map[string]any{"clientInfo": map[string]string{"name": "risu_subscription_bridge", "title": "Risu Subscription Bridge", "version": version}, "capabilities": map[string]bool{"experimentalApi": true}}, nil); err != nil {
		c.shutdown()
		return nil, err
	}
	_ = c.send(map[string]any{"method": "initialized", "params": map[string]any{}})
	return c, nil
}
func (c *codex) send(v any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_, err := c.in.Write(append(marshal(v), '\n'))
	return err
}
func (c *codex) rpc(ctx context.Context, method string, params, out any) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	c.mu.Lock()
	c.seq++
	id := c.seq
	key := string(marshal(id))
	ch := make(chan rpcMessage, 1)
	c.pending[key] = ch
	c.mu.Unlock()
	defer func() { c.mu.Lock(); delete(c.pending, key); c.mu.Unlock() }()
	if err := c.send(map[string]any{"id": id, "method": method, "params": params}); err != nil {
		return problem(503, "harness_stopped", "Codex is unavailable. Restart the bridge.")
	}
	select {
	case m := <-ch:
		if len(m.Error) > 0 && string(m.Error) != "null" {
			return problem(502, "rpc_error", "Codex RPC failed.")
		}
		if out != nil {
			return json.Unmarshal(m.Result, out)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		return problem(503, "harness_stopped", "Codex stopped. Restart the bridge.")
	}
}
func (c *codex) shutdown() {
	c.once.Do(func() {
		close(c.done)
		_ = c.in.Close()
		if c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
		}
	})
}
func (c *codex) alive() bool {
	select {
	case <-c.done:
		return false
	default:
		return true
	}
}

type account struct {
	Connected bool   `json:"connected"`
	Type      string `json:"type"`
	Plan      string `json:"plan"`
}

func (c *codex) account(ctx context.Context) (account, error) {
	var r struct {
		Account *struct {
			Type string `json:"type"`
			Plan string `json:"planType"`
		} `json:"account"`
	}
	err := c.rpc(ctx, "account/read", map[string]bool{"refreshToken": false}, &r)
	a := account{}
	if r.Account != nil {
		a = account{r.Account.Type == "chatgpt", r.Account.Type, r.Account.Plan}
	}
	return a, err
}

type model struct {
	ID          string `json:"id"`
	Model       string `json:"model"`
	DisplayName string `json:"displayName"`
	IsDefault   bool   `json:"isDefault"`
	Efforts     []struct {
		Effort string `json:"reasoningEffort"`
	} `json:"supportedReasoningEfforts"`
}

func (m model) name() string {
	if m.Model != "" {
		return m.Model
	}
	return m.ID
}
func (c *codex) models(ctx context.Context) ([]model, error) {
	ms := []model{}
	cursor := ""
	for {
		p := map[string]any{"limit": 100}
		if cursor != "" {
			p["cursor"] = cursor
		}
		var r struct {
			Data       []model `json:"data"`
			NextCursor string  `json:"nextCursor"`
		}
		if err := c.rpc(ctx, "model/list", p, &r); err != nil {
			return nil, err
		}
		ms = append(ms, r.Data...)
		cursor = r.NextCursor
		if cursor == "" {
			return ms, nil
		}
	}
}
func chooseModel(ms []model, name, effort string) (model, error) {
	var selected *model
	for i := range ms {
		if name == "subscription-default" {
			if selected == nil || ms[i].IsDefault {
				selected = &ms[i]
			}
		} else if ms[i].name() == name || ms[i].ID == name {
			selected = &ms[i]
			break
		}
	}
	if selected == nil {
		return model{}, problem(400, "unknown_model", "Choose an available model.")
	}
	if effort != "" {
		ok := false
		for _, e := range selected.Efforts {
			if e.Effort == effort {
				ok = true
			}
		}
		if !ok {
			return model{}, problem(400, "effort_invalid", "선택 모델이 추론 강도를 지원하지 않습니다. 기본값으로 바꾸세요.")
		}
	}
	return *selected, nil
}

type generationResult struct {
	Model        string           `json:"model"`
	Usage        map[string]int64 `json:"usage"`
	FirstTokenMS *int64           `json:"firstTokenMs"`
	ElapsedMS    int64            `json:"elapsedMs"`
}

func (c *codex) generate(ctx context.Context, r chatRequest, s settings, cwd string, delta func(string)) (result generationResult, err error) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()
	a, err := c.account(ctx)
	if err != nil {
		return result, err
	}
	if !a.Connected {
		return result, problem(401, "login_required", "Sign in with ChatGPT on the local setup page.")
	}
	ms, err := c.models(ctx)
	if err != nil {
		return result, err
	}
	name := r.Model
	if name == "subscription-default" {
		name = s.Model
	}
	m, err := chooseModel(ms, name, s.Effort)
	if err != nil {
		return result, err
	}
	result.Model = m.name()
	p := map[string]any{"model": result.Model, "cwd": cwd, "ephemeral": true, "sandbox": "read-only", "approvalPolicy": "never", "baseInstructions": baseInstructions}
	if s.Instructions != "" {
		p["baseInstructions"] = baseInstructions + "\n\n" + s.Instructions
	}
	if s.Verbosity != "" {
		p["config"] = map[string]string{"model_verbosity": s.Verbosity}
	}
	var thread struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err = c.rpc(ctx, "thread/start", p, &thread); err != nil {
		return result, err
	}
	tid := thread.Thread.ID
	if tid == "" {
		return result, problem(502, "rpc_error", "Missing thread ID.")
	}
	ch := make(chan rpcMessage, 2048)
	c.mu.Lock()
	c.events[tid] = ch
	c.mu.Unlock()
	turnID := ""
	defer func() {
		if err != nil && turnID != "" {
			clean, done := context.WithTimeout(context.Background(), 5*time.Second)
			_ = c.rpc(clean, "turn/interrupt", map[string]string{"threadId": tid, "turnId": turnID}, nil)
			done()
		}
		clean, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		if e := c.rpc(clean, "thread/unsubscribe", map[string]string{"threadId": tid}, nil); e != nil {
			c.shutdown()
			err = problem(503, "cleanup_failed", "Thread cleanup failed. Restart the bridge.")
		}
		c.mu.Lock()
		delete(c.events, tid)
		c.mu.Unlock()
	}()
	p = map[string]any{"threadId": tid, "input": []any{map[string]any{"type": "text", "text": promptFor(r.Messages), "text_elements": []any{}}}}
	if s.Effort != "" {
		p["effort"] = s.Effort
	}
	var turn struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	// Keep start bounded independently of the client so its returned turn ID is available for cancellation.
	startCtx, finish := context.WithTimeout(context.Background(), 30*time.Second)
	err = c.rpc(startCtx, "turn/start", p, &turn)
	finish()
	turnID = turn.Turn.ID
	if err != nil {
		return result, err
	}
	for {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-c.done:
			return result, problem(503, "harness_stopped", "Codex stopped during generation.")
		case event := <-ch:
			var p struct {
				Delta string `json:"delta"`
				Turn  struct {
					ID, Status string
					Error      json.RawMessage
				} `json:"turn"`
				TokenUsage struct {
					Last *struct{ InputTokens, OutputTokens, TotalTokens int64 } `json:"last"`
				} `json:"tokenUsage"`
			}
			if json.Unmarshal(event.Params, &p) != nil {
				continue
			}
			switch event.Method {
			case "turn/started":
				turnID = p.Turn.ID
			case "item/agentMessage/delta":
				if result.FirstTokenMS == nil {
					n := time.Since(start).Milliseconds()
					result.FirstTokenMS = &n
				}
				delta(p.Delta)
			case "thread/tokenUsage/updated":
				if u := p.TokenUsage.Last; u != nil {
					result.Usage = map[string]int64{"prompt_tokens": u.InputTokens, "completion_tokens": u.OutputTokens, "total_tokens": u.TotalTokens}
				}
			case "turn/completed":
				if p.Turn.Status != "completed" {
					var e struct {
						Info json.RawMessage `json:"codexErrorInfo"`
					}
					_ = json.Unmarshal(p.Turn.Error, &e)
					if isQuota(string(e.Info)) {
						return result, problem(429, "rate_limit", "Subscription limit reached. Wait for reset.")
					}
					return result, problem(502, "generation_failed", "Generation failed.")
				}
				result.ElapsedMS = time.Since(start).Milliseconds()
				return result, nil
			}
		}
	}
}
