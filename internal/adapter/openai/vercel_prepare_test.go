package openai

import (
	"ds2api/internal/auth"
	"net/http/httptest"
	"testing"
	"time"
)

func int64Metric(t *testing.T, m map[string]any, key string) int64 {
	t.Helper()
	v, ok := m[key]
	if !ok {
		t.Fatalf("missing metric %q in %#v", key, m)
	}
	switch x := v.(type) {
	case int:
		return int64(x)
	case int64:
		return x
	case uint64:
		return int64(x)
	case float64:
		return int64(x)
	default:
		t.Fatalf("metric %q unexpected type %T", key, v)
		return 0
	}
}

func TestIsVercelStreamPrepareRequest(t *testing.T) {
	req := httptest.NewRequest("POST", "/v1/chat/completions?__stream_prepare=1", nil)
	if !isVercelStreamPrepareRequest(req) {
		t.Fatalf("expected prepare request to be detected")
	}

	req2 := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	if isVercelStreamPrepareRequest(req2) {
		t.Fatalf("expected non-prepare request")
	}
}

func TestIsVercelStreamReleaseRequest(t *testing.T) {
	req := httptest.NewRequest("POST", "/v1/chat/completions?__stream_release=1", nil)
	if !isVercelStreamReleaseRequest(req) {
		t.Fatalf("expected release request to be detected")
	}

	req2 := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	if isVercelStreamReleaseRequest(req2) {
		t.Fatalf("expected non-release request")
	}
}

func TestVercelInternalSecret(t *testing.T) {
	t.Run("prefer explicit secret", func(t *testing.T) {
		t.Setenv("DS2API_VERCEL_INTERNAL_SECRET", "stream-secret")
		t.Setenv("DS2API_ADMIN_KEY", "admin-fallback")
		if got := vercelInternalSecret(); got != "stream-secret" {
			t.Fatalf("expected explicit secret, got %q", got)
		}
	})

	t.Run("fallback to admin key", func(t *testing.T) {
		t.Setenv("DS2API_VERCEL_INTERNAL_SECRET", "")
		t.Setenv("DS2API_ADMIN_KEY", "admin-fallback")
		if got := vercelInternalSecret(); got != "admin-fallback" {
			t.Fatalf("expected admin key fallback, got %q", got)
		}
	})

	t.Run("default admin when env missing", func(t *testing.T) {
		t.Setenv("DS2API_VERCEL_INTERNAL_SECRET", "")
		t.Setenv("DS2API_ADMIN_KEY", "")
		if got := vercelInternalSecret(); got != "admin" {
			t.Fatalf("expected default admin fallback, got %q", got)
		}
	})
}

func TestStreamLeaseLifecycle(t *testing.T) {
	h := &Handler{}
	leaseID := h.holdStreamLease(&auth.RequestAuth{UseConfigToken: false})
	if leaseID == "" {
		t.Fatalf("expected non-empty lease id")
	}
	if ok := h.releaseStreamLease(leaseID); !ok {
		t.Fatalf("expected lease release success")
	}
	if ok := h.releaseStreamLease(leaseID); ok {
		t.Fatalf("expected duplicate release to fail")
	}
}

func TestStreamLeaseTTL(t *testing.T) {
	t.Setenv("DS2API_VERCEL_STREAM_LEASE_TTL_SECONDS", "120")
	if got := streamLeaseTTL(); got != 120*time.Second {
		t.Fatalf("expected ttl=120s, got %v", got)
	}
	t.Setenv("DS2API_VERCEL_STREAM_LEASE_TTL_SECONDS", "invalid")
	if got := streamLeaseTTL(); got != 15*time.Minute {
		t.Fatalf("expected default ttl on invalid value, got %v", got)
	}
}

func TestStreamLeaseStats(t *testing.T) {
	h := &Handler{}

	leaseID := h.holdStreamLease(&auth.RequestAuth{UseConfigToken: false})
	if leaseID == "" {
		t.Fatal("expected lease id")
	}
	stats := h.StreamLeaseStats()
	if got := int64Metric(t, stats, "active"); got != 1 {
		t.Fatalf("active=%d want=1", got)
	}
	if got := int64Metric(t, stats, "created_total"); got != 1 {
		t.Fatalf("created_total=%d want=1", got)
	}

	if ok := h.releaseStreamLease(leaseID); !ok {
		t.Fatal("expected lease release success")
	}
	stats = h.StreamLeaseStats()
	if got := int64Metric(t, stats, "active"); got != 0 {
		t.Fatalf("active=%d want=0", got)
	}
	if got := int64Metric(t, stats, "released_total"); got != 1 {
		t.Fatalf("released_total=%d want=1", got)
	}
	if got := int64Metric(t, stats, "estimated_unreleased"); got != 0 {
		t.Fatalf("estimated_unreleased=%d want=0", got)
	}

	if ok := h.releaseStreamLease("missing-lease"); ok {
		t.Fatal("expected missing lease release to fail")
	}
	stats = h.StreamLeaseStats()
	if got := int64Metric(t, stats, "release_not_found_total"); got != 1 {
		t.Fatalf("release_not_found_total=%d want=1", got)
	}

	h.leaseMu.Lock()
	h.streamLeases = map[string]streamLease{
		"expired-1": {
			Auth:      &auth.RequestAuth{UseConfigToken: false},
			ExpiresAt: time.Now().Add(-time.Second),
		},
	}
	h.leaseMu.Unlock()
	h.sweepExpiredStreamLeases()
	stats = h.StreamLeaseStats()
	if got := int64Metric(t, stats, "expired_total"); got != 1 {
		t.Fatalf("expired_total=%d want=1", got)
	}
	if got := int64Metric(t, stats, "sweep_runs_total"); got < 1 {
		t.Fatalf("sweep_runs_total=%d want>=1", got)
	}
	if got := int64Metric(t, stats, "estimated_unreleased"); got != 0 {
		t.Fatalf("estimated_unreleased=%d want=0", got)
	}
}
