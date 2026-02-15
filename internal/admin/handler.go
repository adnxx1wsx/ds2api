package admin

import (
	"bufio"
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"ds2api/internal/account"
	authn "ds2api/internal/auth"
	"ds2api/internal/config"
	"ds2api/internal/deepseek"
	"ds2api/internal/sse"
)

type Handler struct {
	Store *config.Store
	Pool  *account.Pool
	DS    *deepseek.Client
}

func RegisterRoutes(r chi.Router, h *Handler) {

	r.Post("/login", h.login)
	r.Get("/verify", h.verify)
	r.Group(func(pr chi.Router) {
		pr.Use(h.requireAdmin)
		pr.Get("/vercel/config", h.getVercelConfig)
		pr.Get("/config", h.getConfig)
		pr.Post("/config", h.updateConfig)
		pr.Post("/keys", h.addKey)
		pr.Delete("/keys/{key}", h.deleteKey)
		pr.Get("/accounts", h.listAccounts)
		pr.Post("/accounts", h.addAccount)
		pr.Delete("/accounts/{identifier}", h.deleteAccount)
		pr.Get("/queue/status", h.queueStatus)
		pr.Post("/accounts/test", h.testSingleAccount)
		pr.Post("/accounts/test-all", h.testAllAccounts)
		pr.Post("/import", h.batchImport)
		pr.Post("/test", h.testAPI)
		pr.Post("/vercel/sync", h.syncVercel)
		pr.Get("/vercel/status", h.vercelStatus)
		pr.Get("/export", h.exportConfig)
	})
}

func (h *Handler) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := authn.VerifyAdminRequest(r); err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"detail": err.Error()})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	_ = json.NewDecoder(r.Body).Decode(&req)
	adminKey, _ := req["admin_key"].(string)
	expireHours := intFrom(req["expire_hours"])
	if expireHours <= 0 {
		expireHours = 24
	}
	if adminKey != authn.AdminKey() {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"detail": "Invalid admin key"})
		return
	}
	token, err := authn.CreateJWT(expireHours)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "token": token, "expires_in": expireHours * 3600})
}

func (h *Handler) verify(w http.ResponseWriter, r *http.Request) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(strings.ToLower(header), "bearer ") {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"detail": "No credentials provided"})
		return
	}
	token := strings.TrimSpace(header[7:])
	payload, err := authn.VerifyJWT(token)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"detail": err.Error()})
		return
	}
	exp, _ := payload["exp"].(float64)
	remaining := int64(exp) - time.Now().Unix()
	if remaining < 0 {
		remaining = 0
	}
	writeJSON(w, http.StatusOK, map[string]any{"valid": true, "expires_at": int64(exp), "remaining_seconds": remaining})
}

func (h *Handler) getVercelConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"has_token":  strings.TrimSpace(os.Getenv("VERCEL_TOKEN")) != "",
		"project_id": strings.TrimSpace(os.Getenv("VERCEL_PROJECT_ID")),
		"team_id":    nilIfEmpty(strings.TrimSpace(os.Getenv("VERCEL_TEAM_ID"))),
	})
}

func (h *Handler) getConfig(w http.ResponseWriter, _ *http.Request) {
	snap := h.Store.Snapshot()
	safe := map[string]any{
		"keys":     snap.Keys,
		"accounts": []map[string]any{},
		"claude_mapping": func() map[string]string {
			if len(snap.ClaudeMapping) > 0 {
				return snap.ClaudeMapping
			}
			return snap.ClaudeModelMap
		}(),
	}
	accounts := make([]map[string]any, 0, len(snap.Accounts))
	for _, acc := range snap.Accounts {
		token := strings.TrimSpace(acc.Token)
		preview := ""
		if token != "" {
			if len(token) > 20 {
				preview = token[:20] + "..."
			} else {
				preview = token
			}
		}
		accounts = append(accounts, map[string]any{
			"email":         acc.Email,
			"mobile":        acc.Mobile,
			"has_password":  strings.TrimSpace(acc.Password) != "",
			"has_token":     token != "",
			"token_preview": preview,
		})
	}
	safe["accounts"] = accounts
	writeJSON(w, http.StatusOK, safe)
}

func (h *Handler) updateConfig(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "invalid json"})
		return
	}
	old := h.Store.Snapshot()
	err := h.Store.Update(func(c *config.Config) error {
		if keys, ok := toStringSlice(req["keys"]); ok {
			c.Keys = keys
		}
		if accountsRaw, ok := req["accounts"].([]any); ok {
			existing := map[string]config.Account{}
			for _, a := range old.Accounts {
				existing[a.Identifier()] = a
			}
			accounts := make([]config.Account, 0, len(accountsRaw))
			for _, item := range accountsRaw {
				m, ok := item.(map[string]any)
				if !ok {
					continue
				}
				acc := toAccount(m)
				id := acc.Identifier()
				if prev, ok := existing[id]; ok {
					if strings.TrimSpace(acc.Password) == "" {
						acc.Password = prev.Password
					}
					if strings.TrimSpace(acc.Token) == "" {
						acc.Token = prev.Token
					}
				}
				accounts = append(accounts, acc)
			}
			c.Accounts = accounts
		}
		if m, ok := req["claude_mapping"].(map[string]any); ok {
			newMap := map[string]string{}
			for k, v := range m {
				newMap[k] = fmt.Sprintf("%v", v)
			}
			c.ClaudeMapping = newMap
		}
		return nil
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"detail": err.Error()})
		return
	}
	h.Pool.Reset()
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "配置已更新"})
}

func (h *Handler) addKey(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	_ = json.NewDecoder(r.Body).Decode(&req)
	key, _ := req["key"].(string)
	key = strings.TrimSpace(key)
	if key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "Key 不能为空"})
		return
	}
	err := h.Store.Update(func(c *config.Config) error {
		for _, k := range c.Keys {
			if k == key {
				return fmt.Errorf("Key 已存在")
			}
		}
		c.Keys = append(c.Keys, key)
		return nil
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "total_keys": len(h.Store.Snapshot().Keys)})
}

func (h *Handler) deleteKey(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	err := h.Store.Update(func(c *config.Config) error {
		idx := -1
		for i, k := range c.Keys {
			if k == key {
				idx = i
				break
			}
		}
		if idx < 0 {
			return fmt.Errorf("Key 不存在")
		}
		c.Keys = append(c.Keys[:idx], c.Keys[idx+1:]...)
		return nil
	})
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "total_keys": len(h.Store.Snapshot().Keys)})
}

func (h *Handler) listAccounts(w http.ResponseWriter, r *http.Request) {
	page := intFromQuery(r, "page", 1)
	pageSize := intFromQuery(r, "page_size", 10)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 1
	}
	if pageSize > 100 {
		pageSize = 100
	}
	accounts := h.Store.Snapshot().Accounts
	total := len(accounts)
	reverseAccounts(accounts)
	totalPages := 1
	if total > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	items := make([]map[string]any, 0, end-start)
	for _, acc := range accounts[start:end] {
		token := strings.TrimSpace(acc.Token)
		preview := ""
		if token != "" {
			if len(token) > 20 {
				preview = token[:20] + "..."
			} else {
				preview = token
			}
		}
		items = append(items, map[string]any{"email": acc.Email, "mobile": acc.Mobile, "has_password": acc.Password != "", "has_token": token != "", "token_preview": preview})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "page": page, "page_size": pageSize, "total_pages": totalPages})
}

func (h *Handler) addAccount(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	_ = json.NewDecoder(r.Body).Decode(&req)
	acc := toAccount(req)
	if acc.Identifier() == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "需要 email 或 mobile"})
		return
	}
	err := h.Store.Update(func(c *config.Config) error {
		for _, a := range c.Accounts {
			if acc.Email != "" && a.Email == acc.Email {
				return fmt.Errorf("邮箱已存在")
			}
			if acc.Mobile != "" && a.Mobile == acc.Mobile {
				return fmt.Errorf("手机号已存在")
			}
		}
		c.Accounts = append(c.Accounts, acc)
		return nil
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": err.Error()})
		return
	}
	h.Pool.Reset()
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "total_accounts": len(h.Store.Snapshot().Accounts)})
}

func (h *Handler) deleteAccount(w http.ResponseWriter, r *http.Request) {
	identifier := chi.URLParam(r, "identifier")
	err := h.Store.Update(func(c *config.Config) error {
		idx := -1
		for i, a := range c.Accounts {
			if a.Email == identifier || a.Mobile == identifier {
				idx = i
				break
			}
		}
		if idx < 0 {
			return fmt.Errorf("账号不存在")
		}
		c.Accounts = append(c.Accounts[:idx], c.Accounts[idx+1:]...)
		return nil
	})
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"detail": err.Error()})
		return
	}
	h.Pool.Reset()
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "total_accounts": len(h.Store.Snapshot().Accounts)})
}

func (h *Handler) queueStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.Pool.Status())
}

func (h *Handler) testSingleAccount(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	_ = json.NewDecoder(r.Body).Decode(&req)
	identifier, _ := req["identifier"].(string)
	if strings.TrimSpace(identifier) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "需要账号标识（email 或 mobile）"})
		return
	}
	acc, ok := h.Store.FindAccount(identifier)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"detail": "账号不存在"})
		return
	}
	model, _ := req["model"].(string)
	if model == "" {
		model = "deepseek-chat"
	}
	message, _ := req["message"].(string)
	result := h.testAccount(r.Context(), acc, model, message)
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) testAllAccounts(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	_ = json.NewDecoder(r.Body).Decode(&req)
	model, _ := req["model"].(string)
	if model == "" {
		model = "deepseek-chat"
	}
	accounts := h.Store.Snapshot().Accounts
	if len(accounts) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"total": 0, "success": 0, "failed": 0, "results": []any{}})
		return
	}
	results := make([]map[string]any, 0, len(accounts))
	success := 0
	for _, acc := range accounts {
		res := h.testAccount(r.Context(), acc, model, "")
		if ok, _ := res["success"].(bool); ok {
			success++
		}
		results = append(results, res)
		time.Sleep(time.Second)
	}
	writeJSON(w, http.StatusOK, map[string]any{"total": len(accounts), "success": success, "failed": len(accounts) - success, "results": results})
}

func (h *Handler) testAccount(ctx context.Context, acc config.Account, model, message string) map[string]any {
	start := time.Now()
	result := map[string]any{"account": acc.Identifier(), "success": false, "response_time": 0, "message": "", "model": model}
	token := strings.TrimSpace(acc.Token)
	if token == "" {
		newToken, err := h.DS.Login(ctx, acc)
		if err != nil {
			result["message"] = "登录失败: " + err.Error()
			return result
		}
		token = newToken
		_ = h.Store.UpdateAccountToken(acc.Identifier(), token)
	}
	authCtx := &authn.RequestAuth{UseConfigToken: false, DeepSeekToken: token}
	sessionID, err := h.DS.CreateSession(ctx, authCtx, 1)
	if err != nil {
		newToken, loginErr := h.DS.Login(ctx, acc)
		if loginErr != nil {
			result["message"] = "创建会话失败: " + err.Error()
			return result
		}
		token = newToken
		authCtx.DeepSeekToken = token
		_ = h.Store.UpdateAccountToken(acc.Identifier(), token)
		sessionID, err = h.DS.CreateSession(ctx, authCtx, 1)
		if err != nil {
			result["message"] = "创建会话失败: " + err.Error()
			return result
		}
	}
	if strings.TrimSpace(message) == "" {
		result["success"] = true
		result["message"] = "API 测试成功（仅会话创建）"
		result["response_time"] = int(time.Since(start).Milliseconds())
		return result
	}
	thinking, search, ok := config.GetModelConfig(model)
	if !ok {
		thinking, search = false, false
	}
	pow, err := h.DS.GetPow(ctx, authCtx, 1)
	if err != nil {
		result["message"] = "获取 PoW 失败: " + err.Error()
		return result
	}
	payload := map[string]any{"chat_session_id": sessionID, "prompt": "<｜User｜>" + message, "ref_file_ids": []any{}, "thinking_enabled": thinking, "search_enabled": search}
	resp, err := h.DS.CallCompletion(ctx, authCtx, payload, pow, 1)
	if err != nil {
		result["message"] = "请求失败: " + err.Error()
		return result
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		result["message"] = fmt.Sprintf("请求失败: HTTP %d", resp.StatusCode)
		return result
	}
	text := strings.Builder{}
	think := strings.Builder{}
	currentType := "text"
	if thinking {
		currentType = "thinking"
	}
	scanner := bufio.NewScanner(resp.Body)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 2*1024*1024)
	for scanner.Scan() {
		chunk, done, parsed := sse.ParseDeepSeekSSELine(scanner.Bytes())
		if !parsed {
			continue
		}
		if done {
			break
		}
		parts, finished, newType := sse.ParseSSEChunkForContent(chunk, thinking, currentType)
		currentType = newType
		if finished {
			break
		}
		for _, p := range parts {
			if p.Type == "thinking" {
				think.WriteString(p.Text)
			} else {
				text.WriteString(p.Text)
			}
		}
	}
	result["success"] = true
	result["response_time"] = int(time.Since(start).Milliseconds())
	if text.Len() > 0 {
		result["message"] = text.String()
	} else {
		result["message"] = "（无回复内容）"
	}
	if think.Len() > 0 {
		result["thinking"] = think.String()
	}
	return result
}

func (h *Handler) batchImport(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "无效的 JSON 格式"})
		return
	}
	importedKeys, importedAccounts := 0, 0
	err := h.Store.Update(func(c *config.Config) error {
		if keys, ok := req["keys"].([]any); ok {
			existing := map[string]bool{}
			for _, k := range c.Keys {
				existing[k] = true
			}
			for _, k := range keys {
				key := strings.TrimSpace(fmt.Sprintf("%v", k))
				if key == "" || existing[key] {
					continue
				}
				c.Keys = append(c.Keys, key)
				existing[key] = true
				importedKeys++
			}
		}
		if accounts, ok := req["accounts"].([]any); ok {
			existing := map[string]bool{}
			for _, a := range c.Accounts {
				existing[a.Identifier()] = true
			}
			for _, item := range accounts {
				m, ok := item.(map[string]any)
				if !ok {
					continue
				}
				acc := toAccount(m)
				id := acc.Identifier()
				if id == "" || existing[id] {
					continue
				}
				c.Accounts = append(c.Accounts, acc)
				existing[id] = true
				importedAccounts++
			}
		}
		return nil
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"detail": err.Error()})
		return
	}
	h.Pool.Reset()
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "imported_keys": importedKeys, "imported_accounts": importedAccounts})
}

func (h *Handler) testAPI(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	_ = json.NewDecoder(r.Body).Decode(&req)
	model, _ := req["model"].(string)
	message, _ := req["message"].(string)
	apiKey, _ := req["api_key"].(string)
	if model == "" {
		model = "deepseek-chat"
	}
	if message == "" {
		message = "你好"
	}
	if apiKey == "" {
		keys := h.Store.Snapshot().Keys
		if len(keys) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "没有可用的 API Key"})
			return
		}
		apiKey = keys[0]
	}
	host := r.Host
	scheme := "http"
	if strings.Contains(strings.ToLower(host), "vercel") || strings.Contains(strings.ToLower(r.Header.Get("X-Forwarded-Proto")), "https") {
		scheme = "https"
	}
	payload := map[string]any{"model": model, "messages": []map[string]any{{"role": "user", "content": message}}, "stream": false}
	b, _ := json.Marshal(payload)
	request, _ := http.NewRequestWithContext(r.Context(), http.MethodPost, fmt.Sprintf("%s://%s/v1/chat/completions", scheme, host), bytes.NewReader(b))
	request.Header.Set("Authorization", "Bearer "+apiKey)
	request.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(request)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"success": false, "error": err.Error()})
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusOK {
		var parsed any
		_ = json.Unmarshal(body, &parsed)
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "status_code": resp.StatusCode, "response": parsed})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": false, "status_code": resp.StatusCode, "response": string(body)})
}

func (h *Handler) syncVercel(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "invalid json"})
		return
	}
	vercelToken, _ := req["vercel_token"].(string)
	projectID, _ := req["project_id"].(string)
	teamID, _ := req["team_id"].(string)
	autoValidate := true
	if v, ok := req["auto_validate"].(bool); ok {
		autoValidate = v
	}
	saveCreds := true
	if v, ok := req["save_credentials"].(bool); ok {
		saveCreds = v
	}
	usePreconfig := vercelToken == "__USE_PRECONFIG__" || strings.TrimSpace(vercelToken) == ""
	if usePreconfig {
		vercelToken = strings.TrimSpace(os.Getenv("VERCEL_TOKEN"))
	}
	if strings.TrimSpace(projectID) == "" {
		projectID = strings.TrimSpace(os.Getenv("VERCEL_PROJECT_ID"))
	}
	if strings.TrimSpace(teamID) == "" {
		teamID = strings.TrimSpace(os.Getenv("VERCEL_TEAM_ID"))
	}
	if vercelToken == "" || projectID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "需要 Vercel Token 和 Project ID"})
		return
	}
	validated, failed := 0, []string{}
	if autoValidate {
		for _, acc := range h.Store.Snapshot().Accounts {
			if strings.TrimSpace(acc.Token) != "" {
				continue
			}
			token, err := h.DS.Login(r.Context(), acc)
			if err != nil {
				failed = append(failed, acc.Identifier())
			} else {
				validated++
				_ = h.Store.UpdateAccountToken(acc.Identifier(), token)
			}
			time.Sleep(500 * time.Millisecond)
		}
	}

	cfgJSON, _, err := h.Store.ExportJSONAndBase64()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"detail": err.Error()})
		return
	}
	cfgB64 := base64.StdEncoding.EncodeToString([]byte(cfgJSON))
	client := &http.Client{Timeout: 30 * time.Second}
	params := url.Values{}
	if teamID != "" {
		params.Set("teamId", teamID)
	}
	headers := map[string]string{"Authorization": "Bearer " + vercelToken}
	envResp, status, err := vercelRequest(r.Context(), client, http.MethodGet, "https://api.vercel.com/v9/projects/"+projectID+"/env", params, headers, nil)
	if err != nil || status != http.StatusOK {
		writeJSON(w, statusOr(status, http.StatusInternalServerError), map[string]any{"detail": "获取环境变量失败"})
		return
	}
	envs, _ := envResp["envs"].([]any)
	existingEnvID := findEnvID(envs, "DS2API_CONFIG_JSON")
	if existingEnvID != "" {
		_, status, err = vercelRequest(r.Context(), client, http.MethodPatch, "https://api.vercel.com/v9/projects/"+projectID+"/env/"+existingEnvID, params, headers, map[string]any{"value": cfgB64})
	} else {
		_, status, err = vercelRequest(r.Context(), client, http.MethodPost, "https://api.vercel.com/v10/projects/"+projectID+"/env", params, headers, map[string]any{"key": "DS2API_CONFIG_JSON", "value": cfgB64, "type": "encrypted", "target": []string{"production", "preview"}})
	}
	if err != nil || (status != http.StatusOK && status != http.StatusCreated) {
		writeJSON(w, statusOr(status, http.StatusInternalServerError), map[string]any{"detail": "更新环境变量失败"})
		return
	}
	savedCreds := []string{}
	if saveCreds && !usePreconfig {
		creds := [][2]string{{"VERCEL_TOKEN", vercelToken}, {"VERCEL_PROJECT_ID", projectID}}
		if teamID != "" {
			creds = append(creds, [2]string{"VERCEL_TEAM_ID", teamID})
		}
		for _, kv := range creds {
			id := findEnvID(envs, kv[0])
			if id != "" {
				_, status, _ = vercelRequest(r.Context(), client, http.MethodPatch, "https://api.vercel.com/v9/projects/"+projectID+"/env/"+id, params, headers, map[string]any{"value": kv[1]})
			} else {
				_, status, _ = vercelRequest(r.Context(), client, http.MethodPost, "https://api.vercel.com/v10/projects/"+projectID+"/env", params, headers, map[string]any{"key": kv[0], "value": kv[1], "type": "encrypted", "target": []string{"production", "preview"}})
			}
			if status == http.StatusOK || status == http.StatusCreated {
				savedCreds = append(savedCreds, kv[0])
			}
		}
	}
	projectResp, status, _ := vercelRequest(r.Context(), client, http.MethodGet, "https://api.vercel.com/v9/projects/"+projectID, params, headers, nil)
	manual := true
	deployURL := ""
	if status == http.StatusOK {
		if link, ok := projectResp["link"].(map[string]any); ok {
			if linkType, _ := link["type"].(string); linkType == "github" {
				repoID := intFrom(link["repoId"])
				ref, _ := link["productionBranch"].(string)
				if ref == "" {
					ref = "main"
				}
				depResp, depStatus, _ := vercelRequest(r.Context(), client, http.MethodPost, "https://api.vercel.com/v13/deployments", params, headers, map[string]any{"name": projectID, "project": projectID, "target": "production", "gitSource": map[string]any{"type": "github", "repoId": repoID, "ref": ref}})
				if depStatus == http.StatusOK || depStatus == http.StatusCreated {
					deployURL, _ = depResp["url"].(string)
					manual = false
				}
			}
		}
	}
	_ = h.Store.SetVercelSync(h.computeSyncHash(), time.Now().Unix())
	result := map[string]any{"success": true, "validated_accounts": validated}
	if manual {
		result["message"] = "配置已同步到 Vercel，请手动触发重新部署"
		result["manual_deploy_required"] = true
	} else {
		result["message"] = "配置已同步，正在重新部署..."
		result["deployment_url"] = deployURL
	}
	if len(failed) > 0 {
		result["failed_accounts"] = failed
	}
	if len(savedCreds) > 0 {
		result["saved_credentials"] = savedCreds
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) vercelStatus(w http.ResponseWriter, _ *http.Request) {
	snap := h.Store.Snapshot()
	current := h.computeSyncHash()
	synced := snap.VercelSyncHash != "" && snap.VercelSyncHash == current
	writeJSON(w, http.StatusOK, map[string]any{"synced": synced, "last_sync_time": nilIfZero(snap.VercelSyncTime), "has_synced_before": snap.VercelSyncHash != ""})
}

func (h *Handler) exportConfig(w http.ResponseWriter, _ *http.Request) {
	jsonStr, b64, err := h.Store.ExportJSONAndBase64()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"json": jsonStr, "base64": b64})
}

func (h *Handler) computeSyncHash() string {
	snap := h.Store.Snapshot()
	syncable := map[string]any{"keys": snap.Keys, "accounts": []map[string]any{}}
	accounts := make([]map[string]any, 0, len(snap.Accounts))
	for _, a := range snap.Accounts {
		m := map[string]any{}
		if a.Email != "" {
			m["email"] = a.Email
		}
		if a.Mobile != "" {
			m["mobile"] = a.Mobile
		}
		if a.Password != "" {
			m["password"] = a.Password
		}
		accounts = append(accounts, m)
	}
	sort.Slice(accounts, func(i, j int) bool {
		ai := fmt.Sprintf("%v%v", accounts[i]["email"], accounts[i]["mobile"])
		aj := fmt.Sprintf("%v%v", accounts[j]["email"], accounts[j]["mobile"])
		return ai < aj
	})
	syncable["accounts"] = accounts
	b, _ := json.Marshal(syncable)
	sum := md5.Sum(b)
	return fmt.Sprintf("%x", sum)
}

func vercelRequest(ctx context.Context, client *http.Client, method, endpoint string, params url.Values, headers map[string]string, body any) (map[string]any, int, error) {
	if len(params) > 0 {
		endpoint += "?" + params.Encode()
	}
	var reader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, 0, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	parsed := map[string]any{}
	_ = json.Unmarshal(b, &parsed)
	if len(parsed) == 0 {
		parsed["raw"] = string(b)
	}
	return parsed, resp.StatusCode, nil
}

func findEnvID(envs []any, key string) string {
	for _, item := range envs {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if k, _ := m["key"].(string); k == key {
			id, _ := m["id"].(string)
			return id
		}
	}
	return ""
}

func reverseAccounts(a []config.Account) {
	for i, j := 0, len(a)-1; i < j; i, j = i+1, j-1 {
		a[i], a[j] = a[j], a[i]
	}
}

func intFromQuery(r *http.Request, key string, d int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return d
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return d
	}
	return n
}

func intFrom(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	default:
		return 0
	}
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nilIfZero(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

func toStringSlice(v any) ([]string, bool) {
	arr, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		out = append(out, strings.TrimSpace(fmt.Sprintf("%v", item)))
	}
	return out, true
}

func toAccount(m map[string]any) config.Account {
	return config.Account{Email: strings.TrimSpace(fmt.Sprintf("%v", m["email"])), Mobile: strings.TrimSpace(fmt.Sprintf("%v", m["mobile"])), Password: strings.TrimSpace(fmt.Sprintf("%v", m["password"])), Token: strings.TrimSpace(fmt.Sprintf("%v", m["token"]))}
}

func statusOr(v int, d int) int {
	if v == 0 {
		return d
	}
	return v
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
