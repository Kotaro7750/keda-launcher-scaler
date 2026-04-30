package arbitrator

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"reflect"
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
	updates, cancelSubscription := router.Subscribe(key)
	select {
	case <-updates:
	case <-time.After(time.Second):
		t.Fatal("expected subscription to receive initial state")
	}
	cancelSubscription()

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
	updates, cancelSubscription := router.Subscribe(key)
	select {
	case <-updates:
	case <-time.After(time.Second):
		t.Fatal("expected subscription to receive initial state")
	}
	cancelSubscription()

	now := time.Now()
	requestCh <- RequestWindow{
		RequestID:    "request-1",
		ScaledObject: key,
		StartAt:      now.Add(-time.Second),
		EndAt:        now.Add(time.Minute),
	}
	waitForRouterActive(t, router, key)

	updates, cancelSubscription = router.Subscribe(key)
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

func TestArbitratorRouterListScaledObjects_ReadOnlySnapshot(t *testing.T) {
	router := NewArbitratorRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), make(chan RequestWindow))

	if got := router.ListScaledObjects(); len(got) != 0 {
		t.Fatalf("ListScaledObjects() = %v, want empty", got)
	}

	router.mu.RLock()
	if len(router.arbitrators) != 0 {
		t.Fatalf("arbitrator count before lookup = %d, want %d", len(router.arbitrators), 0)
	}
	router.mu.RUnlock()

	key := types.ScaledObjectKey{Namespace: "default", Name: "worker"}
	if router.HasScaledObject(key) {
		t.Fatal("HasScaledObject returned true for unknown key")
	}

	router.mu.RLock()
	if len(router.arbitrators) != 0 {
		t.Fatalf("arbitrator count after HasScaledObject = %d, want %d", len(router.arbitrators), 0)
	}
	router.mu.RUnlock()

	updates, cancel := router.Subscribe(key)
	defer cancel()
	select {
	case <-updates:
	case <-time.After(time.Second):
		t.Fatal("expected subscription to receive initial state")
	}

	if !router.HasScaledObject(key) {
		t.Fatal("HasScaledObject returned false for known key")
	}

	got := router.ListScaledObjects()
	want := []types.ScaledObjectKey{key}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListScaledObjects() = %v, want %v", got, want)
	}

	router.mu.RLock()
	if len(router.arbitrators) != 1 {
		t.Fatalf("arbitrator count after ListScaledObjects = %d, want %d", len(router.arbitrators), 1)
	}
	router.mu.RUnlock()
}

func TestArbitratorRouterDeleteRequest_RequiresKnownScaledObject(t *testing.T) {
	requestCh := make(chan RequestWindow, 1)
	router := NewArbitratorRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), requestCh)

	unknownKey := types.ScaledObjectKey{Namespace: "default", Name: "missing"}
	if _, err := router.DeleteRequest(unknownKey, "request-0"); err != ErrScaledObjectNotFound {
		t.Fatalf("DeleteRequest() error = %v, want %v", err, ErrScaledObjectNotFound)
	}

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
	updates, cancelSubscription := router.Subscribe(key)
	defer cancelSubscription()

	select {
	case active, ok := <-updates:
		if !ok {
			t.Fatal("expected subscription channel to be open")
		}
		if active {
			t.Fatal("expected newly created ScaledObject to start inactive")
		}
	case <-time.After(time.Second):
		t.Fatal("expected subscription to receive initial state")
	}

	now := time.Now()
	req := RequestWindow{
		RequestID:    "request-1",
		ScaledObject: key,
		StartAt:      now.Add(-time.Minute),
		EndAt:        now.Add(time.Minute),
	}

	requestCh <- req
	waitForRouterActive(t, router, key)

	select {
	case active, ok := <-updates:
		if !ok {
			t.Fatal("expected subscription channel to stay open after activation")
		}
		if !active {
			t.Fatal("expected subscriber to receive active state before delete")
		}
	case <-time.After(time.Second):
		t.Fatal("expected subscriber to receive active state before delete")
	}

	deleted, err := router.DeleteRequest(key, req.RequestID)
	if err != nil {
		t.Fatalf("DeleteRequest() error = %v", err)
	}
	if deleted != req {
		t.Fatalf("DeleteRequest() = %+v, want %+v", deleted, req)
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

	if _, err := router.DeleteRequest(key, req.RequestID); !errors.Is(err, ErrRequestWindowNotFound) {
		t.Fatalf("repeat DeleteRequest() error = %v, want %v", err, ErrRequestWindowNotFound)
	}

	if _, err := router.DeleteRequest(unknownKey, req.RequestID); !errors.Is(err, ErrScaledObjectNotFound) {
		t.Fatalf("unknown key DeleteRequest() error = %v, want %v", err, ErrScaledObjectNotFound)
	}

	router.mu.RLock()
	if len(router.arbitrators) != 1 {
		t.Fatalf("arbitrator count = %d, want %d", len(router.arbitrators), 1)
	}
	router.mu.RUnlock()
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
