package openai

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"ds2api/internal/auth"
	"ds2api/internal/deepseek"
	openaifmt "ds2api/internal/format/openai"
	"ds2api/internal/sse"
	streamengine "ds2api/internal/stream"
)

func (h *Handler) GetResponseByID(w http.ResponseWriter, r *http.Request) {
	a, err := h.Auth.DetermineCaller(r)
	if err != nil {
		writeOpenAIError(w, http.StatusUnauthorized, err.Error())
		return
	}

	id := strings.TrimSpace(chi.URLParam(r, "response_id"))
	if id == "" {
		writeOpenAIError(w, http.StatusBadRequest, "response_id is required.")
		return
	}
	owner := responseStoreOwner(a)
	if owner == "" {
		writeOpenAIError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	st := h.getResponseStore()
	item, ok := st.get(owner, id)
	if !ok {
		writeOpenAIError(w, http.StatusNotFound, "Response not found.")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) Responses(w http.ResponseWriter, r *http.Request) {
	a, err := h.Auth.Determine(r)
	if err != nil {
		status := http.StatusUnauthorized
		detail := err.Error()
		if err == auth.ErrNoAccount {
			status = http.StatusTooManyRequests
		}
		writeOpenAIError(w, status, detail)
		return
	}
	defer h.Auth.Release(a)
	r = r.WithContext(auth.WithAuth(r.Context(), a))
	owner := responseStoreOwner(a)
	if owner == "" {
		writeOpenAIError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid json")
		return
	}
	stdReq, err := normalizeOpenAIResponsesRequest(h.Store, req)
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, err.Error())
		return
	}

	sessionID, err := h.DS.CreateSession(r.Context(), a, 3)
	if err != nil {
		if a.UseConfigToken {
			writeOpenAIError(w, http.StatusUnauthorized, "Account token is invalid. Please re-login the account in admin.")
		} else {
			writeOpenAIError(w, http.StatusUnauthorized, "Invalid token. If this should be a DS2API key, add it to config.keys first.")
		}
		return
	}
	pow, err := h.DS.GetPow(r.Context(), a, 3)
	if err != nil {
		writeOpenAIError(w, http.StatusUnauthorized, "Failed to get PoW (invalid token or unknown error).")
		return
	}
	payload := stdReq.CompletionPayload(sessionID)
	resp, err := h.DS.CallCompletion(r.Context(), a, payload, pow, 3)
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, "Failed to get completion.")
		return
	}

	responseID := "resp_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if stdReq.Stream {
		h.handleResponsesStream(w, r, resp, owner, responseID, stdReq.ResponseModel, stdReq.FinalPrompt, stdReq.Thinking, stdReq.Search, stdReq.ToolNames)
		return
	}
	h.handleResponsesNonStream(w, resp, owner, responseID, stdReq.ResponseModel, stdReq.FinalPrompt, stdReq.Thinking, stdReq.ToolNames)
}

func (h *Handler) handleResponsesNonStream(w http.ResponseWriter, resp *http.Response, owner, responseID, model, finalPrompt string, thinkingEnabled bool, toolNames []string) {
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		writeOpenAIError(w, resp.StatusCode, strings.TrimSpace(string(body)))
		return
	}
	result := sse.CollectStream(resp, thinkingEnabled, true)
	responseObj := openaifmt.BuildResponseObject(responseID, model, finalPrompt, result.Thinking, result.Text, toolNames)
	h.getResponseStore().put(owner, responseID, responseObj)
	writeJSON(w, http.StatusOK, responseObj)
}

func (h *Handler) handleResponsesStream(w http.ResponseWriter, r *http.Request, resp *http.Response, owner, responseID, model, finalPrompt string, thinkingEnabled, searchEnabled bool, toolNames []string) {
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		writeOpenAIError(w, resp.StatusCode, strings.TrimSpace(string(body)))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	rc := http.NewResponseController(w)
	canFlush := rc.Flush() == nil

	initialType := "text"
	if thinkingEnabled {
		initialType = "thinking"
	}
	bufferToolContent := len(toolNames) > 0 && h.toolcallFeatureMatchEnabled()
	emitEarlyToolDeltas := h.toolcallEarlyEmitHighConfidence()

	streamRuntime := newResponsesStreamRuntime(
		w,
		rc,
		canFlush,
		responseID,
		model,
		finalPrompt,
		thinkingEnabled,
		searchEnabled,
		toolNames,
		bufferToolContent,
		emitEarlyToolDeltas,
		func(obj map[string]any) {
			h.getResponseStore().put(owner, responseID, obj)
		},
	)
	// If downstream is already closed, runtime marks itself non-writable.
	// We still enter ConsumeSSE so upstream body is canceled via request context
	// and account slots are released deterministically.
	_ = streamRuntime.sendCreated()

	streamengine.ConsumeSSE(streamengine.ConsumeConfig{
		Context:             r.Context(),
		Body:                resp.Body,
		ThinkingEnabled:     thinkingEnabled,
		InitialType:         initialType,
		KeepAliveInterval:   time.Duration(deepseek.KeepAliveTimeout) * time.Second,
		IdleTimeout:         time.Duration(deepseek.StreamIdleTimeout) * time.Second,
		MaxKeepAliveNoInput: deepseek.MaxKeepaliveCount,
	}, streamengine.ConsumeHooks{
		OnKeepAlive: func() bool {
			return streamRuntime.sendKeepAlive()
		},
		OnParsed: streamRuntime.onParsed,
		OnFinalize: func(_ streamengine.StopReason, _ error) {
			streamRuntime.finalize()
		},
	})
}

func responsesMessagesFromRequest(req map[string]any) []any {
	if msgs, ok := req["messages"].([]any); ok && len(msgs) > 0 {
		return prependInstructionMessage(msgs, req["instructions"])
	}
	if rawInput, ok := req["input"]; ok {
		if msgs := normalizeResponsesInputAsMessages(rawInput); len(msgs) > 0 {
			return prependInstructionMessage(msgs, req["instructions"])
		}
	}
	return nil
}

func prependInstructionMessage(messages []any, instructions any) []any {
	sys, _ := instructions.(string)
	sys = strings.TrimSpace(sys)
	if sys == "" {
		return messages
	}
	out := make([]any, 0, len(messages)+1)
	out = append(out, map[string]any{"role": "system", "content": sys})
	out = append(out, messages...)
	return out
}

func normalizeResponsesInputAsMessages(input any) []any {
	switch v := input.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return []any{map[string]any{"role": "user", "content": v}}
	case []any:
		if len(v) == 0 {
			return nil
		}
		// If caller already provides role-shaped items, keep as-is.
		if first, ok := v[0].(map[string]any); ok {
			if _, hasRole := first["role"]; hasRole {
				return v
			}
		}
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				if t, _ := m["type"].(string); strings.EqualFold(strings.TrimSpace(t), "input_text") {
					if txt, _ := m["text"].(string); strings.TrimSpace(txt) != "" {
						parts = append(parts, txt)
						continue
					}
				}
			}
			if s := strings.TrimSpace(fmt.Sprintf("%v", item)); s != "" {
				parts = append(parts, s)
			}
		}
		if len(parts) == 0 {
			return nil
		}
		return []any{map[string]any{"role": "user", "content": strings.Join(parts, "\n")}}
	case map[string]any:
		if txt, _ := v["text"].(string); strings.TrimSpace(txt) != "" {
			return []any{map[string]any{"role": "user", "content": txt}}
		}
		if content, ok := v["content"].(string); ok && strings.TrimSpace(content) != "" {
			return []any{map[string]any{"role": "user", "content": content}}
		}
	}
	return nil
}
