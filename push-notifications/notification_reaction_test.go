// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func helperCreateReactionEvent(t *testing.T, id string, pubKey string, content string, postID string, postAuthorPubKey string) *model.Event {
	t.Helper()

	tags := nostr.Tags{
		{"e", postID},
		{"p", postAuthorPubKey},
	}

	return &model.Event{
		Event: nostr.Event{
			ID:      id,
			PubKey:  pubKey,
			Kind:    nostr.KindReaction,
			Content: content,
			Tags:    tags,
		},
	}
}

func TestHandleReactionNotification(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:     make(map[DeviceID]DeviceInfo),
		userDevices: make(map[string][]DeviceID),
	}

	event := helperCreateReactionEvent(
		t,
		"reaction_id",
		"reactor_pubkey",
		"+",
		"post_id",
		"post_author_pubkey",
	)

	result := pm.handleReactionNotification(event)

	require.Nil(t, result, "Notification result should be nil with current implementation")
}

func TestHandleReactionNotificationSelfReaction(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:     make(map[DeviceID]DeviceInfo),
		userDevices: make(map[string][]DeviceID),
	}

	event := helperCreateReactionEvent(
		t,
		"self_reaction_id",
		"same_pubkey",
		"+",
		"self_post_id",
		"same_pubkey",
	)

	result := pm.handleReactionNotification(event)

	require.Nil(t, result, "Notification result should be nil with current implementation")
}

func TestHandleReactionNotificationNoAuthor(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:     make(map[DeviceID]DeviceInfo),
		userDevices: make(map[string][]DeviceID),
	}

	event := &model.Event{
		Event: nostr.Event{
			ID:      "reaction_without_author",
			PubKey:  "reactor_pubkey",
			Kind:    nostr.KindReaction,
			Content: "+",
			Tags: nostr.Tags{
				{"e", "post_id_without_author"},
			},
		},
	}

	result := pm.handleReactionNotification(event)

	require.Nil(t, result, "Notification result should be nil with current implementation")
}

func TestHandleReactionNotificationNoPostAuthorDevices(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:     make(map[DeviceID]DeviceInfo),
		userDevices: make(map[string][]DeviceID),
	}

	event := helperCreateReactionEvent(
		t,
		"reaction_id_no_devices",
		"reactor_pubkey",
		"+",
		"post_id_no_devices",
		"author_without_devices",
	)

	result := pm.handleReactionNotification(event)

	require.Nil(t, result, "Notification result should be nil with current implementation")
}

func TestHandleReactionNotificationDifferentReactionTypes(t *testing.T) {
	testCases := []struct {
		name         string
		reactionType string
		expectResult bool
	}{
		{
			name:         "Reaction type: +",
			reactionType: "+",
			expectResult: true,
		},
		{
			name:         "Reaction type: -",
			reactionType: "-",
			expectResult: true,
		},
		{
			name:         "Reaction type: ❤️",
			reactionType: "❤️",
			expectResult: true,
		},
		{
			name:         "Reaction type: 👍",
			reactionType: "👍",
			expectResult: true,
		},
		{
			name:         "Reaction type: 😂",
			reactionType: "😂",
			expectResult: true,
		},
		{
			name:         "Reaction type: 🎉",
			reactionType: "🎉",
			expectResult: true,
		},
		{
			name:         "Reaction type: 🗑️",
			reactionType: "🗑️",
			expectResult: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pm := &PushNotificationManager{
				devices:     make(map[DeviceID]DeviceInfo),
				userDevices: make(map[string][]DeviceID),
			}

			event := helperCreateReactionEvent(
				t,
				"reaction_id_"+tc.reactionType,
				"reactor_pubkey",
				tc.reactionType,
				"post_id",
				"author_pubkey",
			)

			result := pm.handleReactionNotification(event)

			require.Nil(t, result, "Notification result should be nil with current implementation")
		})
	}
}

func TestHandleReactionNotificationMultipleDevices(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:     make(map[DeviceID]DeviceInfo),
		userDevices: make(map[string][]DeviceID),
	}

	authorPubKey := "multi_device_author"
	reactorPubKey := "reactor_pubkey"

	device1 := DeviceID("device1")
	device2 := DeviceID("device2")
	device3 := DeviceID("device3")

	filtersReactionsEnabled := nostr.Filters{
		{
			Kinds: []int{nostr.KindReaction},
		},
	}

	filtersReactionsDisabled := nostr.Filters{
		{
			Kinds: []int{nostr.KindTextNote},
		},
	}

	device1Event := helperCreateTestDeviceRegistrationEvent(
		t,
		authorPubKey,
		string(device1),
		[]string{"t", "ios", "token", "token1"},
		filtersReactionsEnabled,
	)

	device2Event := helperCreateTestDeviceRegistrationEvent(
		t,
		authorPubKey,
		string(device2),
		[]string{"t", "android", "token", "token2"},
		filtersReactionsEnabled,
	)

	device3Event := helperCreateTestDeviceRegistrationEvent(
		t,
		authorPubKey,
		string(device3),
		[]string{"t", "web", "token", "token3"},
		filtersReactionsDisabled,
	)

	require.NoError(t, pm.processDeviceRegistrationEvent(device1Event))
	require.NoError(t, pm.processDeviceRegistrationEvent(device2Event))
	require.NoError(t, pm.processDeviceRegistrationEvent(device3Event))

	event := helperCreateReactionEvent(
		t,
		"reaction_id_multi_devices",
		reactorPubKey,
		"❤️",
		"post_id_multi_devices",
		authorPubKey,
	)

	notifications := pm.handleReactionNotification(event)

	require.NotNil(t, notifications, "Notifications should not be nil")

	require.Equal(t, 2, len(notifications), "Should create notifications for 2 devices")

	foundDevice1 := false
	foundDevice2 := false

	for _, notification := range notifications {
		if notification.Target.ID == device1Event.ID {
			foundDevice1 = true
			deviceType := notification.Target.GetTag("t").Value()
			require.Equal(t, "ios", deviceType, "Device type should be ios")
		} else if notification.Target.ID == device2Event.ID {
			foundDevice2 = true
			deviceType := notification.Target.GetTag("t").Value()
			require.Equal(t, "android", deviceType, "Device type should be android")
		} else if notification.Target.ID == device3Event.ID {
			t.Fatal("Device3 should not receive notification as its filter doesn't include reactions")
		}

		require.Contains(t, notification.Data, "notificationType", "Data should contain notificationType")
		require.Equal(t, string(NotificationTypeReaction), notification.Data["notificationType"], "Notification type should be 'reaction'")
		require.Contains(t, notification.Data, "event", "Payload should contain event")
	}

	require.True(t, foundDevice1, "Device1 should receive notification")
	require.True(t, foundDevice2, "Device2 should receive notification")
}
