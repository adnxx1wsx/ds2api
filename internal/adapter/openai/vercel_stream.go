package openai

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"ds2api/internal/auth"
	"ds2api/internal/config"
	"ds2api/internal/util"
)

type streamLeaseStats struct {
	created         atomic.Uint64
	released        atomic.Uint64
	expired         atomic.Uint64
	releaseNotFound atomic.Uint64
	sweepRuns       atomic.Uint64
}

func (h *Handler) handleVercelStreamPrepare(w http.ResponseWriter, r *http.Request) {
	if !config.IsVercel() {
		http.NotFound(w, r)
		return
	}
	h.startLeaseSweeper()
	h.sweepExpiredStreamLeases()
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
	leased := false
	defer func() {
		if !leased {
			h.Auth.Release(a)
		}
	}()
	r = r.WithContext(auth.WithAuth(r.Context(), a))

	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if !util.ToBool(req["stream"]) {
		writeOpenAIError(w, http.StatusBadRequest, "stream must be true")
		return
	}
	stdReq, err := normalizeOpenAIChatRequest(h.Store, req)
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !stdReq.Stream {
		writeOpenAIError(w, http.StatusBadRequest, "stream must be true")
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
	powHeader, err := h.DS.GetPow(r.Context(), a, 3)
	if err != nil {
		writeOpenAIError(w, http.StatusUnauthorized, "Failed to get PoW (invalid token or unknown error).")
		return
	}
	if strings.TrimSpace(a.DeepSeekToken) == "" {
		writeOpenAIError(w, http.StatusUnauthorized, "Invalid token. If this should be a DS2API key, add it to config.keys first.")
		return
	}

	payload := stdReq.CompletionPayload(sessionID)
	leaseID := h.holdStreamLease(a)
	if leaseID == "" {
		writeOpenAIError(w, http.StatusInternalServerError, "failed to create stream lease")
		return
	}
	leased = true
	writeJSON(w, http.StatusOK, map[string]any{
		"session_id":               sessionID,
		"lease_id":                 leaseID,
		"model":                    stdReq.ResponseModel,
		"final_prompt":             stdReq.FinalPrompt,
		"thinking_enabled":         stdReq.Thinking,
		"search_enabled":           stdReq.Search,
		"tool_names":               stdReq.ToolNames,
		"toolcall_feature_match":   h.toolcallFeatureMatchEnabled(),
		"toolcall_early_emit_high": h.toolcallEarlyEmitHighConfidence(),
		"deepseek_token":           a.DeepSeekToken,
		"pow_header":               powHeader,
		"payload":                  payload,
	})
}

func (h *Handler) handleVercelStreamRelease(w http.ResponseWriter, r *http.Request) {
	if !config.IsVercel() {
		http.NotFound(w, r)
		return
	}
	h.sweepExpiredStreamLeases()
	internalSecret := vercelInternalSecret()
	internalToken := strings.TrimSpace(r.Header.Get("X-Ds2-Internal-Token"))
	if internalSecret == "" || subtle.ConstantTimeCompare([]byte(internalToken), []byte(internalSecret)) != 1 {
		writeOpenAIError(w, http.StatusUnauthorized, "unauthorized internal request")
		return
	}

	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid json")
		return
	}
	leaseID, _ := req["lease_id"].(string)
	leaseID = strings.TrimSpace(leaseID)
	if leaseID == "" {
		writeOpenAIError(w, http.StatusBadRequest, "lease_id is required")
		return
	}
	if !h.releaseStreamLease(leaseID) {
		writeOpenAIError(w, http.StatusNotFound, "stream lease not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func isVercelStreamPrepareRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	return strings.TrimSpace(r.URL.Query().Get("__stream_prepare")) == "1"
}

func isVercelStreamReleaseRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	return strings.TrimSpace(r.URL.Query().Get("__stream_release")) == "1"
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

func (h *Handler) holdStreamLease(a *auth.RequestAuth) string {
	if a == nil {
		return ""
	}
	h.startLeaseSweeper()
	now := time.Now()
	ttl := streamLeaseTTL()
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}

	h.leaseMu.Lock()
	expired := h.popExpiredLeasesLocked(now)
	if h.streamLeases == nil {
		h.streamLeases = make(map[string]streamLease)
	}
	leaseID := newLeaseID()
	h.streamLeases[leaseID] = streamLease{
		Auth:      a,
		ExpiresAt: now.Add(ttl),
	}
	h.leaseStats.created.Add(1)
	h.leaseMu.Unlock()
	h.noteExpiredLeases(len(expired))
	h.releaseExpiredAuths(expired)
	return leaseID
}

func (h *Handler) releaseStreamLease(leaseID string) bool {
	leaseID = strings.TrimSpace(leaseID)
	if leaseID == "" {
		return false
	}

	h.leaseMu.Lock()
	expired := h.popExpiredLeasesLocked(time.Now())
	lease, ok := h.streamLeases[leaseID]
	if ok {
		delete(h.streamLeases, leaseID)
	}
	h.leaseMu.Unlock()
	h.noteExpiredLeases(len(expired))
	h.releaseExpiredAuths(expired)

	if !ok {
		h.leaseStats.releaseNotFound.Add(1)
		return false
	}
	if h.Auth != nil {
		h.Auth.Release(lease.Auth)
	}
	h.leaseStats.released.Add(1)
	return true
}

func (h *Handler) popExpiredLeasesLocked(now time.Time) []*auth.RequestAuth {
	if len(h.streamLeases) == 0 {
		return nil
	}
	expired := make([]*auth.RequestAuth, 0)
	for leaseID, lease := range h.streamLeases {
		if now.After(lease.ExpiresAt) {
			delete(h.streamLeases, leaseID)
			expired = append(expired, lease.Auth)
		}
	}
	return expired
}

func (h *Handler) releaseExpiredAuths(expired []*auth.RequestAuth) {
	if h.Auth == nil || len(expired) == 0 {
		return
	}
	for _, a := range expired {
		h.Auth.Release(a)
	}
}

func (h *Handler) sweepExpiredStreamLeases() {
	h.leaseStats.sweepRuns.Add(1)
	h.leaseMu.Lock()
	expired := h.popExpiredLeasesLocked(time.Now())
	h.leaseMu.Unlock()
	h.noteExpiredLeases(len(expired))
	h.releaseExpiredAuths(expired)
}

func (h *Handler) noteExpiredLeases(n int) {
	if h == nil || n <= 0 {
		return
	}
	h.leaseStats.expired.Add(uint64(n))
}

func (h *Handler) StreamLeaseStats() map[string]any {
	if h == nil {
		return map[string]any{
			"active":                  0,
			"created_total":           int64(0),
			"released_total":          int64(0),
			"expired_total":           int64(0),
			"release_not_found_total": int64(0),
			"sweep_runs_total":        int64(0),
			"estimated_unreleased":    int64(0),
		}
	}
	h.leaseMu.Lock()
	active := len(h.streamLeases)
	h.leaseMu.Unlock()

	created := int64(h.leaseStats.created.Load())
	released := int64(h.leaseStats.released.Load())
	expired := int64(h.leaseStats.expired.Load())
	misses := int64(h.leaseStats.releaseNotFound.Load())
	sweeps := int64(h.leaseStats.sweepRuns.Load())

	estimatedUnreleased := created - released - expired
	if estimatedUnreleased < 0 {
		estimatedUnreleased = 0
	}

	return map[string]any{
		"active":                  active,
		"created_total":           created,
		"released_total":          released,
		"expired_total":           expired,
		"release_not_found_total": misses,
		"sweep_runs_total":        sweeps,
		"estimated_unreleased":    estimatedUnreleased,
		"ttl_seconds":             int64(streamLeaseTTL().Seconds()),
		"sweep_interval_seconds":  int64(streamLeaseSweepInterval().Seconds()),
	}
}

func (h *Handler) startLeaseSweeper() {
	if h == nil {
		return
	}
	h.leaseSweep.Do(func() {
		interval := streamLeaseSweepInterval()
		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for range ticker.C {
				h.sweepExpiredStreamLeases()
			}
		}()
	})
}

func streamLeaseTTL() time.Duration {
	raw := strings.TrimSpace(os.Getenv("DS2API_VERCEL_STREAM_LEASE_TTL_SECONDS"))
	if raw == "" {
		return 15 * time.Minute
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		return 15 * time.Minute
	}
	return time.Duration(seconds) * time.Second
}

func streamLeaseSweepInterval() time.Duration {
	raw := strings.TrimSpace(os.Getenv("DS2API_VERCEL_STREAM_LEASE_SWEEP_SECONDS"))
	if raw != "" {
		seconds, err := strconv.Atoi(raw)
		if err == nil && seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
	}
	ttl := streamLeaseTTL()
	interval := ttl / 4
	if interval < 5*time.Second {
		interval = 5 * time.Second
	}
	if interval > time.Minute {
		interval = time.Minute
	}
	return interval
}

func newLeaseID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err == nil {
		return hex.EncodeToString(buf)
	}
	return fmt.Sprintf("lease-%d", time.Now().UnixNano())
}
