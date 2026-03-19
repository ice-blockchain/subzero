// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"encoding/json"
	"testing"

	"github.com/ice-blockchain/subzero/model"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

func TestKindKindFundSendNotify(t *testing.T) {
	now := nostr.Now()
	masterKey := "recipient_master_pubkey"
	deviceID := "device1"
	devicePubKey := "device_pubkey"
	type fundSendContent struct {
		Amount    string `json:"amount"`
		AmountUSD string `json:"amount_usd"`
		AssetID   string `json:"asset_id,omitempty"`
		TxHash    string `json:"tx_hash"`
		TxURL     string `json:"tx_url"`
		From      string `json:"from"`
		To        string `json:"to"`
	}
	contentData := &fundSendContent{
		Amount:    "1",
		AmountUSD: "1",
		TxHash:    "0x12345",
		TxURL:     "https://bogus.com/tx/0x12345",
		From:      "0x54321",
		To:        "0x54321",
	}
	content, err := json.Marshal(contentData)
	require.NoError(t, err)
	event := &model.Event{
		Event: nostr.Event{
			CreatedAt: now,
			Kind:      model.CustomIONKindFundSendNotify,
			Content:   string(content),
			Tags: nostr.Tags{
				{"network", "BscTestnet"},
				{"asset_class", "Erc20"},
				{"asset_address", "0xe1ab61f7b093435204df32f5b3a405de55445ea8"},
				{"p", masterKey},
			},
		},
	}
	pm := helperNewManager(t)
	filters := model.Filters{
		{
			Kinds: []int{nostr.KindDirectMessage, model.CustomIONKindDirectMessage, model.CustomIONKindFundReceive,
				model.CustomIONKindFundSendNotify, nostr.KindReaction},
		},
	}

	deviceTags := model.Tags{
		{"b", masterKey},
		{"t", "ios"},
		{"d", deviceID},
		{"relay", pm.relayURL},
		{"token", "token1"},
	}
	deviceEvent1 := helperCreateTestDeviceRegistrationEvent(t, devicePubKey, deviceID, deviceTags, filters)
	require.NoError(t, pm.processDeviceRegistrationEvent(t.Context(), deviceEvent1))

	notif, err := pm.handleAnonymousFundSendEvent(event)
	require.NoError(t, err)
	require.Len(t, notif, 1)
	notification := notif[0]
	require.Equal(t, defaultTranslations[NotificationAnonymousTypePaymentReceived].Title, notification.Title, "Title should match for NotificationAnonymousTypePaymentReceived")
	require.Equal(t, defaultTranslations[NotificationAnonymousTypePaymentReceived].Body, notification.Body, "Body should match for NotificationAnonymousTypePaymentReceived")
}
