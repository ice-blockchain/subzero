// SPDX-License-Identifier: ice License 1.0

package statefsm

import (
	"fmt"
	"math"
	"time"
)

type (
	// FSM is a finite state machine to track consecutive successes and failures.
	FSM struct {
		startTime    time.Time     // Timestamp of the first error/success in the current sequence.
		windowSize   time.Duration // Consecutive time window to consider operations.
		threshold    uint32        // Number of consecutive operations to trigger state change.
		okCounter    uint32        // Count of consecutive successful operations.
		errCounter   uint32        // Count of consecutive failed operations.
		currentState state         // Current state of the FSM.
	}

	state uint8
)

const (
	stateHealthy state = iota
	stateUnhealthy
)

// New creates a new FSM with the specified window size and operation threshold.
func New(consecutiveWindow time.Duration, consecutiveOperationThreshold uint32) (fsm FSM) {
	if consecutiveWindow <= 0 {
		panic("consecutiveWindow must be greater than zero")
	}
	if consecutiveOperationThreshold == 0 {
		panic("consecutiveOperationThreshold must be greater than zero")
	}
	return FSM{
		windowSize:   consecutiveWindow,
		threshold:    consecutiveOperationThreshold,
		currentState: stateHealthy,
	}
}

// Push updates the FSM state based on the result of an operation and returns new state.
func (s *FSM) Push(hasError bool, currentTime time.Time) bool {
	if hasError {
		s.okCounter = 0
		startTime := s.startTime
		if s.errCounter == 0 {
			s.errCounter++
			startTime = currentTime
		} else {
			if currentTime.Sub(startTime) <= s.windowSize {
				if s.errCounter < math.MaxUint32 {
					s.errCounter++
				}
			} else {
				// Reset error count if the time window has expired.
				if s.InError() {
					if s.errCounter < math.MaxUint32 {
						s.errCounter++
					}
				} else {
					// Reset to 1 as this is a new error after the window.
					s.errCounter = 1
				}
				startTime = currentTime
			}
		}

		s.startTime = startTime
		if s.errCounter >= s.OperationThreshold() {
			s.currentState = stateUnhealthy
		}
	} else {
		if s.okCounter < math.MaxUint32 {
			s.okCounter++
		}
		if s.currentState == stateUnhealthy && s.okCounter >= s.OperationThreshold() {
			s.currentState = stateHealthy
			s.errCounter = 0
		}
	}
	return s.InError()
}

// InError returns true if the FSM is in an unhealthy state.
func (s FSM) InError() bool {
	return s.currentState == stateUnhealthy
}

// String returns a string representation of the FSM state.
func (s FSM) String() string {
	return fmt.Sprintf("StateFSM{window=%v, threshold=%d, in_error=%v, errs=%d, oks=%d, ts=%s}",
		s.windowSize,
		s.threshold,
		s.InError(),
		s.errCounter,
		s.okCounter,
		s.startTime.Format(time.RFC3339Nano),
	)
}

func (s FSM) CurrentErrors() uint32 {
	return s.errCounter
}

func (s FSM) CurrentSuccesses() uint32 {
	return s.okCounter
}

func (s FSM) OperationThreshold() uint32 {
	return s.threshold
}
