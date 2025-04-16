// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"testing"

	"github.com/ice-blockchain/subzero/model"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

type TestEvent struct {
	model.Event
}

func (e *TestEvent) GetMasterPublicKey() string {
	return e.PubKey
}

func helperCreateTestEvent(t *testing.T, id, pubKey string, kind int, content string, tags nostr.Tags) *TestEvent {
	t.Helper()

	return &TestEvent{
		Event: model.Event{
			Event: nostr.Event{
				ID:      id,
				PubKey:  pubKey,
				Kind:    kind,
				Content: content,
				Tags:    tags,
			},
		},
	}
}

func TestCollectValidDevices(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:         make(map[DeviceID]DeviceInfo),
		userDevices:     make(map[string][]DeviceID),
		filterToDevices: make(map[NotificationType]map[DeviceID]bool),
		invalidTokens:   make(map[DeviceID]InvalidTokenInfo),
	}

	pm.filterToDevices[NotificationTypePost] = make(map[DeviceID]bool)

	pm.devices[DeviceID("device1")] = DeviceInfo{
		DeviceID: "device1",
		FCMToken: "token1",
		PubKey:   "pubkey1",
		Filters: nostr.Filters{
			{
				Kinds: []int{nostr.KindTextNote},
			},
		},
	}

	pm.devices[DeviceID("device2")] = DeviceInfo{
		DeviceID: "device2",
		FCMToken: "token2",
		PubKey:   "pubkey1",
		Filters: nostr.Filters{
			{
				Kinds: []int{nostr.KindReaction},
			},
		},
	}

	pm.devices[DeviceID("device3")] = DeviceInfo{
		DeviceID: "device3",
		FCMToken: "token3",
		PubKey:   "pubkey1",
	}

	pm.userDevices["pubkey1"] = []DeviceID{"device1", "device2", "device3"}

	pm.filterToDevices[NotificationTypePost]["device1"] = true
	pm.filterToDevices[NotificationTypePost]["device3"] = true

	event := &model.Event{
		Event: nostr.Event{
			Kind: nostr.KindTextNote,
		},
	}

	validDevices := pm.collectValidDevices("pubkey1", NotificationTypePost, event)
	require.NotEmpty(t, validDevices, "There should be at least one valid device")
	foundDevice1 := false
	for _, device := range validDevices {
		if device.deviceID == "device1" {
			foundDevice1 = true
			require.Equal(t, "token1", device.token)
		}
	}
	require.True(t, foundDevice1, "device1 should be in the results")
}

func TestAddNotificationsToDevices(t *testing.T) {
	pm := &PushNotificationManager{
		devices:         make(map[DeviceID]DeviceInfo),
		userDevices:     make(map[string][]DeviceID),
		filterToDevices: make(map[NotificationType]map[DeviceID]bool),
		invalidTokens:   make(map[DeviceID]InvalidTokenInfo),
	}

	validDevices := []struct {
		deviceID DeviceID
		token    string
	}{
		{deviceID: "device1", token: "token1"},
		{deviceID: "device2", token: "token2"},
	}

	title := "Test Title"
	body := "Test Body"
	imageURL := "https://example.com/image.jpg"
	data := map[string]interface{}{"key": "value"}

	batch := pm.addNotificationsToDevices(validDevices, title, body, imageURL, data)

	require.NotNil(t, batch, "Result should not be nil")
	require.Len(t, batch.singleNotifications, 0, "There should be 0 single notifications")
	require.Len(t, batch.multicastNotifications, 1, "There should be 1 multicast notification")

	notification := batch.multicastNotifications[0]
	require.Equal(t, title, notification.Title)
	require.Equal(t, body, notification.Body)
	require.Equal(t, imageURL, notification.ImageURL)
	require.Equal(t, data, notification.Data)
	require.Len(t, notification.Target, 2, "There should be 2 tokens in Target")
}
