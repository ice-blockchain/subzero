// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

func TestHandleDirectMessageNotification(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:         make(map[DeviceID]DeviceInfo),
		userDevices:     make(map[string][]DeviceID),
		filterToDevices: make(map[NotificationType]map[DeviceID]bool),
		invalidTokens:   make(map[DeviceID]InvalidTokenInfo),
		translationMgr: &TranslationManager{
			translations: map[NotificationType]map[Language]map[string]string{
				NotificationTypeDirectMessage: {
					Language("en"): {
						"title": "Direct Message",
						"body":  "You have a new message",
					},
				},
			},
		},
	}

	pm.filterToDevices[NotificationTypeDirectMessage] = make(map[DeviceID]bool)

	event := helperCreateTestEvent(
		t,
		"test_id",
		"sender_pubkey",
		nostr.KindGiftWrap,
		"Encrypted message",
		nostr.Tags{
			{"p", "recipient_pubkey"},
		},
	)

	pm.devices[DeviceID("device1")] = DeviceInfo{
		DeviceID: "device1",
		FCMToken: "test_token",
		PubKey:   "recipient_pubkey",
		Filters: nostr.Filters{
			{
				Kinds: []int{nostr.KindGiftWrap},
			},
		},
	}

	pm.userDevices["recipient_pubkey"] = []DeviceID{"device1"}
	pm.filterToDevices[NotificationTypeDirectMessage]["device1"] = true

	notifications := pm.handleDirectMessageNotification(&event.Event, "en")

	require.NotNil(t, notifications)

	require.Len(t, notifications.singleNotifications, 1, "Should create one single notification")
	notification := notifications.singleNotifications[0]
	require.Equal(t, "Direct Message", notification.Title, "Title should match")
	require.Equal(t, "You have a new message", notification.Body, "Body should match")
	require.Equal(t, "test_token", notification.Target.Token, "Token should match")
	require.Equal(t, DeviceID("device1"), notification.Target.DeviceID, "DeviceID should match")
}
