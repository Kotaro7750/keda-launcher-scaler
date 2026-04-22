package receiver

import (
	"context"
	"fmt"

	"github.com/Kotaro7750/graceful"
	"github.com/Kotaro7750/keda-launcher-scaler/internal/server/arbitrator"
)

// ReceiverIF abstracts a launch request source that feeds normalized requests into the arbitrator.
type ReceiverIF interface {
	// Receive receives requests and sends them into passed channel in blocking way.
	// It is concrete type's responsibility to shutdown gracefully when Shutdown() is called and to exit when context is canceled.
	Receive(ctx context.Context, receivedRequestCh chan<- arbitrator.RequestWindow) error
	Shutdown(ctx context.Context) error
}

// Receiver is a wrapper around ReceiverIF that works as a graceful.DaemonComponent
type Receiver struct {
	id                string
	receiverIF        ReceiverIF
	receivedRequestCh chan<- arbitrator.RequestWindow
}

func NewReceiver(id string, receiverIF ReceiverIF, receivedRequestCh chan<- arbitrator.RequestWindow) *Receiver {
	return &Receiver{
		id:                fmt.Sprintf("receiver-%s", id),
		receiverIF:        receiverIF,
		receivedRequestCh: receivedRequestCh,
	}
}

func (r *Receiver) Id() string {
	return r.id
}

func (r *Receiver) SetComponentContext(componentContext graceful.ComponentContext) {
	// No-op
}

func (r *Receiver) Run(ctx context.Context) error {
	return r.receiverIF.Receive(ctx, r.receivedRequestCh)
}

func (r *Receiver) Shutdown(ctx context.Context) error {
	return r.receiverIF.Shutdown(ctx)
}
