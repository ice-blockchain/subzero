// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"errors"
	"strconv"
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
)

func createDummyEvent(kind int) *model.Event {
	return &model.Event{
		Event: nostr.Event{
			Kind: kind,
		},
	}
}

func TestPushStats_RecordSuccess(t *testing.T) {
	stats := newPushStats()

	stats.RecordSuccess(createDummyEvent(nostr.KindTextNote))
	stats.RecordSuccess(createDummyEvent(nostr.KindTextNote))
	stats.RecordSuccess(createDummyEvent(nostr.KindFollowList))

	snapshot := stats.GetStats()

	require.Equal(t, uint64(3), snapshot.TotalSuccess)
	require.Equal(t, uint64(0), snapshot.TotalErrors)
	require.Equal(t, uint64(2), snapshot.SuccessByKind[strconv.Itoa(nostr.KindTextNote)])
	require.Equal(t, uint64(1), snapshot.SuccessByKind[strconv.Itoa(nostr.KindFollowList)])
}

func TestPushStats_RecordError(t *testing.T) {
	stats := newPushStats()

	invalidTokenErr := pn.ErrInvalidDeviceToken
	messageTooLargeErr := pn.ErrMessageTooLarge
	otherErr := errors.New("some other error")

	stats.RecordError(createDummyEvent(nostr.KindTextNote), invalidTokenErr)
	stats.RecordError(createDummyEvent(nostr.KindTextNote), invalidTokenErr)
	stats.RecordError(createDummyEvent(nostr.KindFollowList), messageTooLargeErr)
	stats.RecordError(createDummyEvent(nostr.KindReaction), otherErr)

	snapshot := stats.GetStats()

	require.Equal(t, uint64(0), snapshot.TotalSuccess)
	require.Equal(t, uint64(4), snapshot.TotalErrors)
	require.Equal(t, uint64(2), snapshot.ErrorsByKind[strconv.Itoa(nostr.KindTextNote)]["invalid_token"])
	require.Equal(t, uint64(1), snapshot.ErrorsByKind[strconv.Itoa(nostr.KindFollowList)]["message_too_large"])
	require.Equal(t, uint64(1), snapshot.ErrorsByKind[strconv.Itoa(nostr.KindReaction)]["other_error"])
}

func TestPushStats_MixedStats(t *testing.T) {
	stats := newPushStats()

	stats.RecordSuccess(createDummyEvent(nostr.KindTextNote))
	stats.RecordError(createDummyEvent(nostr.KindTextNote), pn.ErrInvalidDeviceToken)
	stats.RecordSuccess(createDummyEvent(nostr.KindFollowList))
	stats.RecordError(createDummyEvent(nostr.KindFollowList), pn.ErrMessageTooLarge)

	snapshot := stats.GetStats()

	require.Equal(t, uint64(2), snapshot.TotalSuccess)
	require.Equal(t, uint64(2), snapshot.TotalErrors)
	require.Equal(t, uint64(1), snapshot.SuccessByKind[strconv.Itoa(nostr.KindTextNote)])
	require.Equal(t, uint64(1), snapshot.SuccessByKind[strconv.Itoa(nostr.KindFollowList)])
	require.Equal(t, uint64(1), snapshot.ErrorsByKind[strconv.Itoa(nostr.KindTextNote)]["invalid_token"])
	require.Equal(t, uint64(1), snapshot.ErrorsByKind[strconv.Itoa(nostr.KindFollowList)]["message_too_large"])
}

func TestGetExtendedKind(t *testing.T) {
	testCases := []struct {
		name         string
		event        *model.Event
		expectedKind string
	}{
		{
			name: "Kind 1059 with k tag",
			event: &model.Event{
				Event: nostr.Event{
					Kind: nostr.KindGiftWrap,
					Tags: model.Tags{{"k", "7"}},
				},
			},
			expectedKind: "1059+7",
		},
		{
			name: "Kind 1059 without k tag",
			event: &model.Event{
				Event: nostr.Event{
					Kind: nostr.KindGiftWrap,
					Tags: model.Tags{},
				},
			},
			expectedKind: "1059",
		},
		{
			name: "Kind 16 (repost) with valid content",
			event: &model.Event{
				Event: nostr.Event{
					Kind:    nostr.KindGenericRepost,
					Content: `{"kind":30175,"content":"test"}`,
				},
			},
			expectedKind: "16+30175",
		},
		{
			name: "Kind 16 (repost) with invalid content",
			event: &model.Event{
				Event: nostr.Event{
					Kind:    nostr.KindGenericRepost,
					Content: "invalid json",
				},
			},
			expectedKind: "16",
		},
		{
			name: "Regular kind",
			event: &model.Event{
				Event: nostr.Event{
					Kind: nostr.KindTextNote,
				},
			},
			expectedKind: "1",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := getExtendedKind(tc.event)
			require.Equal(t, tc.expectedKind, result)
		})
	}
}

func TestPushStats_ExtendedKind_Integration(t *testing.T) {
	stats := newPushStats()
	event1059 := &model.Event{
		Event: nostr.Event{
			Kind: nostr.KindGiftWrap,
			Tags: model.Tags{{"k", "7"}},
		},
	}
	stats.RecordSuccess(event1059)
	event16 := &model.Event{
		Event: nostr.Event{
			Kind:    nostr.KindGenericRepost,
			Content: `{"kind":30175,"content":"test"}`,
		},
	}
	stats.RecordSuccess(event16)
	eventRegular := &model.Event{
		Event: nostr.Event{
			Kind: nostr.KindTextNote,
		},
	}
	stats.RecordSuccess(eventRegular)

	snapshot := stats.GetStats()

	require.Equal(t, uint64(3), snapshot.TotalSuccess)
	require.Equal(t, uint64(1), snapshot.SuccessByKind["1059+7"])
	require.Equal(t, uint64(1), snapshot.SuccessByKind["16+30175"])
	require.Equal(t, uint64(1), snapshot.SuccessByKind["1"])
}

func TestPushStats_ConcurrentAccess(t *testing.T) {
	stats := newPushStats()
	done := make(chan bool, 2)
	go func() {
		for i := 0; i < 100; i++ {
			stats.RecordSuccess(createDummyEvent(nostr.KindReaction))
		}
		done <- true
	}()
	go func() {
		for i := 0; i < 100; i++ {
			stats.RecordError(createDummyEvent(nostr.KindFollowList), errors.New("test error"))
		}
		done <- true
	}()
	<-done
	<-done

	snapshot := stats.GetStats()
	require.Equal(t, uint64(100), snapshot.TotalSuccess)
	require.Equal(t, uint64(100), snapshot.TotalErrors)
	require.Equal(t, uint64(100), snapshot.SuccessByKind[strconv.Itoa(nostr.KindReaction)])
	require.Equal(t, uint64(100), snapshot.ErrorsByKind[strconv.Itoa(nostr.KindFollowList)]["other_error"])
}

func TestClassifyError_DetailedReasons(t *testing.T) {
	testCases := []struct {
		name           string
		err            error
		expectedReason string
	}{
		{
			name:           "DecryptToken error",
			err:            pn.ErrDecryptToken,
			expectedReason: "decrypt_token_error",
		},
		{
			name:           "MessageTooLarge error",
			err:            pn.ErrMessageTooLarge,
			expectedReason: "message_too_large",
		},
		{
			name:           "InvalidDeviceToken error",
			err:            pn.ErrInvalidDeviceToken,
			expectedReason: "invalid_token",
		},
		{
			name:           "Generic error",
			err:            errors.New("network timeout"),
			expectedReason: "other_error",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reason := classifyError(tc.err)
			require.Equal(t, tc.expectedReason, reason)
		})
	}
}
