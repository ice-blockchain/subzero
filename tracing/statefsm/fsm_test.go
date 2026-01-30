// SPDX-License-Identifier: ice License 1.0

package statefsm

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	testWindowSize         = time.Duration(30 * time.Second)
	testOperationThreshold = 3
)

func helperNewFSM(tb testing.TB, opts ...Option) FSM {
	tb.Helper()
	return New(testWindowSize, testOperationThreshold, opts...)
}

func TestStateFSM(t *testing.T) {
	t.Parallel()

	t.Run("Enters error state after threshold errors", func(t *testing.T) {
		fsm := helperNewFSM(t)

		now := time.Now()
		fsm.Push(true, now)
		fsm.Push(true, now)

		t.Logf("FSM state: %s", fsm.String())
		require.False(t, fsm.InError())

		fsm.Push(true, now)
		t.Logf("FSM state: %s", fsm.String())
		require.True(t, fsm.InError())
		require.EqualValues(t, testOperationThreshold, fsm.CurrentErrors())
	})
	t.Run("Recovers from error state after threshold successes", func(t *testing.T) {
		fsm := helperNewFSM(t)

		now := time.Now()
		for range 2 * fsm.OperationThreshold() {
			fsm.Push(true, now)
		}

		t.Logf("FSM state: %s", fsm.String())
		require.True(t, fsm.InError())

		for range fsm.OperationThreshold() {
			fsm.Push(false, now)
		}

		t.Logf("FSM state: %s", fsm.String())
		require.False(t, fsm.InError())
	})
	t.Run("Does not enter error state if errors are below threshold", func(t *testing.T) {
		fsm := helperNewFSM(t)

		now := time.Now()
		for range fsm.OperationThreshold() - 1 {
			fsm.Push(true, now)
		}

		t.Logf("FSM state: %s", fsm.String())
		require.False(t, fsm.InError())
	})
	t.Run("Does not recover from error state if successes are below threshold", func(t *testing.T) {
		fsm := helperNewFSM(t)

		now := time.Now()
		for range fsm.OperationThreshold() {
			fsm.Push(true, now)
		}

		t.Logf("FSM state: %s", fsm.String())
		require.True(t, fsm.InError())

		for range fsm.OperationThreshold() - 1 {
			fsm.Push(false, now)
		}

		t.Logf("FSM state: %s", fsm.String())
		require.True(t, fsm.InError())
	})
	t.Run("Trips correctly within fast burst", func(t *testing.T) {
		fsm := helperNewFSM(t)
		now := time.Now()

		fsm.Push(true, now)
		fsm.Push(true, now.Add(10*time.Second))
		fsm.Push(true, now.Add(20*time.Second)) // 3rd error within 20s of first

		t.Logf("FSM state: %s", fsm.String())
		require.True(t, fsm.InError())
		require.Zero(t, fsm.CurrentSuccesses())
	})
	t.Run("Does not trip if errors are spaced out", func(t *testing.T) {
		fsm := helperNewFSM(t)
		now := time.Now()

		fsm.Push(true, now)
		fsm.Push(true, now.Add(10*time.Second))
		fsm.Push(true, now.Add(90*time.Second))

		t.Logf("FSM state: %s", fsm.String())
		require.False(t, fsm.InError())
	})
	t.Run("Sustainability: error while down + window expired = maintain threshold", func(t *testing.T) {
		fsm := helperNewFSM(t)
		start := time.Now()

		fsm.Push(true, start)
		fsm.Push(true, start.Add(10*time.Second))
		fsm.Push(true, start.Add(20*time.Second))

		t.Logf("FSM state: %s", fsm.String())
		require.True(t, fsm.InError())

		// The original 'startTime' was T+0. T+90 is clearly expired.
		future := start.Add(90 * time.Second)
		fsm.Push(true, future)

		// State should still be DOWN.
		t.Logf("FSM state: %s", fsm.String())
		require.True(t, fsm.InError())
	})
}

func TestStateFSMWithThreads(t *testing.T) {
	t.Parallel()

	t.Run("Concurrent Push calls are safe", func(t *testing.T) {
		const n = 10
		fsm := helperNewFSM(t, WithThreadSafety())
		now := time.Now()

		done := make(chan bool, n)
		for i := range n {
			go func(idx int) {
				defer func() { done <- true }()

				for j := range n * 10 {
					hasError := idx%2 == 0
					fsm.Push(hasError, now.Add(time.Duration(j)*time.Millisecond))
				}
			}(i)
		}

		for range n {
			<-done
		}

		require.NotNil(t, fsm)
		t.Logf("FSM state: %s", fsm.String())
	})

	t.Run("Concurrent reads are safe", func(t *testing.T) {
		fsm := helperNewFSM(t, WithThreadSafety())
		now := time.Now()

		for i := range testOperationThreshold {
			fsm.Push(true, now.Add(time.Duration(i)*time.Second))
		}
		require.True(t, fsm.InError())

		const n = 20
		done := make(chan bool, n)
		for range n {
			go func() {
				defer func() { done <- true }()

				for range n * 5 {
					_ = fsm.InError()
					_ = fsm.CurrentErrors()
					_ = fsm.CurrentSuccesses()
					_ = fsm.String()
				}
			}()
		}

		for range n {
			<-done
		}

		require.True(t, fsm.InError())
	})

	t.Run("Concurrent reads and writes are safe", func(t *testing.T) {
		fsm := helperNewFSM(t, WithThreadSafety())
		now := time.Now()

		const n = 22
		const r = n / 3
		const w = n - r

		done := make(chan bool, n)

		for i := range w {
			go func(idx int) {
				defer func() { done <- true }()

				for j := range 50 {
					hasError := idx%2 == 0
					fsm.Push(hasError, now.Add(time.Duration(j)*time.Millisecond))
				}
			}(i)
		}

		for range r {
			go func() {
				defer func() { done <- true }()

				for range 50 {
					_ = fsm.InError()
					_ = fsm.CurrentErrors()
					_ = fsm.CurrentSuccesses()
					_ = fsm.String()
				}
			}()
		}

		for range n {
			<-done
		}

		require.NotNil(t, fsm)
		t.Logf("FSM state: %s", fsm.String())
	})
}

func TestFSMPanicWithInvalidValues(t *testing.T) {
	t.Parallel()

	require.Panics(t, func() {
		_ = New(0, 1)
	})
	require.Panics(t, func() {
		_ = New(1, 0)
	})
}
