// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"encoding/json"
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
	}

	pm.filterToDevices[NotificationTypePaymentRequest] = make(map[DeviceID]bool)

	content := paymentRequestContent{
		Amount:    "100",
		AmountUSD: "100.00",
		AssetID:   "ice",
		From:      "sender_address",
		To:        "recipient_address",
	}

	contentBytes, err := json.Marshal(content)
	require.NoError(t, err)

	event := helperCreateTestEvent(
		t,
		"test_id",
		"sender_pubkey",
		model.CustomIONKindFundReceive,
		string(contentBytes),
		nostr.Tags{
			{"p", "recipient_pubkey"},
			{"network", "ion"},
			{"asset_class", "ice"},
			{"asset_address", "ice_address"},
		},
	)

	pm.devices[DeviceID("device1")] = DeviceInfo{
		DeviceID: "device1",
		FCMToken: "test_token",
		PubKey:   "recipient_pubkey",
		Filters: nostr.Filters{
			{
				Kinds: []int{model.CustomIONKindFundReceive},
			},
		},
	}

	pm.userDevices["recipient_pubkey"] = []DeviceID{"device1"}
	pm.filterToDevices[NotificationTypePaymentRequest]["device1"] = true

	notifications := pm.handlePaymentRequestNotification(&event.Event)

	require.NotNil(t, notifications)
	require.Len(t, notifications, 1, "Should create one notification")

	notification := notifications[0]
	require.Equal(t, "test_token", notification.Target.Token, "Token should match")
	require.Equal(t, DeviceID("device1"), notification.Target.DeviceID, "DeviceID should match")

	require.Equal(t, "test_id", notification.Data["eventId"])
	require.Equal(t, "sender_pubkey", notification.Data["authorPubKey"])
	require.Equal(t, string(NotificationTypePaymentRequest), notification.Data["notificationType"])
	require.Equal(t, "100", notification.Data["amount"])
	require.Equal(t, "100.00", notification.Data["amountUsd"])
	require.Equal(t, "ice", notification.Data["assetId"])
	require.Equal(t, "sender_address", notification.Data["from"])
	require.Equal(t, "recipient_address", notification.Data["to"])
	require.Equal(t, "ion", notification.Data["network"])
	require.Equal(t, "ice", notification.Data["assetClass"])
	require.Equal(t, "ice_address", notification.Data["assetAddress"])
}

func TestHandlePaymentReceivedNotification(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:         make(map[DeviceID]DeviceInfo),
		userDevices:     make(map[string][]DeviceID),
		filterToDevices: make(map[NotificationType]map[DeviceID]bool),
	}

	pm.filterToDevices[NotificationTypePaymentReceived] = make(map[DeviceID]bool)

	content := paymentReceivedContent{
		Amount:    "100",
		AmountUSD: "100.00",
		AssetID:   "ice",
		TxHash:    "tx_hash_123",
		TxURL:     "https://example.com/tx/tx_hash_123",
		From:      "sender_address",
		To:        "recipient_address",
	}

	contentBytes, err := json.Marshal(content)
	require.NoError(t, err)
	event := helperCreateTestEvent(
		t,
		"test_id",
		"sender_pubkey",
		model.CustomIONKindFundSendNotify,
		string(contentBytes),
		nostr.Tags{
			{"p", "recipient_pubkey"},
			{"network", "ion"},
			{"asset_class", "ice"},
			{"asset_address", "ice_address"},
			{"request", "request_event_id"},
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
	pm.filterToDevices[NotificationTypePaymentReceived]["device1"] = true

	notifications := pm.handlePaymentReceivedNotification(&event.Event)

	require.NotNil(t, notifications)
	require.Len(t, notifications, 1, "Should create one notification")

	notification := notifications[0]
	require.Equal(t, "test_token", notification.Target.Token, "Token should match")
	require.Equal(t, DeviceID("device1"), notification.Target.DeviceID, "DeviceID should match")

	require.Equal(t, "test_id", notification.Data["eventId"])
	require.Equal(t, "sender_pubkey", notification.Data["authorPubKey"])
	require.Equal(t, string(NotificationTypePaymentReceived), notification.Data["notificationType"])
	require.Equal(t, "100", notification.Data["amount"])
	require.Equal(t, "100.00", notification.Data["amountUsd"])
	require.Equal(t, "ice", notification.Data["assetId"])
	require.Equal(t, "sender_address", notification.Data["from"])
	require.Equal(t, "recipient_address", notification.Data["to"])
	require.Equal(t, "tx_hash_123", notification.Data["txHash"])
	require.Equal(t, "https://example.com/tx/tx_hash_123", notification.Data["txUrl"])
	require.Equal(t, "request_event_id", notification.Data["requestEvent"])
	require.Equal(t, "ion", notification.Data["network"])
	require.Equal(t, "ice", notification.Data["assetClass"])
	require.Equal(t, "ice_address", notification.Data["assetAddress"])
}

func TestExtractPaymentInfoFromEvent(t *testing.T) {
	t.Parallel()

	content := paymentRequestContent{
		Amount:    "100",
		AmountUSD: "100.00",
		AssetID:   "ice",
		From:      "sender_address",
		To:        "recipient_address",
	}

	contentBytes, err := json.Marshal(content)
	require.NoError(t, err)

	requestEvent := helperCreateTestEvent(
		t,
		"test_id",
		"sender_pubkey",
		model.CustomIONKindFundReceive,
		string(contentBytes),
		nostr.Tags{
			{"p", "recipient_pubkey"},
			{"network", "ion"},
			{"asset_class", "ice"},
			{"asset_address", "ice_address"},
		},
	)

	info, err := extractPaymentInfoFromEvent(&requestEvent.Event)
	require.NoError(t, err)
	require.NotNil(t, info)
	require.Equal(t, "test_id", info.EventID)
	require.Equal(t, model.CustomIONKindFundReceive, info.EventKind)
	require.Equal(t, "sender_pubkey", info.AuthorPubKey)
	require.Equal(t, "recipient_pubkey", info.RecipientKey)
	require.Equal(t, "ion", info.Network)
	require.Equal(t, "ice", info.AssetClass)
	require.Equal(t, "ice_address", info.AssetAddress)
	require.Equal(t, "100", info.Amount)
	require.Equal(t, "100.00", info.AmountUSD)
	require.Equal(t, "ice", info.AssetID)
	require.Equal(t, "sender_address", info.From)
	require.Equal(t, "recipient_address", info.To)
	require.Empty(t, info.TxHash)
	require.Empty(t, info.TxURL)

	receiveContent := paymentReceivedContent{
		Amount:    "100",
		AmountUSD: "100.00",
		AssetID:   "ice",
		TxHash:    "tx_hash_123",
		TxURL:     "https://example.com/tx/tx_hash_123",
		From:      "sender_address",
		To:        "recipient_address",
	}

	receiveContentBytes, _ := json.Marshal(receiveContent)

	receiveEvent := helperCreateTestEvent(
		t,
		"test_id",
		"sender_pubkey",
		model.CustomIONKindFundSendNotify,
		string(receiveContentBytes),
		nostr.Tags{
			{"p", "recipient_pubkey"},
			{"network", "ion"},
			{"asset_class", "ice"},
			{"asset_address", "ice_address"},
			{"request", "request_event_id"},
		},
	)

	info, err = extractPaymentInfoFromEvent(&receiveEvent.Event)
	require.NoError(t, err)
	require.NotNil(t, info)
	require.Equal(t, "test_id", info.EventID)
	require.Equal(t, model.CustomIONKindFundSendNotify, info.EventKind)
	require.Equal(t, "sender_pubkey", info.AuthorPubKey)
	require.Equal(t, "recipient_pubkey", info.RecipientKey)
	require.Equal(t, "ion", info.Network)
	require.Equal(t, "ice", info.AssetClass)
	require.Equal(t, "ice_address", info.AssetAddress)
	require.Equal(t, "100", info.Amount)
	require.Equal(t, "100.00", info.AmountUSD)
	require.Equal(t, "ice", info.AssetID)
	require.Equal(t, "sender_address", info.From)
	require.Equal(t, "recipient_address", info.To)
	require.Equal(t, "tx_hash_123", info.TxHash)
	require.Equal(t, "https://example.com/tx/tx_hash_123", info.TxURL)
	require.Equal(t, "request_event_id", info.RequestEvent)
}

func TestPopulateNotificationDataFromPaymentInfo(t *testing.T) {
	t.Parallel()

	paymentRequestInfo := &paymentInfo{
		EventID:      "test_id",
		EventKind:    model.CustomIONKindFundReceive,
		AuthorPubKey: "sender_pubkey",
		RecipientKey: "recipient_pubkey",
		Network:      "ion",
		AssetClass:   "ice",
		AssetAddress: "ice_address",
		Amount:       "100",
		AmountUSD:    "100.00",
		AssetID:      "ice",
		From:         "sender_address",
		To:           "recipient_address",
	}

	data := populateNotificationDataFromPaymentInfo(paymentRequestInfo, NotificationTypePaymentRequest)
	require.NotNil(t, data)
	require.Equal(t, "test_id", data["eventId"])
	require.Equal(t, "sender_pubkey", data["authorPubKey"])
	require.Equal(t, string(NotificationTypePaymentRequest), data["notificationType"])
	require.Equal(t, "100", data["amount"])
	require.Equal(t, "100.00", data["amountUsd"])
	require.Equal(t, "ice", data["assetId"])
	require.Equal(t, "sender_address", data["from"])
	require.Equal(t, "recipient_address", data["to"])
	require.Equal(t, "ion", data["network"])
	require.Equal(t, "ice", data["assetClass"])
	require.Equal(t, "ice_address", data["assetAddress"])
	require.NotContains(t, data, "txHash")
	require.NotContains(t, data, "txUrl")
	require.NotContains(t, data, "requestEvent")

	paymentReceivedInfo := &paymentInfo{
		EventID:      "test_id",
		EventKind:    model.CustomIONKindFundSendNotify,
		AuthorPubKey: "sender_pubkey",
		RecipientKey: "recipient_pubkey",
		Network:      "ion",
		AssetClass:   "ice",
		AssetAddress: "ice_address",
		Amount:       "100",
		AmountUSD:    "100.00",
		AssetID:      "ice",
		From:         "sender_address",
		To:           "recipient_address",
		TxHash:       "tx_hash_123",
		TxURL:        "https://example.com/tx/tx_hash_123",
		RequestEvent: "request_event_id",
	}

	data = populateNotificationDataFromPaymentInfo(paymentReceivedInfo, NotificationTypePaymentReceived)
	require.NotNil(t, data)
	require.Equal(t, "test_id", data["eventId"])
	require.Equal(t, "sender_pubkey", data["authorPubKey"])
	require.Equal(t, string(NotificationTypePaymentReceived), data["notificationType"])
	require.Equal(t, "100", data["amount"])
	require.Equal(t, "100.00", data["amountUsd"])
	require.Equal(t, "ice", data["assetId"])
	require.Equal(t, "sender_address", data["from"])
	require.Equal(t, "recipient_address", data["to"])
	require.Equal(t, "ion", data["network"])
	require.Equal(t, "ice", data["assetClass"])
	require.Equal(t, "ice_address", data["assetAddress"])
	require.Equal(t, "tx_hash_123", data["txHash"])
	require.Equal(t, "https://example.com/tx/tx_hash_123", data["txUrl"])
	require.Equal(t, "request_event_id", data["requestEvent"])

	require.Nil(t, populateNotificationDataFromPaymentInfo(nil, NotificationTypePaymentReceived))
}
