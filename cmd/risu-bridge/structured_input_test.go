package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestStructuredInputPreservesRolesAndOrder(t *testing.T) {
	ms := []message{{Role: "system", Content: "character"}, {Role: "user", Name: "민수", Content: "hi"}, {Role: "assistant", Content: "hello"}, {Role: "developer", Content: "late instruction"}, {Role: "user", Content: "now"}}
	items, input := structuredInput(ms)
	if input != "now" || len(items) != 4 {
		t.Fatalf("%+v %q", items, input)
	}
	for i, item := range items {
		if item.Role != ms[i].Role || item.Type != "message" {
			t.Fatal(item)
		}
		want := "input_text"
		if item.Role == "assistant" {
			want = "output_text"
		}
		if item.Content[0].Type != want {
			t.Fatal(item)
		}
	}
	if items[1].Content[0].Text != "[speaker name: \"민수\"]\nhi" {
		t.Fatal(items[1])
	}
	if items[3].Content[0].Text != "late instruction" {
		t.Fatal(items[3])
	}
	for _, role := range []string{"assistant", "system", "developer"} {
		history, trigger := structuredInput([]message{{Role: role, Content: "tail"}})
		if len(history) != 1 || history[0].Role != role || trigger == "tail" || trigger == "" {
			t.Fatalf("%+v %q", history, trigger)
		}
	}
	history, empty := structuredInput([]message{{Role: "user", Content: ""}})
	if len(history) != 0 || empty != "" {
		t.Fatal("empty input changed")
	}
}

func TestStructuredCodexTransport(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "injection failure cleans up"}[failure], func(t *testing.T) {
			bin, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			marker := filepath.Join(dir, "input")
			cleanup := filepath.Join(dir, "cleanup")
			env := append(os.Environ(), "RISU_FAKE_CODEX=1", "RISU_INPUT_MARKER="+marker, "RISU_CLEANUP_MARKER="+cleanup)
			if failure {
				env = append(env, "RISU_INJECT_FAIL=1")
			}
			c, err := startCodex(bin, dir, env)
			if err != nil {
				t.Fatal(err)
			}
			defer c.shutdown()
			ms := []message{{Role: "system", Content: "character"}, {Role: "user", Content: "old user"}, {Role: "assistant", Content: "old reply"}, {Role: "developer", Content: "instruction"}, {Role: "user", Content: "new user"}}
			_, err = c.generate(context.Background(), chatRequest{Model: "subscription-default", Messages: ms}, settings{Model: "subscription-default"}, dir, func(string) {})
			if (err != nil) != failure {
				t.Fatalf("failure=%v err=%v", failure, err)
			}
			raw, err := os.ReadFile(marker)
			if err != nil {
				t.Fatal(err)
			}
			var p struct {
				ThreadID string            `json:"threadId"`
				Items    []responseMessage `json:"items"`
			}
			if err = json.Unmarshal(raw, &p); err != nil {
				t.Fatal(err)
			}
			expected, _ := structuredInput(ms)
			if p.ThreadID != "t1" || !reflect.DeepEqual(p.Items, expected) {
				t.Fatalf("%s", raw)
			}
			turn, readErr := os.ReadFile(marker + ".turn")
			if failure {
				if !os.IsNotExist(readErr) {
					t.Fatal("generation started after injection failure")
				}
			} else {
				if readErr != nil {
					t.Fatal(readErr)
				}
				var input struct {
					Input []struct {
						Text string `json:"text"`
					} `json:"input"`
				}
				if json.Unmarshal(turn, &input) != nil || len(input.Input) != 1 || input.Input[0].Text != "new user" {
					t.Fatalf("%s", turn)
				}
				if strings.Contains(string(turn), "old reply") {
					t.Fatal("history duplicated")
				}
			}
			if _, err = os.Stat(cleanup); err != nil {
				t.Fatal("cleanup missing", err)
			}
		})
	}
}
