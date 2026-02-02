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

func helperNewFSM(tb testing.TB) FSM {
	tb.Helper()
	return New(testWindowSize, testOperationThreshold)
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
