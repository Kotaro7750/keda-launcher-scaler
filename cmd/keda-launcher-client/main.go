package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Kotaro7750/keda-launcher-scaler/internal/client/config"
	"github.com/Kotaro7750/keda-launcher-scaler/internal/common/observability"
	"github.com/Kotaro7750/keda-launcher-scaler/pkg/client"
	httpclient "github.com/Kotaro7750/keda-launcher-scaler/pkg/client/http"
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
	httpClient := &http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}
	receiver, err := httpclient.New(cfg.ReceiverURL, httpclient.WithHTTPClient(httpClient))
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

func launchRequest(cfg config.Config) client.LaunchRequest {
	requestID := cfg.RequestID
	if requestID == "" {
		requestID = fmt.Sprintf("keda-launcher-client:%s/%s", cfg.ScaledObjectNamespace, cfg.ScaledObjectName)
	}

	return client.LaunchRequest{
		RequestID: requestID,
		ScaledObject: client.ScaledObject{
			Namespace: cfg.ScaledObjectNamespace,
			Name:      cfg.ScaledObjectName,
		},
		Duration: cfg.RequestDuration,
	}
}

func sendAndLog(ctx context.Context, logger *slog.Logger, receiver *httpclient.HTTPClient, request client.LaunchRequest) {
	if _, err := receiver.Launch(ctx, request); err != nil {
		logger.Error(
			"Launch request failed",
			"error", err,
			"requestId", request.RequestID,
			"scaledObject.namespace", request.ScaledObject.Namespace,
			"scaledObject.name", request.ScaledObject.Name,
		)
		return
	}

	logger.Info(
		"Launch request accepted",
		"requestId", request.RequestID,
		"scaledObject.namespace", request.ScaledObject.Namespace,
		"scaledObject.name", request.ScaledObject.Name,
		"duration", request.Duration.String(),
	)
}
