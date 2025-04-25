// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"context"
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

func TestCreateNotifications(t *testing.T) {
	pm := &PushNotificationManager{
		userDevicesMap: make(map[PublicKey]map[DeviceID]DeviceInfo),
	}

	t.Run("Empty device list returns nil", func(t *testing.T) {
		notifications := pm.createNotifications(nil, NotificationTypeReaction, &model.Event{})
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

		notifications := pm.createNotifications(deviceEvents, NotificationTypeReaction, event)
		require.Len(t, notifications, 2)

		for _, notification := range notifications {
			require.Contains(t, notification.Data, "event")
			require.Equal(t, event.String(), notification.Data["event"])
		}

		for _, notification := range notifications {
			deviceType := notification.Target.GetTag("t").Value()
			if deviceType == "android" {
				require.Contains(t, notification.Data, "title")
				require.Contains(t, notification.Data, "body")
				require.Contains(t, notification.Data, "imageUrl")
			} else {
				require.Equal(t, DefaultTranslations[NotificationTypeReaction].Title, notification.Title)
				require.Equal(t, DefaultTranslations[NotificationTypeReaction].Body, notification.Body)
				require.Equal(t, DefaultTranslations[NotificationTypeReaction].ImageURL, notification.ImageURL)
			}
		}
	})
}

func TestCollectUserValidDevices(t *testing.T) {
	pm := &PushNotificationManager{
		userDevicesMap: make(map[PublicKey]map[DeviceID]DeviceInfo),
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

		deviceInfo := DeviceInfo{
			DeviceID: deviceID,
			Filters:  filters,
			Event:    deviceEvent,
		}

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
	}

	t.Run("Returns nil when reference pubkey is empty", func(t *testing.T) {
		event := &model.Event{
			Event: nostr.Event{
				Kind: nostr.KindTextNote,
			},
		}

		notifications := pm.handleEventWithPublicKey(event)
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

		notifications := pm.handleEventWithPublicKey(event)
		require.Nil(t, notifications)
	})
}

func TestPushNotificationManager_SendNotifications(t *testing.T) {
	mockClient := new(MockPushClient)
	client := pn.Client(mockClient)

	pm := &PushNotificationManager{
		userDevicesMap:         make(map[PublicKey]map[DeviceID]DeviceInfo),
		pushNotificationClient: &client,
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

		notifications, err := pm.processEvent(t.Context(), event.Kind, event)
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

		notifications, err := pm.processEvent(t.Context(), event.Kind, event)
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

		notifications, err := pm.processEvent(t.Context(), event.Kind, event)
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
	}

	t.Run("Returns empty when no events", func(t *testing.T) {
		single, topic, err := pm.collectNotifications(t.Context(), nil)
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
}
