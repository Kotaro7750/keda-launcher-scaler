package receiver

import (
	"fmt"
	"time"

	"github.com/Kotaro7750/keda-launcher-scaler/internal/server/arbitrator"
	"github.com/Kotaro7750/keda-launcher-scaler/internal/server/types"
)

// RequestInput is the transport-neutral launch request shape shared by receivers.
type RequestInput struct {
	RequestID    string
	ScaledObject types.ScaledObjectKey
	StartAt      *time.Time
	EndAt        *time.Time
	Duration     *time.Duration
}

// NormalizeRequest converts a receiver input into the arbitrator's request window.
func NormalizeRequest(input RequestInput, now time.Time) (arbitrator.RequestWindow, error) {
	if input.RequestID == "" {
		return arbitrator.RequestWindow{}, fmt.Errorf("requestId is required")
	}
	if input.ScaledObject.Namespace == "" || input.ScaledObject.Name == "" {
		return arbitrator.RequestWindow{}, fmt.Errorf("scaledObject namespace and name are required")
	}

	hasEndAt := input.EndAt != nil
	hasDuration := input.Duration != nil
	switch {
	case hasEndAt && hasDuration:
		return arbitrator.RequestWindow{}, fmt.Errorf("endAt and duration are mutually exclusive")
	case !hasEndAt && !hasDuration:
		return arbitrator.RequestWindow{}, fmt.Errorf("either endAt or duration must be provided")
	case hasDuration && *input.Duration <= 0:
		return arbitrator.RequestWindow{}, fmt.Errorf("duration must be positive")
	}

	effectiveStart := now
	if input.StartAt != nil {
		effectiveStart = input.StartAt.UTC()
	}

	if hasEndAt {
		if input.EndAt.UTC().Before(effectiveStart) {
			return arbitrator.RequestWindow{}, fmt.Errorf("endAt must be after or equal to startAt")
		}
		return arbitrator.RequestWindow{
			RequestID:    arbitrator.RequestId(input.RequestID),
			ScaledObject: input.ScaledObject,
			StartAt:      effectiveStart,
			EndAt:        input.EndAt.UTC(),
		}, nil
	}

	return arbitrator.RequestWindow{
		RequestID:    arbitrator.RequestId(input.RequestID),
		ScaledObject: input.ScaledObject,
		StartAt:      effectiveStart,
		EndAt:        effectiveStart.Add(*input.Duration),
	}, nil
}
