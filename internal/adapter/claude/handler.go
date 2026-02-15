package claude

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"ds2api/internal/auth"
	"ds2api/internal/config"
	"ds2api/internal/deepseek"
	"ds2api/internal/sse"
	"ds2api/internal/util"
)

type Handler struct {
	Store *config.Store
	Auth  *auth.Resolver
	DS    *deepseek.Client
}

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Get("/anthropic/v1/models", h.ListModels)
	r.Post("/anthropic/v1/messages", h.Messages)
	r.Post("/anthropic/v1/messages/count_tokens", h.CountTokens)
}

func (h *Handler) ListModels(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, config.ClaudeModelsResponse())
}

func (h *Handler) Messages(w http.ResponseWriter, r *http.Request) {
	a, err := h.Auth.Determine(r)
	if err != nil {
		status := http.StatusUnauthorized
		detail := err.Error()
		if err == auth.ErrNoAccount {
			status = http.StatusTooManyRequests
		}
		writeJSON(w, status, map[string]any{"error": map[string]any{"type": "invalid_request_error", "message": detail}})
		return
	}
	defer h.Auth.Release(a)

	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"type": "invalid_request_error", "message": "invalid json"}})
		return
	}
	model, _ := req["model"].(string)
	messagesRaw, _ := req["messages"].([]any)
	if model == "" || len(messagesRaw) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"type": "invalid_request_error", "message": "Request must include 'model' and 'messages'."}})
		return
	}

	normalized := normalizeClaudeMessages(messagesRaw)
	payload := cloneMap(req)
	payload["messages"] = normalized
	toolsRequested, _ := req["tools"].([]any)
	if len(toolsRequested) > 0 && !hasSystemMessage(normalized) {
		payload["messages"] = append([]any{map[string]any{"role": "system", "content": buildClaudeToolPrompt(toolsRequested)}}, normalized...)
	}

	dsPayload := util.ConvertClaudeToDeepSeek(payload, h.Store)
	dsModel, _ := dsPayload["model"].(string)
	thinkingEnabled, searchEnabled, ok := config.GetModelConfig(dsModel)
	if !ok {
		thinkingEnabled = false
		searchEnabled = false
	}
	_ = searchEnabled
	finalPrompt := util.MessagesPrepare(toMessageMaps(dsPayload["messages"]))

	sessionID, err := h.DS.CreateSession(r.Context(), a, 3)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": map[string]any{"type": "api_error", "message": "invalid token."}})
		return
	}
	pow, err := h.DS.GetPow(r.Context(), a, 3)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": map[string]any{"type": "api_error", "message": "Failed to get PoW"}})
		return
	}
	requestPayload := map[string]any{
		"chat_session_id":   sessionID,
		"parent_message_id": nil,
		"prompt":            finalPrompt,
		"ref_file_ids":      []any{},
		"thinking_enabled":  thinkingEnabled,
		"search_enabled":    searchEnabled,
	}
	resp, err := h.DS.CallCompletion(r.Context(), a, requestPayload, pow, 3)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{"type": "api_error", "message": "Failed to get Claude response."}})
		return
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{"type": "api_error", "message": string(body)}})
		return
	}

	fullText, fullThinking := collectDeepSeek(resp, thinkingEnabled)
	toolNames := extractClaudeToolNames(toolsRequested)
	detected := util.ParseToolCalls(fullText, toolNames)
	if toBool(req["stream"]) {
		h.writeClaudeStream(w, r, model, normalized, fullText, detected)
		return
	}
	content := make([]map[string]any, 0, 4)
	if fullThinking != "" {
		content = append(content, map[string]any{"type": "thinking", "thinking": fullThinking})
	}
	stopReason := "end_turn"
	if len(detected) > 0 {
		stopReason = "tool_use"
		for i, tc := range detected {
			content = append(content, map[string]any{
				"type":  "tool_use",
				"id":    fmt.Sprintf("toolu_%d_%d", time.Now().Unix(), i),
				"name":  tc.Name,
				"input": tc.Input,
			})
		}
	} else {
		if fullText == "" {
			fullText = "抱歉，没有生成有效的响应内容。"
		}
		content = append(content, map[string]any{"type": "text", "text": fullText})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":            fmt.Sprintf("msg_%d", time.Now().UnixNano()),
		"type":          "message",
		"role":          "assistant",
		"model":         model,
		"content":       content,
		"stop_reason":   stopReason,
		"stop_sequence": nil,
		"usage": map[string]any{
			"input_tokens":  util.EstimateTokens(fmt.Sprintf("%v", normalized)),
			"output_tokens": util.EstimateTokens(fullThinking) + util.EstimateTokens(fullText),
		},
	})
}

func (h *Handler) CountTokens(w http.ResponseWriter, r *http.Request) {
	a, err := h.Auth.Determine(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
		return
	}
	defer h.Auth.Release(a)

	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	model, _ := req["model"].(string)
	messages, _ := req["messages"].([]any)
	if model == "" || len(messages) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Request must include 'model' and 'messages'."})
		return
	}
	inputTokens := 0
	if sys, ok := req["system"].(string); ok {
		inputTokens += util.EstimateTokens(sys)
	}
	for _, item := range messages {
		msg, ok := item.(map[string]any)
		if !ok {
			continue
		}
		inputTokens += 2
		inputTokens += util.EstimateTokens(extractMessageContent(msg["content"]))
	}
	if tools, ok := req["tools"].([]any); ok {
		for _, t := range tools {
			b, _ := json.Marshal(t)
			inputTokens += util.EstimateTokens(string(b))
		}
	}
	if inputTokens < 1 {
		inputTokens = 1
	}
	writeJSON(w, http.StatusOK, map[string]any{"input_tokens": inputTokens})
}

func collectDeepSeek(resp *http.Response, thinkingEnabled bool) (string, string) {
	defer resp.Body.Close()
	text := strings.Builder{}
	thinking := strings.Builder{}
	currentType := "text"
	if thinkingEnabled {
		currentType = "thinking"
	}
	scanner := bufio.NewScanner(resp.Body)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 2*1024*1024)
	for scanner.Scan() {
		chunk, done, ok := sse.ParseDeepSeekSSELine(scanner.Bytes())
		if !ok {
			continue
		}
		if done {
			break
		}
		parts, finished, newType := sse.ParseSSEChunkForContent(chunk, thinkingEnabled, currentType)
		currentType = newType
		if finished {
			break
		}
		for _, p := range parts {
			if p.Type == "thinking" {
				thinking.WriteString(p.Text)
			} else {
				text.WriteString(p.Text)
			}
		}
	}
	return text.String(), thinking.String()
}

func (h *Handler) writeClaudeStream(w http.ResponseWriter, r *http.Request, model string, messages []any, fullText string, detected []util.ParsedToolCall) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{"type": "api_error", "message": "streaming unsupported"}})
		return
	}
	send := func(v any) {
		b, _ := json.Marshal(v)
		_, _ = w.Write([]byte("data: "))
		_, _ = w.Write(b)
		_, _ = w.Write([]byte("\n\n"))
		flusher.Flush()
	}
	messageID := fmt.Sprintf("msg_%d", time.Now().UnixNano())
	inputTokens := util.EstimateTokens(fmt.Sprintf("%v", messages))
	send(map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":            messageID,
			"type":          "message",
			"role":          "assistant",
			"model":         model,
			"content":       []any{},
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage":         map[string]any{"input_tokens": inputTokens, "output_tokens": 0},
		},
	})
	outputTokens := 0
	stopReason := "end_turn"
	if len(detected) > 0 {
		stopReason = "tool_use"
		for i, tc := range detected {
			send(map[string]any{"type": "content_block_start", "index": i, "content_block": map[string]any{"type": "tool_use", "id": fmt.Sprintf("toolu_%d_%d", time.Now().Unix(), i), "name": tc.Name, "input": tc.Input}})
			send(map[string]any{"type": "content_block_stop", "index": i})
			outputTokens += util.EstimateTokens(fmt.Sprintf("%v", tc.Input))
		}
	} else {
		if fullText != "" {
			send(map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "text", "text": ""}})
			send(map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": fullText}})
			send(map[string]any{"type": "content_block_stop", "index": 0})
			outputTokens = util.EstimateTokens(fullText)
		}
	}
	send(map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": stopReason, "stop_sequence": nil}, "usage": map[string]any{"output_tokens": outputTokens}})
	send(map[string]any{"type": "message_stop"})
	_ = r
}

func normalizeClaudeMessages(messages []any) []any {
	out := make([]any, 0, len(messages))
	for _, m := range messages {
		msg, ok := m.(map[string]any)
		if !ok {
			continue
		}
		copied := cloneMap(msg)
		switch content := msg["content"].(type) {
		case []any:
			parts := make([]string, 0, len(content))
			for _, block := range content {
				b, ok := block.(map[string]any)
				if !ok {
					continue
				}
				typeStr, _ := b["type"].(string)
				if typeStr == "text" {
					if t, ok := b["text"].(string); ok {
						parts = append(parts, t)
					}
				}
				if typeStr == "tool_result" {
					parts = append(parts, fmt.Sprintf("%v", b["content"]))
				}
			}
			copied["content"] = strings.Join(parts, "\n")
		}
		out = append(out, copied)
	}
	return out
}

func buildClaudeToolPrompt(tools []any) string {
	parts := []string{"You are Claude, a helpful AI assistant. You have access to these tools:"}
	for _, t := range tools {
		m, ok := t.(map[string]any)
		if !ok {
			continue
		}
		name, _ := m["name"].(string)
		desc, _ := m["description"].(string)
		schema, _ := json.Marshal(m["input_schema"])
		parts = append(parts, fmt.Sprintf("Tool: %s\nDescription: %s\nParameters: %s", name, desc, schema))
	}
	parts = append(parts, "When you need to use tools, you can call multiple tools in one response. Output ONLY JSON like {\"tool_calls\":[{\"name\":\"tool\",\"input\":{}}]}")
	return strings.Join(parts, "\n\n")
}

func hasSystemMessage(messages []any) bool {
	for _, m := range messages {
		msg, ok := m.(map[string]any)
		if ok && msg["role"] == "system" {
			return true
		}
	}
	return false
}

func extractClaudeToolNames(tools []any) []string {
	out := make([]string, 0, len(tools))
	for _, t := range tools {
		m, ok := t.(map[string]any)
		if !ok {
			continue
		}
		if name, ok := m["name"].(string); ok && name != "" {
			out = append(out, name)
		}
	}
	return out
}

func toMessageMaps(v any) []map[string]any {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(arr))
	for _, item := range arr {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func extractMessageContent(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []any:
		parts := make([]string, 0, len(x))
		for _, it := range x {
			parts = append(parts, fmt.Sprintf("%v", it))
		}
		return strings.Join(parts, "\n")
	default:
		return fmt.Sprintf("%v", x)
	}
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func toBool(v any) bool {
	b, _ := v.(bool)
	return b
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
