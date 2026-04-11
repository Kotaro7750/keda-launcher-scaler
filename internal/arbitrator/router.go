package arbitrator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Kotaro7750/graceful"
	"github.com/Kotaro7750/keda-launcher-scaler/internal/types"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

type ArbitratorRouterIF interface {
	Subscribe(types.ScaledObjectKey) (<-chan bool, func())
	Run(context.Context) error
	IsActive(types.ScaledObjectKey) bool
}

type ArbitratorInfo struct {
	arbitrator *Arbitrator
	inputCh    chan RequestWindow
}

func newArbitratorInfo() *ArbitratorInfo {
	inputCh := make(chan RequestWindow)
	return &ArbitratorInfo{
		arbitrator: newArbitrator(inputCh),
		inputCh:    inputCh,
	}
}

type arbitratorRouterStatus int

const (
	arbitratorRouterStatusInitialized arbitratorRouterStatus = iota
	arbitratorRouterStatusRunning
	arbitratorRouterStatusShuttingDown
	arbitratorRouterStatusStopped
)

// canSubscribe reports whether new subscriptions should be accepted.
func (s arbitratorRouterStatus) canSubscribe() bool {
	return s == arbitratorRouterStatusInitialized || s == arbitratorRouterStatusRunning
}

// canStartArbitrator reports whether newly registered arbitrators should be run now.
func (s arbitratorRouterStatus) canStartArbitrator() bool {
	return s == arbitratorRouterStatusRunning
}

type ArbitratorRouter struct {
	logger      *slog.Logger
	mu          sync.RWMutex
	receivedCh  <-chan RequestWindow
	runCtx      context.Context
	runCancel   context.CancelFunc
	status      arbitratorRouterStatus
	arbitrators map[types.ScaledObjectKey]*ArbitratorInfo
}

// NewArbitratorRouter creates a router that fans request windows out by ScaledObject.
func NewArbitratorRouter(logger *slog.Logger, receivedCh <-chan RequestWindow) *ArbitratorRouter {
	return &ArbitratorRouter{
		logger:      logger,
		mu:          sync.RWMutex{},
		receivedCh:  receivedCh,
		runCtx:      context.Background(),
		status:      arbitratorRouterStatusInitialized,
		arbitrators: make(map[types.ScaledObjectKey]*ArbitratorInfo),
	}
}

// Id returns the graceful component identifier.
func (a *ArbitratorRouter) Id() string {
	return "arbitrator-router"
}

// SetComponentContext satisfies graceful.DaemonComponent.
func (a *ArbitratorRouter) SetComponentContext(componentContext graceful.ComponentContext) {
	// No-op
}

// IsActive reports the current active state for a ScaledObject.
func (a *ArbitratorRouter) IsActive(key types.ScaledObjectKey) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	arbitrator, ok := a.arbitrators[key]
	if !ok {
		return false
	}

	return arbitrator.arbitrator.checkIfActive(time.Now())
}

// Subscribe registers for active-state updates for a ScaledObject.
func (a *ArbitratorRouter) Subscribe(key types.ScaledObjectKey) (<-chan bool, func()) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.status.canSubscribe() {
		ch := make(chan bool)
		close(ch)
		return ch, func() {}
	}

	arbitrator := a.ensureArbitratorLocked(key)

	return arbitrator.arbitrator.subscribe(uuid.New())
}

// ensureArbitratorLocked returns the arbitrator for key, creating it if needed.
func (a *ArbitratorRouter) ensureArbitratorLocked(key types.ScaledObjectKey) *ArbitratorInfo {
	arbitrator, ok := a.arbitrators[key]
	if !ok {
		arbitrator = newArbitratorInfo()
		a.arbitrators[key] = arbitrator
		a.startArbitratorLocked(key, arbitrator)
	}
	return arbitrator
}

// startArbitratorLocked starts an arbitrator when the router is running.
func (a *ArbitratorRouter) startArbitratorLocked(key types.ScaledObjectKey, arbitratorInfo *ArbitratorInfo) {
	if !a.status.canStartArbitrator() {
		return
	}

	runCtx := a.runCtx
	go func() {
		if err := arbitratorInfo.arbitrator.Run(runCtx); err != nil && !errors.Is(err, context.Canceled) {
			a.logger.Error("Arbitrator exited with error", "scaledObject", key, "error", err)
		}
	}()
}

// route sends a request window to the ScaledObject-specific arbitrator.
func (a *ArbitratorRouter) route(ctx context.Context, req RequestWindow) error {
	a.mu.Lock()
	arbitratorInfo := a.ensureArbitratorLocked(req.ScaledObject)
	a.mu.Unlock()

	select {
	case arbitratorInfo.inputCh <- req:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Shutdown stops the router and all registered arbitrators.
func (a *ArbitratorRouter) Shutdown(ctx context.Context) error {
	a.mu.Lock()
	cancel := a.runCancel
	a.status = arbitratorRouterStatusShuttingDown
	arbitrators := make([]*ArbitratorInfo, 0, len(a.arbitrators))
	for _, arbitratorInfo := range a.arbitrators {
		arbitrators = append(arbitrators, arbitratorInfo)
	}
	a.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	errGroup, errGroupCtx := errgroup.WithContext(ctx)
	for _, arbitratorInfo := range arbitrators {
		errGroup.Go(func() error {
			return arbitratorInfo.arbitrator.Shutdown(errGroupCtx)
		})
	}

	return errGroup.Wait()
}

// Run consumes request windows and routes them to ScaledObject-specific arbitrators.
func (a *ArbitratorRouter) Run(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)

	a.mu.Lock()
	a.runCtx = runCtx
	a.runCancel = cancel
	a.status = arbitratorRouterStatusRunning
	for key, arbitratorInfo := range a.arbitrators {
		a.startArbitratorLocked(key, arbitratorInfo)
	}
	a.mu.Unlock()

	defer func() {
		cancel()
		a.mu.Lock()
		a.runCancel = nil
		a.status = arbitratorRouterStatusStopped
		a.mu.Unlock()
	}()

	for {
		select {
		case <-runCtx.Done():
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return nil
		case req, ok := <-a.receivedCh:
			if !ok {
				return fmt.Errorf("receivedCh closed")
			}
			if err := a.route(runCtx, req); err != nil {
				return err
			}
		}
	}
}
