// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
)

type MockPushClient struct {
	mock.Mock
}

func (m *MockPushClient) SendSingle(ctx context.Context, notification *pn.Notification[*pn.DeviceRegistrationEvent]) error {
	args := m.Called(ctx, notification)
	return args.Error(0)
}

func (m *MockPushClient) SendTopic(ctx context.Context, notification *pn.Notification[pn.SubscriptionTopic]) error {
	args := m.Called(ctx, notification)
	return args.Error(0)
}

func helperDecompressZlibAndDecodeBase64(t *testing.T, compressed string) string {
	t.Helper()

	decoded, err := base64.StdEncoding.DecodeString(compressed)
	require.NoError(t, err, "Should decode base64 data")

	zr, err := zlib.NewReader(bytes.NewReader(decoded))
	require.NoError(t, err, "Should create zlib reader")
	defer zr.Close()

	decompressed, err := io.ReadAll(zr)
	require.NoError(t, err, "Should decompress zlib data")

	return string(decompressed)
}

func helperCreateTestCompressorPool() *sync.Pool {
	return &sync.Pool{
		New: func() interface{} {
			buf := &bytes.Buffer{}
			base64Encoder := base64.NewEncoder(base64.StdEncoding, buf)
			zlibWriter, _ := zlib.NewWriterLevel(base64Encoder, zlib.BestCompression)

			return &compressorPoolItem{
				buf:           buf,
				base64Encoder: base64Encoder,
				zlibWriter:    zlibWriter,
			}
		},
	}
}

func TestCreateNotifications(t *testing.T) {
	pm := &PushNotificationManager{
		userDevicesMap: make(map[PublicKey]map[DeviceID]DeviceInfo),
		compressorPool: helperCreateTestCompressorPool(),
	}

	t.Run("Empty device list returns nil", func(t *testing.T) {
		notifications, err := pm.createNotifications(nil, NotificationTypeReaction, &model.Event{})
		require.NoError(t, err)
		require.Nil(t, notifications)
	})

	t.Run("Creates notifications with correct data", func(t *testing.T) {
		event := &model.Event{Event: nostr.Event{ID: "test-event-id", Content: "test content"}}

		deviceEvents := []*DeviceRegistrationEvent{
			{
				Event: nostr.Event{
					Tags: nostr.Tags{
						nostr.Tag{"t", "ios"},
					},
				},
			},
			{
				Event: nostr.Event{
					Tags: nostr.Tags{
						nostr.Tag{"t", "android"},
					},
				},
			},
		}

		notifications, err := pm.createNotifications(deviceEvents, NotificationTypeReaction, event)
		require.NoError(t, err)
		require.Len(t, notifications, 2)

		for _, notification := range notifications {
			require.Contains(t, notification.Data, "event")
			require.Contains(t, notification.Data, "compression")
			require.Equal(t, "zlib", notification.Data["compression"])

			compressedEvent, ok := notification.Data["event"].(string)
			require.True(t, ok, "event should be a string")

			decompressedEvent := helperDecompressZlibAndDecodeBase64(t, compressedEvent)
			require.Equal(t, event.String(), string(decompressedEvent), "Decompressed event should match original")
			require.Equal(t, CompressionMethodZlib, notification.Data["compression"], "Compression method should be zlib")
		}

		for _, notification := range notifications {
			deviceType := notification.Target.GetTag("t").Value()
			if deviceType == "android" {
				require.Empty(t, notification.Title)
				require.Empty(t, notification.Body)
				require.Empty(t, notification.ImageURL)
			} else {
				require.Equal(t, DefaultTranslations[NotificationTypeReaction].Title, notification.Title)
				require.Equal(t, DefaultTranslations[NotificationTypeReaction].Body, notification.Body)
				require.Equal(t, DefaultTranslations[NotificationTypeReaction].ImageURL, notification.ImageURL)
			}
		}
	})

	t.Run("Correctly serializes relevant events", func(t *testing.T) {
		event := &model.Event{Event: nostr.Event{ID: "test-event-id", Content: "test content"}}

		relevantEvents := []*model.Event{
			{
				Event: nostr.Event{
					ID:      "relevant-event-1",
					Content: `{"name":"user1","display_name":"User One"}`,
					Kind:    nostr.KindProfileMetadata,
				},
			},
			{
				Event: nostr.Event{
					ID:      "relevant-event-2",
					Content: `{"name":"user2","display_name":"User Two"}`,
					Kind:    nostr.KindProfileMetadata,
				},
			},
		}

		deviceEvents := []*DeviceRegistrationEvent{
			{
				Event: nostr.Event{
					Tags: nostr.Tags{
						nostr.Tag{"t", "ios"},
					},
				},
			},
		}

		notifications, err := pm.createNotifications(deviceEvents, NotificationTypeMentionReply, event, relevantEvents...)
		require.NoError(t, err)
		require.Len(t, notifications, 1)

		require.Contains(t, notifications[0].Data, "relevant_events")
		require.Contains(t, notifications[0].Data, "compression")
		require.Equal(t, "zlib", notifications[0].Data["compression"])

		compressedRelevantEvents, ok := notifications[0].Data["relevant_events"].(string)
		require.True(t, ok, "relevant_events should be a string")

		decompressedEvents := helperDecompressZlibAndDecodeBase64(t, compressedRelevantEvents)
		combinedContent := strings.Join([]string{
			relevantEvents[0].Content,
			relevantEvents[1].Content,
		}, ",")
		require.Equal(t, `[`+combinedContent+`]`, string(decompressedEvents), "Decompressed events should match combined content")
		require.Equal(t, CompressionMethodZlib, notifications[0].Data["compression"], "Compression method should be zlib")
		decompressedStr := string(decompressedEvents)
		require.Contains(t, decompressedStr, `"name":"user1"`)
		require.Contains(t, decompressedStr, `"display_name":"User One"`)
		require.Contains(t, decompressedStr, `"name":"user2"`)
		require.Contains(t, decompressedStr, `"display_name":"User Two"`)
	})
}

func TestCollectUserValidDevices(t *testing.T) {
	pm := &PushNotificationManager{
		userDevicesMap: make(map[PublicKey]map[DeviceID]DeviceInfo),
		compressorPool: helperCreateTestCompressorPool(),
	}

	t.Run("Returns nil when user has no devices", func(t *testing.T) {
		devices := pm.collectUserValidDevices("non-existent-user", &model.Event{})
		require.Nil(t, devices)
	})

	t.Run("Returns devices that match filters", func(t *testing.T) {
		pubKey := "test-pub-key"
		deviceID := DeviceID("test-device-id")

		event := &model.Event{
			Event: nostr.Event{
				Kind: nostr.KindTextNote,
				Tags: nostr.Tags{
					nostr.Tag{"p", pubKey},
				},
			},
		}

		filters := nostr.Filters{
			{
				Kinds: []int{nostr.KindTextNote},
				Tags:  nostr.TagMap{"p": []nostr.TagValues{{&pubKey}}},
			},
		}

		deviceEvent := &model.Event{
			Event: nostr.Event{
				Tags: nostr.Tags{
					nostr.Tag{"d", string(deviceID)},
				},
			},
		}

		deviceInfo := DeviceInfo{Filters: filters, Event: deviceEvent}

		pm.deviceMutex.Lock()
		pm.userDevicesMap[pubKey] = map[DeviceID]DeviceInfo{
			deviceID: deviceInfo,
		}
		pm.deviceMutex.Unlock()

		devices := pm.collectUserValidDevices(pubKey, event)

		require.Len(t, devices, 1)
		require.Equal(t, deviceEvent, devices[0])
	})
}

func TestHandleEventWithPublicKey(t *testing.T) {
	pm := &PushNotificationManager{
		userDevicesMap: make(map[PublicKey]map[DeviceID]DeviceInfo),
		compressorPool: helperCreateTestCompressorPool(),
	}

	t.Run("Returns nil when reference pubkey is empty", func(t *testing.T) {
		event := &model.Event{
			Event: nostr.Event{
				Kind: nostr.KindTextNote,
			},
		}

		notifications, err := pm.handleEventWithPublicKey(event, NotificationTypeRepost)
		require.NoError(t, err)
		require.Nil(t, notifications)
	})

	t.Run("Returns nil when reference pubkey is the same as master pubkey", func(t *testing.T) {
		masterPubKey := "master-pub-key"
		event := &model.Event{
			Event: nostr.Event{
				Kind:   nostr.KindTextNote,
				PubKey: masterPubKey,
				Tags: nostr.Tags{
					nostr.Tag{"p", masterPubKey},
				},
			},
		}

		notifications, err := pm.handleEventWithPublicKey(event, NotificationTypeRepost)
		require.NoError(t, err)
		require.Nil(t, notifications)
	})
}

func TestPushNotificationManager_SendNotifications(t *testing.T) {
	mockClient := new(MockPushClient)
	client := pn.Client(mockClient)

	pm := &PushNotificationManager{
		userDevicesMap:         make(map[PublicKey]map[DeviceID]DeviceInfo),
		pushNotificationClient: &client,
		compressorPool:         helperCreateTestCompressorPool(),
	}

	t.Run("Returns nil when no notifications", func(t *testing.T) {
		err := pm.sendNotifications(t.Context(), nil, nil)
		require.NoError(t, err)
	})

	t.Run("Sends all notifications and collects errors", func(t *testing.T) {
		singleNotification := &pn.Notification[*DeviceRegistrationEvent]{
			Target: &DeviceRegistrationEvent{
				Event: nostr.Event{
					Tags: nostr.Tags{
						nostr.Tag{"d", "device-1"},
						nostr.Tag{"token", "valid-token"},
					},
				},
			},
			Title: "Test Title",
			Body:  "Test Body",
		}

		topicNotification := &pn.Notification[pn.SubscriptionTopic]{
			Target: "test-topic",
			Title:  "Topic Title",
			Body:   "Topic Body",
		}

		mockClient.On("SendSingle", t.Context(), singleNotification).Return(nil)
		mockClient.On("SendTopic", t.Context(), topicNotification).Return(nil)

		err := pm.sendNotifications(t.Context(), []*pn.Notification[*DeviceRegistrationEvent]{singleNotification},
			[]*pn.Notification[pn.SubscriptionTopic]{topicNotification})

		require.NoError(t, err)
		mockClient.AssertExpectations(t)
	})

	t.Run("Handles errors from notifications", func(t *testing.T) {
		singleNotification := &pn.Notification[*DeviceRegistrationEvent]{
			Target: &DeviceRegistrationEvent{
				Event: nostr.Event{
					Tags: nostr.Tags{
						nostr.Tag{"d", "device-2"},
						nostr.Tag{"token", "invalid-token"},
					},
				},
			},
		}

		sendError := errors.New("failed to send notification")
		mockClient.On("SendSingle", t.Context(), singleNotification).Return(sendError)

		require.Error(t, pm.sendNotifications(t.Context(), []*pn.Notification[*DeviceRegistrationEvent]{singleNotification}, nil))
		mockClient.AssertExpectations(t)
	})
}

func TestPushNotificationManager_HandleInvalidDeviceTokens(t *testing.T) {
	mockClient := new(MockPushClient)
	client := pn.Client(mockClient)

	pm := &PushNotificationManager{
		userDevicesMap:         make(map[PublicKey]map[DeviceID]DeviceInfo),
		pushNotificationClient: &client,
		compressorPool:         helperCreateTestCompressorPool(),
	}

	t.Run("Returns nil when no invalid devices", func(t *testing.T) {
		err := pm.handleInvalidDeviceTokens(t.Context(), nil)
		require.NoError(t, err)
	})
}

func TestPushNotificationManager_AcceptEvents(t *testing.T) {
	mockClient := new(MockPushClient)
	client := pn.Client(mockClient)

	pm := &PushNotificationManager{
		userDevicesMap:         make(map[PublicKey]map[DeviceID]DeviceInfo),
		pushNotificationClient: &client,
		compressorPool:         helperCreateTestCompressorPool(),
	}

	t.Run("Returns nil when no events", func(t *testing.T) {
		err := pm.AcceptEvents(t.Context(), nil)
		require.NoError(t, err)
	})
}

func TestPushNotificationManager_ProcessEvent(t *testing.T) {
	mockClient := new(MockPushClient)
	client := pn.Client(mockClient)

	pm := &PushNotificationManager{
		userDevicesMap:         make(map[PublicKey]map[DeviceID]DeviceInfo),
		pushNotificationClient: &client,
		compressorPool:         helperCreateTestCompressorPool(),
	}

	t.Run("Handles TextNote with q tag correctly", func(t *testing.T) {
		event := &model.Event{
			Event: nostr.Event{
				ID:   "test-event-id",
				Kind: nostr.KindTextNote,
				Tags: nostr.Tags{
					nostr.Tag{"q", "query-value"},
					nostr.Tag{"p", "recipient-pubkey"},
				},
			},
		}

		notifications, err := pm.processEvent(t.Context(), event)
		require.NoError(t, err)
		require.Nil(t, notifications)
	})

	t.Run("Handles GiftWrap event correctly", func(t *testing.T) {
		event := &model.Event{
			Event: nostr.Event{
				ID:      "test-event-id",
				Kind:    nostr.KindGiftWrap,
				Content: "encrypted-content",
			},
		}

		notifications, err := pm.processEvent(t.Context(), event)
		require.NoError(t, err)
		require.Nil(t, notifications)
	})

	t.Run("Returns nil for unsupported event kinds", func(t *testing.T) {
		event := &model.Event{
			Event: nostr.Event{
				ID:   "test-event-id",
				Kind: 999,
			},
		}

		notifications, err := pm.processEvent(t.Context(), event)
		require.NoError(t, err)
		require.Nil(t, notifications)
	})

	t.Run("Handles valid GenericRepost event correctly", func(t *testing.T) {
		repostedEvent := &model.Event{
			Event: nostr.Event{
				Kind: model.CustomIONKindEditableTextNote,
			},
		}

		repostedEventJSON, err := json.Marshal(repostedEvent)
		require.NoError(t, err)

		event := &model.Event{
			Event: nostr.Event{
				ID:      "test-repost-id",
				Kind:    nostr.KindGenericRepost,
				Content: string(repostedEventJSON),
				Tags: nostr.Tags{
					nostr.Tag{"p", "recipient-pubkey"},
				},
			},
		}

		notifications, err := pm.processEvent(t.Context(), event)
		require.NoError(t, err)
		require.Nil(t, notifications)
	})

	t.Run("Handles non-editable GenericRepost event correctly", func(t *testing.T) {
		repostedEvent := &model.Event{
			Event: nostr.Event{
				Kind: nostr.KindTextNote,
			},
		}

		repostedEventJSON, err := json.Marshal(repostedEvent)
		require.NoError(t, err)

		event := &model.Event{
			Event: nostr.Event{
				ID:      "test-repost-id",
				Kind:    nostr.KindGenericRepost,
				Content: string(repostedEventJSON),
				Tags: nostr.Tags{
					nostr.Tag{"p", "recipient-pubkey"},
				},
			},
		}

		notifications, err := pm.processEvent(t.Context(), event)
		require.NoError(t, err)
		require.Nil(t, notifications)
	})
}

func TestPushNotificationManager_CollectNotifications(t *testing.T) {
	mockClient := new(MockPushClient)
	client := pn.Client(mockClient)

	pm := &PushNotificationManager{
		userDevicesMap:         make(map[PublicKey]map[DeviceID]DeviceInfo),
		pushNotificationClient: &client,
		compressorPool:         helperCreateTestCompressorPool(),
	}

	t.Run("Returns empty when no events", func(t *testing.T) {
		single, topic, err := pm.collectNotifications(t.Context(), nil)
		require.NoError(t, err)
		require.Empty(t, single)
		require.Empty(t, topic)
	})
	t.Run("Skips unsupported kinds", func(t *testing.T) {
		unsupported := &model.Event{
			Event: nostr.Event{
				ID:   "unsupported-kind",
				Kind: 9999,
			},
		}

		single, topic, err := pm.collectNotifications(t.Context(), []*model.Event{unsupported})
		require.NoError(t, err)
		require.Empty(t, single)
		require.Empty(t, topic)
	})

	t.Run("Processes multiple events correctly", func(t *testing.T) {
		events := []*model.Event{
			{
				Event: nostr.Event{
					ID:   "test-event-1",
					Kind: nostr.KindTextNote,
					Tags: nostr.Tags{
						nostr.Tag{"q", "query-value"},
						nostr.Tag{"p", "recipient-pubkey"},
					},
				},
			},
			{
				Event: nostr.Event{
					ID:   "test-event-2",
					Kind: nostr.KindRepost,
					Tags: nostr.Tags{
						nostr.Tag{"p", "recipient-pubkey"},
					},
				},
			},
		}

		single, topic, err := pm.collectNotifications(t.Context(), events)
		require.NoError(t, err)
		require.Empty(t, single)
		require.Empty(t, topic)
	})

	t.Run("Processes ephemeral events correctly", func(t *testing.T) {
		mainEventID := "main-event-id"
		mainEvent := &model.Event{
			Event: nostr.Event{
				ID:   mainEventID,
				Kind: nostr.KindTextNote,
				Tags: nostr.Tags{
					nostr.Tag{"p", "recipient-pubkey"},
				},
			},
		}

		ephemeralEvent := &model.Event{
			Event: nostr.Event{
				Kind: model.CustomIONKindEphemeralEmbedding,
				Tags: nostr.Tags{
					nostr.Tag{"e", mainEventID},
				},
			},
		}

		events := []*model.Event{mainEvent, ephemeralEvent}

		single, topic, err := pm.collectNotifications(t.Context(), events)
		require.NoError(t, err)
		require.Empty(t, single)
		require.Empty(t, topic)
	})

	t.Run("Processes ephemeral events with a tag correctly", func(t *testing.T) {
		pubKey := "test-pub-key"
		mainEvent := &model.Event{
			Event: nostr.Event{
				ID:     "main-event-id",
				Kind:   nostr.KindTextNote,
				PubKey: pubKey,
			},
		}

		ephemeralEvent := &model.Event{
			Event: nostr.Event{
				Kind: model.CustomIONKindEphemeralEmbedding,
				Tags: nostr.Tags{
					nostr.Tag{"a", fmt.Sprintf("30023:%v", pubKey)},
				},
			},
		}

		events := []*model.Event{mainEvent, ephemeralEvent}

		single, topic, err := pm.collectNotifications(t.Context(), events)
		require.NoError(t, err)
		require.Empty(t, single)
		require.Empty(t, topic)
	})

	t.Run("Skips ephemeral events when shouldSkipEphemeralEvent returns true", func(t *testing.T) {
		mainEvent := &model.Event{
			Event: nostr.Event{
				ID:   "main-event-id",
				Kind: model.CustomIONKindEphemeralEmbedding,
				Tags: nostr.Tags{
					nostr.Tag{"e", "some-other-event-id"},
				},
			},
		}

		events := []*model.Event{mainEvent}

		single, topic, err := pm.collectNotifications(t.Context(), events)
		require.NoError(t, err)
		require.Empty(t, single)
		require.Empty(t, topic)
	})

	t.Run("Processes events without ephemeral events correctly", func(t *testing.T) {
		recipientPubKey := "recipient-pubkey-for-process-test"
		devicePubKey := "device-pubkey-for-process-test"
		deviceID := "device-id-for-process-test"
		deviceTags := nostr.Tags{
			{"d", deviceID},
			{"t", "ios"},
			{"token", "test-token-for-process-test"},
		}

		deviceEvent := &model.Event{
			Event: nostr.Event{
				ID:     "device-event-id-for-process-test",
				PubKey: devicePubKey,
				Tags:   deviceTags,
			},
		}
		pm.deviceMutex.Lock()
		if _, ok := pm.userDevicesMap[recipientPubKey]; !ok {
			pm.userDevicesMap[recipientPubKey] = make(map[DeviceID]DeviceInfo)
		}
		pm.userDevicesMap[recipientPubKey][DeviceID(deviceID)] = DeviceInfo{
			Event: deviceEvent,
			Filters: nostr.Filters{
				{
					Kinds: []int{nostr.KindTextNote},
				},
			},
		}
		pm.deviceMutex.Unlock()

		mainEvent := &model.Event{
			Event: nostr.Event{
				ID:   "main-event-id",
				Kind: nostr.KindTextNote,
				Tags: nostr.Tags{
					nostr.Tag{"p", recipientPubKey},
				},
			},
		}

		events := []*model.Event{mainEvent}

		single, topic, err := pm.collectNotifications(t.Context(), events)
		require.NoError(t, err)
		require.NotEmpty(t, single, "Should process events even without ephemeral events")
		require.Empty(t, topic)
	})

	t.Run("Processes KindGiftWrap events correctly without requiring ephemeral events", func(t *testing.T) {
		recipientPubKey := "recipient-pubkey"
		devicePubKey := "device-pubkey"
		deviceID := "device-id"
		deviceTags := nostr.Tags{
			{"d", deviceID},
			{"t", "ios"},
			{"token", "test-token"},
		}

		deviceEvent := &model.Event{
			Event: nostr.Event{
				ID:     "device-event-id",
				PubKey: devicePubKey,
				Tags:   deviceTags,
			},
		}
		pm.deviceMutex.Lock()
		if _, ok := pm.userDevicesMap[recipientPubKey]; !ok {
			pm.userDevicesMap[recipientPubKey] = make(map[DeviceID]DeviceInfo)
		}
		pm.userDevicesMap[recipientPubKey][DeviceID(deviceID)] = DeviceInfo{Event: deviceEvent}
		pm.deviceMutex.Unlock()

		giftWrapEvent := &model.Event{
			Event: nostr.Event{
				ID:      "gift-wrap-id",
				Kind:    nostr.KindGiftWrap,
				Content: "encrypted-content",
				Tags: nostr.Tags{
					nostr.Tag{"k", strconv.Itoa(nostr.KindDirectMessage)},
					nostr.Tag{"p", recipientPubKey, "", devicePubKey},
				},
			},
		}

		events := []*model.Event{giftWrapEvent}

		single, topic, err := pm.collectNotifications(t.Context(), events)
		require.NoError(t, err)
		require.NotEmpty(t, single)
		require.Empty(t, topic)
	})
}

func TestShouldProcessGenericRepostEvent(t *testing.T) {
	t.Parallel()

	t.Run("valid_editable_text_note_repost", func(t *testing.T) {
		t.Parallel()

		repostedEvent := &model.Event{
			Event: nostr.Event{
				Kind: model.CustomIONKindEditableTextNote,
			},
		}

		repostedEventJSON, err := json.Marshal(repostedEvent)
		require.NoError(t, err)

		event := &model.Event{
			Event: nostr.Event{
				Kind:    nostr.KindGenericRepost,
				Content: string(repostedEventJSON),
			},
		}

		shouldProcess, err := shouldProcessGenericRepostEvent(event)
		require.NoError(t, err)
		require.True(t, shouldProcess)
	})

	t.Run("non_editable_text_note_repost", func(t *testing.T) {
		t.Parallel()

		repostedEvent := &model.Event{
			Event: nostr.Event{
				Kind: nostr.KindTextNote,
			},
		}

		repostedEventJSON, err := json.Marshal(repostedEvent)
		require.NoError(t, err)

		event := &model.Event{
			Event: nostr.Event{
				Kind:    nostr.KindGenericRepost,
				Content: string(repostedEventJSON),
			},
		}

		shouldProcess, err := shouldProcessGenericRepostEvent(event)
		require.NoError(t, err)
		require.False(t, shouldProcess)
	})
}

func TestGetTranslationWithRelevantInfo(t *testing.T) {
	pm := &PushNotificationManager{
		userDevicesMap: make(map[PublicKey]map[DeviceID]DeviceInfo),
		compressorPool: helperCreateTestCompressorPool(),
	}
	t.Run("Returns default translation", func(t *testing.T) {
		translation := pm.getTranslation(NotificationTypeReaction)
		require.Equal(t, DefaultTranslations[NotificationTypeReaction].Title, translation.Title)
		require.Equal(t, DefaultTranslations[NotificationTypeReaction].Body, translation.Body)
		require.Equal(t, DefaultTranslations[NotificationTypeReaction].ImageURL, translation.ImageURL)

		translation = pm.getTranslation(NotificationTypeMentionReply)
		require.Equal(t, DefaultTranslations[NotificationTypeMentionReply].Title, translation.Title)
		require.Equal(t, DefaultTranslations[NotificationTypeMentionReply].Body, translation.Body)
		require.Equal(t, DefaultTranslations[NotificationTypeMentionReply].ImageURL, translation.ImageURL)

		translation = pm.getTranslation(NotificationTypeNewFollower)
		require.Equal(t, DefaultTranslations[NotificationTypeNewFollower].Title, translation.Title)
		require.Equal(t, DefaultTranslations[NotificationTypeNewFollower].Body, translation.Body)
		require.Equal(t, DefaultTranslations[NotificationTypeNewFollower].ImageURL, translation.ImageURL)

		translation = pm.getTranslation(NotificationTypeRepost)
		require.Equal(t, DefaultTranslations[NotificationTypeRepost].Title, translation.Title)
		require.Equal(t, DefaultTranslations[NotificationTypeRepost].Body, translation.Body)
		require.Equal(t, DefaultTranslations[NotificationTypeRepost].ImageURL, translation.ImageURL)

		translation = pm.getTranslation(NotificationTypeReaction)
		require.Equal(t, DefaultTranslations[NotificationTypeReaction].Title, translation.Title)
		require.Equal(t, DefaultTranslations[NotificationTypeReaction].Body, translation.Body)
		require.Equal(t, DefaultTranslations[NotificationTypeReaction].ImageURL, translation.ImageURL)
	})
}
func TestShouldSkipEphemeralEventForGiftWrap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		kind     int
		expected bool
	}{
		{"GiftWrap", nostr.KindGiftWrap, true},
		{"SystemMessage", model.CustomIONSystemMessage, true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			event := &model.Event{
				Event: nostr.Event{
					Kind: tc.kind,
					Tags: nostr.Tags{
						nostr.Tag{"k", strconv.Itoa(tc.kind)},
						nostr.Tag{"p", "recipient_pubkey", "device_pubkey"},
					},
				},
			}

			result := shouldSkipEphemeralEvent(event)
			require.Equal(t, tc.expected, result, "Unexpected result for kind %d (%s)", tc.kind, tc.name)
		})
	}
}

func TestProcessEventWithReaction(t *testing.T) {
	t.Parallel()

	pm := &PushNotificationManager{
		userDevicesMap: make(map[PublicKey]map[DeviceID]DeviceInfo),
		relayURL:       testRelayURL,
		compressorPool: helperCreateTestCompressorPool(),
	}
	recipientPubKey := "recipient_master_pubkey"
	deviceID := "device1"
	devicePubKey := "device_pubkey"
	senderPubKey := "sender_pubkey"

	filterJSON := `[{"kinds":[7],"#p":["recipient_master_pubkey"]}]`
	var filters nostr.Filters
	require.NoError(t, json.Unmarshal([]byte(filterJSON), &filters))

	deviceTags := nostr.Tags{
		{"t", "ios"},
		{"d", deviceID},
		{"relay", "wss://relay.example.com"},
		{"token", "token1"},
	}

	deviceEvent := &model.Event{
		Event: nostr.Event{
			ID:      "test_id_" + deviceID,
			PubKey:  devicePubKey,
			Kind:    model.CustomIONKindDeviceRegistration,
			Content: filterJSON,
			Tags:    deviceTags,
		},
	}

	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent))

	pm.deviceMutex.Lock()
	deviceInfo, ok := pm.userDevicesMap[devicePubKey][DeviceID(deviceID)]
	require.True(t, ok, "Device should exist in userDevicesMap")

	if _, ok := pm.userDevicesMap[recipientPubKey]; !ok {
		pm.userDevicesMap[recipientPubKey] = make(map[DeviceID]DeviceInfo)
	}
	pm.userDevicesMap[recipientPubKey][DeviceID(deviceID)] = deviceInfo
	pm.deviceMutex.Unlock()

	event := &model.Event{
		Event: nostr.Event{
			ID:      "reaction-event-id",
			Kind:    nostr.KindReaction,
			PubKey:  senderPubKey,
			Content: "+",
			Tags: nostr.Tags{
				{"p", recipientPubKey},
				{"e", "original-note-id"},
			},
		},
	}

	match := filters.Match(&event.Event)
	require.True(t, match, "Event should match filter")

	notifications, err := pm.handleEventWithPublicKey(event, NotificationTypeReaction)
	require.NoError(t, err)
	require.NotNil(t, notifications, "Notifications should not be nil when calling handleEventWithPublicKey directly")
	require.Len(t, notifications, 1, "Should create one notification when calling handleEventWithPublicKey directly")

	notification := notifications[0]
	require.Equal(t, DefaultTranslations[NotificationTypeReaction].Title, notification.Title, "Title should match")
	require.Equal(t, DefaultTranslations[NotificationTypeReaction].Body, notification.Body, "Body should match")
	require.Equal(t, deviceEvent, notification.Target, "Target should be the device event")
	require.Contains(t, notification.Data, "event", "Data should contain event")

	compressedEvent, ok := notification.Data["event"].(string)
	require.True(t, ok, "event should be a string")

	decompressedEvent := helperDecompressZlibAndDecodeBase64(t, compressedEvent)
	require.Equal(t, event.String(), string(decompressedEvent), "Decompressed event should match original")
	require.Equal(t, CompressionMethodZlib, notification.Data["compression"], "Compression method should be zlib")

	notificationsFromProcessEvent, err := pm.processEvent(t.Context(), event)
	require.NoError(t, err)
	require.NotNil(t, notificationsFromProcessEvent, "Notifications should not be nil")
	require.Len(t, notificationsFromProcessEvent, 1, "Should create one notification")

	notificationFromProcessEvent := notificationsFromProcessEvent[0]
	require.Equal(t, DefaultTranslations[NotificationTypeReaction].Title, notificationFromProcessEvent.Title, "Title should match")
	require.Equal(t, DefaultTranslations[NotificationTypeReaction].Body, notificationFromProcessEvent.Body, "Body should match")
	require.Equal(t, deviceEvent, notificationFromProcessEvent.Target, "Target should be the device event")
	require.Contains(t, notificationFromProcessEvent.Data, "event", "Data should contain event")

	compressedEvent, ok = notificationFromProcessEvent.Data["event"].(string)
	require.True(t, ok, "event should be a string")

	decompressedEvent = helperDecompressZlibAndDecodeBase64(t, compressedEvent)
	require.Equal(t, event.String(), string(decompressedEvent), "Decompressed event should match original")
	require.Equal(t, CompressionMethodZlib, notificationFromProcessEvent.Data["compression"], "Compression method should be zlib")
}
