package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Kotaro7750/graceful"
	"github.com/Kotaro7750/keda-launcher-scaler/internal/arbitrator"
	"github.com/Kotaro7750/keda-launcher-scaler/internal/config"
	"github.com/Kotaro7750/keda-launcher-scaler/internal/observability"
	"github.com/Kotaro7750/keda-launcher-scaler/internal/receiver"
	httpreceiver "github.com/Kotaro7750/keda-launcher-scaler/internal/receiver/http"
	"github.com/Kotaro7750/keda-launcher-scaler/internal/scaler"
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

	tracerProvider, shutdownTracer, err := observability.NewTracerProvider(ctx, cfg)
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

	return runScaler(ctx, logger, cfg)
}

func runScaler(ctx context.Context, logger *slog.Logger, cfg config.Config) error {
	requestCh := make(chan arbitrator.RequestWindow, cfg.RequestBufferSize)

	// Construct receivers group
	httpReceiver := receiver.NewReceiver("http", httpreceiver.NewReceiverIF(cfg.HTTPListenAddress, logger), requestCh)
	receiverGroup := graceful.NewGracefulDaemonGroup("receivers", httpReceiver).WithLogger(logger)

	// Construct arbitrator router
	arbitratorRouter := arbitrator.NewArbitratorRouter(logger, requestCh)

	// Construct gRPC server
	scalerServer := scaler.NewScalerServer(cfg.GRPCListenAddress, arbitratorRouter, logger)

	// Construct main chain
	chain := graceful.NewGracefulDaemonChain("main", receiverGroup, arbitratorRouter, scalerServer).WithLogger(logger)

	errCh := make(chan error, 1)
	go func() {
		errCh <- chain.Run(context.Background())
	}()

	select {
	case <-ctx.Done():
		logger.Info("Shutdown signal received, exiting...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		return chain.Shutdown(shutdownCtx)
	case err := <-errCh:
		logger.Error("Component exited with error, exiting...", "error", err)
		return err
	}
}
