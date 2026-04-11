package scaler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"

	"github.com/kedacore/keda/v2/pkg/scalers/externalscaler"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/Kotaro7750/graceful"
	"github.com/Kotaro7750/keda-launcher-scaler/internal/arbitrator"
)

type ScalerServer struct {
	logger            *slog.Logger
	server            *grpc.Server
	grpcListenAddress string
	arbitratorRouter  arbitrator.ArbitratorRouterIF
}

func NewScalerServer(grpcListenAddress string, arbitratorRouter arbitrator.ArbitratorRouterIF, logger *slog.Logger) *ScalerServer {
	server := grpc.NewServer()
	reflection.Register(server)
	externalscaler.RegisterExternalScalerServer(server, NewService(arbitratorRouter))

	return &ScalerServer{
		logger:            logger,
		server:            server,
		grpcListenAddress: grpcListenAddress,
		arbitratorRouter:  arbitratorRouter,
	}
}

func (s *ScalerServer) Id() string {
	return "scaler-server"
}

func (s *ScalerServer) SetComponentContext(componentContext graceful.ComponentContext) {
	// No-op
}

func (s *ScalerServer) Run(ctx context.Context) error {
	listener, err := net.Listen("tcp", s.grpcListenAddress)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.grpcListenAddress, err)
	}
	defer listener.Close()

	s.logger.Info("gRPC server starting", "listenAddress", s.grpcListenAddress)

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.server.Serve(listener)
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, grpc.ErrServerStopped) {
			return nil
		}
		return err
	case <-ctx.Done():
		s.server.Stop()
		<-errCh
		return ctx.Err()
	}

}

func (s *ScalerServer) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down gRPC server")
	doneCh := make(chan struct{})

	go func() {
		defer close(doneCh)
		s.server.GracefulStop()
	}()

	select {
	case <-doneCh:
		return nil
	case <-ctx.Done():
		s.server.Stop()
		return ctx.Err()
	}
}
