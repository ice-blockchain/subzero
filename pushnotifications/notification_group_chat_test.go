// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

func TestHandleGroupChatMessagesNotification(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:         make(map[DeviceID]DeviceInfo),
		userDevices:     make(map[string][]DeviceID),
		filterToDevices: make(map[NotificationType]map[DeviceID]bool),
		invalidTokens:   make(map[DeviceID]InvalidTokenInfo),
		translationMgr: &TranslationManager{
			translations: map[NotificationType]map[Language]map[string]string{
				NotificationTypeGroupChatMessage: {
					Language("en"): {
						"title": "New Group Message",
						"body":  "There's a new message in the group",
					},
				},
			},
		},
	}

	pm.filterToDevices[NotificationTypeGroupChatMessage] = make(map[DeviceID]bool)

	event := helperCreateTestEvent(
		t,
		"test_id",
		"user_pubkey",
		nostr.KindSimpleGroupChatMessage,
		"Group chat message content",
		nostr.Tags{
			{"p", "group_pubkey"},
		},
	)

	pm.devices[DeviceID("device1")] = DeviceInfo{
		DeviceID: "device1",
		FCMToken: "test_token",
		PubKey:   "group_pubkey",
		Filters: nostr.Filters{
			{
				Kinds: []int{nostr.KindSimpleGroupChatMessage},
			},
		},
	}

	pm.userDevices["group_pubkey"] = []DeviceID{"device1"}
	pm.filterToDevices[NotificationTypeGroupChatMessage]["device1"] = true

	notifications := pm.handleGroupChatMessagesNotification(&event.Event, "en")

	require.NotNil(t, notifications)

	require.Len(t, notifications.singleNotifications, 1, "Should create one single notification")
	notification := notifications.singleNotifications[0]
	require.Equal(t, "New Group Message", notification.Title, "Title should match")
	require.Equal(t, "There's a new message in the group", notification.Body, "Body should match")
	require.Equal(t, "test_token", notification.Target.Token, "Token should match")
	require.Equal(t, DeviceID("device1"), notification.Target.DeviceID, "DeviceID should match")
}
