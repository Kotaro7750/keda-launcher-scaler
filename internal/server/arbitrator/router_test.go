package arbitrator

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/Kotaro7750/keda-launcher-scaler/internal/server/types"
)

func TestArbitratorRouter_RoutesRequestWithoutSubscriber(t *testing.T) {
	requestCh := make(chan RequestWindow, 1)
	router := NewArbitratorRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), requestCh)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	doneCh := make(chan error, 1)
	go func() {
		doneCh <- router.Run(ctx)
	}()
	waitForRouterRunning(t, router)
	defer func() {
		cancel()
		<-doneCh
	}()

	key := types.ScaledObjectKey{Namespace: "default", Name: "worker"}
	now := time.Now()
	requestCh <- RequestWindow{
		RequestID:    "request-1",
		ScaledObject: key,
		StartAt:      now.Add(-time.Second),
		EndAt:        now.Add(time.Minute),
	}

	deadline := time.After(time.Second)
	for {
		if router.IsActive(key) {
			return
		}

		select {
		case <-deadline:
			t.Fatal("expected routed request to activate scaled object")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func TestArbitratorRouterSubscribe_ReceivesCurrentStateAfterRequest(t *testing.T) {
	requestCh := make(chan RequestWindow, 1)
	router := NewArbitratorRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), requestCh)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	doneCh := make(chan error, 1)
	go func() {
		doneCh <- router.Run(ctx)
	}()
	waitForRouterRunning(t, router)
	defer func() {
		cancel()
		<-doneCh
	}()

	key := types.ScaledObjectKey{Namespace: "default", Name: "worker"}
	now := time.Now()
	requestCh <- RequestWindow{
		RequestID:    "request-1",
		ScaledObject: key,
		StartAt:      now.Add(-time.Second),
		EndAt:        now.Add(time.Minute),
	}
	waitForRouterActive(t, router, key)

	updates, cancelSubscription := router.Subscribe(key)
	defer cancelSubscription()

	select {
	case active, ok := <-updates:
		if !ok {
			t.Fatal("expected subscription channel to be open")
		}
		if !active {
			t.Fatal("expected late subscriber to receive current active state")
		}
	case <-time.After(time.Second):
		t.Fatal("expected late subscriber to receive current active state")
	}
}

func TestArbitratorRouterSubscribe_ReusesArbitrator(t *testing.T) {
	requestCh := make(chan RequestWindow)
	router := NewArbitratorRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), requestCh)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	doneCh := make(chan error, 1)
	go func() {
		doneCh <- router.Run(ctx)
	}()
	waitForRouterRunning(t, router)
	defer func() {
		cancel()
		<-doneCh
	}()

	key := types.ScaledObjectKey{Namespace: "default", Name: "worker"}
	_, cancelFirst := router.Subscribe(key)
	defer cancelFirst()
	_, cancelSecond := router.Subscribe(key)
	defer cancelSecond()

	router.mu.RLock()
	defer router.mu.RUnlock()

	info, ok := router.arbitrators[key]
	if !ok {
		t.Fatal("expected arbitrator to be created")
	}
	if info == nil {
		t.Fatal("expected arbitrator info to be stored")
	}
	if len(router.arbitrators) != 1 {
		t.Fatalf("arbitrator count = %d, want %d", len(router.arbitrators), 1)
	}
}

func waitForRouterRunning(t *testing.T, router *ArbitratorRouter) {
	t.Helper()

	deadline := time.After(time.Second)
	for {
		router.mu.RLock()
		running := router.status == arbitratorRouterStatusRunning
		router.mu.RUnlock()
		if running {
			return
		}

		select {
		case <-deadline:
			t.Fatal("expected router to start")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func waitForRouterActive(t *testing.T, router *ArbitratorRouter, key types.ScaledObjectKey) {
	t.Helper()

	deadline := time.After(time.Second)
	for {
		if router.IsActive(key) {
			return
		}

		select {
		case <-deadline:
			t.Fatal("expected routed request to activate scaled object")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func TestArbitratorRouterShutdown_ClosesOpenStreams(t *testing.T) {
	router := NewArbitratorRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), make(chan RequestWindow))
	key := types.ScaledObjectKey{Namespace: "default", Name: "worker"}

	updates, cancel := router.Subscribe(key)
	defer cancel()

	select {
	case <-updates:
	case <-time.After(time.Second):
		t.Fatal("expected subscription to receive initial state")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutdownCancel()
	if err := router.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	select {
	case _, ok := <-updates:
		if ok {
			t.Fatal("expected subscription channel to be closed")
		}
	case <-time.After(time.Second):
		t.Fatal("expected subscription channel to close")
	}
}

func TestArbitratorRouterShutdown_StopsRunCleanly(t *testing.T) {
	router := NewArbitratorRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), make(chan RequestWindow))

	doneCh := make(chan error, 1)
	go func() {
		doneCh <- router.Run(context.Background())
	}()
	waitForRouterRunning(t, router)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := router.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	select {
	case err := <-doneCh:
		if err != nil {
			t.Fatalf("Run returned error after Shutdown: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not return after Shutdown")
	}
}
