// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"testing"

	"github.com/ice-blockchain/subzero/validation"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

func TestHandleReactionNotification(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:         make(map[DeviceID]DeviceInfo),
		userDevices:     make(map[string][]DeviceID),
		filterToDevices: make(map[NotificationType]map[DeviceID]bool),
	}

	pm.filterToDevices[NotificationTypeReaction] = make(map[DeviceID]bool)

	event := helperCreateTestEvent(
		t,
		"test_id",
		"test_pubkey",
		nostr.KindReaction,
		"+",
		nostr.Tags{
			{"e", "original_event_id"},
			{"p", "original_author_pubkey"},
		},
	)

	pm.devices[DeviceID("device1")] = DeviceInfo{
		Platform: validation.DeviceTokenOSIOS,
		DeviceID: "device1",
		FCMToken: "test_token",
		PubKey:   "original_author_pubkey",
		Filters: nostr.Filters{
			{
				Kinds: []int{nostr.KindReaction},
			},
		},
	}

	pm.userDevices["original_author_pubkey"] = []DeviceID{"device1"}
	pm.filterToDevices[NotificationTypeReaction]["device1"] = true

	notifications := pm.handleReactionNotification(&event.Event)

	require.NotNil(t, notifications)

	require.Len(t, notifications, 1, "There should be one single notification")
	notification := notifications[0]
	require.Equal(t, "New reaction", notification.Title, "Title should match")
	require.Equal(t, "Someone reacted to your post", notification.Body, "Body should match")
	require.Equal(t, "test_token", notification.Target.Token, "Token should match")
	require.Equal(t, DeviceID("device1"), notification.Target.DeviceID, "DeviceID should match")
}

func TestHandleReactionNotificationWithVariousTypes(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:         make(map[DeviceID]DeviceInfo),
		userDevices:     make(map[string][]DeviceID),
		filterToDevices: make(map[NotificationType]map[DeviceID]bool),
	}

	pm.filterToDevices[NotificationTypeReaction] = make(map[DeviceID]bool)

	reactions := []string{"+", "❤️", "👍", "😂", "🔥"}

	for _, reaction := range reactions {
		event := helperCreateTestEvent(
			t,
			"test_id_"+reaction,
			"test_pubkey",
			nostr.KindReaction,
			reaction,
			nostr.Tags{
				{"e", "original_event_id"},
				{"p", "original_author_pubkey"},
			},
		)

		pm.devices[DeviceID("device1")] = DeviceInfo{
			Platform: validation.DeviceTokenOSIOS,
			DeviceID: "device1",
			FCMToken: "test_token",
			PubKey:   "original_author_pubkey",
			Filters: nostr.Filters{
				{
					Kinds: []int{nostr.KindReaction},
				},
			},
		}

		pm.userDevices["original_author_pubkey"] = []DeviceID{"device1"}
		pm.filterToDevices[NotificationTypeReaction]["device1"] = true

		notifications := pm.handleReactionNotification(&event.Event)

		require.NotNil(t, notifications)
		require.Len(t, notifications, 1, "There should be one single notification for reaction: "+reaction)
		notification := notifications[0]
		require.Equal(t, "New reaction", notification.Title, "Title should match")
		require.Equal(t, "Someone reacted to your post", notification.Body, "Body should match")
		require.Equal(t, "test_token", notification.Target.Token, "Token should match")
		require.Equal(t, DeviceID("device1"), notification.Target.DeviceID, "DeviceID should match")

		require.NotNil(t, notification.Data, "Notification should have data")
		require.Equal(t, reaction, notification.Data["reaction"], "Reaction content should match")
	}
}

func TestHandleSelfReactionNotification(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:         make(map[DeviceID]DeviceInfo),
		userDevices:     make(map[string][]DeviceID),
		filterToDevices: make(map[NotificationType]map[DeviceID]bool),
	}

	pm.filterToDevices[NotificationTypeReaction] = make(map[DeviceID]bool)

	event := helperCreateTestEvent(
		t,
		"test_id_self_reaction",
		"self_pubkey",
		nostr.KindReaction,
		"+",
		nostr.Tags{
			{"e", "original_event_id"},
			{"p", "self_pubkey"},
		},
	)

	pm.devices[DeviceID("device1")] = DeviceInfo{
		DeviceID: "device1",
		FCMToken: "test_token",
		PubKey:   "self_pubkey",
		Filters: nostr.Filters{
			{
				Kinds: []int{nostr.KindReaction},
			},
		},
	}

	pm.userDevices["self_pubkey"] = []DeviceID{"device1"}
	pm.filterToDevices[NotificationTypeReaction]["device1"] = true

	notifications := pm.handleReactionNotification(&event.Event)

	require.Nil(t, notifications, "Self-reactions should not generate notifications")
}
