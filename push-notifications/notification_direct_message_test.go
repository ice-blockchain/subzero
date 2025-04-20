// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"encoding/json"
	"testing"

	"github.com/ice-blockchain/subzero/model"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/stretchr/testify/require"
)

func TestHandleDirectMessageNotification(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:         make(map[DeviceID]DeviceInfo),
		userDevices:     make(map[string][]DeviceID),
		filterToDevices: make(map[NotificationType]map[DeviceID]bool),
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

	notifications := pm.handleDirectMessageNotification(&event.Event)

	require.NotNil(t, notifications)

	require.Len(t, notifications, 1, "Should create one single notification")
	notification := notifications[0]
	require.Equal(t, "test_token", notification.Target.Token, "Token should match")
	require.Equal(t, DeviceID("device1"), notification.Target.DeviceID, "DeviceID should match")

	require.Equal(t, "test_id", notification.Data["eventId"])
	require.Equal(t, "sender_pubkey", notification.Data["authorPubKey"])
	require.Equal(t, string(NotificationTypeDirectMessage), notification.Data["notificationType"])
	require.Equal(t, "Encrypted message", notification.Data["content"])
}

func TestHandleDirectMessageNotificationWithoutRecipient(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:         make(map[DeviceID]DeviceInfo),
		userDevices:     make(map[string][]DeviceID),
		filterToDevices: make(map[NotificationType]map[DeviceID]bool),
	}

	pm.filterToDevices[NotificationTypeDirectMessage] = make(map[DeviceID]bool)

	event := helperCreateTestEvent(
		t,
		"test_id",
		"sender_pubkey",
		nostr.KindGiftWrap,
		"Encrypted message",
		nostr.Tags{},
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

	notifications := pm.handleDirectMessageNotification(&event.Event)

	require.Nil(t, notifications)
}

func TestHandleDirectMessageNotificationSelfSent(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:         make(map[DeviceID]DeviceInfo),
		userDevices:     make(map[string][]DeviceID),
		filterToDevices: make(map[NotificationType]map[DeviceID]bool),
	}

	pm.filterToDevices[NotificationTypeDirectMessage] = make(map[DeviceID]bool)

	event := helperCreateTestEvent(
		t,
		"test_id",
		"sender_pubkey",
		nostr.KindGiftWrap,
		"Encrypted message",
		nostr.Tags{
			{"p", "sender_pubkey"},
		},
	)

	pm.devices[DeviceID("device1")] = DeviceInfo{
		DeviceID: "device1",
		FCMToken: "test_token",
		PubKey:   "sender_pubkey",
		Filters: nostr.Filters{
			{
				Kinds: []int{nostr.KindGiftWrap},
			},
		},
	}

	pm.userDevices["sender_pubkey"] = []DeviceID{"device1"}
	pm.filterToDevices[NotificationTypeDirectMessage]["device1"] = true

	notifications := pm.handleDirectMessageNotification(&event.Event)

	require.Nil(t, notifications)
}

func TestHandleDirectMessageNotificationWithNeventLink(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:         make(map[DeviceID]DeviceInfo),
		userDevices:     make(map[string][]DeviceID),
		filterToDevices: make(map[NotificationType]map[DeviceID]bool),
	}

	pm.filterToDevices[NotificationTypeDirectMessage] = make(map[DeviceID]bool)

	paymentEvent := nostr.Event{
		ID:        "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
		PubKey:    "12345678901234567890123456789012345678901234567890123456789012ab",
		CreatedAt: nostr.Now(),
		Kind:      model.CustomIONKindFundReceive,
		Tags:      nostr.Tags{{"p", "recipient_pubkey"}, {"network", "bitcoin"}, {"asset_class", "btc"}, {"asset_address", "btc_address"}},
	}
	paymentEvent.ID = paymentEvent.GetID()

	content := paymentRequestContent{
		Amount:    "100",
		AmountUSD: "5.00",
		AssetID:   "btc",
		From:      "sender_address",
		To:        "recipient_address",
	}
	contentBytes, err := json.Marshal(content)
	require.NoError(t, err)

	paymentEvent.Content = string(contentBytes)

	relays := []string{"wss://relay.example.com"}
	nevent, err := nip19.EncodeEvent(paymentEvent.ID, relays, paymentEvent.PubKey)
	require.NoError(t, err)
	neventLink := "nostr:" + nevent

	messageWithNevent := "Check this out: " + neventLink + " and more text"

	directMessageEvent := helperCreateTestEvent(
		t,
		"test_direct_message_id",
		"sender_pubkey",
		nostr.KindGiftWrap,
		messageWithNevent,
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

	t.Run("Direct message with nevent link", func(t *testing.T) {
		notifications := pm.handleDirectMessageNotification(&directMessageEvent.Event)

		require.NotNil(t, notifications)
		require.Len(t, notifications, 1, "Should create one notification")

		notification := notifications[0]
		require.Equal(t, "test_token", notification.Target.Token, "Token should match")
		require.Equal(t, DeviceID("device1"), notification.Target.DeviceID, "DeviceID should match")

		require.Equal(t, messageWithNevent, notification.Data["content"])

		require.Equal(t, "test_direct_message_id", notification.Data["eventId"])
		require.Equal(t, "sender_pubkey", notification.Data["authorPubKey"])
		require.Equal(t, string(NotificationTypeDirectMessage), notification.Data["notificationType"])
	})

	t.Run("Direct message without nevent link", func(t *testing.T) {
		normalMessage := "This is a regular message without payment info"

		regularEvent := helperCreateTestEvent(
			t,
			"regular_message_id",
			"sender_pubkey",
			nostr.KindGiftWrap,
			normalMessage,
			nostr.Tags{
				{"p", "recipient_pubkey"},
			},
		)

		notifications := pm.handleDirectMessageNotification(&regularEvent.Event)

		require.NotNil(t, notifications)
		require.Len(t, notifications, 1, "Should create one notification")

		notification := notifications[0]
		require.Equal(t, normalMessage, notification.Data["content"])
		require.Equal(t, "regular_message_id", notification.Data["eventId"])
		require.Equal(t, "sender_pubkey", notification.Data["authorPubKey"])
		require.Equal(t, string(NotificationTypeDirectMessage), notification.Data["notificationType"])
	})

	t.Run("Direct message with invalid nevent link", func(t *testing.T) {
		invalidNeventMessage := "Check this out: nostr:invalid_nevent_link and more text"

		invalidEvent := helperCreateTestEvent(
			t,
			"invalid_nevent_message_id",
			"sender_pubkey",
			nostr.KindGiftWrap,
			invalidNeventMessage,
			nostr.Tags{
				{"p", "recipient_pubkey"},
			},
		)

		notifications := pm.handleDirectMessageNotification(&invalidEvent.Event)

		require.NotNil(t, notifications)
		require.Len(t, notifications, 1, "Should create one notification")

		notification := notifications[0]
		require.Equal(t, invalidNeventMessage, notification.Data["content"])
		require.Equal(t, "invalid_nevent_message_id", notification.Data["eventId"])
	})

	t.Run("Direct message with multiple nevent links", func(t *testing.T) {
		secondPaymentEvent := nostr.Event{
			ID:        "fedcba0987654321fedcba0987654321fedcba0987654321fedcba0987654321",
			PubKey:    "ba21098765432109876543210987654321098765432109876543210987654321",
			CreatedAt: nostr.Now(),
			Kind:      model.CustomIONKindFundSendNotify,
			Tags:      nostr.Tags{{"p", "recipient_pubkey"}, {"network", "ethereum"}, {"asset_class", "eth"}},
		}
		secondPaymentEvent.ID = secondPaymentEvent.GetID()

		secondNevent, err := nip19.EncodeEvent(secondPaymentEvent.ID, relays, secondPaymentEvent.PubKey)
		require.NoError(t, err)
		secondNeventLink := "nostr:" + secondNevent

		multipleNeventMessage := "First payment: " + neventLink + " Second payment: " + secondNeventLink

		multipleEvent := helperCreateTestEvent(
			t,
			"multiple_nevent_message_id",
			"sender_pubkey",
			nostr.KindGiftWrap,
			multipleNeventMessage,
			nostr.Tags{
				{"p", "recipient_pubkey"},
			},
		)

		notifications := pm.handleDirectMessageNotification(&multipleEvent.Event)

		require.NotNil(t, notifications)
		require.Len(t, notifications, 1, "Should create one notification")

		notification := notifications[0]
		require.Equal(t, multipleNeventMessage, notification.Data["content"])
		require.Equal(t, "multiple_nevent_message_id", notification.Data["eventId"])
	})
}

func TestHandleDirectMessageNotificationNoValidDevices(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:         make(map[DeviceID]DeviceInfo),
		userDevices:     make(map[string][]DeviceID),
		filterToDevices: make(map[NotificationType]map[DeviceID]bool),
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
		Invalid:  true,
		Filters: nostr.Filters{
			{
				Kinds: []int{nostr.KindGiftWrap},
			},
		},
	}

	pm.userDevices["recipient_pubkey"] = []DeviceID{"device1"}
	pm.filterToDevices[NotificationTypeDirectMessage]["device1"] = true

	notifications := pm.handleDirectMessageNotification(&event.Event)

	require.Empty(t, notifications)
}

func TestHandleDirectMessageNotificationMultipleDevices(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:         make(map[DeviceID]DeviceInfo),
		userDevices:     make(map[string][]DeviceID),
		filterToDevices: make(map[NotificationType]map[DeviceID]bool),
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
		FCMToken: "ios_token",
		PubKey:   "recipient_pubkey",
		Platform: "ios",
		Filters: nostr.Filters{
			{
				Kinds: []int{nostr.KindGiftWrap},
			},
		},
	}

	pm.devices[DeviceID("device2")] = DeviceInfo{
		DeviceID: "device2",
		FCMToken: "android_token",
		PubKey:   "recipient_pubkey",
		Platform: "android",
		Filters: nostr.Filters{
			{
				Kinds: []int{nostr.KindGiftWrap},
			},
		},
	}

	pm.devices[DeviceID("device3")] = DeviceInfo{
		DeviceID: "device3",
		FCMToken: "web_token",
		PubKey:   "recipient_pubkey",
		Platform: "web",
		Filters: nostr.Filters{
			{
				Kinds: []int{nostr.KindGiftWrap},
			},
		},
	}

	pm.userDevices["recipient_pubkey"] = []DeviceID{"device1", "device2", "device3"}
	pm.filterToDevices[NotificationTypeDirectMessage]["device1"] = true
	pm.filterToDevices[NotificationTypeDirectMessage]["device2"] = true
	pm.filterToDevices[NotificationTypeDirectMessage]["device3"] = true

	notifications := pm.handleDirectMessageNotification(&event.Event)

	require.NotNil(t, notifications)
	require.Len(t, notifications, 3, "Should create three notifications (one for each device)")

	tokenMap := make(map[string]bool)
	deviceIDMap := make(map[DeviceID]bool)

	for _, notification := range notifications {
		tokenMap[notification.Target.Token] = true
		deviceIDMap[notification.Target.DeviceID] = true
	}

	require.True(t, tokenMap["ios_token"], "Should have notification for iOS device")
	require.True(t, tokenMap["android_token"], "Should have notification for Android device")
	require.True(t, tokenMap["web_token"], "Should have notification for Web device")

	require.True(t, deviceIDMap[DeviceID("device1")], "Should have notification for device1")
	require.True(t, deviceIDMap[DeviceID("device2")], "Should have notification for device2")
	require.True(t, deviceIDMap[DeviceID("device3")], "Should have notification for device3")
}
