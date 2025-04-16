// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"testing"

	"github.com/ice-blockchain/subzero/model"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

func TestHandlePaymentRequestNotification(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:         make(map[DeviceID]DeviceInfo),
		userDevices:     make(map[string][]DeviceID),
		filterToDevices: make(map[NotificationType]map[DeviceID]bool),
		invalidTokens:   make(map[DeviceID]InvalidTokenInfo),
		translationMgr: &TranslationManager{
			translations: map[NotificationType]map[Language]map[string]string{
				NotificationTypePaymentRequest: {
					Language("en"): {
						"title": "Payment Request",
						"body":  "You received a payment request",
					},
				},
			},
		},
	}

	pm.filterToDevices[NotificationTypePaymentRequest] = make(map[DeviceID]bool)

	event := helperCreateTestEvent(
		t,
		"test_id",
		"sender_pubkey",
		model.CustomIONKindFundSendNotify,
		"Payment request",
		nostr.Tags{
			{"p", "recipient_pubkey"},
			{"l", "address123"},
			{"amount", "100"},
			{"network", "bitcoin"},
		},
	)

	pm.devices[DeviceID("device1")] = DeviceInfo{
		DeviceID: "device1",
		FCMToken: "test_token",
		PubKey:   "recipient_pubkey",
		Filters: nostr.Filters{
			{
				Kinds: []int{model.CustomIONKindFundSendNotify},
			},
		},
	}

	pm.userDevices["recipient_pubkey"] = []DeviceID{"device1"}
	pm.filterToDevices[NotificationTypePaymentRequest]["device1"] = true

	notifications := pm.handlePaymentRequestNotification(&event.Event, "en")

	require.NotNil(t, notifications)

	require.Len(t, notifications.singleNotifications, 1, "Should create one single notification")
	notification := notifications.singleNotifications[0]
	require.Equal(t, "Payment Request", notification.Title, "Title should match")
	require.Equal(t, "You received a payment request", notification.Body, "Body should match")
	require.Equal(t, "test_token", notification.Target.Token, "Token should match")
	require.Equal(t, DeviceID("device1"), notification.Target.DeviceID, "DeviceID should match")
}
