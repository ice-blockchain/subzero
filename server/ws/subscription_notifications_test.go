// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/pushnotifications"
)

type mockPushNotificationManager struct {
	notifiedEvents    []*model.Event
	deletionEvents    []*model.Event
	notificationError error
}

func (m *mockPushNotificationManager) NotifyFCM(ctx context.Context, language pushnotifications.Language, events []*model.Event) error {
	m.notifiedEvents = append(m.notifiedEvents, events...)
	if m.notificationError != nil {
		return m.notificationError
	}

	return nil
}

func (m *mockPushNotificationManager) ProcessDeletionEvents(ctx context.Context, events []*model.Event) error {
	m.deletionEvents = append(m.deletionEvents, events...)

	return nil
}

func TestNotifications(t *testing.T) {
	t.Cleanup(func() {
		RegisterReqMustAuthenticate(nil)
		RegisterEventMustAuthenticate(nil)
	})

	var capturedEvents []*model.Event
	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		capturedEvents = append(capturedEvents, events...)
		t.Logf("received events: %v", events)
		return nil
	})

	RegisterWSSubscriptionListener(func(ctx context.Context, subscription *model.Subscription) EventIterator {
		t.Logf("received subscription: %v", subscription)
		return helperNewIterator(t, []*model.Event{})
	})

	RegisterReqMustAuthenticate(func(ctx context.Context, subscription *model.Subscription) bool {
		return true
	})
	RegisterEventMustAuthenticate(func(ctx context.Context, events ...*model.Event) bool {
		return true
	})

	priv, pub := model.GenerateKeyPair()
	masterPriv, masterPub := model.GenerateKeyPair()

	var attestation model.Event
	attestation.Kind = model.CustomIONKindAttestation
	attestation.CreatedAt = 1
	attestation.Tags = model.Tags{
		{model.TagAttestationName, pub, "", model.CustomIONAttestationKindActive + ":1"},
	}
	helperSignWithMinLeadingZeroBits(t, &attestation, masterPriv)
	require.NoError(t, query.AcceptEvents(t.Context(), &attestation))

	receiver := helperMustNewRelay(t, pubsubServers[0])
	t.Run("Auth", func(t *testing.T) {
		var note model.Event
		note.Kind = nostr.KindTextNote
		note.CreatedAt = 1
		note.Content = "test auth"
		helperSignWithMinLeadingZeroBits(t, &note, priv)
		err := receiver.Publish(t.Context(), note.Event)
		require.Error(t, err)
		helperDoAuth(t, receiver.Relay, priv, masterPub)
	})

	sub, err := receiver.Subscribe(t.Context(), []model.Filter{
		{
			Kinds: []int{nostr.KindTextNote, nostr.KindChannelMessage},
			Tags: model.TagMap{}.
				Append("p", model.PointerOf(pub)),
		},
	})
	require.NoError(t, err)

	mockManager := &mockPushNotificationManager{}

	tSend, tFinish := time.After(time.Second), time.After(time.Second*2)
	var receivedCount int
loop:
	for {
		select {
		case <-tFinish:
			break loop

		case <-tSend:
			var textNote, channelMsg, directMsg model.Event

			textNote.Kind = nostr.KindTextNote
			textNote.CreatedAt = nostr.Now()
			textNote.Content = "Test notification"
			textNote.Tags = model.Tags{
				{"p", pub},
			}

			channelMsg.Kind = nostr.KindChannelMessage
			channelMsg.CreatedAt = nostr.Now()
			channelMsg.Content = "Channel message"
			channelMsg.Tags = model.Tags{
				{"p", pub},
				{"e", "channel-id-" + strconv.FormatInt(time.Now().Unix(), 10)},
			}

			directMsg.Kind = nostr.KindEncryptedDirectMessage
			directMsg.CreatedAt = nostr.Now()
			directMsg.Content = "Private message"
			directMsg.Tags = model.Tags{
				{"p", pub},
			}

			helperSignWithMinLeadingZeroBits(t, &textNote, masterPriv)
			helperSignWithMinLeadingZeroBits(t, &channelMsg, masterPriv)
			helperSignWithMinLeadingZeroBits(t, &directMsg, masterPriv)

			sender := helperMustNewRelay(t, pubsubServers[0])
			require.NoError(t, sender.PublishMany(t.Context(), &textNote.Event, &channelMsg.Event, &directMsg.Event))
			helperMustCloseRelay(t, sender)

			mockManager.NotifyFCM(t.Context(), pushnotifications.Language("en"), []*model.Event{&textNote, &channelMsg, &directMsg})

		case ev := <-sub.Events:
			t.Logf("received event in subscription: %v", ev)
			receivedCount++
			require.True(t, ev.Tags.ContainsAny("p", []string{pub}))

		case <-sub.EndOfStoredEvents:
			t.Logf("end of stored events")

		case reason := <-sub.ClosedReason:
			t.Fatalf("subscription closed: %v", reason)
		}
	}

	require.GreaterOrEqual(t, receivedCount, 1, "Should receive at least one event")
	require.Equal(t, 3, len(mockManager.notifiedEvents), "Incorrect number of processed notifications")

	helperMustCloseRelay(t, receiver)
}

func TestNotificationsError(t *testing.T) {
	t.Cleanup(func() {
		RegisterReqMustAuthenticate(nil)
		RegisterEventMustAuthenticate(nil)
	})

	RegisterReqMustAuthenticate(func(ctx context.Context, subscription *model.Subscription) bool {
		return true
	})
	RegisterEventMustAuthenticate(func(ctx context.Context, events ...*model.Event) bool {
		return true
	})

	priv, pub := model.GenerateKeyPair()
	masterPriv, masterPub := model.GenerateKeyPair()

	var attestation model.Event
	attestation.Kind = model.CustomIONKindAttestation
	attestation.CreatedAt = 1
	attestation.Tags = model.Tags{
		{model.TagAttestationName, pub, "", model.CustomIONAttestationKindActive + ":1"},
	}
	helperSignWithMinLeadingZeroBits(t, &attestation, masterPriv)
	require.NoError(t, query.AcceptEvents(t.Context(), &attestation))

	receiver := helperMustNewRelay(t, pubsubServers[0])
	helperDoAuth(t, receiver.Relay, priv, masterPub)

	mockManager := &mockPushNotificationManager{
		notificationError: fmt.Errorf("test notification error"),
	}

	var textNote model.Event
	textNote.Kind = nostr.KindTextNote
	textNote.CreatedAt = nostr.Now()
	textNote.Content = "Test notification with error"
	textNote.Tags = model.Tags{
		{"p", pub},
	}
	helperSignWithMinLeadingZeroBits(t, &textNote, masterPriv)

	sender := helperMustNewRelay(t, pubsubServers[0])
	require.NoError(t, sender.Publish(t.Context(), textNote.Event))
	helperMustCloseRelay(t, sender)

	err := mockManager.NotifyFCM(t.Context(), pushnotifications.Language("en"), []*model.Event{&textNote})
	require.Error(t, err)
	require.Equal(t, 1, len(mockManager.notifiedEvents))

	helperMustCloseRelay(t, receiver)
}
