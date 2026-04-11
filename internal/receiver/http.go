package receiver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/Kotaro7750/keda-launcher-scaler/internal/arbitrator"
	"github.com/Kotaro7750/keda-launcher-scaler/internal/types"
	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// HTTPReceiverIF accepts launch requests over HTTP and forwards normalized windows downstream.
type HTTPReceiverIF struct {
	logger  *slog.Logger
	address string
	server  *http.Server
}

type requestPayload struct {
	RequestID    string              `json:"requestId" validate:"required"`
	ScaledObject scaledObjectPayload `json:"scaledObject"`
	StartAt      *time.Time          `json:"startAt,omitempty"`
	EndAt        *time.Time          `json:"endAt,omitempty"`
	Duration     string              `json:"duration,omitempty"`
}

type scaledObjectPayload struct {
	Namespace string `json:"namespace" validate:"required"`
	Name      string `json:"name" validate:"required"`
}

var requestValidator = validator.New()

func NewHTTPReceiverIF(address string, logger *slog.Logger) *HTTPReceiverIF {
	return &HTTPReceiverIF{
		logger:  logger,
		address: address,
		server: &http.Server{
			Addr:              address,
			ReadHeaderTimeout: 5 * time.Second,
		},
	}
}

func (r *HTTPReceiverIF) Receive(ctx context.Context, receivedRequestCh chan<- arbitrator.RequestWindow) error {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.POST("/requests", r.handleRequests(receivedRequestCh))

	r.server.Handler = otelhttp.NewHandler(e, "receiver.http")

	errCh := make(chan error)
	go func() {
		r.logger.Info("HTTP receiver starting", "listenAddress", r.address)
		errCh <- r.server.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *HTTPReceiverIF) Shutdown(ctx context.Context) error {
	return r.server.Shutdown(ctx)
}

func normalizeRequest(
	requestID string,
	scaledObject types.ScaledObjectKey,
	startAt *time.Time,
	endAt *time.Time,
	duration time.Duration,
	now time.Time,
) (arbitrator.RequestWindow, error) {
	if requestID == "" {
		return arbitrator.RequestWindow{}, fmt.Errorf("requestId is required")
	}
	if scaledObject.Namespace == "" || scaledObject.Name == "" {
		return arbitrator.RequestWindow{}, fmt.Errorf("scaledObject namespace and name are required")
	}

	effectiveStart := now
	if startAt != nil {
		effectiveStart = startAt.UTC()
	}

	switch {
	case endAt != nil:
		if endAt.UTC().Before(effectiveStart) {
			return arbitrator.RequestWindow{}, fmt.Errorf("endAt must be after or equal to startAt")
		}
		return arbitrator.RequestWindow{
			RequestID:    arbitrator.RequestId(requestID),
			ScaledObject: scaledObject,
			StartAt:      effectiveStart,
			EndAt:        endAt.UTC(),
		}, nil
	case duration > 0:
		return arbitrator.RequestWindow{
			RequestID:    arbitrator.RequestId(requestID),
			ScaledObject: scaledObject,
			StartAt:      effectiveStart,
			EndAt:        effectiveStart.Add(duration),
		}, nil
	default:
		return arbitrator.RequestWindow{}, fmt.Errorf("either endAt or duration must be provided")
	}
}

func (r *HTTPReceiverIF) handleRequests(out chan<- arbitrator.RequestWindow) echo.HandlerFunc {
	return func(c echo.Context) error {
		var payload requestPayload
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
		}
		if err := requestValidator.Struct(payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}

		var duration time.Duration
		if payload.Duration != "" {
			parsed, err := time.ParseDuration(payload.Duration)
			if err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			}
			duration = parsed
		}

		normalized, err := normalizeRequest(
			payload.RequestID,
			types.ScaledObjectKey{
				Namespace: payload.ScaledObject.Namespace,
				Name:      payload.ScaledObject.Name,
			},
			payload.StartAt,
			payload.EndAt,
			duration,
			time.Now().UTC(),
		)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}

		select {
		case out <- normalized:
		case <-c.Request().Context().Done():
			return echo.NewHTTPError(http.StatusRequestTimeout, "request canceled")
		}

		return c.JSON(http.StatusAccepted, struct {
			RequestID    string                `json:"requestId"`
			ScaledObject types.ScaledObjectKey `json:"scaledObject"`
			StartAt      time.Time             `json:"effectiveStart"`
			EndAt        time.Time             `json:"effectiveEnd"`
		}{
			RequestID:    string(normalized.RequestID),
			ScaledObject: normalized.ScaledObject,
			StartAt:      normalized.StartAt,
			EndAt:        normalized.EndAt,
		})
	}
}
