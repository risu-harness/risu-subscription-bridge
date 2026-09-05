package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

// Audit is an opt-in, latest-request snapshot of the bridge boundary, not a
// capture of the final service-side model prompt. Never copy HTTP headers or
// arbitrary request fields into it.
func (b *bridge) writeAudit(raw map[string]json.RawMessage, r chatRequest, s settings) error {
	incoming := map[string]json.RawMessage{}
	for _, key := range []string{"messages", "model", "stream", "stop", "temperature", "top_p", "top_k", "min_p", "frequency_penalty", "presence_penalty", "logit_bias", "seed", "max_tokens", "max_completion_tokens"} {
		if value, ok := raw[key]; ok {
			incoming[key] = value
		}
	}
	instructions := baseInstructions
	if s.Instructions != "" {
		instructions += "\n\n" + s.Instructions
	}
	items, input := structuredInput(r.Messages)
	rows := []any{}
	total := 0
	for i, m := range r.Messages {
		n := utf8.RuneCountInString(m.Content)
		total += n
		rows = append(rows, map[string]any{"index": i, "role": m.Role, "characters": n, "hasName": m.Name != ""})
	}
	matches := []any{}
	// Exact sentence matches are evidence only; paraphrases and multilingual
	// semantic overlap must be assessed from the captured text, not guessed.
	for _, sentence := range strings.Split(baseInstructions, ". ") {
		sentence = strings.TrimSuffix(sentence, ".")
		indices := []int{}
		for i, m := range r.Messages {
			if strings.Contains(strings.ToLower(m.Content), strings.ToLower(sentence)) {
				indices = append(indices, i)
			}
		}
		matches = append(matches, map[string]any{"bridgeSentence": sentence, "exactMatchMessageIndices": indices})
	}
	report := map[string]any{
		"schemaVersion": 1, "capturedAt": time.Now().UTC(), "provider": "chatgpt",
		"scope":    "Bridge input and planned App Server input only; not final model wire input. Character counts are not tokens. Exact matches do not measure semantic redundancy.",
		"incoming": incoming, "settings": s, "ignoredParameters": r.Ignored,
		"appServerInput": map[string]any{"baseInstructions": instructions, "historyItems": items, "turnInput": input},
		"summary":        map[string]any{"messageCharacters": total, "baseInstructionCharacters": utf8.RuneCountInString(baseInstructions), "additionalInstructionCharacters": utf8.RuneCountInString(s.Instructions), "messages": rows},
		"literalOverlap": matches,
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(b.runtime, "audit-latest.json"), data)
}
