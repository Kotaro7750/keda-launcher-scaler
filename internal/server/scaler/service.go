package scaler

import (
	"context"

	"github.com/Kotaro7750/keda-launcher-scaler/internal/server/arbitrator"
	"github.com/Kotaro7750/keda-launcher-scaler/internal/server/types"
	"github.com/kedacore/keda/v2/pkg/scalers/externalscaler"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const METRIC_NAME = "keda-launcher-active"

// Service exposes the KEDA external scaler gRPC API backed by responders.
type Service struct {
	externalscaler.UnimplementedExternalScalerServer

	arbitratorRouter arbitrator.ArbitratorRouterIF
}

func NewService(arbitratorRouter arbitrator.ArbitratorRouterIF) *Service {
	return &Service{arbitratorRouter: arbitratorRouter}
}

func (s *Service) IsActive(ctx context.Context, ref *externalscaler.ScaledObjectRef) (*externalscaler.IsActiveResponse, error) {
	key, err := scaledObjectKeyFromRef(ref)
	if err != nil {
		return nil, err
	}
	active := s.arbitratorRouter.IsActive(key)
	return &externalscaler.IsActiveResponse{Result: active}, nil
}

func (s *Service) StreamIsActive(ref *externalscaler.ScaledObjectRef, stream externalscaler.ExternalScaler_StreamIsActiveServer) error {
	key, err := scaledObjectKeyFromRef(ref)
	if err != nil {
		return err
	}

	updates, cancel := s.arbitratorRouter.Subscribe(key)
	defer cancel()

	for {
		select {
		case <-stream.Context().Done():
			return nil
		case active, ok := <-updates:
			if !ok {
				return nil
			}
			if err := stream.Send(&externalscaler.IsActiveResponse{Result: active}); err != nil {
				return err
			}
		}
	}
}

func (s *Service) GetMetricSpec(ctx context.Context, ref *externalscaler.ScaledObjectRef) (*externalscaler.GetMetricSpecResponse, error) {
	if _, err := scaledObjectKeyFromRef(ref); err != nil {
		return nil, err
	}

	return &externalscaler.GetMetricSpecResponse{
		MetricSpecs: []*externalscaler.MetricSpec{
			{
				MetricName: METRIC_NAME,
				TargetSize: 1,
			},
		},
	}, nil
}

func (s *Service) GetMetrics(ctx context.Context, request *externalscaler.GetMetricsRequest) (*externalscaler.GetMetricsResponse, error) {
	ref := request.GetScaledObjectRef()
	key := types.ScaledObjectKey{
		Namespace: ref.GetNamespace(),
		Name:      ref.GetName(),
	}
	if key.Namespace == "" || key.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name and namespace are required")
	}

	value := int64(0)
	if s.arbitratorRouter.IsActive(key) {
		value = int64(1)
	}

	return &externalscaler.GetMetricsResponse{
		MetricValues: []*externalscaler.MetricValue{
			{
				MetricName:  METRIC_NAME,
				MetricValue: value,
			},
		},
	}, nil
}

func scaledObjectKeyFromRef(ref *externalscaler.ScaledObjectRef) (types.ScaledObjectKey, error) {
	if ref.GetNamespace() == "" || ref.GetName() == "" {
		return types.ScaledObjectKey{}, status.Error(codes.InvalidArgument, "name and namespace are required")
	}
	return types.ScaledObjectKey{
		Namespace: ref.GetNamespace(),
		Name:      ref.GetName(),
	}, nil
}
