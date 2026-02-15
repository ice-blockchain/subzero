// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/json"
	"slices"
	"strconv"
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

const testRelayURL = "wss://relay.example.com"

func helperCreateTestDeviceRegistrationEvent(tb testing.TB, pubKey string, deviceID string, tags model.Tags, filters model.Filters) *model.Event {
	tb.Helper()

	if !slices.ContainsFunc(tags, func(tag model.Tag) bool {
		return len(tag) > 2 && tag.Key() == "relay"
	}) {
		tags = append(tags, model.Tag{"relay", testRelayURL})
	}

	if !slices.ContainsFunc(tags, func(tag model.Tag) bool {
		return len(tag) > 2 && tag.Key() == "d"
	}) {
		tags = append(tags, model.Tag{"d", cmp.Or(deviceID, rand.Text())})
	}

	return &model.Event{
		Event: nostr.Event{
			ID:      "test_id_" + deviceID,
			PubKey:  pubKey,
			Kind:    model.CustomIONKindDeviceRegistration,
			Content: filters.String(),
			Tags:    tags,
		},
	}
}

func TestProcessDeviceRegistrationEvent(t *testing.T) {
	t.Parallel()

	t.Run("basic_device_registration", func(t *testing.T) {
		pm := helperNewManager(t)
		filters := model.Filters{
			{
				Kinds: []int{nostr.KindTextNote, model.CustomIONKindFundReceive},
			},
		}

		masterPubKey := "master_pubkey"
		deviceID := "device1"
		deviceTags := model.Tags{
			{"t", "ios"},
			{"d", deviceID},
			{"relay", "wss://relay.example.com"},
			{"token", "token1"},
		}

		event := helperCreateTestDeviceRegistrationEvent(t, masterPubKey, deviceID, deviceTags, filters)

		err := pm.processDeviceRegistrationEvent(event)
		require.NoError(t, err)

		require.Len(t, pm.userDevicesMap, 1)
		require.Contains(t, pm.userDevicesMap, masterPubKey)
		require.Len(t, pm.userDevicesMap[masterPubKey], 1)
		require.Contains(t, pm.userDevicesMap[masterPubKey], deviceID)

		deviceInfo := pm.userDevicesMap[masterPubKey][deviceID]
		require.Equal(t, event, deviceInfo.Event)

		var parsedFilters model.Filters
		require.NoError(t, json.Unmarshal([]byte(event.Content), &parsedFilters))
		require.Equal(t, parsedFilters, deviceInfo.Filters)
	})

	t.Run("invalid_filter_json", func(t *testing.T) {
		pm := helperNewManager(t)

		masterPubKey := "master_pubkey"
		deviceID := "device3"
		deviceTags := model.Tags{
			{"t", "ios"},
			{"d", deviceID},
			{"relay", "wss://relay.example.com"},
			{"token", "token3"},
		}

		event := &model.Event{
			Event: nostr.Event{
				ID:      "test_id_" + deviceID,
				PubKey:  masterPubKey,
				Kind:    model.CustomIONKindDeviceRegistration,
				Content: "{invalid json",
				Tags:    deviceTags,
			},
		}

		require.Error(t, pm.processDeviceRegistrationEvent(event))
		require.Len(t, pm.userDevicesMap, 0)
	})
}

func TestRemoveDeviceFromCache(t *testing.T) {
	t.Parallel()

	t.Run("basic_device_removal", func(t *testing.T) {
		pm := helperNewManager(t)

		filters := model.Filters{
			{
				Kinds: []int{nostr.KindTextNote},
			},
		}

		masterPubKey := "master_pubkey"
		deviceID := "device1"
		deviceTags := model.Tags{
			{"t", "ios"},
			{"d", deviceID},
			{"relay", "wss://relay.example.com"},
			{"token", "token1"},
		}

		event := helperCreateTestDeviceRegistrationEvent(t, masterPubKey, deviceID, deviceTags, filters)

		require.NoError(t, pm.processDeviceRegistrationEvent(event))
		require.Len(t, pm.userDevicesMap[masterPubKey], 1)

		pm.removeDeviceFromCache(event)
		require.Len(t, pm.userDevicesMap, 0)
	})

	t.Run("device_belongs_to_another_user", func(t *testing.T) {
		pm := helperNewManager(t)

		filters := model.Filters{
			{
				Kinds: []int{nostr.KindTextNote},
			},
		}

		masterPubKey := "master_pubkey"
		otherPubKey := "other_pubkey"
		deviceID := "device3"
		deviceTags := model.Tags{
			{"t", "ios"},
			{"d", deviceID},
			{"relay", "wss://relay.example.com"},
			{"token", "token3"},
		}

		event := helperCreateTestDeviceRegistrationEvent(t, masterPubKey, deviceID, deviceTags, filters)
		require.NoError(t, pm.processDeviceRegistrationEvent(event))

		require.Len(t, pm.userDevicesMap, 1)
		require.Contains(t, pm.userDevicesMap[masterPubKey], deviceID)

		pm.userDevicesMap[otherPubKey] = make(map[string]DeviceInfo)
		pm.userDevicesMap[otherPubKey][deviceID] = DeviceInfo{
			Event:   event,
			Filters: filters,
		}
		require.Len(t, pm.userDevicesMap, 2)
		require.Contains(t, pm.userDevicesMap[masterPubKey], deviceID)
	})
}

func TestShouldProcessDeletionEvent(t *testing.T) {
	t.Parallel()

	pm := helperNewManager(t)

	t.Run("not_a_deletion_event", func(t *testing.T) {
		event := &model.Event{
			Event: nostr.Event{
				Kind: nostr.KindTextNote,
			},
		}

		should := pm.shouldProcessDeletionEvent(event)
		require.False(t, should)
	})

	t.Run("deletion_event_without_k_tag", func(t *testing.T) {
		event := &model.Event{
			Event: nostr.Event{
				Kind: nostr.KindDeletion,
				Tags: model.Tags{},
			},
		}

		should := pm.shouldProcessDeletionEvent(event)
		require.True(t, should)
	})

	t.Run("deletion_event_with_matching_k_tag", func(t *testing.T) {
		event := &model.Event{
			Event: nostr.Event{
				Kind: nostr.KindDeletion,
				Tags: model.Tags{
					{"k", strconv.Itoa(model.CustomIONKindDeviceRegistration)},
				},
			},
		}

		should := pm.shouldProcessDeletionEvent(event)
		require.True(t, should)
	})

	t.Run("deletion_event_with_non_matching_k_tag", func(t *testing.T) {
		event := &model.Event{
			Event: nostr.Event{
				Kind: nostr.KindDeletion,
				Tags: model.Tags{
					{"k", strconv.Itoa(nostr.KindTextNote)},
				},
			},
		}

		should := pm.shouldProcessDeletionEvent(event)
		require.False(t, should)
	})

	t.Run("deletion_event_with_multiple_k_tags", func(t *testing.T) {
		event := &model.Event{
			Event: nostr.Event{
				Kind: nostr.KindDeletion,
				Tags: model.Tags{
					{"k", strconv.Itoa(nostr.KindTextNote)},
					{"k", strconv.Itoa(model.CustomIONKindDeviceRegistration)},
				},
			},
		}

		should := pm.shouldProcessDeletionEvent(event)
		require.True(t, should)
	})
}

func TestManageDeviceRegistrationEvents(t *testing.T) {
	t.Parallel()

	t.Run("empty_events_list", func(t *testing.T) {
		pm := helperNewManager(t)

		err := pm.ManageDeviceRegistrationEvents(context.Background(), []*model.Event{})
		require.NoError(t, err)
	})

	t.Run("process_registration_events", func(t *testing.T) {
		pm := helperNewManager(t)
		filters := model.Filters{
			{
				Kinds: []int{nostr.KindTextNote},
			},
		}
		masterPubKey := "master_pubkey"
		deviceID := "device1"
		deviceTags := model.Tags{
			{"t", "ios"},
			{"d", deviceID},
			{"relay", "wss://relay.example.com"},
			{"token", "token1"},
		}

		event := helperCreateTestDeviceRegistrationEvent(t, masterPubKey, deviceID, deviceTags, filters)

		err := pm.ManageDeviceRegistrationEvents(context.Background(), []*model.Event{event})
		require.NoError(t, err)

		require.Len(t, pm.userDevicesMap, 1)
		require.Contains(t, pm.userDevicesMap[masterPubKey], deviceID)
	})
}

func TestProcessDeviceRegistrationBatch(t *testing.T) {
	t.Parallel()

	pm := helperNewManager(t)

	devices := []struct {
		pubKey   string
		deviceID string
		platform string
	}{
		{"user1", "device1", "ios"},
		{"user1", "device2", "android"},
		{"user2", "device3", "web"},
	}

	for _, d := range devices {
		filters := model.Filters{
			{
				Kinds: []int{nostr.KindTextNote},
			},
		}

		tags := model.Tags{
			{"t", d.platform},
			{"d", d.deviceID},
			{"relay", "wss://relay.example.com"},
			{"token", "token_" + d.deviceID},
		}

		event := helperCreateTestDeviceRegistrationEvent(t, d.pubKey, d.deviceID, tags, filters)
		require.NoError(t, pm.processDeviceRegistrationEvent(event))
	}

	require.Len(t, pm.userDevicesMap, 2)
	require.Len(t, pm.userDevicesMap["user1"], 2)
	require.Len(t, pm.userDevicesMap["user2"], 1)
}
