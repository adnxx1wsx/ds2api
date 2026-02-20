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

type responsesStreamRuntime struct {
	w        http.ResponseWriter
	rc       *http.ResponseController
	canFlush bool
	writable bool

	responseID  string
	model       string
	finalPrompt string
	toolNames   []string

	thinkingEnabled bool
	searchEnabled   bool

	bufferToolContent   bool
	emitEarlyToolDeltas bool
	toolCallsEmitted    bool

	sieve             toolStreamSieveState
	thinking          strings.Builder
	text              strings.Builder
	streamToolCallIDs map[int]string

	persistResponse func(obj map[string]any)
}

func newResponsesStreamRuntime(
	w http.ResponseWriter,
	rc *http.ResponseController,
	canFlush bool,
	responseID string,
	model string,
	finalPrompt string,
	thinkingEnabled bool,
	searchEnabled bool,
	toolNames []string,
	bufferToolContent bool,
	emitEarlyToolDeltas bool,
	persistResponse func(obj map[string]any),
) *responsesStreamRuntime {
	return &responsesStreamRuntime{
		w:                   w,
		rc:                  rc,
		canFlush:            canFlush,
		writable:            true,
		responseID:          responseID,
		model:               model,
		finalPrompt:         finalPrompt,
		thinkingEnabled:     thinkingEnabled,
		searchEnabled:       searchEnabled,
		toolNames:           toolNames,
		bufferToolContent:   bufferToolContent,
		emitEarlyToolDeltas: emitEarlyToolDeltas,
		streamToolCallIDs:   map[int]string{},
		persistResponse:     persistResponse,
	}
}

func (s *responsesStreamRuntime) sendEvent(event string, payload map[string]any) bool {
	if !s.writable {
		return false
	}
	b, _ := json.Marshal(payload)
	if _, err := s.w.Write([]byte("event: " + event + "\n")); err != nil {
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

func (s *responsesStreamRuntime) sendKeepAlive() bool {
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

func (s *responsesStreamRuntime) sendCreated() bool {
	return s.sendEvent("response.created", openaifmt.BuildResponsesCreatedPayload(s.responseID, s.model))
}

func (s *responsesStreamRuntime) sendDone() bool {
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

func (s *responsesStreamRuntime) finalize() {
	if !s.writable {
		return
	}
	finalThinking := s.thinking.String()
	finalText := s.text.String()
	if s.bufferToolContent {
		for _, evt := range flushToolSieve(&s.sieve, s.toolNames) {
			if evt.Content != "" {
				if !s.sendEvent("response.output_text.delta", openaifmt.BuildResponsesTextDeltaPayload(s.responseID, evt.Content)) {
					return
				}
			}
			if len(evt.ToolCalls) > 0 {
				s.toolCallsEmitted = true
				if !s.sendEvent("response.output_tool_call.done", openaifmt.BuildResponsesToolCallDonePayload(s.responseID, util.FormatOpenAIStreamToolCalls(evt.ToolCalls))) {
					return
				}
			}
		}
	}

	obj := openaifmt.BuildResponseObject(s.responseID, s.model, s.finalPrompt, finalThinking, finalText, s.toolNames)
	if s.toolCallsEmitted {
		obj["status"] = "completed"
	}
	if s.persistResponse != nil {
		s.persistResponse(obj)
	}
	if !s.sendEvent("response.completed", openaifmt.BuildResponsesCompletedPayload(obj)) {
		return
	}
	s.sendDone()
}

func (s *responsesStreamRuntime) onParsed(parsed sse.LineResult) streamengine.ParsedDecision {
	if !s.writable {
		return streamengine.ParsedDecision{Stop: true, StopReason: streamengine.StopReasonHandlerRequested}
	}
	if !parsed.Parsed {
		return streamengine.ParsedDecision{}
	}
	if parsed.ContentFilter || parsed.ErrorMessage != "" || parsed.Stop {
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
			if !s.sendEvent("response.reasoning.delta", openaifmt.BuildResponsesReasoningDeltaPayload(s.responseID, p.Text)) {
				return streamengine.ParsedDecision{Stop: true, StopReason: streamengine.StopReasonHandlerRequested}
			}
			continue
		}

		s.text.WriteString(p.Text)
		if !s.bufferToolContent {
			if !s.sendEvent("response.output_text.delta", openaifmt.BuildResponsesTextDeltaPayload(s.responseID, p.Text)) {
				return streamengine.ParsedDecision{Stop: true, StopReason: streamengine.StopReasonHandlerRequested}
			}
			continue
		}
		for _, evt := range processToolSieveChunk(&s.sieve, p.Text, s.toolNames) {
			if evt.Content != "" {
				if !s.sendEvent("response.output_text.delta", openaifmt.BuildResponsesTextDeltaPayload(s.responseID, evt.Content)) {
					return streamengine.ParsedDecision{Stop: true, StopReason: streamengine.StopReasonHandlerRequested}
				}
			}
			if len(evt.ToolCallDeltas) > 0 {
				if !s.emitEarlyToolDeltas {
					continue
				}
				s.toolCallsEmitted = true
				if !s.sendEvent("response.output_tool_call.delta", openaifmt.BuildResponsesToolCallDeltaPayload(s.responseID, formatIncrementalStreamToolCallDeltas(evt.ToolCallDeltas, s.streamToolCallIDs))) {
					return streamengine.ParsedDecision{Stop: true, StopReason: streamengine.StopReasonHandlerRequested}
				}
			}
			if len(evt.ToolCalls) > 0 {
				s.toolCallsEmitted = true
				if !s.sendEvent("response.output_tool_call.done", openaifmt.BuildResponsesToolCallDonePayload(s.responseID, util.FormatOpenAIStreamToolCalls(evt.ToolCalls))) {
					return streamengine.ParsedDecision{Stop: true, StopReason: streamengine.StopReasonHandlerRequested}
				}
			}
		}
	}

	return streamengine.ParsedDecision{ContentSeen: contentSeen}
}
