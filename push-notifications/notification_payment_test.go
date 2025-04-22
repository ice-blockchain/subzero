// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func helperCreateGiftWrapPaymentEvent(t *testing.T, id string, senderPubKey string, recipientPubKey string, wrappedKind int, tags []string) *model.Event {
	t.Helper()

	eventTags := nostr.Tags{}
	if recipientPubKey != "" {
		eventTags = append(eventTags, nostr.Tag{"p", recipientPubKey})
	}

	eventTags = append(eventTags, nostr.Tag{"k", strconv.Itoa(wrappedKind)})

	for i := 0; i < len(tags); i += 2 {
		if i+1 < len(tags) {
			eventTags = append(eventTags, nostr.Tag{tags[i], tags[i+1]})
		}
	}

	return &model.Event{
		Event: nostr.Event{
			ID:      id,
			PubKey:  senderPubKey,
			Kind:    nostr.KindGiftWrap,
			Content: "",
			Tags:    eventTags,
		},
	}
}

func TestHandlePaymentReceivedNotification(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:     make(map[DeviceID]DeviceInfo),
		userDevices: make(map[string][]DeviceID),
	}

	senderPubKey := "sender_pubkey"
	recipientPubKey := "recipient_pubkey"

	paymentEvent := helperCreateGiftWrapPaymentEvent(
		t,
		"payment_received_id",
		senderPubKey,
		recipientPubKey,
		model.CustomIONKindFundReceive,
		[]string{
			"amount", "100.00",
			"amount_usd", "5.00",
			"asset_id", "btc",
			"asset_class", "btc",
			"network", "bitcoin",
		},
	)

	filters := nostr.Filters{
		{
			Kinds: []int{nostr.KindGiftWrap},
		},
	}

	deviceEvent := helperCreateTestDeviceRegistrationEvent(
		t,
		recipientPubKey,
		"device1",
		[]string{"t", "ios", "token", "test_token"},
		filters,
	)

	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent))

	notifications := pm.processGiftWrapEvent(paymentEvent)

	require.NotNil(t, notifications, "Notifications should not be nil")

	require.Len(t, notifications, 1, "Should create one notification")

	notification := notifications[0]
	require.Equal(t, DefaultTranslations[NotificationTypePaymentReceived].Title, notification.Title, "Title should match")
	require.Equal(t, DefaultTranslations[NotificationTypePaymentReceived].Body, notification.Body, "Body should match")
	require.Equal(t, deviceEvent, notification.Target, "Target should match")

	require.Contains(t, notification.Data, "event", "Data should contain event")
	require.Contains(t, notification.Data, "notificationType", "Data should contain notificationType")
	require.Equal(t, string(NotificationTypePaymentReceived), notification.Data["notificationType"], "NotificationType should match")
}

func TestHandlePaymentRequestNotification(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:     make(map[DeviceID]DeviceInfo),
		userDevices: make(map[string][]DeviceID),
	}

	requesterPubKey := "requester_pubkey"
	payerPubKey := "payer_pubkey"

	paymentRequestEvent := helperCreateGiftWrapPaymentEvent(
		t,
		"payment_request_id",
		requesterPubKey,
		payerPubKey,
		model.CustomIONKindFundSendNotify,
		[]string{
			"amount", "50.00",
			"amount_usd", "2.50",
			"asset_id", "eth",
			"asset_class", "eth",
			"network", "ethereum",
		},
	)

	filters := nostr.Filters{
		{
			Kinds: []int{nostr.KindGiftWrap},
		},
	}

	deviceEvent := helperCreateTestDeviceRegistrationEvent(
		t,
		payerPubKey,
		"device1",
		[]string{"t", "ios", "token", "test_token"},
		filters,
	)

	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent))

	notifications := pm.processGiftWrapEvent(paymentRequestEvent)

	require.NotNil(t, notifications, "Notifications should not be nil")

	require.Len(t, notifications, 1, "Should create one notification")

	notification := notifications[0]
	require.Equal(t, DefaultTranslations[NotificationTypePaymentRequest].Title, notification.Title, "Title should match")
	require.Equal(t, DefaultTranslations[NotificationTypePaymentRequest].Body, notification.Body, "Body should match")
	require.Equal(t, deviceEvent, notification.Target, "Target should match")

	require.Contains(t, notification.Data, "event", "Data should contain event")
	require.Contains(t, notification.Data, "notificationType", "Data should contain notificationType")
	require.Equal(t, string(NotificationTypePaymentRequest), notification.Data["notificationType"], "NotificationType should match")
}

func TestHandlePaymentNotificationSelfPayment(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:     make(map[DeviceID]DeviceInfo),
		userDevices: make(map[string][]DeviceID),
	}

	userPubKey := "user_pubkey"

	selfPaymentEvent := helperCreateGiftWrapPaymentEvent(
		t,
		"self_payment_id",
		userPubKey,
		userPubKey,
		model.CustomIONKindFundReceive,
		[]string{
			"amount", "100.00",
			"asset_id", "btc",
		},
	)

	filters := nostr.Filters{
		{
			Kinds: []int{nostr.KindGiftWrap},
		},
	}

	deviceEvent := helperCreateTestDeviceRegistrationEvent(
		t,
		userPubKey,
		"device1",
		[]string{"t", "ios", "token", "test_token"},
		filters,
	)

	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent))

	notifications := pm.processGiftWrapEvent(selfPaymentEvent)

	require.Empty(t, notifications, "Should not create notifications for self-payments")
}

func TestHandlePaymentNotificationNoRecipient(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:     make(map[DeviceID]DeviceInfo),
		userDevices: make(map[string][]DeviceID),
	}

	noRecipientEvent := helperCreateGiftWrapPaymentEvent(
		t,
		"payment_no_recipient_id",
		"sender_pubkey",
		"",
		model.CustomIONKindFundReceive,
		[]string{
			"amount", "100.00",
			"asset_id", "btc",
		},
	)

	notifications := pm.processGiftWrapEvent(noRecipientEvent)

	require.Nil(t, notifications, "Should not create notifications for payments without recipient")
}

func TestHandlePaymentNotificationDifferentAssetTypes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		amount     string
		amountUSD  string
		assetID    string
		assetClass string
		network    string
	}{
		{
			name:       "Bitcoin payment",
			amount:     "0.01",
			amountUSD:  "300.00",
			assetID:    "btc",
			assetClass: "btc",
			network:    "bitcoin",
		},
		{
			name:       "Ethereum payment",
			amount:     "1.5",
			amountUSD:  "2500.00",
			assetID:    "eth",
			assetClass: "eth",
			network:    "ethereum",
		},
		{
			name:       "USDT payment",
			amount:     "100.00",
			amountUSD:  "100.00",
			assetID:    "usdt",
			assetClass: "usdt",
			network:    "ethereum",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pm := &PushNotificationManager{
				devices:     make(map[DeviceID]DeviceInfo),
				userDevices: make(map[string][]DeviceID),
			}

			senderPubKey := "sender_" + tc.assetID
			recipientPubKey := "recipient_" + tc.assetID

			paymentEvent := helperCreateGiftWrapPaymentEvent(
				t,
				"payment_"+tc.assetID,
				senderPubKey,
				recipientPubKey,
				model.CustomIONKindFundReceive,
				[]string{
					"amount", tc.amount,
					"amount_usd", tc.amountUSD,
					"asset_id", tc.assetID,
					"asset_class", tc.assetClass,
					"network", tc.network,
				},
			)

			filters := nostr.Filters{
				{
					Kinds: []int{nostr.KindGiftWrap},
				},
			}

			deviceEvent := helperCreateTestDeviceRegistrationEvent(
				t,
				recipientPubKey,
				"device_"+tc.assetID,
				[]string{"t", "ios", "token", "test_token_" + tc.assetID},
				filters,
			)

			require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent))

			notifications := pm.processGiftWrapEvent(paymentEvent)

			require.NotNil(t, notifications, "Notifications should not be nil")

			require.Len(t, notifications, 1, "Should create one notification")

			notification := notifications[0]
			require.Equal(t, DefaultTranslations[NotificationTypePaymentReceived].Title, notification.Title, "Title should match")
			require.Equal(t, DefaultTranslations[NotificationTypePaymentReceived].Body, notification.Body, "Body should match")
			require.Equal(t, deviceEvent, notification.Target, "Target should match")

			require.Contains(t, notification.Data, "event", "Data should contain event")
			require.Contains(t, notification.Data, "notificationType", "Data should contain notificationType")
			require.Equal(t, string(NotificationTypePaymentReceived), notification.Data["notificationType"], "NotificationType should match")
		})
	}
}

func TestHandlePaymentNotificationMultipleDevices(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:     make(map[DeviceID]DeviceInfo),
		userDevices: make(map[string][]DeviceID),
	}

	senderPubKey := "multi_sender_pubkey"
	recipientPubKey := "multi_recipient_pubkey"

	paymentEvent := helperCreateGiftWrapPaymentEvent(
		t,
		"multi_payment_id",
		senderPubKey,
		recipientPubKey,
		model.CustomIONKindFundReceive,
		[]string{
			"amount", "250.00",
			"amount_usd", "250.00",
			"asset_id", "usdc",
			"asset_class", "usdc",
			"network", "polygon",
		},
	)

	filters := nostr.Filters{
		{
			Kinds: []int{nostr.KindGiftWrap},
		},
	}

	deviceCount := 3
	deviceEvents := make([]*model.Event, deviceCount)
	for i := 0; i < deviceCount; i++ {
		deviceEvents[i] = helperCreateTestDeviceRegistrationEvent(
			t,
			recipientPubKey,
			fmt.Sprintf("multi_device%d", i+1),
			[]string{"t", "ios", "token", fmt.Sprintf("multi_token%d", i+1)},
			filters,
		)

		require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvents[i]))
	}

	notifications := pm.processGiftWrapEvent(paymentEvent)

	require.NotNil(t, notifications, "Notifications should not be nil")

	require.Len(t, notifications, deviceCount, "Should create notifications for all devices")

	targetDeviceIDs := make(map[string]bool)
	for _, device := range deviceEvents {
		targetDeviceIDs[device.Tags.GetD()] = false
	}

	for _, notification := range notifications {
		deviceID := notification.Target.Tags.GetD()
		_, exists := targetDeviceIDs[deviceID]
		require.True(t, exists, "Notification should be for a registered device")
		targetDeviceIDs[deviceID] = true

		require.Equal(t, DefaultTranslations[NotificationTypePaymentReceived].Title, notification.Title, "Title should match")
		require.Equal(t, DefaultTranslations[NotificationTypePaymentReceived].Body, notification.Body, "Body should match")
		require.Contains(t, notification.Data, "event", "Data should contain event")
		require.Contains(t, notification.Data, "notificationType", "Data should contain notificationType")
		require.Equal(t, string(NotificationTypePaymentReceived), notification.Data["notificationType"], "NotificationType should match")
	}

	for deviceID, processed := range targetDeviceIDs {
		require.True(t, processed, "Device %s should have received a notification", deviceID)
	}
}
