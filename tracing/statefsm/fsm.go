// SPDX-License-Identifier: ice License 1.0

package statefsm

import (
	"fmt"
	"math"
	"sync"
	"time"
)

type (
	// FSM is a finite state machine to track consecutive successes and failures.
	FSM struct {
		startTime    time.Time     // Timestamp of the first error/success in the current sequence.
		mu           *sync.RWMutex // Optional mutex for thread safety.
		windowSize   time.Duration // Consecutive time window to consider operations.
		threshold    uint32        // Number of consecutive operations to trigger state change.
		okCounter    uint32        // Count of consecutive successful operations.
		errCounter   uint32        // Count of consecutive failed operations.
		currentState state         // Current state of the FSM.
	}
	Option func(*FSM)

	state uint8
)

const (
	stateHealthy state = iota
	stateUnhealthy
)

func WithThreadSafety() Option {
	return func(fsm *FSM) {
		fsm.mu = new(sync.RWMutex)
	}
}

// New creates a new FSM with the specified window size and operation threshold.
func New(consecutiveWindow time.Duration, consecutiveOperationThreshold uint32, opts ...Option) (fsm FSM) {
	if consecutiveWindow <= 0 {
		panic("consecutiveWindow must be greater than zero")
	}
	if consecutiveOperationThreshold == 0 {
		panic("consecutiveOperationThreshold must be greater than zero")
	}

	fsm = FSM{
		windowSize:   consecutiveWindow,
		threshold:    consecutiveOperationThreshold,
		currentState: stateHealthy,
	}
	for _, opt := range opts {
		opt(&fsm)
	}

	return fsm
}

// Push updates the FSM state based on the result of an operation and returns new state.
func (s *FSM) Push(hasError bool, currentTime time.Time) bool {
	if s.mu != nil {
		s.mu.Lock()
		defer s.mu.Unlock()
	}

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
				if s.inError() {
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
		if s.inError() && s.okCounter >= s.OperationThreshold() {
			s.currentState = stateHealthy
			s.errCounter = 0
			s.startTime = time.Time{}
		}
	}
	return s.inError()
}

func (s *FSM) inError() bool {
	return s.currentState == stateUnhealthy
}

// InError returns true if the FSM is in an unhealthy state.
func (s *FSM) InError() bool {
	if s.mu != nil {
		s.mu.RLock()
		defer s.mu.RUnlock()
	}
	return s.inError()
}

// String returns a string representation of the FSM state.
func (s *FSM) String() string {
	if s.mu != nil {
		s.mu.RLock()
		defer s.mu.RUnlock()
	}
	return fmt.Sprintf("StateFSM{window=%v, threshold=%d, state=%v, errs=%d, oks=%d, ts=%s}",
		s.windowSize,
		s.threshold,
		s.currentState,
		s.errCounter,
		s.okCounter,
		s.startTime.Format(time.RFC3339Nano),
	)
}

// CurrentErrors returns the current count of consecutive errors.
func (s *FSM) CurrentErrors() uint32 {
	if s.mu != nil {
		s.mu.RLock()
		defer s.mu.RUnlock()
	}
	return s.errCounter
}

// CurrentSuccesses returns the current count of consecutive successes.
func (s *FSM) CurrentSuccesses() uint32 {
	if s.mu != nil {
		s.mu.RLock()
		defer s.mu.RUnlock()
	}
	return s.okCounter
}

// OperationThreshold returns the configured threshold for consecutive operations to trigger state change.
func (s *FSM) OperationThreshold() uint32 {
	return s.threshold
}
