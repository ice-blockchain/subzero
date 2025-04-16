// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/pushnotifications/internal"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

func TestIsTokenInvalid(t *testing.T) {
	pm := &PushNotificationManager{
		devices:       make(map[pn.DeviceID]DeviceInfo),
		userDevices:   make(map[string][]pn.DeviceID),
		invalidTokens: make(map[pn.DeviceID]InvalidTokenInfo),
	}
	require.False(t, pm.isTokenInvalid("nonexistent_device", "any_token"), "Should be false for nonexistent device")

	pm.invalidTokens["device1"] = InvalidTokenInfo{
		DeviceID:     "device1",
		MasterPubKey: "pubkey1",
		Token:        "token1",
		CreatedAt:    time.Now(),
	}
	require.True(t, pm.isTokenInvalid("device1", "token1"), "Should be true for device with correct invalid token")
	require.False(t, pm.isTokenInvalid("device1", "wrong_token"), "Should be false for device with incorrect token")
}

func TestMarkTokenAsInvalid(t *testing.T) {
	pm := &PushNotificationManager{
		invalidTokens: make(map[DeviceID]InvalidTokenInfo),
	}
	invalidToken := InvalidTokenInfo{
		DeviceID:     "device1",
		MasterPubKey: "pubkey1",
		Token:        "token1",
		CreatedAt:    time.Now(),
	}

	pm.deviceMutex.Lock()
	pm.invalidTokens["device1"] = invalidToken
	pm.deviceMutex.Unlock()

	require.Contains(t, pm.invalidTokens, DeviceID("device1"), "Token should be added to invalid tokens map")
	require.Equal(t, "pubkey1", pm.invalidTokens["device1"].MasterPubKey, "MasterPubKey should be equal to pubkey1")
	require.Equal(t, "token1", pm.invalidTokens["device1"].Token, "Token should be equal to token1")
}

func TestSyncInvalidTokens(t *testing.T) {
	pm := &PushNotificationManager{
		invalidTokens: make(map[DeviceID]InvalidTokenInfo),
	}
	token1 := InvalidTokenInfo{
		DeviceID:     "device1",
		MasterPubKey: "pubkey1",
		Token:        "token1",
		CreatedAt:    time.Now(),
	}
	token2 := InvalidTokenInfo{
		DeviceID:     "device2",
		MasterPubKey: "pubkey2",
		Token:        "token2",
		CreatedAt:    time.Now(),
	}
	pm.deviceMutex.Lock()
	pm.invalidTokens["device1"] = token1
	pm.invalidTokens["device2"] = token2
	pm.deviceMutex.Unlock()

	require.Len(t, pm.invalidTokens, 2, "There should be 2 invalid tokens")
	require.Contains(t, pm.invalidTokens, DeviceID("device1"), "Token device1 should be added to invalid tokens map")
	require.Contains(t, pm.invalidTokens, DeviceID("device2"), "Token device2 should be added to invalid tokens map")
}

func TestProcessDeviceRegistrationEvent(t *testing.T) {
	pm := &PushNotificationManager{
		devices:         make(map[pn.DeviceID]DeviceInfo),
		userDevices:     make(map[string][]pn.DeviceID),
		filterToDevices: make(map[NotificationType]map[pn.DeviceID]bool),
		invalidTokens:   make(map[pn.DeviceID]InvalidTokenInfo),
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
		invalidTokens:   make(map[pn.DeviceID]InvalidTokenInfo),
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

func TestEventMatchesDeviceFilters(t *testing.T) {
	pm := &PushNotificationManager{}

	event1 := &model.Event{
		Event: nostr.Event{
			Kind: nostr.KindTextNote,
			Tags: nostr.Tags{
				{"p", "pubkey1"},
			},
		},
	}
	filters1 := nostr.Filters{
		{
			Kinds: []int{nostr.KindTextNote},
		},
	}
	require.True(t, pm.eventMatchesDeviceFilters(event1, filters1), "Event should match filters by kind")
	event2 := &model.Event{
		Event: nostr.Event{
			Kind:   nostr.KindTextNote,
			PubKey: "pubkey1",
			Tags: nostr.Tags{
				{"p", "pubkey1"},
			},
		},
	}

	filters2 := nostr.Filters{
		{
			Kinds:   []int{nostr.KindTextNote},
			Authors: []string{"pubkey1"},
		},
	}
	require.True(t, pm.eventMatchesDeviceFilters(event2, filters2), "Event should match filters by author")
	event3 := &model.Event{
		Event: nostr.Event{
			Kind: nostr.KindReaction,
			Tags: nostr.Tags{
				{"p", "pubkey1"},
			},
		},
	}
	filters3 := nostr.Filters{
		{
			Kinds: []int{nostr.KindTextNote},
		},
	}

	require.False(t, pm.eventMatchesDeviceFilters(event3, filters3), "Event should not match filters by kind")

	event4 := &model.Event{
		Event: nostr.Event{
			Kind:   nostr.KindTextNote,
			PubKey: "pubkey1",
			Tags: nostr.Tags{
				{"p", "pubkey1"},
			},
		},
	}

	filters4 := nostr.Filters{
		{
			Kinds:   []int{nostr.KindTextNote},
			Authors: []string{"pubkey2"},
		},
	}

	require.False(t, pm.eventMatchesDeviceFilters(event4, filters4), "Event should not match filters by author")

	event5 := &model.Event{
		Event: nostr.Event{
			Kind:   nostr.KindChannelMessage,
			PubKey: "pubkey1",
			Tags: nostr.Tags{
				{"e", "event1"},
				{"p", "pubkey1"},
			},
		},
	}

	filters5 := nostr.Filters{
		{
			Kinds:   []int{nostr.KindTextNote},
			Authors: []string{"pubkey1"},
		},
		{
			Kinds:   []int{nostr.KindChannelMessage},
			Authors: []string{"pubkey1"},
		},
	}

	require.True(t, pm.eventMatchesDeviceFilters(event5, filters5), "Event should match complex filters")
}

func TestFullSyncDevices(t *testing.T) {
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

func TestProcessDeletionEvents(t *testing.T) {
	pm := &PushNotificationManager{
		devices:         make(map[DeviceID]DeviceInfo),
		userDevices:     make(map[PublicKey][]DeviceID),
		filterToDevices: make(map[NotificationType]map[DeviceID]bool),
		invalidTokens:   make(map[DeviceID]InvalidTokenInfo),
	}

	pm.filterToDevices[NotificationTypePost] = make(map[DeviceID]bool)
	pm.filterToDevices[NotificationTypeDirectMessage] = make(map[DeviceID]bool)

	deviceID1 := DeviceID("device-1")
	deviceID2 := DeviceID("device-2")
	deviceID3 := DeviceID("device-3")
	masterPubKey1 := "pub-key-1"
	masterPubKey2 := "pub-key-2"

	pm.devices[deviceID1] = DeviceInfo{
		DeviceID:    deviceID1,
		Platform:    "android",
		FCMToken:    "token-1",
		LastUpdated: time.Now(),
		PubKey:      masterPubKey1,
	}
	pm.devices[deviceID2] = DeviceInfo{
		DeviceID:    deviceID2,
		Platform:    "ios",
		FCMToken:    "token-2",
		LastUpdated: time.Now(),
		PubKey:      masterPubKey2,
	}
	pm.devices[deviceID3] = DeviceInfo{
		DeviceID:    deviceID3,
		Platform:    "android",
		FCMToken:    "token-3",
		LastUpdated: time.Now(),
		PubKey:      masterPubKey1,
	}

	pm.userDevices[masterPubKey1] = []DeviceID{deviceID1, deviceID3}
	pm.userDevices[masterPubKey2] = []DeviceID{deviceID2}

	pm.filterToDevices[NotificationTypePost][deviceID1] = true
	pm.filterToDevices[NotificationTypeDirectMessage][deviceID2] = true
	pm.filterToDevices[NotificationTypePost][deviceID3] = true

	deletionEvent1 := &model.Event{
		Event: nostr.Event{
			ID:        "deletion-1",
			Kind:      nostr.KindDeletion,
			PubKey:    masterPubKey1,
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Tags: []nostr.Tag{
				{"e", "event-id-registration-device1"},
				{"k", strconv.Itoa(model.CustomIONKindDeviceRegistration)},
			},
		},
	}

	deletionEvent2 := &model.Event{
		Event: nostr.Event{
			ID:        "deletion-2",
			Kind:      nostr.KindDeletion,
			PubKey:    masterPubKey2,
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Tags: []nostr.Tag{
				{"e", "event-id-registration-device2"},
			},
		},
	}

	deletionEvent3 := &model.Event{
		Event: nostr.Event{
			ID:        "deletion-3",
			Kind:      nostr.KindDeletion,
			PubKey:    masterPubKey1,
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Tags: []nostr.Tag{
				{"e", "event-id-registration-device3"},
				{"k", "1000"},
			},
		},
	}
	nonDeletionEvent := &model.Event{
		Event: nostr.Event{
			ID:        "non-deletion",
			Kind:      nostr.KindTextNote,
			PubKey:    masterPubKey1,
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
		},
	}

	events := []*model.Event{deletionEvent1, nonDeletionEvent, deletionEvent2, deletionEvent3}

	var filteredEvents []*model.Event
	for _, event := range events {
		if pm.shouldProcessDeletionEvent(event) {
			filteredEvents = append(filteredEvents, event)
		}
	}
	require.Len(t, filteredEvents, 2, "Only two events should pass the filter")
	require.Contains(t,
		[]string{filteredEvents[0].ID, filteredEvents[1].ID},
		"deletion-1",
		"Event deletion-1 should pass the filter")
	require.Contains(t,
		[]string{filteredEvents[0].ID, filteredEvents[1].ID},
		"deletion-2",
		"Event deletion-2 should pass the filter")

	removePM := &PushNotificationManager{
		devices:         make(map[DeviceID]DeviceInfo),
		userDevices:     make(map[PublicKey][]DeviceID),
		filterToDevices: make(map[NotificationType]map[DeviceID]bool),
	}

	removePM.filterToDevices[NotificationTypePost] = make(map[DeviceID]bool)
	removePM.filterToDevices[NotificationTypeDirectMessage] = make(map[DeviceID]bool)

	removePM.devices[deviceID1] = DeviceInfo{
		DeviceID:    deviceID1,
		Platform:    "android",
		FCMToken:    "token-1",
		LastUpdated: time.Now(),
		PubKey:      masterPubKey1,
	}

	removePM.userDevices[masterPubKey1] = []DeviceID{deviceID1}

	removePM.filterToDevices[NotificationTypePost][deviceID1] = true

	deviceToRemoveMap := map[DeviceID]deviceToRemove{
		deviceID1: {
			deviceID:     deviceID1,
			masterPubKey: masterPubKey1,
		},
	}

	require.NoError(t, removePM.removeDevices(t.Context(), deviceToRemoveMap), "removeDevices should execute without errors")

	_, exists1 := removePM.devices[deviceID1]
	require.False(t, exists1, "Device should be removed from devices")

	devices1, ok1 := removePM.userDevices[masterPubKey1]
	require.True(t, ok1, "User should remain in the list")
	require.Empty(t, devices1, "User's device list should be empty")

	require.False(t, removePM.filterToDevices[NotificationTypePost][deviceID1],
		"Device should be removed from the filter")
}

func TestShouldProcessDeletionEvent(t *testing.T) {
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
