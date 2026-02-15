package util

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

var toolCallPattern = regexp.MustCompile(`\{\s*["']tool_calls["']\s*:\s*\[(.*?)\]\s*\}`)

type ParsedToolCall struct {
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

func ParseToolCalls(text string, availableToolNames []string) []ParsedToolCall {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	m := toolCallPattern.FindStringSubmatch(text)
	if len(m) < 2 {
		return nil
	}
	payload := "{" + `"tool_calls":[` + m[1] + "]}"
	var obj struct {
		ToolCalls []ParsedToolCall `json:"tool_calls"`
	}
	if err := json.Unmarshal([]byte(payload), &obj); err != nil {
		return nil
	}
	allowed := map[string]struct{}{}
	for _, name := range availableToolNames {
		allowed[name] = struct{}{}
	}
	out := make([]ParsedToolCall, 0, len(obj.ToolCalls))
	for _, tc := range obj.ToolCalls {
		if tc.Name == "" {
			continue
		}
		if len(allowed) > 0 {
			if _, ok := allowed[tc.Name]; !ok {
				continue
			}
		}
		if tc.Input == nil {
			tc.Input = map[string]any{}
		}
		out = append(out, tc)
	}
	return out
}

func FormatOpenAIToolCalls(calls []ParsedToolCall) []map[string]any {
	out := make([]map[string]any, 0, len(calls))
	for _, c := range calls {
		args, _ := json.Marshal(c.Input)
		out = append(out, map[string]any{
			"id":   "call_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
			"type": "function",
			"function": map[string]any{
				"name":      c.Name,
				"arguments": string(args),
			},
		})
	}
	return out
}
