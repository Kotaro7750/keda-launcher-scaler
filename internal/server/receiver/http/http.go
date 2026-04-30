package httpreceiver

import (
	"context"
	"errors"
	"log/slog"
	stdhttp "net/http"
	"sort"
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
	logger         *slog.Logger
	address        string
	requestManager requestManager
	server         *stdhttp.Server
}

func NewReceiverIF(address string, logger *slog.Logger, requestManager requestManager) *ReceiverIF {
	return &ReceiverIF{
		logger:         logger,
		address:        address,
		requestManager: requestManager,
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
		out:            receivedRequestCh,
		requestManager: r.requestManager,
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

type requestManager interface {
	HasScaledObject(types.ScaledObjectKey) bool
	ListScaledObjects() []types.ScaledObjectKey
	DeleteRequest(types.ScaledObjectKey, arbitrator.RequestId) (arbitrator.RequestWindow, error)
}

type httpReceiverServer struct {
	out            chan<- arbitrator.RequestWindow
	requestManager requestManager
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

	if s.requestManager == nil {
		return nil, echo.NewHTTPError(stdhttp.StatusInternalServerError, "request manager is not configured")
	}

	if !s.requestManager.HasScaledObject(normalized.ScaledObject) {
		return nil, echo.NewHTTPError(stdhttp.StatusNotFound, "target ScaledObject is not known to this receiver")
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

func (s *httpReceiverServer) ListScaledObjects(ctx context.Context, request ListScaledObjectsRequestObject) (ListScaledObjectsResponseObject, error) {
	scaledObjects := make([]ScaledObject, 0)
	if s.requestManager != nil {
		keys := s.requestManager.ListScaledObjects()
		scaledObjects = make([]ScaledObject, 0, len(keys))
		for _, key := range keys {
			scaledObjects = append(scaledObjects, ScaledObject{
				Namespace: key.Namespace,
				Name:      key.Name,
			})
		}
		sort.Slice(scaledObjects, func(i, j int) bool {
			if scaledObjects[i].Namespace != scaledObjects[j].Namespace {
				return scaledObjects[i].Namespace < scaledObjects[j].Namespace
			}
			return scaledObjects[i].Name < scaledObjects[j].Name
		})
	}

	return ListScaledObjects200JSONResponse{
		ScaledObjects: scaledObjects,
	}, nil
}

func (s *httpReceiverServer) DeleteScaledObjectRequest(ctx context.Context, request DeleteScaledObjectRequestRequestObject) (DeleteScaledObjectRequestResponseObject, error) {
	if s.requestManager == nil {
		return nil, echo.NewHTTPError(stdhttp.StatusNotImplemented, "request manager is not configured")
	}

	key := types.ScaledObjectKey{
		Namespace: request.Namespace,
		Name:      request.Name,
	}

	deleted, err := s.requestManager.DeleteRequest(key, arbitrator.RequestId(request.RequestId))
	if err != nil {
		switch {
		case errors.Is(err, arbitrator.ErrRequestWindowNotFound),
			errors.Is(err, arbitrator.ErrScaledObjectNotFound):
			return nil, echo.NewHTTPError(stdhttp.StatusNotFound, err.Error())
		default:
			return nil, echo.NewHTTPError(stdhttp.StatusInternalServerError, err.Error())
		}
	}

	return DeleteScaledObjectRequest200JSONResponse{
		RequestId: string(deleted.RequestID),
		ScaledObject: ScaledObject{
			Namespace: deleted.ScaledObject.Namespace,
			Name:      deleted.ScaledObject.Name,
		},
		EffectiveStart: deleted.StartAt,
		EffectiveEnd:   deleted.EndAt,
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
