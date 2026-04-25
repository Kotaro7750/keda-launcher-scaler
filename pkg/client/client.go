package client

import (
	"context"
	"time"
)

// ScaledObject identifies the target ScaledObject for a launch request.
type ScaledObject struct {
	Namespace string
	Name      string
}

// LaunchRequest is the transport-agnostic launch request shape.
type LaunchRequest struct {
	RequestID    string
	ScaledObject ScaledObject
	StartAt      *time.Time
	Duration     time.Duration
	EndAt        *time.Time
}

// AcceptedRequest is the transport-agnostic accepted response shape.
type AcceptedRequest struct {
	RequestID      string
	ScaledObject   ScaledObject
	EffectiveStart time.Time
	EffectiveEnd   time.Time
}

// Client sends launch requests using a transport-specific implementation.
type Client interface {
	Launch(ctx context.Context, req LaunchRequest) (AcceptedRequest, error)
}
