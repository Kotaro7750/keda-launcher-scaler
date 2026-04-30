package scaler

import (
	"context"
	"testing"

	"github.com/Kotaro7750/keda-launcher-scaler/internal/server/arbitrator"
	"github.com/Kotaro7750/keda-launcher-scaler/internal/server/types"
	"github.com/kedacore/keda/v2/pkg/scalers/externalscaler"
	"google.golang.org/grpc/metadata"
)

type recordingArbitratorRouter struct {
	ensureCalled []types.ScaledObjectKey
	active       bool
	subscribeCh  <-chan bool
}

func (r *recordingArbitratorRouter) Subscribe(types.ScaledObjectKey) (<-chan bool, func()) {
	if r.subscribeCh != nil {
		return r.subscribeCh, func() {}
	}
	ch := make(chan bool)
	close(ch)
	return ch, func() {}
}

func (r *recordingArbitratorRouter) Run(context.Context) error {
	return nil
}

func (r *recordingArbitratorRouter) IsActive(types.ScaledObjectKey) bool {
	return r.active
}

func (r *recordingArbitratorRouter) EnsureScaledObject(key types.ScaledObjectKey) {
	r.ensureCalled = append(r.ensureCalled, key)
}

func (r *recordingArbitratorRouter) HasScaledObject(types.ScaledObjectKey) bool {
	return false
}

func (r *recordingArbitratorRouter) ListScaledObjects() []types.ScaledObjectKey {
	return nil
}

func (r *recordingArbitratorRouter) DeleteRequest(types.ScaledObjectKey, arbitrator.RequestId) (arbitrator.RequestWindow, error) {
	return arbitrator.RequestWindow{}, nil
}

func TestServiceIsActiveRegistersScaledObject(t *testing.T) {
	router := &recordingArbitratorRouter{}
	service := NewService(router)

	resp, err := service.IsActive(context.Background(), &externalscaler.ScaledObjectRef{
		Namespace: "default",
		Name:      "worker",
	})
	if err != nil {
		t.Fatalf("IsActive() error = %v", err)
	}
	if resp == nil || resp.Result {
		t.Fatalf("IsActive() response = %+v, want inactive", resp)
	}
	if len(router.ensureCalled) != 1 {
		t.Fatalf("EnsureScaledObject calls = %d, want 1", len(router.ensureCalled))
	}
	if router.ensureCalled[0] != (types.ScaledObjectKey{Namespace: "default", Name: "worker"}) {
		t.Fatalf("EnsureScaledObject key = %+v", router.ensureCalled[0])
	}
}

func TestServiceGetMetricSpecRegistersScaledObject(t *testing.T) {
	router := &recordingArbitratorRouter{}
	service := NewService(router)

	resp, err := service.GetMetricSpec(context.Background(), &externalscaler.ScaledObjectRef{
		Namespace: "default",
		Name:      "worker",
	})
	if err != nil {
		t.Fatalf("GetMetricSpec() error = %v", err)
	}
	if resp == nil || len(resp.MetricSpecs) != 1 {
		t.Fatalf("GetMetricSpec() response = %+v", resp)
	}
	if len(router.ensureCalled) != 1 {
		t.Fatalf("EnsureScaledObject calls = %d, want 1", len(router.ensureCalled))
	}
}

func TestServiceGetMetricsRegistersScaledObject(t *testing.T) {
	router := &recordingArbitratorRouter{}
	service := NewService(router)

	resp, err := service.GetMetrics(context.Background(), &externalscaler.GetMetricsRequest{
		ScaledObjectRef: &externalscaler.ScaledObjectRef{
			Namespace: "default",
			Name:      "worker",
		},
	})
	if err != nil {
		t.Fatalf("GetMetrics() error = %v", err)
	}
	if resp == nil || len(resp.MetricValues) != 1 {
		t.Fatalf("GetMetrics() response = %+v", resp)
	}
	if len(router.ensureCalled) != 1 {
		t.Fatalf("EnsureScaledObject calls = %d, want 1", len(router.ensureCalled))
	}
}

func TestServiceRejectsInvalidRefWithoutRegistering(t *testing.T) {
	router := &recordingArbitratorRouter{}
	service := NewService(router)

	_, err := service.IsActive(context.Background(), &externalscaler.ScaledObjectRef{})
	if err == nil {
		t.Fatal("IsActive() succeeded for invalid ref")
	}
	if len(router.ensureCalled) != 0 {
		t.Fatalf("EnsureScaledObject calls = %d, want 0", len(router.ensureCalled))
	}
}

func TestServiceStreamIsActiveRegistersScaledObjectBeforeStreaming(t *testing.T) {
	updates := make(chan bool, 1)
	updates <- true
	close(updates)

	router := &recordingArbitratorRouter{
		subscribeCh: updates,
	}
	service := NewService(router)
	stream := &recordingActiveStream{ctx: context.Background()}

	if err := service.StreamIsActive(&externalscaler.ScaledObjectRef{
		Namespace: "default",
		Name:      "worker",
	}, stream); err != nil {
		t.Fatalf("StreamIsActive() error = %v", err)
	}
	if len(router.ensureCalled) != 1 {
		t.Fatalf("EnsureScaledObject calls = %d, want 1", len(router.ensureCalled))
	}
	if router.ensureCalled[0] != (types.ScaledObjectKey{Namespace: "default", Name: "worker"}) {
		t.Fatalf("EnsureScaledObject key = %+v", router.ensureCalled[0])
	}
	if len(stream.sent) != 1 || !stream.sent[0].Result {
		t.Fatalf("streamed responses = %+v", stream.sent)
	}
}

type recordingActiveStream struct {
	ctx  context.Context
	sent []*externalscaler.IsActiveResponse
}

func (s *recordingActiveStream) Send(resp *externalscaler.IsActiveResponse) error {
	s.sent = append(s.sent, resp)
	return nil
}

func (s *recordingActiveStream) SetHeader(metadata.MD) error {
	return nil
}

func (s *recordingActiveStream) SendHeader(metadata.MD) error {
	return nil
}

func (s *recordingActiveStream) SetTrailer(metadata.MD) {}

func (s *recordingActiveStream) Context() context.Context {
	if s.ctx == nil {
		return context.Background()
	}
	return s.ctx
}

func (s *recordingActiveStream) SendMsg(any) error {
	return nil
}

func (s *recordingActiveStream) RecvMsg(any) error {
	return nil
}
