package arbitrator

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/Kotaro7750/keda-launcher-scaler/internal/server/types"

	"github.com/google/uuid"
)

type RequestId string

var ErrRequestWindowNotFound = errors.New("request window not found")

type RequestWindow struct {
	RequestID    RequestId
	ScaledObject types.ScaledObjectKey
	StartAt      time.Time
	EndAt        time.Time
}

func (r RequestWindow) notYetStarted(t time.Time) bool {
	return t.Before(r.StartAt)
}

func (r RequestWindow) alreadyEnded(t time.Time) bool {
	return t.After(r.EndAt)
}

func (r RequestWindow) shouldActivate(t time.Time) bool {
	return !r.notYetStarted(t) && !r.alreadyEnded(t)
}

type Arbitrator struct {
	mu             sync.RWMutex
	nextEventTimer *time.Timer
	logger         *slog.Logger

	inputCh   <-chan RequestWindow
	changedCh chan struct{}
	// This channel is closed when Shutdown() is called
	notifyShutdownCh chan struct{}
	shutdownOnce     sync.Once
	shutdownDoneCh   chan struct{}

	requests      map[RequestId]RequestWindow
	subscriptions map[uuid.UUID]chan bool
}

func newArbitrator(in <-chan RequestWindow, logger *slog.Logger) *Arbitrator {
	changedCh := make(chan struct{}, 1)

	arbitrator := Arbitrator{
		mu:               sync.RWMutex{},
		nextEventTimer:   nil,
		logger:           logger,
		inputCh:          in,
		requests:         make(map[RequestId]RequestWindow),
		subscriptions:    make(map[uuid.UUID]chan bool),
		changedCh:        changedCh,
		notifyShutdownCh: make(chan struct{}),
		shutdownDoneCh:   make(chan struct{}),
	}

	return &arbitrator
}

func (a *Arbitrator) Run(ctx context.Context) error {
	wg := sync.WaitGroup{}
	wg.Add(2)

	// Main loop for intaking requests
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case req, ok := <-a.inputCh:
				if !ok {
					return
				}
				a.upsert(req)
			case <-a.notifyShutdownCh:
				return
			}
		}
	}()

	// Main loop for reconciling state and notifying subscribers
	go func() {
		defer wg.Done()
		for {
			var nextEventTimerCh <-chan time.Time
			if a.nextEventTimer != nil {
				nextEventTimerCh = a.nextEventTimer.C
			}

			select {
			case <-ctx.Done():
				return
			case _, ok := <-a.changedCh:
				if !ok {
					return
				}
				a.reconcileAndNotify()
			case _, ok := <-nextEventTimerCh:
				if !ok {
					return
				}
				a.reconcileAndNotify()
			case <-a.notifyShutdownCh:
				return
			}
		}
	}()

	wg.Wait()
	return ctx.Err()
}

func (a *Arbitrator) Shutdown(ctx context.Context) error {
	a.shutdownOnce.Do(func() {
		close(a.notifyShutdownCh)

		a.mu.Lock()
		defer a.mu.Unlock()

		a.closeSubscriptionsLocked()

		if a.nextEventTimer != nil {
			a.nextEventTimer.Stop()
		}
		close(a.shutdownDoneCh)
	})

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-a.shutdownDoneCh:
		return nil
	}
}

func (a *Arbitrator) checkIfActive(t time.Time) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return a.isActiveLocked(t)
}

func (a *Arbitrator) isActiveLocked(t time.Time) bool {
	for _, req := range a.requests {
		if req.shouldActivate(t) {
			return true
		}
	}

	return false
}

func (a *Arbitrator) reconcileAndNotify() {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()

	a.pruneExpiredRequests(now)

	// First, check if current desired state is active or not.
	// This decision does not consider previous state.
	isActive := a.isActiveLocked(now)

	// Notify all subscribers
	for _, ch := range a.subscriptions {
		select {
		case ch <- isActive:
		default:
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- isActive:
			default:
			}
		}
	}

	// Reset the timer for the next event
	if a.nextEventTimer != nil {
		a.nextEventTimer.Stop()
	}
	var earliestTime time.Time

	for _, req := range a.requests {
		if req.notYetStarted(now) {
			if earliestTime.IsZero() || req.StartAt.Before(earliestTime) {
				earliestTime = req.StartAt
			}
		} else if req.shouldActivate(now) {
			if earliestTime.IsZero() || req.EndAt.Before(earliestTime) {
				earliestTime = req.EndAt
			}
		}
	}

	if !earliestTime.IsZero() {
		a.nextEventTimer = time.NewTimer(earliestTime.Sub(now))
	}
}

func (a *Arbitrator) pruneExpiredRequests(t time.Time) {
	for id, req := range a.requests {
		if req.alreadyEnded(t) {
			delete(a.requests, id)
			a.logger.Info(
				"Request window pruned",
				"requestId", string(req.RequestID),
				"scaledObject.namespace", req.ScaledObject.Namespace,
				"scaledObject.name", req.ScaledObject.Name,
				"startAt", req.StartAt,
				"endAt", req.EndAt,
			)
		}
	}
}

func (a *Arbitrator) subscribe(id uuid.UUID) (<-chan bool, func()) {
	a.mu.Lock()
	defer a.mu.Unlock()

	ch := make(chan bool, 1)
	ch <- a.isActiveLocked(time.Now())
	a.subscriptions[id] = ch

	cancel := func() {
		a.mu.Lock()
		defer a.mu.Unlock()

		if _, ok := a.subscriptions[id]; !ok {
			return
		}

		delete(a.subscriptions, id)
		close(ch)
	}

	return ch, cancel
}

func (a *Arbitrator) closeSubscriptionsLocked() {
	for id, ch := range a.subscriptions {
		delete(a.subscriptions, id)
		close(ch)
	}
}

func (a *Arbitrator) upsert(req RequestWindow) {
	a.mu.Lock()
	previous, exists := a.requests[req.RequestID]
	a.requests[req.RequestID] = req
	a.mu.Unlock()

	if exists {
		a.logger.Info(
			"Request window updated",
			"requestId", string(req.RequestID),
			"scaledObject.namespace", req.ScaledObject.Namespace,
			"scaledObject.name", req.ScaledObject.Name,
			"previousStartAt", previous.StartAt,
			"previousEndAt", previous.EndAt,
			"startAt", req.StartAt,
			"endAt", req.EndAt,
		)
	} else {
		a.logger.Info(
			"Request window added",
			"requestId", string(req.RequestID),
			"scaledObject.namespace", req.ScaledObject.Namespace,
			"scaledObject.name", req.ScaledObject.Name,
			"startAt", req.StartAt,
			"endAt", req.EndAt,
		)
	}

	select {
	case a.changedCh <- struct{}{}:
	default:
	}
}

func (a *Arbitrator) delete(requestID RequestId) (RequestWindow, error) {
	a.mu.Lock()
	req, ok := a.requests[requestID]
	if !ok {
		a.mu.Unlock()
		return RequestWindow{}, ErrRequestWindowNotFound
	}
	delete(a.requests, requestID)
	a.mu.Unlock()

	a.logger.Info(
		"Request window deleted",
		"requestId", string(req.RequestID),
		"scaledObject.namespace", req.ScaledObject.Namespace,
		"scaledObject.name", req.ScaledObject.Name,
		"startAt", req.StartAt,
		"endAt", req.EndAt,
	)

	select {
	case a.changedCh <- struct{}{}:
	default:
	}

	return req, nil
}
