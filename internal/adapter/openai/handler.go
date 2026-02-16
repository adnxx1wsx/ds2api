package openai

import (
	"bufio"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
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
	r.Get("/v1/models", h.ListModels)
	r.Post("/v1/chat/completions", h.ChatCompletions)
}

func (h *Handler) ListModels(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, config.OpenAIModelsResponse())
}

func (h *Handler) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	if isVercelStreamPrepareRequest(r) {
		h.handleVercelStreamPrepare(w, r)
		return
	}

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

	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid json")
		return
	}
	model, _ := req["model"].(string)
	messagesRaw, _ := req["messages"].([]any)
	if model == "" || len(messagesRaw) == 0 {
		writeOpenAIError(w, http.StatusBadRequest, "Request must include 'model' and 'messages'.")
		return
	}
	thinkingEnabled, searchEnabled, ok := config.GetModelConfig(model)
	if !ok {
		writeOpenAIError(w, http.StatusServiceUnavailable, fmt.Sprintf("Model '%s' is not available.", model))
		return
	}

	messages := normalizeMessages(messagesRaw)
	toolNames := []string{}
	if tools, ok := req["tools"].([]any); ok && len(tools) > 0 {
		messages, toolNames = injectToolPrompt(messages, tools)
	}
	finalPrompt := util.MessagesPrepare(messages)

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
	payload := map[string]any{
		"chat_session_id":   sessionID,
		"parent_message_id": nil,
		"prompt":            finalPrompt,
		"ref_file_ids":      []any{},
		"thinking_enabled":  thinkingEnabled,
		"search_enabled":    searchEnabled,
	}
	resp, err := h.DS.CallCompletion(r.Context(), a, payload, pow, 3)
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, "Failed to get completion.")
		return
	}
	if toBool(req["stream"]) {
		h.handleStream(w, r, resp, sessionID, model, finalPrompt, thinkingEnabled, searchEnabled, toolNames)
		return
	}
	h.handleNonStream(w, r.Context(), resp, sessionID, model, finalPrompt, thinkingEnabled, searchEnabled, toolNames)
}

func (h *Handler) handleVercelStreamPrepare(w http.ResponseWriter, r *http.Request) {
	if !config.IsVercel() {
		http.NotFound(w, r)
		return
	}
	internalSecret := vercelInternalSecret()
	internalToken := strings.TrimSpace(r.Header.Get("X-Ds2-Internal-Token"))
	if internalSecret == "" || subtle.ConstantTimeCompare([]byte(internalToken), []byte(internalSecret)) != 1 {
		writeOpenAIError(w, http.StatusUnauthorized, "unauthorized internal request")
		return
	}

	a, err := h.Auth.Determine(r)
	if err != nil {
		status := http.StatusUnauthorized
		if err == auth.ErrNoAccount {
			status = http.StatusTooManyRequests
		}
		writeOpenAIError(w, status, err.Error())
		return
	}
	defer h.Auth.Release(a)
	r = r.WithContext(auth.WithAuth(r.Context(), a))

	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if !toBool(req["stream"]) {
		writeOpenAIError(w, http.StatusBadRequest, "stream must be true")
		return
	}
	if tools, ok := req["tools"].([]any); ok && len(tools) > 0 {
		writeOpenAIError(w, http.StatusBadRequest, "tools are not supported by vercel stream prepare")
		return
	}

	model, _ := req["model"].(string)
	messagesRaw, _ := req["messages"].([]any)
	if model == "" || len(messagesRaw) == 0 {
		writeOpenAIError(w, http.StatusBadRequest, "Request must include 'model' and 'messages'.")
		return
	}
	thinkingEnabled, searchEnabled, ok := config.GetModelConfig(model)
	if !ok {
		writeOpenAIError(w, http.StatusServiceUnavailable, fmt.Sprintf("Model '%s' is not available.", model))
		return
	}

	messages := normalizeMessages(messagesRaw)
	finalPrompt := util.MessagesPrepare(messages)

	sessionID, err := h.DS.CreateSession(r.Context(), a, 3)
	if err != nil {
		if a.UseConfigToken {
			writeOpenAIError(w, http.StatusUnauthorized, "Account token is invalid. Please re-login the account in admin.")
		} else {
			writeOpenAIError(w, http.StatusUnauthorized, "Invalid token. If this should be a DS2API key, add it to config.keys first.")
		}
		return
	}
	powHeader, err := h.DS.GetPow(r.Context(), a, 3)
	if err != nil {
		writeOpenAIError(w, http.StatusUnauthorized, "Failed to get PoW (invalid token or unknown error).")
		return
	}
	if strings.TrimSpace(a.DeepSeekToken) == "" {
		writeOpenAIError(w, http.StatusUnauthorized, "Invalid token. If this should be a DS2API key, add it to config.keys first.")
		return
	}

	payload := map[string]any{
		"chat_session_id":   sessionID,
		"parent_message_id": nil,
		"prompt":            finalPrompt,
		"ref_file_ids":      []any{},
		"thinking_enabled":  thinkingEnabled,
		"search_enabled":    searchEnabled,
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"session_id":       sessionID,
		"model":            model,
		"final_prompt":     finalPrompt,
		"thinking_enabled": thinkingEnabled,
		"search_enabled":   searchEnabled,
		"deepseek_token":   a.DeepSeekToken,
		"pow_header":       powHeader,
		"payload":          payload,
	})
}

func (h *Handler) handleNonStream(w http.ResponseWriter, ctx context.Context, resp *http.Response, completionID, model, finalPrompt string, thinkingEnabled, searchEnabled bool, toolNames []string) {
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		writeOpenAIError(w, resp.StatusCode, string(body))
		return
	}
	thinking := strings.Builder{}
	text := strings.Builder{}
	currentType := "text"
	if thinkingEnabled {
		currentType = "thinking"
	}
	_ = ctx
	_ = deepseek.ScanSSELines(resp, func(line []byte) bool {
		chunk, done, ok := sse.ParseDeepSeekSSELine(line)
		if !ok {
			return true
		}
		if done {
			return false
		}
		if _, hasErr := chunk["error"]; hasErr {
			return false
		}
		parts, finished, newType := sse.ParseSSEChunkForContent(chunk, thinkingEnabled, currentType)
		currentType = newType
		if finished {
			return false
		}
		for _, p := range parts {
			if searchEnabled && sse.IsCitation(p.Text) {
				continue
			}
			if p.Type == "thinking" {
				thinking.WriteString(p.Text)
			} else {
				text.WriteString(p.Text)
			}
		}
		return true
	})

	finalThinking := thinking.String()
	finalText := text.String()
	detected := util.ParseToolCalls(finalText, toolNames)
	finishReason := "stop"
	messageObj := map[string]any{"role": "assistant", "content": finalText}
	if thinkingEnabled && finalThinking != "" {
		messageObj["reasoning_content"] = finalThinking
	}
	if len(detected) > 0 {
		finishReason = "tool_calls"
		messageObj["tool_calls"] = util.FormatOpenAIToolCalls(detected)
		messageObj["content"] = nil
	}
	promptTokens := util.EstimateTokens(finalPrompt)
	reasoningTokens := util.EstimateTokens(finalThinking)
	completionTokens := util.EstimateTokens(finalText)

	writeJSON(w, http.StatusOK, map[string]any{
		"id":      completionID,
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{{"index": 0, "message": messageObj, "finish_reason": finishReason}},
		"usage": map[string]any{
			"prompt_tokens":     promptTokens,
			"completion_tokens": reasoningTokens + completionTokens,
			"total_tokens":      promptTokens + reasoningTokens + completionTokens,
			"completion_tokens_details": map[string]any{
				"reasoning_tokens": reasoningTokens,
			},
		},
	})
}

func (h *Handler) handleStream(w http.ResponseWriter, r *http.Request, resp *http.Response, completionID, model, finalPrompt string, thinkingEnabled, searchEnabled bool, toolNames []string) {
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		writeOpenAIError(w, resp.StatusCode, string(body))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	rc := http.NewResponseController(w)
	canFlush := rc.Flush() == nil
	if !canFlush {
		config.Logger.Warn("[stream] response writer does not support flush; streaming may be buffered")
	}

	lines := make(chan []byte, 128)
	done := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(resp.Body)
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 2*1024*1024)
		for scanner.Scan() {
			b := append([]byte{}, scanner.Bytes()...)
			lines <- b
		}
		close(lines)
		done <- scanner.Err()
	}()

	created := time.Now().Unix()
	firstChunkSent := false
	bufferToolContent := len(toolNames) > 0
	currentType := "text"
	if thinkingEnabled {
		currentType = "thinking"
	}
	thinking := strings.Builder{}
	text := strings.Builder{}
	lastContent := time.Now()
	hasContent := false
	keepaliveTicker := time.NewTicker(time.Duration(deepseek.KeepAliveTimeout) * time.Second)
	defer keepaliveTicker.Stop()
	keepaliveCountWithoutContent := 0

	sendChunk := func(v any) {
		b, _ := json.Marshal(v)
		_, _ = w.Write([]byte("data: "))
		_, _ = w.Write(b)
		_, _ = w.Write([]byte("\n\n"))
		if canFlush {
			_ = rc.Flush()
		}
	}
	sendDone := func() {
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		if canFlush {
			_ = rc.Flush()
		}
	}

	finalize := func(finishReason string) {
		finalThinking := thinking.String()
		finalText := text.String()
		detected := util.ParseToolCalls(finalText, toolNames)
		if len(detected) > 0 {
			finishReason = "tool_calls"
			delta := map[string]any{
				"tool_calls": util.FormatOpenAIStreamToolCalls(detected),
			}
			if !firstChunkSent {
				delta["role"] = "assistant"
				firstChunkSent = true
			}
			sendChunk(map[string]any{
				"id":      completionID,
				"object":  "chat.completion.chunk",
				"created": created,
				"model":   model,
				"choices": []map[string]any{{"delta": delta, "index": 0}},
			})
		} else if bufferToolContent && strings.TrimSpace(finalText) != "" {
			delta := map[string]any{
				"content": finalText,
			}
			if !firstChunkSent {
				delta["role"] = "assistant"
				firstChunkSent = true
			}
			sendChunk(map[string]any{
				"id":      completionID,
				"object":  "chat.completion.chunk",
				"created": created,
				"model":   model,
				"choices": []map[string]any{{"delta": delta, "index": 0}},
			})
		}
		promptTokens := util.EstimateTokens(finalPrompt)
		reasoningTokens := util.EstimateTokens(finalThinking)
		completionTokens := util.EstimateTokens(finalText)
		sendChunk(map[string]any{
			"id":      completionID,
			"object":  "chat.completion.chunk",
			"created": created,
			"model":   model,
			"choices": []map[string]any{{"delta": map[string]any{}, "index": 0, "finish_reason": finishReason}},
			"usage": map[string]any{
				"prompt_tokens":     promptTokens,
				"completion_tokens": reasoningTokens + completionTokens,
				"total_tokens":      promptTokens + reasoningTokens + completionTokens,
				"completion_tokens_details": map[string]any{
					"reasoning_tokens": reasoningTokens,
				},
			},
		})
		sendDone()
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepaliveTicker.C:
			if !hasContent {
				keepaliveCountWithoutContent++
				if keepaliveCountWithoutContent >= deepseek.MaxKeepaliveCount {
					finalize("stop")
					return
				}
			}
			if hasContent && time.Since(lastContent) > time.Duration(deepseek.StreamIdleTimeout)*time.Second {
				finalize("stop")
				return
			}
			if canFlush {
				_, _ = w.Write([]byte(": keep-alive\n\n"))
				_ = rc.Flush()
			}
		case line, ok := <-lines:
			if !ok {
				// Ensure scanner completion is observed only after all queued
				// SSE lines are drained, avoiding early finalize races.
				_ = <-done
				finalize("stop")
				return
			}
			chunk, doneSignal, parsed := sse.ParseDeepSeekSSELine(line)
			if !parsed {
				continue
			}
			if doneSignal {
				finalize("stop")
				return
			}
			if _, hasErr := chunk["error"]; hasErr || chunk["code"] == "content_filter" {
				finalize("content_filter")
				return
			}
			parts, finished, newType := sse.ParseSSEChunkForContent(chunk, thinkingEnabled, currentType)
			currentType = newType
			if finished {
				finalize("stop")
				return
			}
			newChoices := make([]map[string]any, 0, len(parts))
			for _, p := range parts {
				if searchEnabled && sse.IsCitation(p.Text) {
					continue
				}
				if p.Text == "" {
					continue
				}
				hasContent = true
				lastContent = time.Now()
				keepaliveCountWithoutContent = 0
				delta := map[string]any{}
				if !firstChunkSent {
					delta["role"] = "assistant"
					firstChunkSent = true
				}
				if p.Type == "thinking" {
					if thinkingEnabled {
						thinking.WriteString(p.Text)
						delta["reasoning_content"] = p.Text
					}
				} else {
					text.WriteString(p.Text)
					if !bufferToolContent {
						delta["content"] = p.Text
					}
				}
				if len(delta) > 0 {
					newChoices = append(newChoices, map[string]any{"delta": delta, "index": 0})
				}
			}
			if len(newChoices) > 0 {
				sendChunk(map[string]any{
					"id":      completionID,
					"object":  "chat.completion.chunk",
					"created": created,
					"model":   model,
					"choices": newChoices,
				})
			}
		}
	}
}

func normalizeMessages(raw []any) []map[string]any {
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if ok {
			out = append(out, m)
		}
	}
	return out
}

func injectToolPrompt(messages []map[string]any, tools []any) ([]map[string]any, []string) {
	toolSchemas := make([]string, 0, len(tools))
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		tool, ok := t.(map[string]any)
		if !ok {
			continue
		}
		fn, _ := tool["function"].(map[string]any)
		if len(fn) == 0 {
			fn = tool
		}
		name, _ := fn["name"].(string)
		desc, _ := fn["description"].(string)
		schema, _ := fn["parameters"].(map[string]any)
		if name == "" {
			name = "unknown"
		}
		names = append(names, name)
		if desc == "" {
			desc = "No description available"
		}
		b, _ := json.Marshal(schema)
		toolSchemas = append(toolSchemas, fmt.Sprintf("Tool: %s\nDescription: %s\nParameters: %s", name, desc, string(b)))
	}
	if len(toolSchemas) == 0 {
		return messages, names
	}
	toolPrompt := "You have access to these tools:\n\n" + strings.Join(toolSchemas, "\n\n") + "\n\nWhen you need to use tools, output ONLY this JSON format (no other text):\n{\"tool_calls\": [{\"name\": \"tool_name\", \"input\": {\"param\": \"value\"}}]}\n\nIMPORTANT: If calling tools, output ONLY the JSON. The response must start with { and end with }"

	for i := range messages {
		if messages[i]["role"] == "system" {
			old, _ := messages[i]["content"].(string)
			messages[i]["content"] = strings.TrimSpace(old + "\n\n" + toolPrompt)
			return messages, names
		}
	}
	messages = append([]map[string]any{{"role": "system", "content": toolPrompt}}, messages...)
	return messages, names
}

func toBool(v any) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeOpenAIError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"message": message,
			"type":    openAIErrorType(status),
		},
	})
}

func openAIErrorType(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "invalid_request_error"
	case http.StatusUnauthorized:
		return "authentication_error"
	case http.StatusForbidden:
		return "permission_error"
	case http.StatusTooManyRequests:
		return "rate_limit_error"
	case http.StatusServiceUnavailable:
		return "service_unavailable_error"
	default:
		if status >= 500 {
			return "api_error"
		}
		return "invalid_request_error"
	}
}

func isVercelStreamPrepareRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	return strings.TrimSpace(r.URL.Query().Get("__stream_prepare")) == "1"
}

func vercelInternalSecret() string {
	if v := strings.TrimSpace(os.Getenv("DS2API_VERCEL_INTERNAL_SECRET")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("DS2API_ADMIN_KEY")); v != "" {
		return v
	}
	return "admin"
}
