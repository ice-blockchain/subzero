// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

func TestHandleChannelMessagesNotification(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:         make(map[DeviceID]DeviceInfo),
		userDevices:     make(map[string][]DeviceID),
		filterToDevices: make(map[NotificationType]map[DeviceID]bool),
		invalidTokens:   make(map[DeviceID]InvalidTokenInfo),
		translationMgr: &TranslationManager{
			translations: map[NotificationType]map[Language]map[string]string{
				NotificationTypeChannelMessage: {
					Language("en"): {
						"title": "You received a new message from {{channel_name}}",
						"body":  "{{message}}",
					},
				},
			},
		},
	}
	pm.filterToDevices[NotificationTypeChannelMessage] = make(map[DeviceID]bool)
	event := helperCreateTestEvent(
		t,
		"test_id",
		"user_pubkey",
		nostr.KindChannelMessage,
		"Channel message content",
		nostr.Tags{
			{"p", "channel_pubkey"},
		},
	)
	pm.devices[DeviceID("device1")] = DeviceInfo{
		DeviceID: "device1",
		FCMToken: "test_token",
		PubKey:   "channel_pubkey",
		Filters: nostr.Filters{
			{
				Kinds: []int{nostr.KindChannelMessage},
			},
		},
	}

	pm.userDevices["channel_pubkey"] = []DeviceID{"device1"}
	pm.filterToDevices[NotificationTypeChannelMessage]["device1"] = true

	notifications := pm.handleChannelMessagesNotification(&event.Event, "en")

	require.NotNil(t, notifications)

	require.Len(t, notifications.singleNotifications, 1, "Should create one single notification")
	notification := notifications.singleNotifications[0]
	require.Contains(t, notification.Title, "channel_pubkey", "Title should contain channel name")
	require.Contains(t, notification.Body, "Channel message content", "Body should contain message content")
	require.Equal(t, "test_token", notification.Target.Token, "Token should match")
	require.Equal(t, DeviceID("device1"), notification.Target.DeviceID, "DeviceID should match")
}
