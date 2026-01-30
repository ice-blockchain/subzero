// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/ice-blockchain/subzero/tracing/statefsm"
)

const (
	// Threshold for consecutive failed operations before marking the database as unhealthy.
	consecutiveOperationThreshold = 3
	// Time window to consider operations as consecutive.
	consecutiveWindow = time.Minute
)

type (
	Status struct {
		LastWrite         time.Time // Timestamp of the last event write operation.
		LastRead          time.Time // Timestamp of the last event read operation.
		InReadErrorState  bool      // Indicates if the last N read operations failed.
		InWriteErrorState bool      // Indicates if the last N write operations failed.
	}

	operationType int

	operationStatus struct {
		Timestamp time.Time
		Err       error
		Type      operationType
		Ack       chan struct{}
	}
	statusTracker struct {
		In  chan operationStatus
		Req chan chan Status
	}
)

const (
	operationTypeRead operationType = iota + 1
	operationTypeWrite
)

func newStatusTracker() *statusTracker {
	return &statusTracker{
		In:  make(chan operationStatus, 100),
		Req: make(chan chan Status, 10),
	}
}

func (st *statusTracker) Start(ctx context.Context) {
	go st.worker(ctx)
}

func (st *statusTracker) worker(ctx context.Context) {
	var lastReadTime, lastWriteTime time.Time

	readFSM := statefsm.New(consecutiveWindow, consecutiveOperationThreshold)
	writeFSM := statefsm.New(consecutiveWindow, consecutiveOperationThreshold)

	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return

		case status := <-st.In:
			switch status.Type {
			case operationTypeRead:
				lastReadTime = status.Timestamp
				readFSM.Push(status.Err != nil, status.Timestamp)

			case operationTypeWrite:
				lastWriteTime = status.Timestamp
				writeFSM.Push(status.Err != nil, status.Timestamp)

			default:
				log.Warn().Str("context", "db-tracker").Int("operation_type", int(status.Type)).Msg("unknown operation type")
			}
			if status.Ack != nil {
				close(status.Ack)
			}

		case respChan := <-st.Req:
			select {
			case respChan <- Status{
				LastRead:          lastReadTime,
				LastWrite:         lastWriteTime,
				InReadErrorState:  readFSM.InError(),
				InWriteErrorState: writeFSM.InError(),
			}:
			case <-ctx.Done():
				return
			}
		}
	}
}

func (st *statusTracker) Submit(ctx context.Context, opType operationType, err error) {
	st.submitOp(ctx, opType, true, err)
}

func (st *statusTracker) SubmitSync(ctx context.Context, opType operationType, err error) bool {
	return st.submitOp(ctx, opType, false, err)
}

func (st *statusTracker) submitOp(ctx context.Context, opType operationType, async bool, err error) bool {
	data := operationStatus{
		Timestamp: time.Now().UTC(),
		Err:       err,
		Type:      opType,
	}

	if async {
		select {
		case st.In <- data:
			return true
		case <-ctx.Done():
		default:
			// Drop the status update if the channel is full, meaning we already have plenty of data to process.
		}
		return false
	}

	// For synchronous submission, use an acknowledgment channel.
	data.Ack = make(chan struct{})

	select {
	case st.In <- data:
		select {
		case <-data.Ack:
			return true
		case <-ctx.Done():
		}
	case <-ctx.Done():
	}

	return false
}

func (st *statusTracker) Get(ctx context.Context) (*Status, error) {
	respChan := make(chan Status, 1) // Allow buffer to avoid workers blocking.

	select {
	case st.Req <- respChan:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	select {
	case status := <-respChan:
		return &status, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
