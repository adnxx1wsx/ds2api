package claude

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"ds2api/internal/sse"
	streamengine "ds2api/internal/stream"
	"ds2api/internal/util"
)

type claudeStreamRuntime struct {
	w        http.ResponseWriter
	rc       *http.ResponseController
	canFlush bool
	writable bool

	model     string
	toolNames []string
	messages  []any

	thinkingEnabled   bool
	searchEnabled     bool
	bufferToolContent bool

	messageID string
	thinking  strings.Builder
	text      strings.Builder

	nextBlockIndex     int
	thinkingBlockOpen  bool
	thinkingBlockIndex int
	textBlockOpen      bool
	textBlockIndex     int
	ended              bool
	upstreamErr        string
}

func newClaudeStreamRuntime(
	w http.ResponseWriter,
	rc *http.ResponseController,
	canFlush bool,
	model string,
	messages []any,
	thinkingEnabled bool,
	searchEnabled bool,
	toolNames []string,
) *claudeStreamRuntime {
	return &claudeStreamRuntime{
		w:                  w,
		rc:                 rc,
		canFlush:           canFlush,
		writable:           true,
		model:              model,
		messages:           messages,
		thinkingEnabled:    thinkingEnabled,
		searchEnabled:      searchEnabled,
		bufferToolContent:  len(toolNames) > 0,
		toolNames:          toolNames,
		messageID:          fmt.Sprintf("msg_%d", time.Now().UnixNano()),
		thinkingBlockIndex: -1,
		textBlockIndex:     -1,
	}
}

func (s *claudeStreamRuntime) send(event string, v any) bool {
	if !s.writable {
		return false
	}
	b, _ := json.Marshal(v)
	if _, err := s.w.Write([]byte("event: ")); err != nil {
		s.writable = false
		return false
	}
	if _, err := s.w.Write([]byte(event)); err != nil {
		s.writable = false
		return false
	}
	if _, err := s.w.Write([]byte("\n")); err != nil {
		s.writable = false
		return false
	}
	if _, err := s.w.Write([]byte("data: ")); err != nil {
		s.writable = false
		return false
	}
	if _, err := s.w.Write(b); err != nil {
		s.writable = false
		return false
	}
	if _, err := s.w.Write([]byte("\n\n")); err != nil {
		s.writable = false
		return false
	}
	if s.canFlush {
		if err := s.rc.Flush(); err != nil {
			s.writable = false
			return false
		}
	}
	return true
}

func (s *claudeStreamRuntime) sendError(message string) {
	if !s.writable {
		return
	}
	msg := strings.TrimSpace(message)
	if msg == "" {
		msg = "upstream stream error"
	}
	s.send("error", map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":    "api_error",
			"message": msg,
			"code":    "internal_error",
			"param":   nil,
		},
	})
}

func (s *claudeStreamRuntime) sendPing() bool {
	return s.send("ping", map[string]any{"type": "ping"})
}

func (s *claudeStreamRuntime) sendMessageStart() bool {
	inputTokens := util.EstimateTokens(fmt.Sprintf("%v", s.messages))
	return s.send("message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":            s.messageID,
			"type":          "message",
			"role":          "assistant",
			"model":         s.model,
			"content":       []any{},
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage":         map[string]any{"input_tokens": inputTokens, "output_tokens": 0},
		},
	})
}

func (s *claudeStreamRuntime) closeThinkingBlock() {
	if !s.thinkingBlockOpen {
		return
	}
	if !s.send("content_block_stop", map[string]any{
		"type":  "content_block_stop",
		"index": s.thinkingBlockIndex,
	}) {
		return
	}
	s.thinkingBlockOpen = false
	s.thinkingBlockIndex = -1
}

func (s *claudeStreamRuntime) closeTextBlock() {
	if !s.textBlockOpen {
		return
	}
	if !s.send("content_block_stop", map[string]any{
		"type":  "content_block_stop",
		"index": s.textBlockIndex,
	}) {
		return
	}
	s.textBlockOpen = false
	s.textBlockIndex = -1
}

func (s *claudeStreamRuntime) finalize(stopReason string) {
	if s.ended || !s.writable {
		return
	}
	s.ended = true

	s.closeThinkingBlock()
	s.closeTextBlock()

	finalThinking := s.thinking.String()
	finalText := s.text.String()

	if s.bufferToolContent {
		detected := util.ParseToolCalls(finalText, s.toolNames)
		if len(detected) > 0 {
			stopReason = "tool_use"
			for i, tc := range detected {
				idx := s.nextBlockIndex + i
				if !s.send("content_block_start", map[string]any{
					"type":  "content_block_start",
					"index": idx,
					"content_block": map[string]any{
						"type":  "tool_use",
						"id":    fmt.Sprintf("toolu_%d_%d", time.Now().Unix(), idx),
						"name":  tc.Name,
						"input": tc.Input,
					},
				}) {
					return
				}
				if !s.send("content_block_stop", map[string]any{
					"type":  "content_block_stop",
					"index": idx,
				}) {
					return
				}
			}
			s.nextBlockIndex += len(detected)
		} else if finalText != "" {
			idx := s.nextBlockIndex
			s.nextBlockIndex++
			if !s.send("content_block_start", map[string]any{
				"type":  "content_block_start",
				"index": idx,
				"content_block": map[string]any{
					"type": "text",
					"text": "",
				},
			}) {
				return
			}
			if !s.send("content_block_delta", map[string]any{
				"type":  "content_block_delta",
				"index": idx,
				"delta": map[string]any{
					"type": "text_delta",
					"text": finalText,
				},
			}) {
				return
			}
			if !s.send("content_block_stop", map[string]any{
				"type":  "content_block_stop",
				"index": idx,
			}) {
				return
			}
		}
	}

	outputTokens := util.EstimateTokens(finalThinking) + util.EstimateTokens(finalText)
	if !s.send("message_delta", map[string]any{
		"type": "message_delta",
		"delta": map[string]any{
			"stop_reason":   stopReason,
			"stop_sequence": nil,
		},
		"usage": map[string]any{
			"output_tokens": outputTokens,
		},
	}) {
		return
	}
	s.send("message_stop", map[string]any{"type": "message_stop"})
}

func (s *claudeStreamRuntime) onParsed(parsed sse.LineResult) streamengine.ParsedDecision {
	if !s.writable {
		return streamengine.ParsedDecision{Stop: true, StopReason: streamengine.StopReasonHandlerRequested}
	}
	if !parsed.Parsed {
		return streamengine.ParsedDecision{}
	}
	if parsed.ErrorMessage != "" {
		s.upstreamErr = parsed.ErrorMessage
		return streamengine.ParsedDecision{Stop: true, StopReason: streamengine.StopReason("upstream_error")}
	}
	if parsed.Stop {
		return streamengine.ParsedDecision{Stop: true}
	}

	contentSeen := false
	for _, p := range parsed.Parts {
		if p.Text == "" {
			continue
		}
		if p.Type != "thinking" && s.searchEnabled && sse.IsCitation(p.Text) {
			continue
		}
		contentSeen = true

		if p.Type == "thinking" {
			if !s.thinkingEnabled {
				continue
			}
			s.thinking.WriteString(p.Text)
			s.closeTextBlock()
			if !s.thinkingBlockOpen {
				s.thinkingBlockIndex = s.nextBlockIndex
				s.nextBlockIndex++
				if !s.send("content_block_start", map[string]any{
					"type":  "content_block_start",
					"index": s.thinkingBlockIndex,
					"content_block": map[string]any{
						"type":     "thinking",
						"thinking": "",
					},
				}) {
					return streamengine.ParsedDecision{Stop: true, StopReason: streamengine.StopReasonHandlerRequested}
				}
				s.thinkingBlockOpen = true
			}
			if !s.send("content_block_delta", map[string]any{
				"type":  "content_block_delta",
				"index": s.thinkingBlockIndex,
				"delta": map[string]any{
					"type":     "thinking_delta",
					"thinking": p.Text,
				},
			}) {
				return streamengine.ParsedDecision{Stop: true, StopReason: streamengine.StopReasonHandlerRequested}
			}
			continue
		}

		s.text.WriteString(p.Text)
		if s.bufferToolContent {
			continue
		}
		s.closeThinkingBlock()
		if !s.textBlockOpen {
			s.textBlockIndex = s.nextBlockIndex
			s.nextBlockIndex++
			if !s.send("content_block_start", map[string]any{
				"type":  "content_block_start",
				"index": s.textBlockIndex,
				"content_block": map[string]any{
					"type": "text",
					"text": "",
				},
			}) {
				return streamengine.ParsedDecision{Stop: true, StopReason: streamengine.StopReasonHandlerRequested}
			}
			s.textBlockOpen = true
		}
		if !s.send("content_block_delta", map[string]any{
			"type":  "content_block_delta",
			"index": s.textBlockIndex,
			"delta": map[string]any{
				"type": "text_delta",
				"text": p.Text,
			},
		}) {
			return streamengine.ParsedDecision{Stop: true, StopReason: streamengine.StopReasonHandlerRequested}
		}
	}

	return streamengine.ParsedDecision{ContentSeen: contentSeen}
}

func (s *claudeStreamRuntime) onFinalize(reason streamengine.StopReason, scannerErr error) {
	if string(reason) == "upstream_error" {
		s.sendError(s.upstreamErr)
		return
	}
	if scannerErr != nil {
		s.sendError(scannerErr.Error())
		return
	}
	s.finalize("end_turn")
}
