package httpreceiver

import (
	"context"
	"errors"
	"log/slog"
	stdhttp "net/http"
	"time"

	"github.com/Kotaro7750/keda-launcher-scaler/internal/server/arbitrator"
	"github.com/Kotaro7750/keda-launcher-scaler/internal/server/receiver"
	"github.com/Kotaro7750/keda-launcher-scaler/internal/server/types"
	"github.com/labstack/echo/v4"
	echomiddleware "github.com/oapi-codegen/echo-middleware"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// ReceiverIF accepts launch requests over HTTP and forwards normalized windows downstream.
type ReceiverIF struct {
	logger  *slog.Logger
	address string
	server  *stdhttp.Server
}

func NewReceiverIF(address string, logger *slog.Logger) *ReceiverIF {
	return &ReceiverIF{
		logger:  logger,
		address: address,
		server: &stdhttp.Server{
			Addr:              address,
			ReadHeaderTimeout: 5 * time.Second,
		},
	}
}

func (r *ReceiverIF) Receive(ctx context.Context, receivedRequestCh chan<- arbitrator.RequestWindow) error {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	swagger, err := GetSwagger()
	if err != nil {
		return err
	}
	swagger.Servers = nil
	e.Use(echomiddleware.OapiRequestValidator(swagger))
	RegisterHandlers(e, NewStrictHandler(&httpReceiverServer{
		out: receivedRequestCh,
	}, nil))

	r.server.Handler = otelhttp.NewHandler(e, "receiver.http")

	errCh := make(chan error)
	go func() {
		r.logger.Info("HTTP receiver starting", "listenAddress", r.address)
		errCh <- r.server.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, stdhttp.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *ReceiverIF) Shutdown(ctx context.Context) error {
	return r.server.Shutdown(ctx)
}

type httpReceiverServer struct {
	out chan<- arbitrator.RequestWindow
}

func (s *httpReceiverServer) PostRequests(ctx context.Context, request PostRequestsRequestObject) (PostRequestsResponseObject, error) {
	input, err := requestInputFromHTTP(request)
	if err != nil {
		return nil, echo.NewHTTPError(stdhttp.StatusBadRequest, err.Error())
	}

	normalized, err := receiver.NormalizeRequest(input, time.Now().UTC())
	if err != nil {
		return nil, echo.NewHTTPError(stdhttp.StatusBadRequest, err.Error())
	}

	select {
	case s.out <- normalized:
	case <-ctx.Done():
		return nil, echo.NewHTTPError(stdhttp.StatusRequestTimeout, "request canceled")
	}

	return PostRequests202JSONResponse{
		RequestId: string(normalized.RequestID),
		ScaledObject: ScaledObject{
			Namespace: normalized.ScaledObject.Namespace,
			Name:      normalized.ScaledObject.Name,
		},
		EffectiveStart: normalized.StartAt,
		EffectiveEnd:   normalized.EndAt,
	}, nil
}

func requestInputFromHTTP(request PostRequestsRequestObject) (receiver.RequestInput, error) {
	if request.Body == nil {
		return receiver.RequestInput{}, errors.New("invalid request body")
	}

	var duration *time.Duration
	if request.Body.Duration != nil {
		parsed, err := time.ParseDuration(*request.Body.Duration)
		if err != nil {
			return receiver.RequestInput{}, err
		}
		duration = &parsed
	}

	return receiver.RequestInput{
		RequestID: request.Body.RequestId,
		ScaledObject: types.ScaledObjectKey{
			Namespace: request.Body.ScaledObject.Namespace,
			Name:      request.Body.ScaledObject.Name,
		},
		StartAt:  request.Body.StartAt,
		EndAt:    request.Body.EndAt,
		Duration: duration,
	}, nil
}
