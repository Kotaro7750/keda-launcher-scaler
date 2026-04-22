package arbitrator

import (
	"context"
	"testing"
	"time"
)

func TestArbitratorReconcileAndNotify_PrunesExpiredRequestWindows(t *testing.T) {
	arbitrator := newArbitrator(make(chan RequestWindow))
	now := time.Now()

	arbitrator.requests["expired"] = RequestWindow{
		RequestID: "expired",
		StartAt:   now.Add(-2 * time.Minute),
		EndAt:     now.Add(-time.Minute),
	}
	arbitrator.requests["active"] = RequestWindow{
		RequestID: "active",
		StartAt:   now.Add(-time.Minute),
		EndAt:     now.Add(time.Minute),
	}

	arbitrator.reconcileAndNotify()

	if _, ok := arbitrator.requests["expired"]; ok {
		t.Fatal("expected expired request window to be pruned")
	}
	if _, ok := arbitrator.requests["active"]; !ok {
		t.Fatal("expected active request window to remain")
	}
}

func TestArbitratorShutdown_IsIdempotent(t *testing.T) {
	arbitrator := newArbitrator(make(chan RequestWindow))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := arbitrator.Shutdown(ctx); err != nil {
		t.Fatalf("first shutdown failed: %v", err)
	}
	if err := arbitrator.Shutdown(ctx); err != nil {
		t.Fatalf("second shutdown failed: %v", err)
	}
}
