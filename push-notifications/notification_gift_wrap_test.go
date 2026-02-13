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
)

func helperCreateGiftWrapEvent(t *testing.T, id string, authorPubKey string, tags model.Tags) *model.Event {
	t.Helper()

	return &model.Event{
		Event: nostr.Event{
			ID:      id,
			PubKey:  authorPubKey,
			Kind:    nostr.KindGiftWrap,
			Content: "",
			Tags:    tags,
		},
	}
}

func TestHandleGiftWrapEventEdgeCases(t *testing.T) {
	t.Parallel()

	pm := helperNewManager(t)

	recipientMasterPubKey := "recipient_master_pubkey"
	deviceID := "device1"
	devicePubKey := "device_pubkey"

	filters := model.Filters{
		{
			Kinds: []int{nostr.KindDirectMessage, model.CustomIONKindDirectMessage, model.CustomIONKindFundReceive,
				model.CustomIONKindFundSendNotify, nostr.KindReaction},
		},
	}

	deviceTags := model.Tags{
		{"t", "ios"},
		{"d", deviceID},
		{"relay", "wss://relay.example.com"},
		{"token", "token1"},
	}

	deviceEvent1 := helperCreateTestDeviceRegistrationEvent(t, devicePubKey, deviceID, deviceTags, filters)
	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent1))

	pm.deviceMutex.Lock()
	deviceInfo, ok := pm.userDevicesMap[devicePubKey][DeviceID(deviceID)]
	require.True(t, ok, "Device should exist in userDevicesMap")

	if _, ok := pm.userDevicesMap[recipientMasterPubKey]; !ok {
		pm.userDevicesMap[recipientMasterPubKey] = make(map[DeviceID]DeviceInfo)
	}
	pm.userDevicesMap[recipientMasterPubKey][DeviceID(deviceID)] = deviceInfo
	pm.deviceMutex.Unlock()

	t.Run("self recipient", func(t *testing.T) {
		event := helperCreateGiftWrapEvent(
			t,
			"test_self_recipient",
			"sender_pubkey",
			model.Tags{
				{"k", strconv.Itoa(nostr.KindDirectMessage)},
				{"p", "sender_pubkey", "", devicePubKey},
				{"expiration", strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)},
			},
		)

		notifications, err := pm.handleGiftWrapEvent(event)
		require.NoError(t, err)
		require.Nil(t, notifications)
	})

	t.Run("p tag without device pubkey", func(t *testing.T) {
		event := helperCreateGiftWrapEvent(
			t,
			"test_p_tag_without_device",
			"sender_pubkey",
			model.Tags{
				{"k", strconv.Itoa(nostr.KindDirectMessage)},
				{"p", recipientMasterPubKey, ""},
				{"expiration", strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)},
			},
		)

		notifications, err := pm.handleGiftWrapEvent(event)
		require.NoError(t, err)
		require.Nil(t, notifications)
	})

	t.Run("device not found", func(t *testing.T) {
		event := helperCreateGiftWrapEvent(
			t,
			"test_device_not_found",
			"sender_pubkey",
			model.Tags{
				{"k", strconv.Itoa(nostr.KindDirectMessage)},
				{"p", recipientMasterPubKey, "", "unknown_device_pubkey"},
				{"expiration", strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)},
			},
		)

		notifications, err := pm.handleGiftWrapEvent(event)
		require.NoError(t, err)
		require.Nil(t, notifications)
	})
}

func TestHandleGiftWrapEvent(t *testing.T) {
	t.Parallel()

	recipientMasterPubKey := "recipient_master_pubkey"
	deviceID := "device1"
	devicePubKey := "device_pubkey"
	senderPubKey := "sender_pubkey"

	tests := []struct {
		name        string
		kind        int
		notifyType  NotificationType
		description string
	}{
		{"DirectMessage", nostr.KindDirectMessage, NotificationTypeDirectMessage, "standard direct message"},
		{"IONDirectMessage", model.CustomIONKindDirectMessage, NotificationTypeDirectMessage, "ION direct message"},
		{"FundReceive", model.CustomIONKindFundReceive, NotificationTypePaymentReceived, "fund receive"},
		{"FundSendNotify", model.CustomIONKindFundSendNotify, NotificationTypePaymentRequest, "fund send notify"},
		{"Reaction", nostr.KindReaction, NotificationTypeReaction, "reaction"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			localPM := helperNewManager(t)

			filters := model.Filters{
				{
					Kinds: []int{nostr.KindGiftWrap, tc.kind},
				},
			}

			deviceTags := model.Tags{
				{"t", "ios"},
				{"d", deviceID},
				{"relay", "wss://relay.example.com"},
				{"token", "token1"},
			}

			deviceEvent := helperCreateTestDeviceRegistrationEvent(t, devicePubKey, deviceID, deviceTags, filters)
			require.NoError(t, localPM.processDeviceRegistrationEvent(deviceEvent))

			localPM.deviceMutex.Lock()
			deviceInfo, ok := localPM.userDevicesMap[devicePubKey][DeviceID(deviceID)]
			require.True(t, ok, "Device should exist in userDevicesMap")

			if _, ok := localPM.userDevicesMap[recipientMasterPubKey]; !ok {
				localPM.userDevicesMap[recipientMasterPubKey] = make(map[DeviceID]DeviceInfo)
			}
			localPM.userDevicesMap[recipientMasterPubKey][DeviceID(deviceID)] = deviceInfo
			localPM.deviceMutex.Unlock()

			eventID := "test_gift_wrap_" + tc.name
			event := helperCreateGiftWrapEvent(
				t,
				eventID,
				senderPubKey,
				model.Tags{
					{"k", strconv.Itoa(tc.kind)},
					{"p", recipientMasterPubKey, "", devicePubKey},
					{"expiration", strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)},
				},
			)

			notifications, err := localPM.handleGiftWrapEvent(event)
			require.NoError(t, err)
			require.NotNil(t, notifications, "Notifications should not be nil for "+tc.description)
			require.Len(t, notifications, 1, "Should create one notification for "+tc.description)

			notification := notifications[0]
			if tc.notifyType == NotificationTypeReaction {
				require.Equal(t, defaultTranslations[tc.notifyType].Title, notification.Title, "Title should match for "+tc.description)
				require.Equal(t, defaultTranslations[tc.notifyType].Body, notification.Body, "Body should match for "+tc.description)
			} else {
				require.Equal(t, defaultTranslations[tc.notifyType].Title, notification.Title, "Title should match for "+tc.description)
				require.Equal(t, defaultTranslations[tc.notifyType].Body, notification.Body, "Body should match for "+tc.description)
			}
			require.Equal(t, deviceEvent, notification.Target, "Target should match for "+tc.description)
			require.Contains(t, notification.Data, "event", "Data should contain event for "+tc.description)

			compressedEvent, ok := notification.Data["event"].(string)
			require.True(t, ok, "event should be a string for "+tc.description)

			decompressedEvent := helperDecompressZlibAndDecodeBase64(t, compressedEvent)
			require.Equal(t, event.String(), decompressedEvent, "Decompressed event should match original for "+tc.description)
			require.Equal(t, CompressionMethodZlib, notification.Data["compression"], "Compression method should be zlib for "+tc.description)
		})
	}
}

func TestHandleGiftWrapEventWithMultipleDevices(t *testing.T) {
	t.Parallel()

	recipientMasterPubKey := "recipient_with_multiple_devices"
	senderPubKey := "sender_pubkey"

	devices := []struct {
		id       string
		pubKey   string
		platform string
	}{
		{"device1", "device_pubkey1", "ios"},
		{"device2", "device_pubkey2", "android"},
		{"device3", "device_pubkey3", "web"},
	}

	for _, device := range devices {
		t.Run("notify_"+device.platform+"_device", func(t *testing.T) {
			pm := helperNewManager(t)

			filters := model.Filters{
				{
					Kinds: []int{nostr.KindGiftWrap},
					Tags:  model.TagMap{}.SetLiterals("k", strconv.Itoa(nostr.KindDirectMessage)),
				},
			}

			deviceTags := model.Tags{
				{"t", device.platform},
				{"d", device.id},
				{"relay", "wss://relay.example.com"},
				{"token", "token_" + device.id},
			}

			deviceEvent := helperCreateTestDeviceRegistrationEvent(t, device.pubKey, device.id, deviceTags, filters)
			require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent))

			pm.deviceMutex.Lock()
			deviceInfo, ok := pm.userDevicesMap[device.pubKey][DeviceID(device.id)]
			require.True(t, ok, "Device should exist in userDevicesMap")

			if _, ok := pm.userDevicesMap[recipientMasterPubKey]; !ok {
				pm.userDevicesMap[recipientMasterPubKey] = make(map[DeviceID]DeviceInfo)
			}
			pm.userDevicesMap[recipientMasterPubKey][DeviceID(device.id)] = deviceInfo
			pm.deviceMutex.Unlock()

			event := helperCreateGiftWrapEvent(
				t,
				"test_gift_wrap_"+device.id,
				senderPubKey,
				model.Tags{
					{"k", strconv.Itoa(nostr.KindDirectMessage)},
					{"p", recipientMasterPubKey, "", device.pubKey},
					{"expiration", strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)},
				},
			)

			notifications, err := pm.handleGiftWrapEvent(event)
			require.NoError(t, err)
			require.NotNil(t, notifications, "Notifications should not be nil for "+device.platform)
			require.Len(t, notifications, 1, "Should create one notification for "+device.platform)

			notification := notifications[0]

			if device.platform == model.DeviceTokenOSAndroid {
				require.Equal(t, "", notification.Title, "Title should be empty for Android")
				require.Equal(t, "", notification.Body, "Body should be empty for Android")
			} else {
				require.Equal(t, defaultTranslations[NotificationTypeDirectMessage].Title, notification.Title,
					"Title should match for "+device.platform)
				require.Equal(t, defaultTranslations[NotificationTypeDirectMessage].Body, notification.Body,
					"Body should match for "+device.platform)
			}

			require.Contains(t, notification.Data, "event", "Data should contain event")

			compressedEvent, ok := notification.Data["event"].(string)
			require.True(t, ok, "event should be a string")

			decompressedEvent := helperDecompressZlibAndDecodeBase64(t, compressedEvent)
			require.Equal(t, event.String(), string(decompressedEvent), "Decompressed event should match original")
			require.Equal(t, CompressionMethodZlib, notification.Data["compression"], "Compression method should be zlib")
		})
	}
}

func TestHandleGiftWrapEventReaction(t *testing.T) {
	t.Parallel()

	pm := helperNewManager(t)

	recipientMasterPubKey := "recipient_master_pubkey"
	deviceID := "device1"
	devicePubKey := "device_pubkey"

	filters := model.Filters{
		{
			Kinds: []int{nostr.KindGiftWrap},
			Tags:  model.TagMap{}.SetLiterals("k", strconv.Itoa(nostr.KindReaction)),
		},
	}

	deviceTags := model.Tags{
		{"t", "ios"},
		{"d", deviceID},
		{"relay", "wss://relay.example.com"},
		{"token", "token1"},
	}

	deviceEvent := helperCreateTestDeviceRegistrationEvent(t, devicePubKey, deviceID, deviceTags, filters)
	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent))

	pm.deviceMutex.Lock()
	deviceInfo, ok := pm.userDevicesMap[devicePubKey][DeviceID(deviceID)]
	require.True(t, ok, "Device should exist in userDevicesMap")

	if _, ok := pm.userDevicesMap[recipientMasterPubKey]; !ok {
		pm.userDevicesMap[recipientMasterPubKey] = make(map[DeviceID]DeviceInfo)
	}
	pm.userDevicesMap[recipientMasterPubKey][DeviceID(deviceID)] = deviceInfo
	pm.deviceMutex.Unlock()

	t.Run("Reaction", func(t *testing.T) {
		event := helperCreateGiftWrapEvent(
			t,
			"test_reaction",
			"sender_pubkey",
			model.Tags{
				{"k", strconv.Itoa(nostr.KindReaction)},
				{"p", recipientMasterPubKey, "", devicePubKey},
				{"expiration", strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)},
			},
		)

		notifications, err := pm.handleGiftWrapEvent(event)
		require.NoError(t, err)
		require.NotNil(t, notifications)
		require.Len(t, notifications, 1)

		notification := notifications[0]
		require.Equal(t, defaultTranslations[NotificationTypeReaction].Title, notification.Title, "Title should match")
		require.Equal(t, defaultTranslations[NotificationTypeReaction].Body, notification.Body, "Body should match for reaction")
		require.Equal(t, defaultTranslations[NotificationTypeReaction].ImageURL, notification.ImageURL, "Image URL should match")
		require.Equal(t, deviceEvent, notification.Target)

		require.Contains(t, notification.Data, "event", "Data should contain event")
		compressedEvent, ok := notification.Data["event"].(string)
		require.True(t, ok, "event should be a string")

		decompressedEvent := helperDecompressZlibAndDecodeBase64(t, compressedEvent)
		require.Equal(t, event.String(), decompressedEvent, "Decompressed event should match original")
		require.Equal(t, CompressionMethodZlib, notification.Data["compression"], "Compression method should be zlib")
	})
}

func TestGiftWrapWithJsonTagFilter(t *testing.T) {
	t.Parallel()

	pm := helperNewManager(t)

	jsonContent := `[{"kinds":[1059],"#k":["1756"],"#p":["58eeb816af31498e81b9843250d7a813ad84934e0cfc563e3fd4753130a4bd78"]},{"kinds":[30175,30023],"#p":["58eeb816af31498e81b9843250d7a813ad84934e0cfc563e3fd4753130a4bd78"]},{"kinds":[16],"#p":["58eeb816af31498e81b9843250d7a813ad84934e0cfc563e3fd4753130a4bd78"],"#k":["30175","30023"]},{"kinds":[6],"#p":["58eeb816af31498e81b9843250d7a813ad84934e0cfc563e3fd4753130a4bd78"]},{"kinds":[30175],"#Q":[[null,null,"58eeb816af31498e81b9843250d7a813ad84934e0cfc563e3fd4753130a4bd78"]]},{"kinds":[1],"#q":[[null,null,"58eeb816af31498e81b9843250d7a813ad84934e0cfc563e3fd4753130a4bd78"]]},{"kinds":[7],"#p":["58eeb816af31498e81b9843250d7a813ad84934e0cfc563e3fd4753130a4bd78"]},{"kinds":[1059],"#k":["7"],"#p":["58eeb816af31498e81b9843250d7a813ad84934e0cfc563e3fd4753130a4bd78"]},{"kinds":[3],"#p":["58eeb816af31498e81b9843250d7a813ad84934e0cfc563e3fd4753130a4bd78"]},{"kinds":[1059],"#k":["30014","14"],"#p":["58eeb816af31498e81b9843250d7a813ad84934e0cfc563e3fd4753130a4bd78"]},{"kinds":[1059],"#k":["1755"],"#p":["58eeb816af31498e81b9843250d7a813ad84934e0cfc563e3fd4753130a4bd78"]}]`

	t.Run("Tests JSON filter in device registration", func(t *testing.T) {
		userPubKey := "58eeb816af31498e81b9843250d7a813ad84934e0cfc563e3fd4753130a4bd78"
		devicePubKey := "9e58d6f86dce97b32a86dc7544f21471a4c25506ec9f0c340184b3b6e11980a5"
		deviceID := "019680b4-4d0c-7c99-884d-64f81d3f8358"

		registrationEvent := &model.Event{
			Event: nostr.Event{
				ID:      "registration-event-id",
				Kind:    model.CustomIONKindDeviceRegistration,
				Content: jsonContent,
				PubKey:  devicePubKey,
				Tags: model.Tags{
					model.Tag{"b", userPubKey},
					model.Tag{"d", deviceID},
					model.Tag{"token", "test-token"},
					model.Tag{"t", "ios"},
				},
			},
		}

		var filters model.Filters
		require.NoError(t, json.Unmarshal([]byte(jsonContent), &filters))

		deviceInfo := DeviceInfo{Filters: filters, Event: registrationEvent}

		if _, ok := pm.userDevicesMap[userPubKey]; !ok {
			pm.userDevicesMap[userPubKey] = make(map[DeviceID]DeviceInfo)
		}
		pm.userDevicesMap[userPubKey][DeviceID(deviceID)] = deviceInfo

		ev := &model.Event{
			Event: nostr.Event{
				ID:     "gift-wrap-event-id",
				Kind:   nostr.KindGiftWrap,
				PubKey: "author-pubkey",
				Tags: model.Tags{
					model.Tag{"k", "14"},
					model.Tag{"p", userPubKey, "", devicePubKey},
				},
			},
		}

		notifications, err := pm.processEvent(t.Context(), ev)
		require.NoError(t, err)
		require.NotNil(t, notifications, "Notifications should not be nil")
		require.Len(t, notifications, 1, "Should create one notification")
		require.Equal(t, notifications[0].Target, registrationEvent, "Target should be the registration event")

		require.Contains(t, notifications[0].Data, "event", "Data should contain event")
		compressedEvent, ok := notifications[0].Data["event"].(string)
		require.True(t, ok, "event should be a string")

		decompressedEvent := helperDecompressZlibAndDecodeBase64(t, compressedEvent)
		require.Equal(t, ev.String(), decompressedEvent, "Decompressed event should match original")
		require.Equal(t, CompressionMethodZlib, notifications[0].Data["compression"], "Compression method should be zlib")
	})
}
