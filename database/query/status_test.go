// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func TestStatusTracker(t *testing.T) {
	t.Parallel()

	tracker := newStatusTracker()
	db := helperNewDatabase(t).
		WithOperationReporter(tracker.Submit)
	defer db.Close()

	workerCtx, cancel := context.WithCancel(t.Context())
	tracker.Start(workerCtx)

	isHealthy := func(t *testing.T) (readOK, writeOK bool) {
		status, err := tracker.Get(t.Context())
		require.NoError(t, err)

		return !status.InReadErrorState, !status.InWriteErrorState
	}

	t.Run("Initial status is healthy", func(t *testing.T) {
		readOK, writeOK := isHealthy(t)
		require.True(t, readOK)
		require.True(t, writeOK)
	})
	t.Run("Insert events successfully", func(t *testing.T) {
		for range consecutiveOperationThreshold * 10 {
			var ev model.Event

			ev.Kind = nostr.KindTextNote
			ev.CreatedAt = nostr.Now()

			require.NoError(t, ev.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.NoError(t, db.AcceptEvents(t.Context(), &ev))
		}
	})
	t.Run("Single error does not trip FSM", func(t *testing.T) {
		require.True(t, tracker.SubmitSync(t.Context(), operationTypeRead, errors.New("simulated read error")))
		require.True(t, tracker.SubmitSync(t.Context(), operationTypeWrite, errors.New("simulated write error")))

		readOK, writeOK := isHealthy(t)
		require.True(t, readOK)
		require.True(t, writeOK)
	})
	t.Run("Simulate read errors to trip FSM", func(t *testing.T) {
		for range consecutiveOperationThreshold {
			require.True(t, tracker.SubmitSync(t.Context(), operationTypeRead, errors.New("simulated read error")))
		}
		readOK, writeOK := isHealthy(t)
		require.False(t, readOK)
		require.True(t, writeOK)

		t.Run("Recover from read errors", func(t *testing.T) {
			for range consecutiveOperationThreshold {
				require.True(t, tracker.SubmitSync(t.Context(), operationTypeRead, nil))
			}

			readOK, writeOK := isHealthy(t)
			require.True(t, readOK)
			require.True(t, writeOK)
		})
	})
	t.Run("Simulate write errors to trip FSM", func(t *testing.T) {
		for range consecutiveOperationThreshold {
			require.True(t, tracker.SubmitSync(t.Context(), operationTypeWrite, errors.New("simulated write error")))
		}
		readOK, writeOK := isHealthy(t)
		require.True(t, readOK)
		require.False(t, writeOK)

		t.Run("Recover from write errors", func(t *testing.T) {
			for range consecutiveOperationThreshold {
				require.True(t, tracker.SubmitSync(t.Context(), operationTypeWrite, nil))
			}
			readOK, writeOK := isHealthy(t)
			require.True(t, readOK)
			require.True(t, writeOK)
		})
	})

	cancel()
}
