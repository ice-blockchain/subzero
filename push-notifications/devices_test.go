// SPDX-License-Identifier: ice License 1.0


package pushnotifications

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
)

func helperCreateTestDeviceRegistrationEvent(t *testing.T, pubKey string, deviceID string, tags []string, filters nostr.Filters) *model.Event {
	t.Helper()

	filtersJSON, err := json.Marshal(filters)
	require.NoError(t, err)

	eventTags := nostr.Tags{
		{"d", deviceID},
	}

	for i := 0; i < len(tags); i += 2 {
		if i+1 < len(tags) {
			eventTags = append(eventTags, nostr.Tag{tags[i], tags[i+1]})
		}
	}

	return &model.Event{
		Event: nostr.Event{
			ID:      "test_id_" + deviceID,
			PubKey:  pubKey,
			Kind:    model.CustomIONKindDeviceRegistration,
			Content: string(filtersJSON),
			Tags:    eventTags,
		},
	}
}

func TestProcessDeviceRegistrationEvent(t *testing.T) {
	pm := &PushNotificationManager{
		devices:     make(map[pn.DeviceID]DeviceInfo),
		userDevices: make(map[string][]pn.DeviceID),
	}

	filters := nostr.Filters{
		{
			Kinds: []int{nostr.KindTextNote, model.CustomIONKindEditableTextNote},
		},
		{
			Kinds: []int{nostr.KindReaction},
		},
	}

	event := helperCreateTestDeviceRegistrationEvent(
		t,
		"test_pubkey",
		"device1",
		[]string{"t", "android", "relay", "wss://relay.example.com", "token", "encrypted_token"},
		filters,
	)

	require.NoError(t, pm.processDeviceRegistrationEvent(event))

	deviceInfo, exists := pm.devices["device1"]
	require.True(t, exists, "Device should be added to devices map")
	require.Equal(t, DeviceID("device1"), deviceInfo.DeviceID, "DeviceID should be equal to device1")
	require.Equal(t, "test_pubkey", deviceInfo.Event.PubKey, "PubKey should be equal to test_pubkey")
	require.Equal(t, "test_id_device1", deviceInfo.Event.ID, "Event ID should be equal to test_id_device1")

	devices, exists := pm.userDevices["test_pubkey"]
	require.True(t, exists, "User should be added to userDevices map")
	require.Contains(t, devices, DeviceID("device1"), "Device should be added to user's devices list")
}

func TestRemoveDevice(t *testing.T) {
	t.Parallel()

	pm := &PushNotificationManager{
		devices:     make(map[DeviceID]DeviceInfo),
		userDevices: make(map[PublicKey][]DeviceID),
	}

	deviceID := DeviceID("device-id-1")
	deviceID2 := DeviceID("device-id-2")
	masterPubKey := "master-pub-key-1"

	filters := nostr.Filters{
		{
			Kinds: []int{nostr.KindTextNote},
		},
	}

	event1 := helperCreateTestDeviceRegistrationEvent(t, masterPubKey, string(deviceID), nil, filters)
	event2 := helperCreateTestDeviceRegistrationEvent(t, masterPubKey, string(deviceID2), nil, filters)

	pm.devices[deviceID] = DeviceInfo{
		DeviceID: deviceID,
		Event:    event1,
	}
	pm.devices[deviceID2] = DeviceInfo{
		DeviceID: deviceID2,
		Event:    event2,
	}
	pm.userDevices[masterPubKey] = []DeviceID{deviceID, deviceID2}

	require.NoError(t, pm.RemoveDevice(t.Context(), deviceID, masterPubKey))

	_, exists := pm.devices[deviceID]
	require.False(t, exists, "Device should be removed from devices map")

	devices, ok := pm.userDevices[masterPubKey]
	require.True(t, ok, "User should remain in the list")
	require.Equal(t, []DeviceID{deviceID2}, devices, "List of user's devices should contain only device2")

	require.NoError(t, pm.RemoveDevice(t.Context(), DeviceID("non-existent"), masterPubKey))

	wrongPubKey := "wrong-pub-key"
	require.Error(t, pm.RemoveDevice(t.Context(), deviceID2, wrongPubKey))
}

func TestShouldProcessDeletionEvent(t *testing.T) {
	t.Parallel()

	pm := &PushNotificationManager{}

	event1 := &model.Event{
		Event: nostr.Event{
			Kind: nostr.KindTextNote,
		},
	}
	require.False(t, pm.shouldProcessDeletionEvent(event1), "Non-deletion event should return false")

	event2 := &model.Event{
		Event: nostr.Event{
			Kind: nostr.KindDeletion,
			Tags: nostr.Tags{},
		},
	}
	require.True(t, pm.shouldProcessDeletionEvent(event2), "Deletion event without k-tags should return true")

	event3 := &model.Event{
		Event: nostr.Event{
			Kind: nostr.KindDeletion,
			Tags: nostr.Tags{
				{"k", strconv.Itoa(model.CustomIONKindDeviceRegistration)},
			},
		},
	}
	require.True(t, pm.shouldProcessDeletionEvent(event3), "Deletion event with device registration k-tag should return true")

	event4 := &model.Event{
		Event: nostr.Event{
			Kind: nostr.KindDeletion,
			Tags: nostr.Tags{
				{"k", strconv.Itoa(nostr.KindTextNote)},
			},
		},
	}
	require.False(t, pm.shouldProcessDeletionEvent(event4), "Deletion event with non-device registration k-tag should return false")
}

func TestProcessDeviceRegistrationEventWithInvalidToken(t *testing.T) {
	t.Parallel()

	pm := &PushNotificationManager{
		devices:     make(map[pn.DeviceID]DeviceInfo),
		userDevices: make(map[string][]pn.DeviceID),
	}

	filters := nostr.Filters{
		{
			Kinds: []int{nostr.KindTextNote},
		},
	}

	event := helperCreateTestDeviceRegistrationEvent(
		t,
		"test_pubkey",
		"device1",
		[]string{"invalid_token", "true"},
		filters,
	)

	require.NoError(t, pm.processDeviceRegistrationEvent(event))

	deviceInfo, exists := pm.devices["device1"]
	require.True(t, exists, "Device should be added to devices map")
	require.Equal(t, DeviceID("device1"), deviceInfo.DeviceID, "DeviceID should be equal to device1")
	require.True(t, deviceInfo.HasInvalidToken, "Device should have invalid token flag")
}

func TestCollectDevicesToRemove(t *testing.T) {
	t.Parallel()

	pm := &PushNotificationManager{
		devices:     make(map[DeviceID]DeviceInfo),
		userDevices: make(map[PublicKey][]DeviceID),
	}

	deletionEvent := &model.Event{
		Event: nostr.Event{
			ID:     "deletion_event",
			PubKey: "pubkey1",
			Kind:   nostr.KindDeletion,
			Tags: nostr.Tags{
				{"e", "registration_event_id"},
				{"k", strconv.Itoa(model.CustomIONKindDeviceRegistration)},
			},
		},
	}

	require.True(t, pm.shouldProcessDeletionEvent(deletionEvent), "Deletion event with device registration k-tag should return true")

	emptyEvents := []*model.Event{}
	deviceToRemoveMap, err := pm.collectDevicesToRemove(t.Context(), emptyEvents)
	require.NoError(t, err)
	require.Nil(t, deviceToRemoveMap, "Empty events slice should return nil map")

	nonDeletionEvents := []*model.Event{
		{
			Event: nostr.Event{
				Kind: nostr.KindTextNote,
			},
		},
	}
	deviceToRemoveMap, err = pm.collectDevicesToRemove(t.Context(), nonDeletionEvents)
	require.NoError(t, err)
	require.Nil(t, deviceToRemoveMap, "Non-deletion events should return nil map")
}

func TestUpdateDevice(t *testing.T) {
	t.Parallel()

	pm := &PushNotificationManager{
		devices:     make(map[DeviceID]DeviceInfo),
		userDevices: make(map[PublicKey][]DeviceID),
	}

	initialFilters := nostr.Filters{
		{
			Kinds: []int{nostr.KindTextNote},
		},
	}

	event1 := helperCreateTestDeviceRegistrationEvent(
		t,
		"test_pubkey",
		"device1",
		[]string{"t", "android", "token", "encrypted_token1"},
		initialFilters,
	)

	require.NoError(t, pm.processDeviceRegistrationEvent(event1))

	deviceInfo, exists := pm.devices["device1"]
	require.True(t, exists, "Device should be added to devices map")
	require.Equal(t, 1, len(deviceInfo.Filters), "Filters should have 1 element")
	require.Equal(t, "encrypted_token1", deviceInfo.Event.GetTag("token").Value(), "Token should match")

	updatedFilters := nostr.Filters{
		{
			Kinds: []int{nostr.KindTextNote, nostr.KindReaction},
		},
		{
			Kinds: []int{nostr.KindFollowList},
		},
	}

	event2 := helperCreateTestDeviceRegistrationEvent(
		t,
		"test_pubkey",
		"device1",
		[]string{"t", "ios", "token", "encrypted_token2"},
		updatedFilters,
	)

	require.NoError(t, pm.processDeviceRegistrationEvent(event2))

	updatedDeviceInfo, exists := pm.devices["device1"]
	require.True(t, exists, "Device should still exist in devices map")
	require.Equal(t, 2, len(updatedDeviceInfo.Filters), "Filters should have 2 elements after update")
	require.Equal(t, "ios", updatedDeviceInfo.Event.GetTag("t").Value(), "Device type should be updated to ios")
	require.Equal(t, "encrypted_token2", updatedDeviceInfo.Event.GetTag("token").Value(), "Token should be updated")
	require.False(t, updatedDeviceInfo.HasInvalidToken, "Device should not have invalid token flag")
}

func TestMultipleDevicesPerUser(t *testing.T) {
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

	userPubKey := "user1_pubkey"

	device1 := helperCreateTestDeviceRegistrationEvent(t, userPubKey, "device1", []string{"t", "android"}, filters)
	device2 := helperCreateTestDeviceRegistrationEvent(t, userPubKey, "device2", []string{"t", "ios"}, filters)
	device3 := helperCreateTestDeviceRegistrationEvent(t, userPubKey, "device3", []string{"t", "web"}, filters)

	require.NoError(t, pm.processDeviceRegistrationEvent(device1))
	require.NoError(t, pm.processDeviceRegistrationEvent(device2))
	require.NoError(t, pm.processDeviceRegistrationEvent(device3))

	userDevices, exists := pm.userDevices[userPubKey]
	require.True(t, exists, "User should be added to userDevices map")
	require.Equal(t, 3, len(userDevices), "User should have 3 devices")
	require.Contains(t, userDevices, DeviceID("device1"), "User's devices should contain device1")
	require.Contains(t, userDevices, DeviceID("device2"), "User's devices should contain device2")
	require.Contains(t, userDevices, DeviceID("device3"), "User's devices should contain device3")

	require.NoError(t, pm.RemoveDevice(t.Context(), "device2", userPubKey))

	userDevices, exists = pm.userDevices[userPubKey]
	require.True(t, exists, "User should still be in userDevices map")
	require.Equal(t, 2, len(userDevices), "User should have 2 devices after removal")
	require.Contains(t, userDevices, DeviceID("device1"), "User's devices should still contain device1")
	require.Contains(t, userDevices, DeviceID("device3"), "User's devices should still contain device3")
	require.NotContains(t, userDevices, DeviceID("device2"), "User's devices should not contain device2 anymore")
}
