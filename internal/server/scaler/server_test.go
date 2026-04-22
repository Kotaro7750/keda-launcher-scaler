package scaler

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/Kotaro7750/keda-launcher-scaler/internal/server/types"
)

type fakeArbitratorRouter struct{}

func (f *fakeArbitratorRouter) Subscribe(types.ScaledObjectKey) (<-chan bool, func()) {
	ch := make(chan bool)
	return ch, func() {}
}

func (f *fakeArbitratorRouter) Run(context.Context) error {
	return nil
}

func (f *fakeArbitratorRouter) IsActive(types.ScaledObjectKey) bool {
	return false
}

func TestScalerServerRun_StopsWhenContextCanceled(t *testing.T) {
	router := &fakeArbitratorRouter{}
	server := NewScalerServer("127.0.0.1:0", router, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())

	doneCh := make(chan error, 1)
	go func() {
		doneCh <- server.Run(ctx)
	}()

	cancel()

	select {
	case err := <-doneCh:
		if err != context.Canceled {
			t.Fatalf("Run error = %v, want %v", err, context.Canceled)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not return after context cancellation")
	}
}
