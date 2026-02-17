// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/push-notifications/internal"
	"github.com/ice-blockchain/subzero/rq"
)

func TestRemoteNotificationSendFlow(t *testing.T) {
	t.Parallel()

	addr, release := query.NewTestDatabase(t.Context())
	defer release()

	// Create push notification manager with mocks.
	pm := helperNewManager(t)

	mockNotificationClient := &mockNotificationClient{
		T:    t,
		Chan: make(chan *internal.Notification[*model.Event], 100),
	}
	pm.pushNotificationClient = internal.Client(mockNotificationClient)

	dbConf := query.Config{
		PrivateKey: pm.privateKey,
		RelayURL:   pm.relayURL,
		WriteURLs:  []string{addr},
	}

	pm.rq = rq.MustNewClient(t.Context(), rq.WithConfig(&rq.Config{
		Config: dbConf,
		ID:     "pn-remote-test",
	}))
	pm.registerWorkers()

	require.NoError(t, pm.rq.Start(t.Context()))
	defer pm.rq.Stop(t.Context())

	const (
		device2Relay1 = "wss://relay3.example.com"
		device2Relay2 = "wss://relay1.example.com"

		device3Relay1 = "wss://relay1.example.com"
		device3Relay2 = "wss://relay2.example.com"
	)

	authorPrivKey, authorPubKey := model.GenerateKeyPair()
	_, subscriber1PubKey := model.GenerateKeyPair()
	_, subscriber2PubKey := model.GenerateKeyPair()
	_, subscriber3PubKey := model.GenerateKeyPair()

	t.Logf("Author: %s", authorPubKey)
	t.Logf("Subscriber1 (local device owner): %s", subscriber1PubKey)
	t.Logf("Subscriber2 (remote device owner): %s", subscriber2PubKey)
	t.Logf("Subscriber3 (remote device owner): %s", subscriber3PubKey)

	filters := model.Filters{
		{
			Kinds:   []int{nostr.KindTextNote, model.CustomIONKindEditableTextNote},
			Authors: []string{authorPubKey},
		},
	}

	device1Tags := model.Tags{
		{"d", "device1"},
		{"t", "ios"},
		{"relay", pm.relayURL},
		{"token", "device1-token"},
	}
	device1Event := helperCreateTestDeviceRegistrationEvent(t, subscriber1PubKey, "device1", device1Tags, filters)
	require.NoError(t, pm.processDeviceRegistrationEvent(device1Event))

	device2ID := subscriber2PubKey + "_device2"
	device2Tags := model.Tags{
		{"d", device2ID},
		{"t", "android"},
		{"relay", device2Relay1},
		{"relay", device2Relay2},
	}
	device2Event := helperCreateTestDeviceRegistrationEvent(t, subscriber2PubKey, device2ID, device2Tags, filters)
	require.NoError(t, pm.processDeviceRegistrationEvent(device2Event))

	device3ID := subscriber3PubKey + "_device3"
	device3Tags := model.Tags{
		{"d", device3ID},
		{"t", "web"},
		{"relay", device3Relay1},
		{"relay", device3Relay2},
		{"relay", device2Relay1},
	}
	device3Event := helperCreateTestDeviceRegistrationEvent(t, subscriber3PubKey, device3ID, device3Tags, filters)
	require.NoError(t, pm.processDeviceRegistrationEvent(device3Event))

	t.Run("Verify device registrations", func(t *testing.T) {
		pm.deviceMutex.RLock()
		defer pm.deviceMutex.RUnlock()

		require.Contains(t, pm.userDevicesMap, subscriber1PubKey)
		require.Len(t, pm.userDevicesMap[subscriber1PubKey], 1)
		require.False(t, pm.userDevicesMap[subscriber1PubKey]["device1"].Remote)

		require.Contains(t, pm.userDevicesMap, subscriber2PubKey)
		require.Len(t, pm.userDevicesMap[subscriber2PubKey], 1)
		require.True(t, pm.userDevicesMap[subscriber2PubKey]["device2"].Remote)

		require.Contains(t, pm.userDevicesMap, subscriber3PubKey)
		require.Len(t, pm.userDevicesMap[subscriber3PubKey], 1)
		require.True(t, pm.userDevicesMap[subscriber3PubKey]["device3"].Remote)
	})

	textNoteEvent := &model.Event{Event: nostr.Event{
		Kind:      nostr.KindTextNote,
		Content:   "Hello from the author! This is a test post.",
		CreatedAt: nostr.Now(),
		Tags: model.Tags{
			{"p", subscriber1PubKey}, // Tag subscriber1.
			{"p", subscriber2PubKey}, // Tag subscriber2.
			{"p", subscriber3PubKey}, // Tag subscriber3.
		},
	}}
	require.NoError(t, textNoteEvent.SignWithAlg(authorPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	t.Logf("Created text note event: %s", textNoteEvent.ID)

	var tagets *notificationTargets
	t.Run("Process event and verify notifications are created", func(t *testing.T) {
		notifications, err := pm.processEvent(t.Context(), textNoteEvent)
		require.NoError(t, err)
		require.NotNil(t, notifications)

		require.Len(t, notifications.Local, 1, "Should have 1 local notification")
		require.Equal(t, device1Event, notifications.Local[0].Target)
		require.Equal(t, defaultTranslations[NotificationTypeMentionReply].Title, notifications.Local[0].Title)
		require.Equal(t, defaultTranslations[NotificationTypeMentionReply].Body, notifications.Local[0].Body)

		require.Len(t, notifications.Remote, 3)

		tagets = notifications
	})

	t.Run("Remote send", func(t *testing.T) {
		pm.broadcaster.(*mockBroadcaster).Reset()
		require.NotNil(t, tagets)

		err := pm.sendNotifications(t.Context(), tagets)
		require.NoError(t, err)

		var receivedBroadcasts []mockedBroadcastEvent
		timeout := time.After(time.Second * 10)

	collectLoop:
		for len(receivedBroadcasts) != 3 {
			select {
			case b := <-pm.broadcaster.(*mockBroadcaster).Chan:
				t.Logf("Received broadcast to relay %s with %d events", b.RelayURL, len(b.Events))
				receivedBroadcasts = append(receivedBroadcasts, b)

			case <-timeout:
				t.Logf("Received %d broadcasts before timeout", len(receivedBroadcasts))
				break collectLoop
			}
		}

		require.Len(t, receivedBroadcasts, 3, "Should have received 3 broadcasts to shared relays")

		for _, broadcast := range receivedBroadcasts {
			require.Len(t, broadcast.Events, 1, "Each broadcast should contain 1 event (ephemeral embedding)")
			ephemeralEvent := broadcast.Events[0]
			require.Equal(t, model.CustomIONKindEphemeralEmbedding, ephemeralEvent.Kind, "Broadcast should be ephemeral embedding")

			ok, err := ephemeralEvent.CheckSignature()
			require.NoError(t, err)
			require.True(t, ok, "Ephemeral event signature should be valid")

			for _, event := range broadcast.Events {
				t.Logf("received event %s from relay %s to relay %s", event.ID, event.GetTag("relay").Value(), broadcast.RelayURL)
				require.Equal(t, pm.relayURL, event.GetTag("relay").Value())
			}
		}
	})
}
