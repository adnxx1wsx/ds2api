package util

import "testing"

func TestParseToolCalls(t *testing.T) {
	text := `prefix {"tool_calls":[{"name":"search","input":{"q":"golang"}}]} suffix`
	calls := ParseToolCalls(text, []string{"search"})
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "search" {
		t.Fatalf("unexpected tool name: %s", calls[0].Name)
	}
}

func TestParseToolCallsRejectUnknown(t *testing.T) {
	text := `{"tool_calls":[{"name":"unknown","input":{}}]}`
	calls := ParseToolCalls(text, []string{"search"})
	if len(calls) != 0 {
		t.Fatalf("expected 0 calls, got %d", len(calls))
	}
}

func TestFormatOpenAIToolCalls(t *testing.T) {
	formatted := FormatOpenAIToolCalls([]ParsedToolCall{{Name: "search", Input: map[string]any{"q": "x"}}})
	if len(formatted) != 1 {
		t.Fatalf("expected 1, got %d", len(formatted))
	}
	fn, _ := formatted[0]["function"].(map[string]any)
	if fn["name"] != "search" {
		t.Fatalf("unexpected function name: %#v", fn)
	}
}
