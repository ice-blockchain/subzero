// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"context"
	"testing"

	"github.com/ice-blockchain/subzero/model"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
)

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

type (
	MockClient struct {
		mock.Mock
	}
	TestEvent struct {
		model.Event
	}
)

func (m *MockClient) SendSingle(ctx context.Context, notification *pn.Notification[pn.DeviceToken]) error {
	args := m.Called(ctx, notification)
	return args.Error(0)
}

func (m *MockClient) SendTopic(ctx context.Context, notification *pn.Notification[pn.SubscriptionTopic]) error {
	args := m.Called(ctx, notification)
	return args.Error(0)
}

func TestCollectValidDevices(t *testing.T) {
	pm := &PushNotificationManager{
		devices:         make(map[DeviceID]DeviceInfo),
		userDevices:     make(map[string][]DeviceID),
		filterToDevices: make(map[NotificationType]map[DeviceID]bool),
	}

	pm.filterToDevices[NotificationTypePost] = make(map[DeviceID]bool)

	pm.devices[DeviceID("device1")] = DeviceInfo{
		DeviceID: "device1",
		FCMToken: "token1",
		PubKey:   "pubkey1",
		Platform: "ios",
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
		Platform: "android",
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
		Platform: "android",
		Filters: nostr.Filters{
			{
				Kinds: []int{nostr.KindTextNote},
			},
		},
	}

	pm.devices[DeviceID("device4")] = DeviceInfo{
		DeviceID: "device4",
		FCMToken: "token4",
		PubKey:   "pubkey1",
		Platform: "ios",
		Invalid:  true,
		Filters: nostr.Filters{
			{
				Kinds: []int{nostr.KindTextNote},
			},
		},
	}

	pm.devices[DeviceID("device5")] = DeviceInfo{
		DeviceID: "device5",
		FCMToken: "token5",
		PubKey:   "pubkey1",
		Platform: "web",
		Filters: nostr.Filters{
			{
				Kinds: []int{nostr.KindTextNote},
			},
		},
	}

	pm.userDevices["pubkey1"] = []DeviceID{"device1", "device2", "device3", "device4", "device5"}

	pm.filterToDevices[NotificationTypePost]["device1"] = true
	pm.filterToDevices[NotificationTypePost]["device2"] = true
	pm.filterToDevices[NotificationTypePost]["device3"] = true
	pm.filterToDevices[NotificationTypePost]["device4"] = true
	pm.filterToDevices[NotificationTypePost]["device5"] = true

	event := &model.Event{
		Event: nostr.Event{
			Kind: nostr.KindTextNote,
		},
	}

	iosDevices, otherDevices := pm.collectValidDevices("pubkey1", NotificationTypePost, event)

	require.Len(t, iosDevices, 1, "There should be one iOS device")
	require.Equal(t, DeviceID("device1"), iosDevices[0].deviceID)
	require.Equal(t, "token1", iosDevices[0].token)

	require.Len(t, otherDevices, 2, "There should be two non-iOS devices (Android and Web)")

	var foundAndroid, foundWeb bool
	for _, device := range otherDevices {
		if device.deviceID == "device3" {
			foundAndroid = true
			require.Equal(t, "token3", device.token)
		} else if device.deviceID == "device5" {
			foundWeb = true
			require.Equal(t, "token5", device.token)
		}
	}
	require.True(t, foundAndroid, "Android device should be included")
	require.True(t, foundWeb, "Web device should be included")

	iosDevices, otherDevices = pm.collectValidDevices("nonexistent", NotificationTypePost, event)
	require.Empty(t, iosDevices, "There should be no iOS devices for non-existent user")
	require.Empty(t, otherDevices, "There should be no Android/Web devices for non-existent user")
}

func TestAddNotificationsToDevices(t *testing.T) {
	pm := &PushNotificationManager{
		devices:         make(map[DeviceID]DeviceInfo),
		userDevices:     make(map[string][]DeviceID),
		filterToDevices: make(map[NotificationType]map[DeviceID]bool),
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
	require.Len(t, batch, 2, "There should be 2 notifications")

	notification1 := batch[0]
	require.Equal(t, title, notification1.Title)
	require.Equal(t, body, notification1.Body)
	require.Equal(t, imageURL, notification1.ImageURL)
	require.Equal(t, data, notification1.Data)
	require.Equal(t, validDevices[0].token, notification1.Target.Token)
	require.Equal(t, validDevices[0].deviceID, notification1.Target.DeviceID)

	notification2 := batch[1]
	require.Equal(t, title, notification2.Title)
	require.Equal(t, body, notification2.Body)
	require.Equal(t, imageURL, notification2.ImageURL)
	require.Equal(t, data, notification2.Data)
	require.Equal(t, validDevices[1].token, notification2.Target.Token)
	require.Equal(t, validDevices[1].deviceID, notification2.Target.DeviceID)

	batch = pm.addNotificationsToDevices([]struct {
		deviceID DeviceID
		token    string
	}{}, title, body, imageURL, data)
	require.Nil(t, batch, "Result should be nil with empty devices list")
}

func TestCreateAndSendNotifications(t *testing.T) {
	pm := &PushNotificationManager{
		devices:         make(map[DeviceID]DeviceInfo),
		userDevices:     make(map[string][]DeviceID),
		filterToDevices: make(map[NotificationType]map[DeviceID]bool),
	}

	iosDevices := []struct {
		deviceID DeviceID
		token    string
	}{
		{deviceID: "ios1", token: "token_ios1"},
		{deviceID: "ios2", token: "token_ios2"},
	}

	otherDevices := []struct {
		deviceID DeviceID
		token    string
	}{
		{deviceID: "android1", token: "token_android1"},
		{deviceID: "android2", token: "token_android2"},
	}

	notificationType := NotificationTypePost
	data := map[string]interface{}{"event_id": "12345"}

	notifications := pm.createAndSendNotifications(iosDevices, otherDevices, notificationType, data)

	require.Len(t, notifications, 4, "There should be 4 notifications")
}
