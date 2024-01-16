package internal

import (
	"context"
	"fmt"

	"github.com/gorilla/websocket"
	"github.com/open-telemetry/opamp-go/client/types"
	"github.com/open-telemetry/opamp-go/internal"
	"github.com/open-telemetry/opamp-go/protobufs"
)

// wsReceiver implements the WebSocket client's receiving portion of OpAMP protocol.
type wsReceiver struct {
	conn      *websocket.Conn
	logger    types.Logger
	sender    *WSSender
	callbacks types.Callbacks
	processor receivedProcessor

	// Indicates that the receiver has fully stopped.
	stopped chan struct{}
	err     error
}

// NewWSReceiver creates a new Receiver that uses WebSocket to receive
// messages from the server.
func NewWSReceiver(
	logger types.Logger,
	callbacks types.Callbacks,
	conn *websocket.Conn,
	sender *WSSender,
	clientSyncedState *ClientSyncedState,
	packagesStateProvider types.PackagesStateProvider,
	capabilities protobufs.AgentCapabilities,
) *wsReceiver {
	w := &wsReceiver{
		conn:      conn,
		logger:    logger,
		sender:    sender,
		callbacks: callbacks,
		processor: newReceivedProcessor(logger, callbacks, sender, clientSyncedState, packagesStateProvider, capabilities),
		stopped:   make(chan struct{}),
	}

	return w
}

// Start starts the receiver loop. To stop the receiver cancel the context.
func (r *wsReceiver) Start(ctx context.Context) {
	go r.ReceiverLoop(ctx)
}

// IsStopped returns a channel that's closed when the receiver is stopped.
func (r *wsReceiver) IsStopped() <-chan struct{} {
	return r.stopped
}

// Err returns the error that caused the receiver loop to exit.
func (r *wsReceiver) Err() error {
	return r.err
}

// ReceiverLoop runs the receiver loop. To stop the receiver cancel the context.
func (r *wsReceiver) ReceiverLoop(ctx context.Context) {
	processorCtx, stopProcessor := context.WithCancel(ctx)

out:
	for {
		var message protobufs.ServerToAgent
		if err := r.receiveMessage(&message); err != nil {
			if ctx.Err() == nil && !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				r.logger.Errorf("Unexpected error while receiving: %v", err)
			}
			r.err = err
			break out
		} else {
			r.processor.ProcessReceivedMessage(processorCtx, &message)
		}
	}

	stopProcessor()
	close(r.stopped)
}

func (r *wsReceiver) receiveMessage(msg *protobufs.ServerToAgent) error {
	_, bytes, err := r.conn.ReadMessage()
	if err != nil {
		return err
	}
	err = internal.DecodeWSMessage(bytes, msg)
	if err != nil {
		return fmt.Errorf("cannot decode received message: %w", err)
	}
	return err
}
