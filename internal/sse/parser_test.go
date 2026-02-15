package sse

import "testing"

func TestParseDeepSeekSSELine(t *testing.T) {
	chunk, done, ok := ParseDeepSeekSSELine([]byte(`data: {"v":"你好"}`))
	if !ok || done {
		t.Fatalf("expected parsed chunk")
	}
	if chunk["v"] != "你好" {
		t.Fatalf("unexpected chunk: %#v", chunk)
	}
}

func TestParseDeepSeekSSELineDone(t *testing.T) {
	_, done, ok := ParseDeepSeekSSELine([]byte(`data: [DONE]`))
	if !ok || !done {
		t.Fatalf("expected done signal")
	}
}

func TestParseSSEChunkForContentSimple(t *testing.T) {
	parts, finished, _ := ParseSSEChunkForContent(map[string]any{"v": "hello"}, false, "text")
	if finished {
		t.Fatal("expected unfinished")
	}
	if len(parts) != 1 || parts[0].Text != "hello" || parts[0].Type != "text" {
		t.Fatalf("unexpected parts: %#v", parts)
	}
}

func TestParseSSEChunkForContentThinking(t *testing.T) {
	parts, finished, _ := ParseSSEChunkForContent(map[string]any{"p": "response/thinking_content", "v": "think"}, true, "thinking")
	if finished {
		t.Fatal("expected unfinished")
	}
	if len(parts) != 1 || parts[0].Type != "thinking" {
		t.Fatalf("unexpected parts: %#v", parts)
	}
}

func TestIsCitation(t *testing.T) {
	if !IsCitation("[citation:1] abc") {
		t.Fatal("expected citation true")
	}
	if IsCitation("normal text") {
		t.Fatal("expected citation false")
	}
}
