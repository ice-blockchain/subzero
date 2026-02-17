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
	"github.com/panjf2000/ants/v2"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
	"github.com/ice-blockchain/subzero/rq"
)

type MockPushClient struct {
	mock.Mock
}

type MockRQ struct {
	mock.Mock
}

func (m *MockPushClient) SendSingle(ctx context.Context, notification *pn.Notification[*model.Event]) error {
	args := m.Called(ctx, notification)
	return args.Error(0)
}

func (m *MockPushClient) SendTopic(ctx context.Context, notification *pn.Notification[pn.SubscriptionTopic]) error {
	args := m.Called(ctx, notification)
	return args.Error(0)
}

func (m *MockRQ) Start(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockRQ) Stop(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockRQ) Close(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockRQ) HealthCheck(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockRQ) Push(ctx context.Context, jobs ...rq.JobArgs) error {
	args := m.Called(ctx, jobs)
	return args.Error(0)
}

func (m *MockRQ) Register() *rq.Register {
	args := m.Called()
	return args.Get(0).(*rq.Register)
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

func helperNewManager(t testing.TB) *PushNotificationManager {
	t.Helper()
	m, _ := helperNewManagerWithClient(t)
	return m
}

func helperNewManagerWithClient(t testing.TB) (*PushNotificationManager, *MockPushClient) {
	t.Helper()

	mockClient := new(MockPushClient)
	client := pn.Client(mockClient)

	return &PushNotificationManager{
		relayURL:               testRelayURL,
		userDevicesMap:         make(map[string]map[string]DeviceInfo),
		pushNotificationClient: client,
		compressorPool:         helperCreateTestCompressorPool(),
		stats:                  newPushStats(),
		antsPool:               helperCreateTestAntsPool(t),
		rq:                     new(MockRQ),
		privateKey:             model.GeneratePrivateKey(),
		broadcaster: &mockBroadcaster{
			T:    t,
			Chan: make(chan mockedBroadcastEvent, 10),
		},
	}, mockClient
}

func TestCreateNotifications(t *testing.T) {
	t.Parallel()

	pm := helperNewManager(t)
	t.Run("Empty device list returns nil", func(t *testing.T) {
		notifications, err := pm.createNotifications(nil, nil, NotificationTypeReaction, &model.Event{})
		require.NoError(t, err)
		require.Empty(t, notifications.Local)
		require.Empty(t, notifications.Remote)
	})

	t.Run("Creates notifications with correct data", func(t *testing.T) {
		event := &model.Event{Event: nostr.Event{ID: "test-event-id", Content: "test content"}}

		deviceEvents := []*model.Event{
			{
				Event: nostr.Event{
					Tags: model.Tags{
						model.Tag{"t", "ios"},
					},
				},
			},
			{
				Event: nostr.Event{
					Tags: model.Tags{
						model.Tag{"t", "android"},
					},
				},
			},
		}

		notifications, err := pm.createNotifications(deviceEvents, nil, NotificationTypeReaction, event)
		require.NoError(t, err)
		require.Len(t, notifications.Local, 2)

		for _, notification := range notifications.Local {
			require.Contains(t, notification.Data, "event")
			require.Contains(t, notification.Data, "compression")
			require.Equal(t, "zlib", notification.Data["compression"])

			compressedEvent, ok := notification.Data["event"].(string)
			require.True(t, ok, "event should be a string")

			decompressedEvent := helperDecompressZlibAndDecodeBase64(t, compressedEvent)
			require.Equal(t, event.String(), string(decompressedEvent), "Decompressed event should match original")
			require.Equal(t, CompressionMethodZlib, notification.Data["compression"], "Compression method should be zlib")
		}

		for _, notification := range notifications.Local {
			deviceType := notification.Target.GetTag("t").Value()
			if deviceType == "android" {
				require.Empty(t, notification.Title)
				require.Empty(t, notification.Body)
				require.Empty(t, notification.ImageURL)
			} else {
				require.Equal(t, defaultTranslations[NotificationTypeReaction].Title, notification.Title)
				require.Equal(t, defaultTranslations[NotificationTypeReaction].Body, notification.Body)
				require.Equal(t, defaultTranslations[NotificationTypeReaction].ImageURL, notification.ImageURL)
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

		deviceEvents := []*model.Event{
			{
				Event: nostr.Event{
					Tags: model.Tags{
						model.Tag{"t", "ios"},
					},
				},
			},
		}

		notifications, err := pm.createNotifications(deviceEvents, nil, NotificationTypeMentionReply, event, relevantEvents...)
		require.NoError(t, err)
		require.Len(t, notifications.Local, 1)

		require.Contains(t, notifications.Local[0].Data, "relevant_events")
		require.Contains(t, notifications.Local[0].Data, "compression")
		require.Equal(t, "zlib", notifications.Local[0].Data["compression"])

		compressedRelevantEvents, ok := notifications.Local[0].Data["relevant_events"].(string)
		require.True(t, ok, "relevant_events should be a string")

		decompressedEvents := helperDecompressZlibAndDecodeBase64(t, compressedRelevantEvents)
		combinedContent := strings.Join([]string{
			relevantEvents[0].Content,
			relevantEvents[1].Content,
		}, ",")
		require.Equal(t, `[`+combinedContent+`]`, string(decompressedEvents), "Decompressed events should match combined content")
		require.Equal(t, CompressionMethodZlib, notifications.Local[0].Data["compression"], "Compression method should be zlib")
		decompressedStr := string(decompressedEvents)
		require.Contains(t, decompressedStr, `"name":"user1"`)
		require.Contains(t, decompressedStr, `"display_name":"User One"`)
		require.Contains(t, decompressedStr, `"name":"user2"`)
		require.Contains(t, decompressedStr, `"display_name":"User Two"`)
	})
}

func TestCollectUserValidDevices(t *testing.T) {
	t.Parallel()

	pm := helperNewManager(t)

	t.Run("Returns nil when user has no devices", func(t *testing.T) {
		devicesLocal, devicesRemote := pm.collectNotificationDevices("non-existent-user", &model.Event{})
		require.Empty(t, devicesLocal)
		require.Empty(t, devicesRemote)
	})

	t.Run("Returns devices that match filters", func(t *testing.T) {
		pubKey := "test-pub-key"
		deviceID := "test-device-id"

		event := &model.Event{
			Event: nostr.Event{
				Kind: nostr.KindTextNote,
				Tags: model.Tags{
					model.Tag{"p", pubKey},
				},
			},
		}

		filters := model.Filters{
			{
				Kinds: []int{nostr.KindTextNote},
				Tags:  model.TagMap{"p": []model.TagValues{{&pubKey}}},
			},
		}

		deviceEvent := &model.Event{
			Event: nostr.Event{
				Tags: model.Tags{
					model.Tag{"d", string(deviceID)},
				},
			},
		}

		deviceInfo := DeviceInfo{Filters: filters, Event: deviceEvent}

		pm.deviceMutex.Lock()
		pm.userDevicesMap[pubKey] = map[string]DeviceInfo{
			deviceID: deviceInfo,
		}
		pm.deviceMutex.Unlock()

		localDevices, remoteDevices := pm.collectNotificationDevices(pubKey, event)
		require.Len(t, localDevices, 1)
		require.Empty(t, remoteDevices)
		require.Equal(t, deviceEvent, localDevices[0])
	})
}

func TestHandleEventWithPublicKey(t *testing.T) {
	t.Parallel()

	pm := helperNewManager(t)

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
				Tags: model.Tags{
					model.Tag{"p", masterPubKey},
				},
			},
		}

		notifications, err := pm.handleEventWithPublicKey(event, NotificationTypeRepost)
		require.NoError(t, err)
		require.Nil(t, notifications)
	})
}

func TestPushNotificationManager_SendNotifications(t *testing.T) {
	t.Parallel()

	pm, mockClient := helperNewManagerWithClient(t)

	t.Run("Returns nil when no notifications", func(t *testing.T) {
		err := pm.sendNotifications(t.Context(), nil)
		require.NoError(t, err)
	})

	t.Run("Sends all notifications and collects errors", func(t *testing.T) {
		singleNotification := &pn.Notification[*model.Event]{
			Target: &model.Event{
				Event: nostr.Event{
					Tags: model.Tags{
						model.Tag{"d", "device-1"},
						model.Tag{"token", "valid-token"},
					},
				},
			},
			Title:       "Test Title",
			Body:        "Test Body",
			SourceEvent: &model.Event{Event: nostr.Event{Kind: nostr.KindTextNote}},
		}

		topicNotification := &pn.Notification[pn.SubscriptionTopic]{
			Target:      "test-topic",
			Title:       "Topic Title",
			Body:        "Topic Body",
			SourceEvent: &model.Event{Event: nostr.Event{Kind: nostr.KindTextNote}},
		}

		mockClient.On("SendSingle", t.Context(), singleNotification).Return(nil)
		mockClient.On("SendTopic", t.Context(), topicNotification).Return(nil)

		err := pm.sendNotifications(t.Context(), &notificationTargets{
			Topic: []*pn.Notification[pn.SubscriptionTopic]{topicNotification},
			Local: []*pn.Notification[*model.Event]{singleNotification},
		})
		require.NoError(t, err)
		mockClient.AssertExpectations(t)
	})

	t.Run("Handles errors from notifications", func(t *testing.T) {
		singleNotification := &pn.Notification[*model.Event]{
			Target: &model.Event{
				Event: nostr.Event{
					Tags: model.Tags{
						model.Tag{"d", "device-2"},
						model.Tag{"token", "invalid-token"},
					},
				},
			},
			SourceEvent: &model.Event{Event: nostr.Event{Kind: nostr.KindTextNote}},
		}

		sendError := errors.New("failed to send notification")
		mockClient.On("SendSingle", t.Context(), singleNotification).Return(sendError)

		require.Error(t, pm.sendNotifications(t.Context(), &notificationTargets{
			Local: []*pn.Notification[*model.Event]{singleNotification},
		}))
		mockClient.AssertExpectations(t)
	})
}

func TestPushNotificationManager_HandleInvalidDeviceTokens(t *testing.T) {
	t.Parallel()

	pm := helperNewManager(t)

	t.Run("Returns nil when no invalid devices", func(t *testing.T) {
		err := pm.handleInvalidDeviceTokens(t.Context(), nil)
		require.NoError(t, err)
	})
}

func TestPushNotificationManager_AcceptEvents(t *testing.T) {
	t.Parallel()

	pm := helperNewManager(t)

	t.Run("Returns nil when no events", func(t *testing.T) {
		err := pm.AcceptEvents(t.Context(), nil)
		require.NoError(t, err)
	})
}

func TestPushNotificationManager_ProcessEvent(t *testing.T) {
	t.Parallel()

	pm := helperNewManager(t)

	t.Run("Handles TextNote with q tag correctly", func(t *testing.T) {
		event := &model.Event{
			Event: nostr.Event{
				ID:   "test-event-id",
				Kind: nostr.KindTextNote,
				Tags: model.Tags{
					model.Tag{"q", "query-value"},
					model.Tag{"p", "recipient-pubkey"},
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
				Tags: model.Tags{
					model.Tag{"p", "recipient-pubkey"},
				},
			},
		}

		notifications, err := pm.processEvent(t.Context(), event)
		require.NoError(t, err)
		require.Empty(t, notifications)
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
				Tags: model.Tags{
					model.Tag{"p", "recipient-pubkey"},
				},
			},
		}

		notifications, err := pm.processEvent(t.Context(), event)
		require.NoError(t, err)
		require.Nil(t, notifications)
	})
}

func TestPushNotificationManager_CollectNotifications(t *testing.T) {
	t.Parallel()

	pm := helperNewManager(t)

	t.Run("Returns empty when no events", func(t *testing.T) {
		targets, err := pm.collectNotifications(t.Context(), nil)
		require.NoError(t, err)
		require.Empty(t, targets)
	})
	t.Run("Skips unsupported kinds", func(t *testing.T) {
		unsupported := &model.Event{
			Event: nostr.Event{
				ID:   "unsupported-kind",
				Kind: 9999,
			},
		}

		targets, err := pm.collectNotifications(t.Context(), []*model.Event{unsupported})
		require.NoError(t, err)
		require.Empty(t, targets)
	})

	t.Run("Processes multiple events correctly", func(t *testing.T) {
		events := []*model.Event{
			{
				Event: nostr.Event{
					ID:   "test-event-1",
					Kind: nostr.KindTextNote,
					Tags: model.Tags{
						model.Tag{"q", "query-value"},
						model.Tag{"p", "recipient-pubkey"},
					},
				},
			},
			{
				Event: nostr.Event{
					ID:   "test-event-2",
					Kind: nostr.KindRepost,
					Tags: model.Tags{
						model.Tag{"p", "recipient-pubkey"},
					},
				},
			},
		}

		targets, err := pm.collectNotifications(t.Context(), events)
		require.NoError(t, err)
		require.Empty(t, targets)
	})

	t.Run("Processes ephemeral events correctly", func(t *testing.T) {
		mainEventID := "main-event-id"
		mainEvent := &model.Event{
			Event: nostr.Event{
				ID:   mainEventID,
				Kind: nostr.KindTextNote,
				Tags: model.Tags{
					model.Tag{"p", "recipient-pubkey"},
				},
			},
		}

		ephemeralEvent := &model.Event{
			Event: nostr.Event{
				Kind: model.CustomIONKindEphemeralEmbedding,
				Tags: model.Tags{
					model.Tag{"e", mainEventID},
				},
			},
		}

		events := []*model.Event{mainEvent, ephemeralEvent}

		targets, err := pm.collectNotifications(t.Context(), events)
		require.NoError(t, err)
		require.Empty(t, targets)
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
				Tags: model.Tags{
					model.Tag{"a", fmt.Sprintf("30023:%v", pubKey)},
				},
			},
		}

		events := []*model.Event{mainEvent, ephemeralEvent}

		targets, err := pm.collectNotifications(t.Context(), events)
		require.NoError(t, err)
		require.Empty(t, targets)
	})

	t.Run("Skips ephemeral events when shouldSkipEphemeralEvent returns true", func(t *testing.T) {
		mainEvent := &model.Event{
			Event: nostr.Event{
				ID:   "main-event-id",
				Kind: model.CustomIONKindEphemeralEmbedding,
				Tags: model.Tags{
					model.Tag{"e", "some-other-event-id"},
				},
			},
		}

		events := []*model.Event{mainEvent}

		targets, err := pm.collectNotifications(t.Context(), events)
		require.NoError(t, err)
		require.Empty(t, targets)
	})

	t.Run("Processes events without ephemeral events correctly", func(t *testing.T) {
		recipientPubKey := "recipient-pubkey-for-process-test"
		devicePubKey := "device-pubkey-for-process-test"
		deviceID := "device-id-for-process-test"
		deviceTags := model.Tags{
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
			pm.userDevicesMap[recipientPubKey] = make(map[string]DeviceInfo)
		}
		pm.userDevicesMap[recipientPubKey][deviceID] = DeviceInfo{
			Event: deviceEvent,
			Filters: model.Filters{
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
				Tags: model.Tags{
					model.Tag{"p", recipientPubKey},
				},
			},
		}

		events := []*model.Event{mainEvent}

		targets, err := pm.collectNotifications(t.Context(), events)
		require.NoError(t, err)
		require.NotEmpty(t, targets.Local, "Should process events even without ephemeral events")
		require.Empty(t, targets.Topic)
		require.Empty(t, targets.Remote)
	})

	t.Run("Processes KindGiftWrap events correctly without requiring ephemeral events", func(t *testing.T) {
		recipientPubKey := "recipient-pubkey"
		devicePubKey := "device-pubkey"
		deviceID := "device-id"
		deviceTags := model.Tags{
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
			pm.userDevicesMap[recipientPubKey] = make(map[string]DeviceInfo)
		}
		pm.userDevicesMap[recipientPubKey][deviceID] = DeviceInfo{Event: deviceEvent}
		pm.deviceMutex.Unlock()

		giftWrapEvent := &model.Event{
			Event: nostr.Event{
				ID:      "gift-wrap-id",
				Kind:    nostr.KindGiftWrap,
				Content: "encrypted-content",
				Tags: model.Tags{
					model.Tag{"k", strconv.Itoa(nostr.KindDirectMessage)},
					model.Tag{"p", recipientPubKey, "", devicePubKey},
				},
			},
		}

		events := []*model.Event{giftWrapEvent}

		targets, err := pm.collectNotifications(t.Context(), events)
		require.NoError(t, err)
		require.NotEmpty(t, targets.Local)
		require.Empty(t, targets.Topic)
		require.Empty(t, targets.Remote)
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
	t.Parallel()

	pm := helperNewManager(t)
	t.Run("Returns default translation", func(t *testing.T) {
		translation := pm.getTranslation(NotificationTypeReaction)
		require.Equal(t, defaultTranslations[NotificationTypeReaction].Title, translation.Title)
		require.Equal(t, defaultTranslations[NotificationTypeReaction].Body, translation.Body)
		require.Equal(t, defaultTranslations[NotificationTypeReaction].ImageURL, translation.ImageURL)

		translation = pm.getTranslation(NotificationTypeMentionReply)
		require.Equal(t, defaultTranslations[NotificationTypeMentionReply].Title, translation.Title)
		require.Equal(t, defaultTranslations[NotificationTypeMentionReply].Body, translation.Body)
		require.Equal(t, defaultTranslations[NotificationTypeMentionReply].ImageURL, translation.ImageURL)

		translation = pm.getTranslation(NotificationTypeNewFollower)
		require.Equal(t, defaultTranslations[NotificationTypeNewFollower].Title, translation.Title)
		require.Equal(t, defaultTranslations[NotificationTypeNewFollower].Body, translation.Body)
		require.Equal(t, defaultTranslations[NotificationTypeNewFollower].ImageURL, translation.ImageURL)

		translation = pm.getTranslation(NotificationTypeRepost)
		require.Equal(t, defaultTranslations[NotificationTypeRepost].Title, translation.Title)
		require.Equal(t, defaultTranslations[NotificationTypeRepost].Body, translation.Body)
		require.Equal(t, defaultTranslations[NotificationTypeRepost].ImageURL, translation.ImageURL)

		translation = pm.getTranslation(NotificationTypeReaction)
		require.Equal(t, defaultTranslations[NotificationTypeReaction].Title, translation.Title)
		require.Equal(t, defaultTranslations[NotificationTypeReaction].Body, translation.Body)
		require.Equal(t, defaultTranslations[NotificationTypeReaction].ImageURL, translation.ImageURL)
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
					Tags: model.Tags{
						model.Tag{"k", strconv.Itoa(tc.kind)},
						model.Tag{"p", "recipient_pubkey", "device_pubkey"},
					},
				},
			}

			result := shouldSkipEphemeralEvent(event)
			require.Equal(t, tc.expected, result, "Unexpected result for kind %d (%s)", tc.kind, tc.name)
		})
	}
}

func TestProcessDeviceRegistrationEventWithRemoteDevice(t *testing.T) {
	t.Parallel()

	pm := helperNewManager(t)
	ev := helperCreateTestDeviceRegistrationEvent(t, "remote-device-id", "masterkey_deviceID", model.Tags{}, model.Filters{{Kinds: []int{nostr.KindTextNote}}})

	err := pm.processDeviceRegistrationEvent(ev)
	require.NoError(t, err)

	devices, ok := pm.userDevicesMap["masterkey"]
	require.True(t, ok, "masterkey should exist in userDevicesMap")
	require.Len(t, devices, 1, "masterkey should have one device registered")

	deviceInfo, ok := devices["deviceID"]
	require.True(t, ok)
	require.Equal(t, ev, deviceInfo.Event)
	require.True(t, deviceInfo.Remote)
}

func TestProcessEventWithReaction(t *testing.T) {
	t.Parallel()

	pm := helperNewManager(t)
	recipientPubKey := "recipient_master_pubkey"
	deviceID := "device1"
	devicePubKey := "device_pubkey"
	senderPubKey := "sender_pubkey"

	filterJSON := `[{"kinds":[7],"#p":["recipient_master_pubkey"]}]`
	var filters model.Filters
	require.NoError(t, json.Unmarshal([]byte(filterJSON), &filters))

	deviceTags := model.Tags{
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
	deviceInfo, ok := pm.userDevicesMap[devicePubKey][deviceID]
	require.True(t, ok, "Device should exist in userDevicesMap")

	if _, ok := pm.userDevicesMap[recipientPubKey]; !ok {
		pm.userDevicesMap[recipientPubKey] = make(map[string]DeviceInfo)
	}
	pm.userDevicesMap[recipientPubKey][deviceID] = deviceInfo
	pm.deviceMutex.Unlock()

	event := &model.Event{
		Event: nostr.Event{
			ID:      "reaction-event-id",
			Kind:    nostr.KindReaction,
			PubKey:  senderPubKey,
			Content: "+",
			Tags: model.Tags{
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
	require.Len(t, notifications.Local, 1, "Should create one notification when calling handleEventWithPublicKey directly")

	notification := notifications.Local[0]
	require.Equal(t, defaultTranslations[NotificationTypeReaction].Title, notification.Title, "Title should match")
	require.Equal(t, defaultTranslations[NotificationTypeReaction].Body, notification.Body, "Body should match")
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
	require.Len(t, notificationsFromProcessEvent.Local, 1, "Should create one notification")

	notificationFromProcessEvent := notificationsFromProcessEvent.Local[0]
	require.Equal(t, defaultTranslations[NotificationTypeReaction].Title, notificationFromProcessEvent.Title, "Title should match")
	require.Equal(t, defaultTranslations[NotificationTypeReaction].Body, notificationFromProcessEvent.Body, "Body should match")
	require.Equal(t, deviceEvent, notificationFromProcessEvent.Target, "Target should be the device event")
	require.Contains(t, notificationFromProcessEvent.Data, "event", "Data should contain event")

	compressedEvent, ok = notificationFromProcessEvent.Data["event"].(string)
	require.True(t, ok, "event should be a string")

	decompressedEvent = helperDecompressZlibAndDecodeBase64(t, compressedEvent)
	require.Equal(t, event.String(), string(decompressedEvent), "Decompressed event should match original")
	require.Equal(t, CompressionMethodZlib, notificationFromProcessEvent.Data["compression"], "Compression method should be zlib")
}

func helperCreateTestAntsPool(t testing.TB) *ants.Pool {
	t.Helper()

	return globalTestAntsPool
}

func TestCreateRemoteNotifications(t *testing.T) {
	t.Parallel()

	pm := helperNewManager(t)

	var device1, device2 model.Event // 4 total relays, 2 shared, 1 unique.
	device1.Tags = model.Tags{
		{"relay", "wss://relay1.EXAmple.com"},
		{"relay", "wss://relay2.example.com"},
		{"relay", "wss://relay3.example.com"},
	}
	device2.Tags = model.Tags{
		{"relay", "wss://relay1.example.com:4443"},
		{"relay", "wss://relay2.example.com"},
		{"relay", "wss://relay4.example.com"},
	}

	targets := pm.createRemoteNotifications(model.Events{&device1, &device2}, &model.Event{
		Event: nostr.Event{
			ID: "test-event-id",
		},
	})
	require.Len(t, targets, 4, "Should create one notification per unique relay")
	for relayURL := range targets {
		r := strings.ToLower(relayURL)
		switch r {
		case "wss://relay1.example.com", "wss://relay2.example.com", "wss://relay3.example.com", "wss://relay4.example.com":
		default:
			t.Fatalf("Unexpected relay URL in notification: %s", relayURL)
		}
	}
}
