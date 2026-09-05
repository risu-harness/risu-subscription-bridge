package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDebugAudit(t *testing.T) {
	b := testBridge(t, &fakeBackend{})
	b.provider = "chatgpt"
	path := filepath.Join(b.runtime, "audit-latest.json")
	if w := request(b, "/v1/chat/completions", chatBody, "", "secret"); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("audit enabled by default")
	}
	b.debug = true
	body := `{"messages":[{"role":"system","content":"Preserve requested character dialogue and image/emotion markup."},{"role":"user","name":"테스트","content":"hello"}],"temperature":0.7,"api_key":"do-not-record"}`
	if w := request(b, "/v1/chat/completions", body, "", "wrong"); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("unauthorized audit")
	}
	if w := request(b, "/v1/chat/completions", body, "", "secret"); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "do-not-record") || strings.Contains(string(data), "Bearer") {
		t.Fatal("credential captured")
	}
	var report struct {
		AppServerInput struct {
			TurnInput string `json:"turnInput"`
		} `json:"appServerInput"`
		LiteralOverlap []struct {
			Indices []int `json:"exactMatchMessageIndices"`
		} `json:"literalOverlap"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(report.AppServerInput.TurnInput, "speaker name") || len(report.LiteralOverlap[2].Indices) != 1 {
		t.Fatal(string(data))
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
	if w := request(b, "/v1/chat/completions", chatBody, "", "secret"); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	data, _ = os.ReadFile(path)
	if strings.Contains(string(data), "테스트") {
		t.Fatal("old request retained")
	}
}
