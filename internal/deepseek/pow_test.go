package deepseek

import (
	"context"
	"testing"
)

func TestPowPoolSizeFromEnv(t *testing.T) {
	t.Setenv("DS2API_POW_POOL_SIZE", "3")
	if got := powPoolSizeFromEnv(); got != 3 {
		t.Fatalf("expected pool size 3, got %d", got)
	}
}

func TestPowSolverAcquireReleaseReusesModule(t *testing.T) {
	t.Setenv("DS2API_POW_POOL_SIZE", "1")
	solver := NewPowSolver("missing-file.wasm")
	if err := solver.init(context.Background()); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	pm1, err := solver.acquireModule(context.Background())
	if err != nil {
		t.Fatalf("acquire first module failed: %v", err)
	}
	solver.releaseModule(pm1)

	pm2, err := solver.acquireModule(context.Background())
	if err != nil {
		t.Fatalf("acquire second module failed: %v", err)
	}
	if pm1 != pm2 {
		t.Fatalf("expected pooled module reuse, got different instances")
	}
	solver.releaseModule(pm2)
}

func TestClientPreloadPowUsesClientSolver(t *testing.T) {
	t.Setenv("DS2API_POW_POOL_SIZE", "1")
	client := NewClient(nil, nil)
	if err := client.PreloadPow(context.Background()); err != nil {
		t.Fatalf("preload failed: %v", err)
	}
	if client.powSolver.runtime == nil || client.powSolver.compiled == nil {
		t.Fatalf("expected client pow solver to be initialized")
	}
}
