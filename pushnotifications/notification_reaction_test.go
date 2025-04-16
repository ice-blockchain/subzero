// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

func TestHandleReactionNotification(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:         make(map[DeviceID]DeviceInfo),
		userDevices:     make(map[string][]DeviceID),
		filterToDevices: make(map[NotificationType]map[DeviceID]bool),
		invalidTokens:   make(map[DeviceID]InvalidTokenInfo),
		translationMgr: &TranslationManager{
			translations: map[NotificationType]map[Language]map[string]string{
				NotificationTypeReaction: {
					Language("en"): {
						"title": "Reaction Notification",
						"body":  "User liked your post",
					},
				},
			},
		},
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

	notifications := pm.handleReactionNotification(&event.Event, "en")

	require.NotNil(t, notifications)

	require.Len(t, notifications.singleNotifications, 1, "There should be one single notification")
	notification := notifications.singleNotifications[0]
	require.Equal(t, "Reaction Notification", notification.Title, "Title should match")
	require.Equal(t, "User liked your post", notification.Body, "Body should match")
	require.Equal(t, "test_token", notification.Target.Token, "Token should match")
	require.Equal(t, DeviceID("device1"), notification.Target.DeviceID, "DeviceID should match")
}
