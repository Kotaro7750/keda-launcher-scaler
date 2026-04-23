package main

import (
	"context"
	"fmt"
	"log/slog"
	stdhttp "net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Kotaro7750/keda-launcher-scaler/internal/client/config"
	httpreceiver "github.com/Kotaro7750/keda-launcher-scaler/internal/client/receiver/http"
	"github.com/Kotaro7750/keda-launcher-scaler/internal/common/observability"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger, err := observability.NewLogger(cfg.LogLevel)
	if err != nil {
		return fmt.Errorf("create logger: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	tracerProvider, shutdownTracer, err := observability.NewTracerProvider(ctx, observability.TracerConfig{
		ServiceName:  cfg.ServiceName,
		OTLPEndpoint: cfg.OTLPEndpoint,
		OTLPInsecure: cfg.OTLPInsecure,
	})
	if err != nil {
		return fmt.Errorf("create tracer provider: %w", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := shutdownTracer(shutdownCtx); err != nil {
			logger.Error("Tracer shutdown failed", "error", err)
		}
	}()
	otel.SetTracerProvider(tracerProvider)

	return runClient(ctx, logger, cfg)
}

func runClient(ctx context.Context, logger *slog.Logger, cfg config.Config) error {
	httpClient := &stdhttp.Client{
		Transport: otelhttp.NewTransport(stdhttp.DefaultTransport),
	}
	receiver, err := httpreceiver.NewClientWithResponses(cfg.ReceiverURL, httpreceiver.WithHTTPClient(httpClient))
	if err != nil {
		return fmt.Errorf("create receiver client: %w", err)
	}
	request := launchRequest(cfg)

	logger.Info(
		"Client starting",
		"receiverURL", cfg.ReceiverURL,
		"scaledObject.namespace", cfg.ScaledObjectNamespace,
		"scaledObject.name", cfg.ScaledObjectName,
		"requestInterval", cfg.RequestInterval.String(),
		"requestDuration", cfg.RequestDuration.String(),
	)

	sendAndLog(ctx, logger, receiver, request)

	ticker := time.NewTicker(cfg.RequestInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Shutdown signal received, exiting...")
			return nil
		case <-ticker.C:
			sendAndLog(ctx, logger, receiver, request)
		}
	}
}

func launchRequest(cfg config.Config) httpreceiver.LaunchRequest {
	duration := cfg.RequestDuration.String()
	requestID := cfg.RequestID
	if requestID == "" {
		requestID = fmt.Sprintf("keda-launcher-client:%s/%s", cfg.ScaledObjectNamespace, cfg.ScaledObjectName)
	}

	return httpreceiver.LaunchRequest{
		RequestId: requestID,
		ScaledObject: httpreceiver.ScaledObject{
			Namespace: cfg.ScaledObjectNamespace,
			Name:      cfg.ScaledObjectName,
		},
		Duration: &duration,
	}
}

func sendAndLog(ctx context.Context, logger *slog.Logger, receiver httpreceiver.ClientWithResponsesInterface, request httpreceiver.LaunchRequest) {
	if err := sendLaunchRequest(ctx, receiver, request); err != nil {
		logger.Error(
			"Launch request failed",
			"error", err,
			"requestId", request.RequestId,
			"scaledObject.namespace", request.ScaledObject.Namespace,
			"scaledObject.name", request.ScaledObject.Name,
		)
		return
	}

	logger.Info(
		"Launch request accepted",
		"requestId", request.RequestId,
		"scaledObject.namespace", request.ScaledObject.Namespace,
		"scaledObject.name", request.ScaledObject.Name,
		"duration", *request.Duration,
	)
}

func sendLaunchRequest(ctx context.Context, receiver httpreceiver.ClientWithResponsesInterface, request httpreceiver.LaunchRequest) error {
	response, err := receiver.PostRequestsWithResponse(ctx, request)
	if err != nil {
		return fmt.Errorf("post request: %w", err)
	}

	if response.StatusCode() == stdhttp.StatusAccepted {
		return nil
	}

	body := strings.TrimSpace(string(response.Body))
	if body == "" {
		return fmt.Errorf("unexpected response status: %s", response.Status())
	}
	return fmt.Errorf("unexpected response status: %s: %s", response.Status(), body)
}
