// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"fmt"
	"os"
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/ice-blockchain/subzero/model"
)

func TestMain(m *testing.M) {
	code := m.Run()
	if code == 0 {
		if err := goleak.Find(); err != nil {
			fmt.Printf("goleak found issues: %v\n", err)
			code = 1
		}
	}
	os.Exit(code)
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

type (
	TestEvent struct {
		model.Event
	}
)

func TestCollectValidDevices(t *testing.T) {
	t.Parallel()

	pm := &PushNotificationManager{
		devices:     make(map[DeviceID]DeviceInfo),
		userDevices: make(map[PublicKey][]DeviceID),
	}

	textFilters := nostr.Filters{
		{
			Kinds: []int{nostr.KindTextNote},
		},
	}

	diffFilters := nostr.Filters{
		{
			Kinds: []int{nostr.KindReaction},
		},
	}

	device1 := helperCreateTestDeviceRegistrationEvent(
		t,
		"pubkey1",
		"device1",
		[]string{"t", "ios", "token", "token1"},
		textFilters,
	)

	device2 := helperCreateTestDeviceRegistrationEvent(
		t,
		"pubkey1",
		"device2",
		[]string{"t", "android", "token", "token2"},
		diffFilters,
	)

	device3 := helperCreateTestDeviceRegistrationEvent(
		t,
		"pubkey1",
		"device3",
		[]string{"t", "web", "token", "token3"},
		textFilters,
	)

	device4 := helperCreateTestDeviceRegistrationEvent(
		t,
		"pubkey1",
		"device4",
		[]string{"t", "ios", "token", "invalid_token"},
		textFilters,
	)

	device5 := helperCreateTestDeviceRegistrationEvent(
		t,
		"pubkey1",
		"device5",
		[]string{"t", "ios", "token", "token5"},
		nostr.Filters{},
	)

	require.NoError(t, pm.processDeviceRegistrationEvent(device1))
	require.NoError(t, pm.processDeviceRegistrationEvent(device2))
	require.NoError(t, pm.processDeviceRegistrationEvent(device3))
	require.NoError(t, pm.processDeviceRegistrationEvent(device4))
	require.NoError(t, pm.processDeviceRegistrationEvent(device5))

	device4Info := pm.devices[DeviceID("device4")]
	device4Info.Event.NotificationTokenInvalid = true
	pm.devices[DeviceID("device4")] = device4Info

	event := &model.Event{
		Event: nostr.Event{
			Kind:    nostr.KindTextNote,
			Content: "Test message",
		},
	}

	validDevices := pm.collectUserValidDevices("pubkey1", event)

	require.Len(t, validDevices, 2, "There should be two valid devices")

	deviceIDs := make(map[string]bool)
	for _, device := range validDevices {
		deviceTag := device.GetTag("d")
		if deviceTag != nil {
			deviceIDs[deviceTag.Value()] = true
		}
	}

	require.True(t, deviceIDs["device1"], "device1 should be included")
	require.True(t, deviceIDs["device3"], "device3 should be included")
	require.False(t, deviceIDs["device5"], "device5 should not be included")
	require.False(t, deviceIDs["device2"], "device2 should not be included due to an incompatible filter")
	require.False(t, deviceIDs["device4"], "device4 should not be included due to an invalid token")

	validDevices = pm.collectUserValidDevices("nonexistent", event)
	require.Empty(t, validDevices, "For nonexistent user, there should be no devices")
}

func TestCreateNotifications(t *testing.T) {
	t.Parallel()

	pm := &PushNotificationManager{
		devices:     make(map[DeviceID]DeviceInfo),
		userDevices: make(map[PublicKey][]DeviceID),
	}

	filters := nostr.Filters{
		{
			Kinds: []int{nostr.KindTextNote},
		},
	}

	androidDevice := helperCreateTestDeviceRegistrationEvent(
		t,
		"android_pubkey",
		"android_device",
		[]string{"t", "android", "token", "android_token"},
		filters,
	)

	iosDevice := helperCreateTestDeviceRegistrationEvent(
		t,
		"ios_pubkey",
		"ios_device",
		[]string{"t", "ios", "token", "ios_token"},
		filters,
	)

	notificationType := NotificationTypePost
	data := map[string]interface{}{
		"eventID": "test_event_id",
		"content": "Hello, world!",
	}

	deviceEvents := []*model.Event{androidDevice, iosDevice}

	notifications := pm.createNotifications(deviceEvents, notificationType, data)

	require.Len(t, notifications, 2, "Should create 2 notifications")

	androidNotification := notifications[0]
	require.Equal(t, androidDevice, androidNotification.Target, "Android notification target should be correct")
	require.Contains(t, androidNotification.Data, "title", "Android notification should have title in data")
	require.Contains(t, androidNotification.Data, "body", "Android notification should have body in data")
	require.Contains(t, androidNotification.Data, "imageURL", "Android notification should have imageURL in data")
	require.Contains(t, androidNotification.Data, "eventID", "Android notification should preserve original data")
	require.Contains(t, androidNotification.Data, "notificationType", "Android notification should have notificationType")
	require.Equal(t, string(notificationType), androidNotification.Data["notificationType"], "Android notification should have correct notificationType")

	iosNotification := notifications[1]
	require.Equal(t, iosDevice, iosNotification.Target, "iOS notification target should be correct")
	require.Equal(t, DefaultTranslations[notificationType].Title, iosNotification.Title, "iOS notification should have correct title")
	require.Equal(t, DefaultTranslations[notificationType].Body, iosNotification.Body, "iOS notification should have correct body")
	require.Equal(t, DefaultTranslations[notificationType].ImageURL, iosNotification.ImageURL, "iOS notification should have correct imageURL")
	require.Contains(t, iosNotification.Data, "eventID", "iOS notification should preserve original data")
	require.Contains(t, iosNotification.Data, "notificationType", "iOS notification should have notificationType")
	require.Equal(t, string(notificationType), iosNotification.Data["notificationType"], "iOS notification should have correct notificationType")

	emptyNotifications := pm.createNotifications([]*model.Event{}, notificationType, data)
	require.Nil(t, emptyNotifications, "Should return nil for empty device list")
}

func TestProcessEvent(t *testing.T) {
	pm := &PushNotificationManager{
		devices:     make(map[DeviceID]DeviceInfo),
		userDevices: make(map[string][]DeviceID),
	}

	textEvent := helperCreateTestEvent(
		t,
		"text_event_id",
		"author_pubkey",
		nostr.KindTextNote,
		"Test Post",
		nostr.Tags{},
	)

	result := pm.processEvent(t.Context(), nostr.KindTextNote, &textEvent.Event)
	require.Empty(t, result, "For text message without subscribers, the result should be an empty array")

	communityEvent := helperCreateTestEvent(
		t,
		"community_event_id",
		"author_pubkey",
		nostr.KindTextNote,
		"Community Message",
		nostr.Tags{
			{"h", "community_id"},
		},
	)

	result = pm.processEvent(t.Context(), nostr.KindTextNote, &communityEvent.Event)
	require.Empty(t, result, "For text message without subscribers, the result should be an empty array")

	repostEvent := helperCreateTestEvent(
		t,
		"repost_event_id",
		"reposter_pubkey",
		nostr.KindRepost,
		"",
		nostr.Tags{
			{"e", "original_event_id"},
			{"p", "original_author_pubkey"},
		},
	)

	result = pm.processEvent(t.Context(), nostr.KindRepost, &repostEvent.Event)
	require.Empty(t, result, "For repost without subscribers, the result should be an empty array")

	systemEvent := helperCreateTestEvent(
		t,
		"system_event_id",
		"system_pubkey",
		model.CustomIONSystemMessage,
		"System Update",
		nostr.Tags{
			{"type", "announcement"},
		},
	)

	result = pm.processEvent(t.Context(), model.CustomIONSystemMessage, &systemEvent.Event)
	require.Empty(t, result, "For system message without devices, the result should be an empty array")
}

func TestHandleInvalidDeviceTokens(t *testing.T) {
	t.Parallel()

	pm := &PushNotificationManager{
		devices:     make(map[DeviceID]DeviceInfo),
		userDevices: make(map[PublicKey][]DeviceID),
	}

	filters := nostr.Filters{
		{
			Kinds: []int{nostr.KindTextNote},
		},
	}

	devicePubkey1 := "pubkey1"
	deviceID1 := "device1"
	deviceEvent1 := helperCreateTestDeviceRegistrationEvent(
		t,
		devicePubkey1,
		deviceID1,
		[]string{"t", "android", "token", "token1"},
		filters,
	)

	devicePubkey2 := "pubkey2"
	deviceID2 := "device2"
	deviceEvent2 := helperCreateTestDeviceRegistrationEvent(
		t,
		devicePubkey2,
		deviceID2,
		[]string{"t", "ios", "token", "token2"},
		filters,
	)

	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent1))
	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent2))

	require.False(t, pm.devices[DeviceID(deviceID1)].Event.NotificationTokenInvalid, "Device 1 should have valid token")
	require.False(t, pm.devices[DeviceID(deviceID2)].Event.NotificationTokenInvalid, "Device 2 should have valid token")

	invalidDevices := []*model.Event{deviceEvent1}
	require.NoError(t, pm.markDevicesAsInvalidInCache(invalidDevices))

	require.True(t, pm.devices[DeviceID(deviceID1)].Event.NotificationTokenInvalid, "Device 1 should have invalid token")
	require.False(t, pm.devices[DeviceID(deviceID2)].Event.NotificationTokenInvalid, "Device 2 should still have valid token")

	require.NoError(t, pm.handleInvalidDeviceTokens(t.Context(), []*model.Event{}))

	invalidDeviceEvent := &model.Event{
		Event: nostr.Event{
			ID:     "invalid_device",
			PubKey: "pubkey3",
			Tags:   nostr.Tags{},
		},
	}
	require.Error(t, pm.markDevicesAsInvalidInCache([]*model.Event{invalidDeviceEvent}), "Should return error for device without d tag")

	nonExistentDeviceEvent := helperCreateTestDeviceRegistrationEvent(
		t,
		"nonexistent_pubkey",
		"nonexistent_device",
		[]string{"t", "web"},
		filters,
	)
	require.NoError(t, pm.markDevicesAsInvalidInCache([]*model.Event{nonExistentDeviceEvent}), "Should not error for non-existent device")
}

func TestCollectUserValidDevices(t *testing.T) {
	t.Parallel()

	pm := &PushNotificationManager{
		devices:     make(map[DeviceID]DeviceInfo),
		userDevices: make(map[PublicKey][]DeviceID),
	}

	textFilters := nostr.Filters{
		{
			Kinds: []int{nostr.KindTextNote},
		},
	}

	reactionFilters := nostr.Filters{
		{
			Kinds: []int{nostr.KindReaction},
		},
	}

	userPubKey := "user_pubkey"

	device1 := helperCreateTestDeviceRegistrationEvent(
		t,
		userPubKey,
		"device1",
		[]string{"t", "android"},
		textFilters,
	)

	device2 := helperCreateTestDeviceRegistrationEvent(
		t,
		userPubKey,
		"device2",
		[]string{"t", "ios"},
		reactionFilters,
	)

	device3 := helperCreateTestDeviceRegistrationEvent(
		t,
		userPubKey,
		"device3",
		[]string{"t", "web", "invalid_token", "true"},
		nostr.Filters{
			{
				Kinds: []int{nostr.KindTextNote, nostr.KindReaction},
			},
		},
	)

	require.NoError(t, pm.processDeviceRegistrationEvent(device1))
	require.NoError(t, pm.processDeviceRegistrationEvent(device2))
	require.NoError(t, pm.processDeviceRegistrationEvent(device3))

	deviceInfo := pm.devices[DeviceID("device3")]
	deviceInfo.Event.NotificationTokenInvalid = true
	pm.devices[DeviceID("device3")] = deviceInfo

	require.Len(t, pm.devices, 3, "Should have 3 devices")

	textEvent := &model.Event{
		Event: nostr.Event{
			Kind: nostr.KindTextNote,
		},
	}

	reactionEvent := &model.Event{
		Event: nostr.Event{
			Kind: nostr.KindReaction,
		},
	}

	textDevices := pm.collectUserValidDevices(userPubKey, textEvent)
	require.Len(t, textDevices, 1, "Should collect 1 device for text note")
	require.Equal(t, device1.ID, textDevices[0].ID, "Should collect device1 for text note")

	reactionDevices := pm.collectUserValidDevices(userPubKey, reactionEvent)
	require.Len(t, reactionDevices, 1, "Should collect 1 device for reaction")
	require.Equal(t, device2.ID, reactionDevices[0].ID, "Should collect device2 for reaction")

	require.NotContains(t, textDevices, device3, "Should not collect device with invalid token")
	require.NotContains(t, reactionDevices, device3, "Should not collect device with invalid token")

	nonExistentDevices := pm.collectUserValidDevices("nonexistent_pubkey", textEvent)
	require.Empty(t, nonExistentDevices, "Should collect no devices for non-existent user")
}
