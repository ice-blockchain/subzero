// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
)

func TestProcessDeviceRegistrationEvent(t *testing.T) {
	pm := &PushNotificationManager{
		devices:         make(map[pn.DeviceID]DeviceInfo),
		userDevices:     make(map[string][]pn.DeviceID),
		filterToDevices: make(map[NotificationType]map[pn.DeviceID]bool),
	}

	for _, notificationType := range []NotificationType{
		NotificationTypePost,
		NotificationTypeChannelMessage,
		NotificationTypeReaction,
		NotificationTypeRepost,
		NotificationTypeDirectMessage,
		NotificationTypePaymentRequest,
		NotificationTypePaymentReceived,
	} {
		pm.filterToDevices[notificationType] = make(map[pn.DeviceID]bool)
	}
	filters := nostr.Filters{
		{
			Kinds: []int{nostr.KindTextNote, model.CustomIONKindEditableTextNote},
		},
		{
			Kinds: []int{nostr.KindReaction},
		},
	}

	filtersJSON, err := json.Marshal(filters)
	require.NoError(t, err)
	event := &model.Event{
		Event: nostr.Event{
			ID:      "test_id",
			PubKey:  "test_pubkey",
			Kind:    model.CustomIONKindDeviceRegistration,
			Content: string(filtersJSON),
			Tags: nostr.Tags{
				{"d", "device1"},
				{"t", "android"},
				{"relay", "wss://relay.example.com"},
				{"token", "encrypted_token"},
			},
		},
	}

	require.NoError(t, pm.processDeviceRegistrationEvent(event))

	deviceInfo, exists := pm.devices["device1"]
	require.True(t, exists, "Device should be added to devices map")
	require.Equal(t, DeviceID("device1"), deviceInfo.DeviceID, "DeviceID should be equal to device1")
	require.Equal(t, "android", deviceInfo.Platform, "Platform should be equal to android")
	require.Equal(t, "wss://relay.example.com", deviceInfo.RelayURL, "RelayURL should be equal to wss://relay.example.com")
	require.Equal(t, "encrypted_token", deviceInfo.FCMToken, "FCMToken should be equal to encrypted_token")
	require.Equal(t, "test_pubkey", deviceInfo.PubKey, "PubKey should be equal to test_pubkey")
	require.Equal(t, "test_id", deviceInfo.DeviceRegistrationEventID, "DeviceRegistrationEventID should be equal to test_id")

	devices, exists := pm.userDevices["test_pubkey"]
	require.True(t, exists, "User should be added to userDevices map")
	require.Contains(t, devices, DeviceID("device1"), "Device should be added to user's devices list")

	require.True(t, pm.filterToDevices[NotificationTypePost]["device1"], "Device should be added to NotificationTypePost category")
	require.True(t, pm.filterToDevices[NotificationTypeReaction]["device1"], "Device should be added to NotificationTypeReaction category")
	require.False(t, pm.filterToDevices[NotificationTypeChannelMessage]["device1"], "Device should not be added to NotificationTypeChannelMessage category")
}

func TestCategorizeDeviceByFilters(t *testing.T) {
	pm := &PushNotificationManager{
		devices:         make(map[pn.DeviceID]DeviceInfo),
		userDevices:     make(map[string][]pn.DeviceID),
		filterToDevices: make(map[NotificationType]map[pn.DeviceID]bool),
	}

	for _, notificationType := range []NotificationType{
		NotificationTypePost,
		NotificationTypeChannelMessage,
		NotificationTypeReaction,
		NotificationTypeRepost,
		NotificationTypeDirectMessage,
		NotificationTypePaymentRequest,
		NotificationTypePaymentReceived,
	} {
		pm.filterToDevices[notificationType] = make(map[pn.DeviceID]bool)
	}

	pm.filterToDevices[NotificationTypePost]["device1"] = true
	pm.filterToDevices[NotificationTypeReaction]["device1"] = true
	filters := nostr.Filters{
		{
			Kinds: []int{nostr.KindChannelMessage},
		},
		{
			Kinds: []int{nostr.KindRepost},
		},
	}

	pm.categorizeDeviceByFilters("device1", filters)

	require.False(t, pm.filterToDevices[NotificationTypePost]["device1"], "Device should be removed from NotificationTypePost category")
	require.False(t, pm.filterToDevices[NotificationTypeReaction]["device1"], "Device should be removed from NotificationTypeReaction category")
	require.True(t, pm.filterToDevices[NotificationTypeChannelMessage]["device1"], "Device should be added to NotificationTypeChannelMessage category")
	require.True(t, pm.filterToDevices[NotificationTypeRepost]["device1"], "Device should be added to NotificationTypeRepost category")
}

func TestFullSyncDevices(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:         make(map[pn.DeviceID]DeviceInfo),
		userDevices:     make(map[string][]pn.DeviceID),
		filterToDevices: make(map[NotificationType]map[pn.DeviceID]bool),
	}

	for _, notificationType := range []NotificationType{
		NotificationTypePost,
		NotificationTypeChannelMessage,
		NotificationTypeReaction,
		NotificationTypeRepost,
		NotificationTypeDirectMessage,
		NotificationTypePaymentRequest,
		NotificationTypePaymentReceived,
	} {
		pm.filterToDevices[notificationType] = make(map[pn.DeviceID]bool)
	}

	pm.devices["device1"] = DeviceInfo{
		DeviceID: "device1",
		PubKey:   "pubkey1",
	}
	pm.userDevices["pubkey1"] = []pn.DeviceID{"device1"}
	pm.filterToDevices[NotificationTypePost]["device1"] = true

	deviceInfo, exists := pm.devices["device1"]
	require.True(t, exists, "Device should be in devices")
	require.Equal(t, DeviceID("device1"), deviceInfo.DeviceID)
	require.Equal(t, "pubkey1", deviceInfo.PubKey)

	devices, exists := pm.userDevices["pubkey1"]
	require.True(t, exists, "User should be in userDevices")
	require.Contains(t, devices, DeviceID("device1"))

	require.True(t, pm.filterToDevices[NotificationTypePost]["device1"], "Device should be in NotificationTypePost category")
}

func TestRemoveDevice(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:         make(map[DeviceID]DeviceInfo),
		userDevices:     make(map[PublicKey][]DeviceID),
		filterToDevices: make(map[NotificationType]map[DeviceID]bool),
	}

	pm.filterToDevices[NotificationTypePost] = make(map[DeviceID]bool)
	pm.filterToDevices[NotificationTypeReaction] = make(map[DeviceID]bool)

	deviceID := DeviceID("device-id-1")
	deviceID2 := DeviceID("device-id-2")
	masterPubKey := "master-pub-key-1"

	pm.devices[deviceID] = DeviceInfo{
		DeviceID: deviceID,
		PubKey:   masterPubKey,
	}
	pm.devices[deviceID2] = DeviceInfo{
		DeviceID: deviceID2,
		PubKey:   masterPubKey,
	}
	pm.userDevices[masterPubKey] = []DeviceID{deviceID, deviceID2}
	pm.filterToDevices[NotificationTypePost][deviceID] = true
	pm.filterToDevices[NotificationTypeReaction][deviceID] = true
	pm.filterToDevices[NotificationTypePost][deviceID2] = true

	require.NoError(t, pm.RemoveDevice(t.Context(), deviceID, masterPubKey))

	_, exists := pm.devices[deviceID]
	require.False(t, exists, "Device should be removed from devices")

	devices, ok := pm.userDevices[masterPubKey]
	require.True(t, ok, "User should remain in the list")
	require.Equal(t, []DeviceID{deviceID2}, devices, "List of user's devices should contain only device2")

	require.False(t, pm.filterToDevices[NotificationTypePost][deviceID], "Device should be removed from NotificationTypePost category")
	require.False(t, pm.filterToDevices[NotificationTypeReaction][deviceID], "Device should be removed from NotificationTypeReaction category")

	require.NoError(t, pm.RemoveDevice(t.Context(), DeviceID("non-existent"), masterPubKey))

	wrongPubKey := "wrong-pub-key"
	require.Error(t, pm.RemoveDevice(t.Context(), deviceID2, wrongPubKey))
}

func TestProcessDeviceRegistrationEvents(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:         make(map[DeviceID]DeviceInfo),
		userDevices:     make(map[PublicKey][]DeviceID),
		filterToDevices: make(map[NotificationType]map[DeviceID]bool),
	}

	pm.filterToDevices[NotificationTypePost] = make(map[DeviceID]bool)
	pm.filterToDevices[NotificationTypeDirectMessage] = make(map[DeviceID]bool)
	pm.filterToDevices[NotificationTypeReaction] = make(map[DeviceID]bool)
	pm.filterToDevices[NotificationTypeReply] = make(map[DeviceID]bool)
	pm.filterToDevices[NotificationTypeMention] = make(map[DeviceID]bool)

	deviceID1 := DeviceID("device-1")
	deviceID2 := DeviceID("device-2")
	deviceID3 := DeviceID("device-3")
	deviceID4 := DeviceID("device-4")
	masterPubKey1 := "pub-key-1"
	masterPubKey2 := "pub-key-2"
	masterPubKey3 := "pub-key-3"

	pm.devices[deviceID1] = DeviceInfo{
		DeviceID: deviceID1,
		Platform: "android",
		FCMToken: "token-1",
		PubKey:   masterPubKey1,
	}
	pm.devices[deviceID2] = DeviceInfo{
		DeviceID: deviceID2,
		Platform: "ios",
		FCMToken: "token-2",
		PubKey:   masterPubKey2,
	}
	pm.devices[deviceID3] = DeviceInfo{
		DeviceID: deviceID3,
		Platform: "android",
		FCMToken: "token-3",
		PubKey:   masterPubKey1,
	}

	pm.userDevices[masterPubKey1] = []DeviceID{deviceID1, deviceID3}
	pm.userDevices[masterPubKey2] = []DeviceID{deviceID2}

	pm.filterToDevices[NotificationTypePost][deviceID1] = true
	pm.filterToDevices[NotificationTypeDirectMessage][deviceID2] = true
	pm.filterToDevices[NotificationTypePost][deviceID3] = true

	// Update device
	filters1 := nostr.Filters{
		{
			Kinds: []int{nostr.KindReaction},
		},
	}
	filtersJSON1, err := json.Marshal(filters1)
	require.NoError(t, err)

	updateEvent := &model.Event{
		Event: nostr.Event{
			ID:        "update-event",
			Kind:      model.CustomIONKindDeviceRegistration,
			PubKey:    masterPubKey1,
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Content:   string(filtersJSON1),
			Tags: []nostr.Tag{
				{"d", string(deviceID1)},
				{"t", "android-updated"},
				{"relay", "wss://relay.updated.com"},
				{"token", "token-1-updated"},
			},
		},
	}

	// Register new device
	filters2 := nostr.Filters{
		{
			Kinds: []int{nostr.KindTextNote},
		},
	}
	filtersJSON2, err := json.Marshal(filters2)
	require.NoError(t, err)

	newDeviceEvent := &model.Event{
		Event: nostr.Event{
			ID:        "new-device-event",
			Kind:      model.CustomIONKindDeviceRegistration,
			PubKey:    masterPubKey3,
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Content:   string(filtersJSON2),
			Tags: []nostr.Tag{
				{"d", string(deviceID4)},
				{"t", "web"},
				{"relay", "wss://relay.new.com"},
				{"token", "token-4"},
			},
		},
	}

	// Invalid event (missing deviceID)
	invalidEvent := &model.Event{
		Event: nostr.Event{
			ID:        "invalid-event",
			Kind:      model.CustomIONKindDeviceRegistration,
			PubKey:    masterPubKey1,
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Content:   "{}",
			Tags: []nostr.Tag{
				{"t", "invalid"},
				{"relay", "wss://relay.invalid.com"},
				{"token", "token-invalid"},
			},
		},
	}

	// Invalid event (invalid token)
	filters4 := nostr.Filters{
		{
			Kinds: []int{nostr.KindTextNote},
		},
	}
	filtersJSON4, err := json.Marshal(filters4)
	require.NoError(t, err)

	invalidTokenEvent := &model.Event{
		Event: nostr.Event{
			ID:        "invalid-token-event",
			Kind:      model.CustomIONKindDeviceRegistration,
			PubKey:    masterPubKey2,
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Content:   string(filtersJSON4),
			Tags: []nostr.Tag{
				{"d", "device-invalid-token"},
				{"t", "ios"},
				{"relay", "wss://relay.ios.com"},
				{"token", "invalid-fcm-token", "invalid"},
			},
		},
	}

	events := []*model.Event{updateEvent, newDeviceEvent, invalidEvent, invalidTokenEvent}
	err = pm.ProcessDeviceRegistrationEvents(t.Context(), events)
	require.NoError(t, err)

	// Updated device
	updatedDevice, exists := pm.devices[deviceID1]
	require.True(t, exists, "Updated device should exist")
	require.Equal(t, "android-updated", updatedDevice.Platform, "Platform should be updated")
	require.Equal(t, "wss://relay.updated.com", updatedDevice.RelayURL, "Relay URL should be updated")
	require.Equal(t, "token-1-updated", updatedDevice.FCMToken, "Token should be updated")
	require.False(t, pm.filterToDevices[NotificationTypePost][deviceID1], "Old notification type should be removed")
	require.True(t, pm.filterToDevices[NotificationTypeReaction][deviceID1], "New notification type should be added")

	// New device
	newDevice, exists := pm.devices[deviceID4]
	require.True(t, exists, "New device should be added")
	require.Equal(t, DeviceID("device-4"), newDevice.DeviceID)
	require.Equal(t, "web", newDevice.Platform)
	require.Equal(t, "token-4", newDevice.FCMToken)
	require.Equal(t, masterPubKey3, newDevice.PubKey)

	newUserDevices, exists := pm.userDevices[masterPubKey3]
	require.True(t, exists, "New user should be added")
	require.Contains(t, newUserDevices, deviceID4)

	require.True(t, pm.filterToDevices[NotificationTypePost][deviceID4], "Post notification should be enabled for new device")

	// Invalid event should not register device
	_, exists = pm.devices[""]
	require.False(t, exists, "Invalid device should not be registered")

	// Device with invalid token should be registered
	invalidTokenDevice, exists := pm.devices["device-invalid-token"]
	require.True(t, exists, "Device with invalid token should be registered")
	require.True(t, invalidTokenDevice.Invalid, "Device should be marked as having invalid token")
	require.Equal(t, "invalid-fcm-token", invalidTokenDevice.FCMToken)
}

func TestShouldProcessDeletionEvent(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{}

	nonDeletionEvent := &model.Event{
		Event: nostr.Event{
			ID:        "non-deletion",
			Kind:      nostr.KindTextNote,
			PubKey:    "test-pub-key",
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
		},
	}
	require.False(t, pm.shouldProcessDeletionEvent(nonDeletionEvent), "Non-deletion events should not be processed")

	deletionWithoutKTag := &model.Event{
		Event: nostr.Event{
			ID:        "deletion-no-k-tag",
			Kind:      nostr.KindDeletion,
			PubKey:    "test-pub-key",
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Tags: []nostr.Tag{
				{"e", "event-1"},
			},
		},
	}
	require.True(t, pm.shouldProcessDeletionEvent(deletionWithoutKTag), "Deletion event without k tag should be processed")

	deletionWithValidKTag := &model.Event{
		Event: nostr.Event{
			ID:        "deletion-valid-k-tag",
			Kind:      nostr.KindDeletion,
			PubKey:    "test-pub-key",
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Tags: []nostr.Tag{
				{"e", "event-2"},
				{"k", strconv.Itoa(model.CustomIONKindDeviceRegistration)},
			},
		},
	}
	require.True(t, pm.shouldProcessDeletionEvent(deletionWithValidKTag), "Deletion event with k=CustomIONKindDeviceRegistration tag should be processed")

	deletionWithInvalidKTag := &model.Event{
		Event: nostr.Event{
			ID:        "deletion-invalid-k-tag",
			Kind:      nostr.KindDeletion,
			PubKey:    "test-pub-key",
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Tags: []nostr.Tag{
				{"e", "event-3"},
				{"k", "1000"},
			},
		},
	}
	require.False(t, pm.shouldProcessDeletionEvent(deletionWithInvalidKTag), "Deletion event with invalid k tag should not be processed")

	deletionWithMultipleKTags := &model.Event{
		Event: nostr.Event{
			ID:        "deletion-multiple-k-tags",
			Kind:      nostr.KindDeletion,
			PubKey:    "test-pub-key",
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Tags: []nostr.Tag{
				{"e", "event-4"},
				{"k", "1000"},
				{"k", strconv.Itoa(model.CustomIONKindDeviceRegistration)},
				{"k", "2000"},
			},
		},
	}
	require.True(t, pm.shouldProcessDeletionEvent(deletionWithMultipleKTags), "Deletion event with multiple k tags, including CustomIONKindDeviceRegistration, should be processed")

	deletionWithAllInvalidKTags := &model.Event{
		Event: nostr.Event{
			ID:        "deletion-all-invalid-k-tags",
			Kind:      nostr.KindDeletion,
			PubKey:    "test-pub-key",
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Tags: []nostr.Tag{
				{"e", "event-5"},
				{"k", "1000"},
				{"k", "2000"},
				{"k", "3000"},
			},
		},
	}
	require.False(t, pm.shouldProcessDeletionEvent(deletionWithAllInvalidKTags), "Deletion event with all invalid k tags should not be processed")
}

func TestProcessDeviceRegistrationEventWithInvalidToken(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:         make(map[pn.DeviceID]DeviceInfo),
		userDevices:     make(map[string][]pn.DeviceID),
		filterToDevices: make(map[NotificationType]map[pn.DeviceID]bool),
	}

	for _, notificationType := range []NotificationType{
		NotificationTypePost,
		NotificationTypeChannelMessage,
		NotificationTypeReaction,
		NotificationTypeRepost,
		NotificationTypeDirectMessage,
		NotificationTypePaymentRequest,
		NotificationTypePaymentReceived,
		NotificationTypeSystem,
		NotificationTypeMention,
		NotificationTypeReply,
	} {
		pm.filterToDevices[notificationType] = make(map[pn.DeviceID]bool)
	}

	filters := nostr.Filters{
		{
			Kinds: []int{nostr.KindTextNote},
		},
	}

	filtersJSON, err := json.Marshal(filters)
	require.NoError(t, err)
	event := &model.Event{
		Event: nostr.Event{
			ID:      "test_id_invalid",
			PubKey:  "test_pubkey_invalid",
			Kind:    model.CustomIONKindDeviceRegistration,
			Content: string(filtersJSON),
			Tags: nostr.Tags{
				{"d", "device_invalid"},
				{"t", "android"},
				{"relay", "wss://relay.example.com"},
				{"token", "invalid_token", "invalid"},
			},
		},
	}

	require.NoError(t, pm.processDeviceRegistrationEvent(event))

	deviceInfo, exists := pm.devices["device_invalid"]
	require.True(t, exists, "Device should be added to devices map even with invalid token")
	require.Equal(t, DeviceID("device_invalid"), deviceInfo.DeviceID)
	require.Equal(t, "invalid_token", deviceInfo.FCMToken)
	require.True(t, deviceInfo.Invalid, "Device should be marked as having an invalid token")
	require.Equal(t, "test_id_invalid", deviceInfo.DeviceRegistrationEventID, "DeviceRegistrationEventID should be equal to test_id_invalid")

	devices, exists := pm.userDevices["test_pubkey_invalid"]
	require.True(t, exists, "User should be added to userDevices map")
	require.Contains(t, devices, DeviceID("device_invalid"), "Device with invalid token should be in user's devices list")
}

func TestDeviceSubscribesToAllNotificationTypes(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:         make(map[pn.DeviceID]DeviceInfo),
		userDevices:     make(map[string][]pn.DeviceID),
		filterToDevices: make(map[NotificationType]map[pn.DeviceID]bool),
	}
	for _, notificationType := range []NotificationType{
		NotificationTypePost,
		NotificationTypeChannelMessage,
		NotificationTypeReaction,
		NotificationTypeRepost,
		NotificationTypeDirectMessage,
		NotificationTypePaymentRequest,
		NotificationTypePaymentReceived,
		NotificationTypeSystem,
		NotificationTypeMention,
		NotificationTypeReply,
	} {
		pm.filterToDevices[notificationType] = make(map[pn.DeviceID]bool)
	}

	filters := nostr.Filters{
		{
			Kinds: []int{nostr.KindTextNote, model.CustomIONKindEditableTextNote},
		},
		{
			Kinds: []int{nostr.KindTextNote},
			Tags:  make(nostr.TagMap).SetLiterals("p", "some-pubkey"),
		},
		{
			Kinds: []int{nostr.KindTextNote},
			Tags:  make(nostr.TagMap).SetLiterals("e", "some-event-id").SetLiterals("a", "some-address"),
		},
		{
			Kinds: []int{model.CustomIONKindEditableTextNote},
			Tags:  make(nostr.TagMap).SetLiterals("q", "some-query").SetLiterals("Q", "some-query-id"),
		},
		{
			Kinds: []int{nostr.KindChannelMessage},
			Tags:  make(nostr.TagMap).SetLiterals("#c", "test-channel"),
		},
		{
			Kinds: []int{nostr.KindReaction},
			Tags:  make(nostr.TagMap).SetLiterals("content", "+"),
		},
		{
			Kinds: []int{nostr.KindRepost, nostr.KindGenericRepost},
			Tags:  make(nostr.TagMap).SetLiterals("e", "reposted-event-id"),
		},
		{
			Kinds: []int{nostr.KindGiftWrap},
			Tags:  make(nostr.TagMap).SetLiterals("p", "recipient-pubkey"),
		},
		{
			Kinds: []int{model.CustomIONKindFundSendNotify},
			Tags:  make(nostr.TagMap).SetLiterals("amount", "1000"),
		},
		{
			Kinds: []int{model.CustomIONKindFundReceive},
			Tags:  make(nostr.TagMap).SetLiterals("p", "sender-pubkey"),
		},
		{
			Kinds: []int{model.CustomIONSystemMessage},
			Tags:  make(nostr.TagMap).SetLiterals("type", "announcement"),
		},
		{
			Kinds:   []int{nostr.KindTextNote},
			Authors: []string{"author1", "author2"},
			Tags:    make(nostr.TagMap).SetLiterals("t", "important"),
		},
	}

	filtersJSON, err := json.Marshal(filters)
	require.NoError(t, err)

	event := &model.Event{
		Event: nostr.Event{
			ID:      "all_notifications_event",
			PubKey:  "all_notifications_pubkey",
			Kind:    model.CustomIONKindDeviceRegistration,
			Content: string(filtersJSON),
			Tags: nostr.Tags{
				{"d", "all_notifications_device"},
				{"t", "android"},
				{"relay", "wss://relay.example.com"},
				{"token", "all_notifications_token"},
			},
		},
	}

	require.NoError(t, pm.processDeviceRegistrationEvent(event))

	deviceInfo, exists := pm.devices["all_notifications_device"]
	require.True(t, exists, "Device should be added to devices map")
	require.Equal(t, DeviceID("all_notifications_device"), deviceInfo.DeviceID)
	require.Equal(t, "all_notifications_token", deviceInfo.FCMToken)
	require.Equal(t, "all_notifications_pubkey", deviceInfo.PubKey)
	require.Equal(t, "all_notifications_event", deviceInfo.DeviceRegistrationEventID, "DeviceRegistrationEventID should be equal to all_notifications_event")

	expectedNotificationTypes := []NotificationType{
		NotificationTypePost,
		NotificationTypeChannelMessage,
		NotificationTypeReaction,
		NotificationTypeRepost,
		NotificationTypeDirectMessage,
		NotificationTypePaymentRequest,
		NotificationTypePaymentReceived,
		NotificationTypeSystem,
		NotificationTypeMention,
		NotificationTypeReply,
	}

	for _, notificationType := range expectedNotificationTypes {
		require.True(t, pm.filterToDevices[notificationType]["all_notifications_device"],
			"Device should be subscribed to %s", notificationType)
	}
}
