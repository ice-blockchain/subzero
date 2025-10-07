// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
)

func TestPushStats_RecordSuccess(t *testing.T) {
	stats := newPushStats()

	// Record some successes
	stats.RecordSuccess(nostr.KindTextNote)
	stats.RecordSuccess(nostr.KindTextNote)
	stats.RecordSuccess(nostr.KindFollowList)

	snapshot := stats.GetStats()

	require.Equal(t, uint64(3), snapshot.TotalSuccess)
	require.Equal(t, uint64(0), snapshot.TotalErrors)
	require.Equal(t, uint64(2), snapshot.SuccessByKind[nostr.KindTextNote])
	require.Equal(t, uint64(1), snapshot.SuccessByKind[nostr.KindFollowList])
}

func TestPushStats_RecordError(t *testing.T) {
	stats := newPushStats()

	invalidTokenErr := pn.ErrInvalidDeviceToken
	messageTooLargeErr := pn.ErrMessageTooLarge
	otherErr := errors.New("some other error")

	stats.RecordError(nostr.KindTextNote, invalidTokenErr)
	stats.RecordError(nostr.KindTextNote, invalidTokenErr)
	stats.RecordError(nostr.KindFollowList, messageTooLargeErr)
	stats.RecordError(nostr.KindReaction, otherErr)

	snapshot := stats.GetStats()

	require.Equal(t, uint64(0), snapshot.TotalSuccess)
	require.Equal(t, uint64(4), snapshot.TotalErrors)
	require.Equal(t, uint64(2), snapshot.ErrorsByKind[nostr.KindTextNote]["invalid_token"])
	require.Equal(t, uint64(1), snapshot.ErrorsByKind[nostr.KindFollowList]["message_too_large"])
	require.Equal(t, uint64(1), snapshot.ErrorsByKind[nostr.KindReaction]["other_error"])
}

func TestPushStats_MixedStats(t *testing.T) {
	stats := newPushStats()

	stats.RecordSuccess(nostr.KindTextNote)
	stats.RecordError(nostr.KindTextNote, pn.ErrInvalidDeviceToken)
	stats.RecordSuccess(nostr.KindFollowList)
	stats.RecordError(nostr.KindFollowList, pn.ErrMessageTooLarge)

	snapshot := stats.GetStats()

	require.Equal(t, uint64(2), snapshot.TotalSuccess)
	require.Equal(t, uint64(2), snapshot.TotalErrors)
	require.Equal(t, uint64(1), snapshot.SuccessByKind[nostr.KindTextNote])
	require.Equal(t, uint64(1), snapshot.SuccessByKind[nostr.KindFollowList])
	require.Equal(t, uint64(1), snapshot.ErrorsByKind[nostr.KindTextNote]["invalid_token"])
	require.Equal(t, uint64(1), snapshot.ErrorsByKind[nostr.KindFollowList]["message_too_large"])
}

func TestClassifyError(t *testing.T) {
	testCases := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "message too large",
			err:      pn.ErrMessageTooLarge,
			expected: "message_too_large",
		},
		{
			name:     "invalid device token",
			err:      pn.ErrInvalidDeviceToken,
			expected: "invalid_token",
		},
		{
			name:     "decrypt token error",
			err:      pn.ErrDecryptToken,
			expected: "decrypt_token_error",
		},
		{
			name:     "unknown error",
			err:      errors.New("some random error"),
			expected: "other_error",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := classifyError(tc.err)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestClassifyError_ClassifyError(t *testing.T) {
	messageTooLargeErr := pn.ErrMessageTooLarge
	invalidTokenErr := pn.ErrInvalidDeviceToken
	decryptTokenErr := pn.ErrDecryptToken

	require.Equal(t, "message_too_large", classifyError(messageTooLargeErr))
	require.Equal(t, "invalid_token", classifyError(invalidTokenErr))
	require.Equal(t, "decrypt_token_error", classifyError(decryptTokenErr))
}

func TestPushStats_StartPeriodicLogging(t *testing.T) {
	stats := newPushStats()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	stats.StartPeriodicLogging(ctx)
	stats.RecordSuccess(1)
	stats.RecordError(nostr.KindFollowList, errors.New("test error"))
	<-ctx.Done()

	snapshot := stats.GetStats()
	require.Equal(t, uint64(1), snapshot.TotalSuccess)
	require.Equal(t, uint64(1), snapshot.TotalErrors)
}

func TestStatsSnapshot_LogStats(t *testing.T) {
	snapshot := StatsSnapshot{
		TotalSuccess:  10,
		TotalErrors:   5,
		SuccessByKind: map[int]uint64{nostr.KindTextNote: 8, nostr.KindFollowList: 2},
		ErrorsByKind: map[int]map[string]uint64{
			nostr.KindTextNote:   {"invalid_token": 3},
			nostr.KindFollowList: {"message_too_large": 2},
		},
		Duration: 5 * time.Minute,
	}
	require.NotPanics(t, func() { logStats(snapshot) })
}

func TestStatsSnapshot_LogStats_ZeroValues(t *testing.T) {
	snapshot := StatsSnapshot{
		TotalSuccess:  0,
		TotalErrors:   0,
		SuccessByKind: map[int]uint64{},
		ErrorsByKind:  map[int]map[string]uint64{},
		Duration:      0,
	}
	require.NotPanics(t, func() { logStats(snapshot) })
}

func TestPushStats_ConcurrentAccess(t *testing.T) {
	stats := newPushStats()
	done := make(chan bool, 2)
	go func() {
		for i := 0; i < 100; i++ {
			stats.RecordSuccess(nostr.KindReaction)
		}
		done <- true
	}()
	go func() {
		for i := 0; i < 100; i++ {
			stats.RecordError(nostr.KindFollowList, errors.New("test error"))
		}
		done <- true
	}()
	<-done
	<-done

	snapshot := stats.GetStats()
	require.Equal(t, uint64(100), snapshot.TotalSuccess)
	require.Equal(t, uint64(100), snapshot.TotalErrors)
	require.Equal(t, uint64(100), snapshot.SuccessByKind[nostr.KindReaction])
	require.Equal(t, uint64(100), snapshot.ErrorsByKind[nostr.KindFollowList]["other_error"])
}
