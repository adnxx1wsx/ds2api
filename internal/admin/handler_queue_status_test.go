package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type mockLeaseStatsProvider struct{}

func (mockLeaseStatsProvider) StreamLeaseStats() map[string]any {
	return map[string]any{
		"active":               2,
		"created_total":        int64(5),
		"released_total":       int64(3),
		"estimated_unreleased": int64(2),
	}
}

func TestQueueStatusIncludesStreamLeaseStatsWhenAvailable(t *testing.T) {
	h := newAdminTestHandler(t, `{
		"accounts":[{"email":"q@test.com","token":"token"}]
	}`)
	h.LeaseStats = mockLeaseStatsProvider{}

	req := httptest.NewRequest(http.MethodGet, "/admin/queue/status", nil)
	rec := httptest.NewRecorder()
	h.queueStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if _, ok := payload["total"]; !ok {
		t.Fatalf("expected queue status fields, payload=%#v", payload)
	}
	leaseRaw, ok := payload["stream_leases"]
	if !ok {
		t.Fatalf("expected stream_leases field, payload=%#v", payload)
	}
	lease, ok := leaseRaw.(map[string]any)
	if !ok {
		t.Fatalf("stream_leases type=%T", leaseRaw)
	}
	if got, _ := lease["active"].(float64); got != 2 {
		t.Fatalf("stream_leases.active=%v want=2", lease["active"])
	}
}
