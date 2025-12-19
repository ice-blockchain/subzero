// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func TestProcessEventWithQuotes(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		userDevicesMap: make(map[PublicKey]map[DeviceID]DeviceInfo),
		compressorPool: helperCreateTestCompressorPool(),
		stats:          newPushStats(),
	}

	recipientPubKey := "58eeb816af31498e81b9843250d7a813ad84934e0cfc563e3fd4753130a4bd78"
	senderPubKey := "9e58d6f86dce97b32a86dc7544f21471a4c25506ec9f0c340184b3b6e11980a5"

	jsonContent := `[{"kinds":[1059],"#k":["1756"],"#p":["58eeb816af31498e81b9843250d7a813ad84934e0cfc563e3fd4753130a4bd78"]},{"kinds":[30175,30023],"#p":["58eeb816af31498e81b9843250d7a813ad84934e0cfc563e3fd4753130a4bd78"]},{"kinds":[16],"#p":["58eeb816af31498e81b9843250d7a813ad84934e0cfc563e3fd4753130a4bd78"],"#k":["30175","30023"]},{"kinds":[6],"#p":["58eeb816af31498e81b9843250d7a813ad84934e0cfc563e3fd4753130a4bd78"]},{"kinds":[30175],"#Q":[[null,null,"58eeb816af31498e81b9843250d7a813ad84934e0cfc563e3fd4753130a4bd78"]]},{"kinds":[1],"#q":[[null,null,"58eeb816af31498e81b9843250d7a813ad84934e0cfc563e3fd4753130a4bd78"]]},{"kinds":[7],"#p":["58eeb816af31498e81b9843250d7a813ad84934e0cfc563e3fd4753130a4bd78"]},{"kinds":[1059],"#k":["7"],"#p":["58eeb816af31498e81b9843250d7a813ad84934e0cfc563e3fd4753130a4bd78"]},{"kinds":[3],"#p":["58eeb816af31498e81b9843250d7a813ad84934e0cfc563e3fd4753130a4bd78"]},{"kinds":[1059],"#k":["30014","14"],"#p":["58eeb816af31498e81b9843250d7a813ad84934e0cfc563e3fd4753130a4bd78"]},{"kinds":[1059],"#k":["1755"],"#p":["58eeb816af31498e81b9843250d7a813ad84934e0cfc563e3fd4753130a4bd78"]}]`
	deviceID := uuid.NewString()

	var filters nostr.Filters
	require.NoError(t, json.Unmarshal([]byte(jsonContent), &filters))

	t.Run("Tests event processing with Q tag in custom editable text note", func(t *testing.T) {
		deviceEvent := &model.Event{
			Event: nostr.Event{
				ID:     "device-event-id",
				PubKey: recipientPubKey,
				Kind:   model.CustomIONKindDeviceRegistration,
				Tags: nostr.Tags{
					{"t", "ios"},
					{"d", deviceID},
					{"token", "test-token"},
				},
			},
		}

		pm.userDevicesMap[recipientPubKey] = map[DeviceID]DeviceInfo{
			DeviceID(deviceID): {
				Filters: filters,
				Event:   deviceEvent,
			},
		}

		event := &model.Event{
			Event: nostr.Event{
				ID:        "94ed698c7827ef0e01a919b37295cd7fcbf33b7c40bde1c79f11459b574f669b",
				PubKey:    senderPubKey,
				CreatedAt: nostr.Timestamp(1746342561),
				Kind:      model.CustomIONKindEditableTextNote,
				Tags: nostr.Tags{
					{"b", senderPubKey},
					{"d", "01969a20-c6f3-7604-a694-aa39acc4830e"},
					{"Q", "30175:4a2787e0a54946b4b6f879f693d8c288c8534f805f97ab048f6c7cf9e63a1939:019695e9-8dfd-760d-95f3-e3bb6a5397fc", "", recipientPubKey},
				},
				Content: "So how is it?\n",
			},
		}

		notifications, err := pm.processEvent(t.Context(), event)
		require.NoError(t, err, "Process event should not return an error")

		notification := notifications[0]
		require.Equal(t, defaultTranslations[NotificationTypeRepost].Title, notification.Title, "Title should match")
		require.Equal(t, defaultTranslations[NotificationTypeRepost].Body, notification.Body, "Body should match")
		require.Equal(t, deviceEvent, notification.Target, "Target should be the device event")
		require.Contains(t, notification.Data, "event", "Data should contain event")

		compressedEvent, ok := notification.Data["event"].(string)
		require.True(t, ok, "event should be a string")

		decompressedEvent := helperDecompressZlibAndDecodeBase64(t, compressedEvent)
		require.Equal(t, event.String(), decompressedEvent, "Decompressed event should match original")
		require.Equal(t, CompressionMethodZlib, notification.Data["compression"], "Compression method should be zlib")
	})

	t.Run("Tests event processing with q tag in KindTextNote", func(t *testing.T) {
		registrationEvent := &model.Event{
			Event: nostr.Event{
				ID:      "registration-event-id",
				Kind:    model.CustomIONKindDeviceRegistration,
				Content: jsonContent,
				PubKey:  recipientPubKey,
				Tags: nostr.Tags{
					nostr.Tag{"b", recipientPubKey},
					nostr.Tag{"d", deviceID},
					nostr.Tag{"token", "test-token"},
					nostr.Tag{"t", "ios"},
				},
			},
		}

		deviceInfo := DeviceInfo{Filters: filters, Event: registrationEvent}

		if _, ok := pm.userDevicesMap[recipientPubKey]; !ok {
			pm.userDevicesMap[recipientPubKey] = make(map[DeviceID]DeviceInfo)
		}
		pm.userDevicesMap[recipientPubKey][DeviceID(deviceID)] = deviceInfo

		ev := &model.Event{
			Event: nostr.Event{
				ID:      "text-note-with-q-tag",
				Kind:    nostr.KindTextNote,
				PubKey:  senderPubKey,
				Content: "This is a text note referring to someone",
				Tags: nostr.Tags{
					nostr.Tag{"q", "quoted-event-id", "", recipientPubKey},
				},
			},
		}

		notifications, err := pm.processEvent(t.Context(), ev)
		require.NoError(t, err)
		require.Len(t, notifications, 1, "Should create one notification")
		require.Equal(t, notifications[0].Target, registrationEvent, "Target should be the registration event")
		require.Contains(t, notifications[0].Data, "event", "Data should contain event")

		compressedEvent, ok := notifications[0].Data["event"].(string)
		require.True(t, ok, "event should be a string")

		decompressedEvent := helperDecompressZlibAndDecodeBase64(t, compressedEvent)
		require.Equal(t, ev.String(), decompressedEvent, "Decompressed event should match original")

		require.Equal(t, defaultTranslations[NotificationTypeRepost].Title, notifications[0].Title, "Title should match mention notification type")
		require.Equal(t, defaultTranslations[NotificationTypeRepost].Body, notifications[0].Body, "Body should match mention notification type")
		require.Equal(t, CompressionMethodZlib, notifications[0].Data["compression"], "Compression method should be zlib")
	})
}
