package arbitrator

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/Kotaro7750/keda-launcher-scaler/internal/server/types"
	"github.com/google/uuid"
)

func TestArbitratorReconcileAndNotify_PrunesExpiredRequestWindows(t *testing.T) {
	arbitrator := newTestArbitrator()
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
	arbitrator := newTestArbitrator()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := arbitrator.Shutdown(ctx); err != nil {
		t.Fatalf("first shutdown failed: %v", err)
	}
	if err := arbitrator.Shutdown(ctx); err != nil {
		t.Fatalf("second shutdown failed: %v", err)
	}
}

func TestArbitratorDeleteRequest_RemovesWindowAndRejectsRepeat(t *testing.T) {
	arbitrator := newTestArbitrator()
	now := time.Now()
	req := RequestWindow{
		RequestID: "request-1",
		ScaledObject: types.ScaledObjectKey{
			Namespace: "default",
			Name:      "worker",
		},
		StartAt: now.Add(-time.Minute),
		EndAt:   now.Add(time.Minute),
	}

	arbitrator.upsert(req)

	deleted, err := arbitrator.delete(req.RequestID)
	if err != nil {
		t.Fatalf("delete() error = %v", err)
	}
	if deleted != req {
		t.Fatalf("delete() = %+v, want %+v", deleted, req)
	}
	if arbitrator.checkIfActive(now) {
		t.Fatal("expected arbitrator to become inactive after deleting last active request")
	}

	_, err = arbitrator.delete(req.RequestID)
	if !errors.Is(err, ErrRequestWindowNotFound) {
		t.Fatalf("repeat delete error = %v, want %v", err, ErrRequestWindowNotFound)
	}
}

func TestArbitratorDeleteRequest_NotifiesInactiveAfterLastActiveWindowRemoved(t *testing.T) {
	arbitrator := newTestArbitrator()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	doneCh := make(chan error, 1)
	go func() {
		doneCh <- arbitrator.Run(ctx)
	}()
	defer func() {
		cancel()
		<-doneCh
	}()

	now := time.Now()
	req := RequestWindow{
		RequestID: "request-1",
		ScaledObject: types.ScaledObjectKey{
			Namespace: "default",
			Name:      "worker",
		},
		StartAt: now.Add(-time.Minute),
		EndAt:   now.Add(time.Minute),
	}

	arbitrator.upsert(req)
	waitForArbitratorActive(t, arbitrator, now)

	updates, cancelSubscription := arbitrator.subscribe(uuid.New())
	defer cancelSubscription()

	select {
	case active, ok := <-updates:
		if !ok {
			t.Fatal("expected subscription channel to be open")
		}
		if !active {
			t.Fatal("expected subscriber to receive current active state")
		}
	case <-time.After(time.Second):
		t.Fatal("expected subscriber to receive current active state")
	}

	if _, err := arbitrator.delete(req.RequestID); err != nil {
		t.Fatalf("delete() error = %v", err)
	}

	select {
	case active, ok := <-updates:
		if !ok {
			t.Fatal("expected subscription channel to stay open after delete")
		}
		if active {
			t.Fatal("expected subscriber to receive inactive state after delete")
		}
	case <-time.After(time.Second):
		t.Fatal("expected subscriber to receive inactive state after delete")
	}
}

func newTestArbitrator() *Arbitrator {
	return newArbitrator(make(chan RequestWindow), slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func waitForArbitratorActive(t *testing.T, arbitrator *Arbitrator, now time.Time) {
	t.Helper()

	deadline := time.After(time.Second)
	for {
		if arbitrator.checkIfActive(now) {
			return
		}

		select {
		case <-deadline:
			t.Fatal("expected arbitrator to become active")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}
