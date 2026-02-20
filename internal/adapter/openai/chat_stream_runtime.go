package openai

import (
	"encoding/json"
	"net/http"
	"strings"

	openaifmt "ds2api/internal/format/openai"
	"ds2api/internal/sse"
	streamengine "ds2api/internal/stream"
	"ds2api/internal/util"
)

type chatStreamRuntime struct {
	w        http.ResponseWriter
	rc       *http.ResponseController
	canFlush bool
	writable bool

	completionID string
	created      int64
	model        string
	finalPrompt  string
	toolNames    []string

	thinkingEnabled bool
	searchEnabled   bool

	firstChunkSent      bool
	bufferToolContent   bool
	emitEarlyToolDeltas bool
	toolCallsEmitted    bool

	toolSieve         toolStreamSieveState
	streamToolCallIDs map[int]string
	thinking          strings.Builder
	text              strings.Builder
}

func newChatStreamRuntime(
	w http.ResponseWriter,
	rc *http.ResponseController,
	canFlush bool,
	completionID string,
	created int64,
	model string,
	finalPrompt string,
	thinkingEnabled bool,
	searchEnabled bool,
	toolNames []string,
	bufferToolContent bool,
	emitEarlyToolDeltas bool,
) *chatStreamRuntime {
	return &chatStreamRuntime{
		w:                   w,
		rc:                  rc,
		canFlush:            canFlush,
		writable:            true,
		completionID:        completionID,
		created:             created,
		model:               model,
		finalPrompt:         finalPrompt,
		toolNames:           toolNames,
		thinkingEnabled:     thinkingEnabled,
		searchEnabled:       searchEnabled,
		bufferToolContent:   bufferToolContent,
		emitEarlyToolDeltas: emitEarlyToolDeltas,
		streamToolCallIDs:   map[int]string{},
	}
}

func (s *chatStreamRuntime) sendKeepAlive() bool {
	if !s.writable {
		return false
	}
	if !s.canFlush {
		return true
	}
	if _, err := s.w.Write([]byte(": keep-alive\n\n")); err != nil {
		s.writable = false
		return false
	}
	if err := s.rc.Flush(); err != nil {
		s.writable = false
		return false
	}
	return true
}

func (s *chatStreamRuntime) sendChunk(v any) bool {
	if !s.writable {
		return false
	}
	b, _ := json.Marshal(v)
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

func (s *chatStreamRuntime) sendDone() bool {
	if !s.writable {
		return false
	}
	if _, err := s.w.Write([]byte("data: [DONE]\n\n")); err != nil {
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

func (s *chatStreamRuntime) finalize(finishReason string) {
	if !s.writable {
		return
	}
	finalThinking := s.thinking.String()
	finalText := s.text.String()
	detected := util.ParseToolCalls(finalText, s.toolNames)
	if len(detected) > 0 && !s.toolCallsEmitted {
		finishReason = "tool_calls"
		delta := map[string]any{
			"tool_calls": util.FormatOpenAIStreamToolCalls(detected),
		}
		if !s.firstChunkSent {
			delta["role"] = "assistant"
			s.firstChunkSent = true
		}
		if !s.sendChunk(openaifmt.BuildChatStreamChunk(
			s.completionID,
			s.created,
			s.model,
			[]map[string]any{openaifmt.BuildChatStreamDeltaChoice(0, delta)},
			nil,
		)) {
			return
		}
	} else if s.bufferToolContent {
		for _, evt := range flushToolSieve(&s.toolSieve, s.toolNames) {
			if evt.Content == "" {
				continue
			}
			delta := map[string]any{
				"content": evt.Content,
			}
			if !s.firstChunkSent {
				delta["role"] = "assistant"
				s.firstChunkSent = true
			}
			if !s.sendChunk(openaifmt.BuildChatStreamChunk(
				s.completionID,
				s.created,
				s.model,
				[]map[string]any{openaifmt.BuildChatStreamDeltaChoice(0, delta)},
				nil,
			)) {
				return
			}
		}
	}

	if len(detected) > 0 || s.toolCallsEmitted {
		finishReason = "tool_calls"
	}
	if !s.sendChunk(openaifmt.BuildChatStreamChunk(
		s.completionID,
		s.created,
		s.model,
		[]map[string]any{openaifmt.BuildChatStreamFinishChoice(0, finishReason)},
		openaifmt.BuildChatUsage(s.finalPrompt, finalThinking, finalText),
	)) {
		return
	}
	s.sendDone()
}

func (s *chatStreamRuntime) onParsed(parsed sse.LineResult) streamengine.ParsedDecision {
	if !s.writable {
		return streamengine.ParsedDecision{Stop: true, StopReason: streamengine.StopReasonHandlerRequested}
	}
	if !parsed.Parsed {
		return streamengine.ParsedDecision{}
	}
	if parsed.ContentFilter || parsed.ErrorMessage != "" {
		return streamengine.ParsedDecision{Stop: true, StopReason: streamengine.StopReason("content_filter")}
	}
	if parsed.Stop {
		return streamengine.ParsedDecision{Stop: true, StopReason: streamengine.StopReasonHandlerRequested}
	}

	newChoices := make([]map[string]any, 0, len(parsed.Parts))
	contentSeen := false
	for _, p := range parsed.Parts {
		if s.searchEnabled && sse.IsCitation(p.Text) {
			continue
		}
		if p.Text == "" {
			continue
		}
		contentSeen = true
		delta := map[string]any{}
		if !s.firstChunkSent {
			delta["role"] = "assistant"
			s.firstChunkSent = true
		}
		if p.Type == "thinking" {
			if s.thinkingEnabled {
				s.thinking.WriteString(p.Text)
				delta["reasoning_content"] = p.Text
			}
		} else {
			s.text.WriteString(p.Text)
			if !s.bufferToolContent {
				delta["content"] = p.Text
			} else {
				events := processToolSieveChunk(&s.toolSieve, p.Text, s.toolNames)
				for _, evt := range events {
					if len(evt.ToolCallDeltas) > 0 {
						if !s.emitEarlyToolDeltas {
							continue
						}
						s.toolCallsEmitted = true
						tcDelta := map[string]any{
							"tool_calls": formatIncrementalStreamToolCallDeltas(evt.ToolCallDeltas, s.streamToolCallIDs),
						}
						if !s.firstChunkSent {
							tcDelta["role"] = "assistant"
							s.firstChunkSent = true
						}
						newChoices = append(newChoices, openaifmt.BuildChatStreamDeltaChoice(0, tcDelta))
						continue
					}
					if len(evt.ToolCalls) > 0 {
						s.toolCallsEmitted = true
						tcDelta := map[string]any{
							"tool_calls": util.FormatOpenAIStreamToolCalls(evt.ToolCalls),
						}
						if !s.firstChunkSent {
							tcDelta["role"] = "assistant"
							s.firstChunkSent = true
						}
						newChoices = append(newChoices, openaifmt.BuildChatStreamDeltaChoice(0, tcDelta))
						continue
					}
					if evt.Content != "" {
						contentDelta := map[string]any{
							"content": evt.Content,
						}
						if !s.firstChunkSent {
							contentDelta["role"] = "assistant"
							s.firstChunkSent = true
						}
						newChoices = append(newChoices, openaifmt.BuildChatStreamDeltaChoice(0, contentDelta))
					}
				}
			}
		}
		if len(delta) > 0 {
			newChoices = append(newChoices, openaifmt.BuildChatStreamDeltaChoice(0, delta))
		}
	}

	if len(newChoices) > 0 {
		if !s.sendChunk(openaifmt.BuildChatStreamChunk(s.completionID, s.created, s.model, newChoices, nil)) {
			return streamengine.ParsedDecision{Stop: true, StopReason: streamengine.StopReasonHandlerRequested}
		}
	}
	return streamengine.ParsedDecision{ContentSeen: contentSeen}
}
