// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
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
				require.Equal(t, DefaultTranslations[NotificationTypeReaction].Body(), notification.Data["body"])
			} else {
				require.Equal(t, DefaultTranslations[NotificationTypeReaction].Title(), notification.Title)
				require.Equal(t, DefaultTranslations[NotificationTypeReaction].Body(), notification.Body)
				require.Equal(t, DefaultTranslations[NotificationTypeReaction].ImageURL(), notification.ImageURL)
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

		notifications := pm.handleEventWithPublicKey(event, NotificationTypeRepost)
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

		notifications := pm.handleEventWithPublicKey(event, NotificationTypeRepost)
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
				Kind: model.CustomIONKindEphemeralEmbeddding,
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
				Kind: model.CustomIONKindEphemeralEmbeddding,
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
				Kind: model.CustomIONKindEphemeralEmbeddding,
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

func TestGetTranslationWithRelatedInfo(t *testing.T) {
	pm := &PushNotificationManager{
		userDevicesMap: make(map[PublicKey]map[DeviceID]DeviceInfo),
	}

	t.Run("Returns default translation when no related events", func(t *testing.T) {
		translation := pm.getTranslationWithRelatedInfo(NotificationTypeReaction)

		require.Equal(t, DefaultTranslations[NotificationTypeReaction].Title(), translation.Title)
		require.Equal(t, DefaultTranslations[NotificationTypeReaction].Body(), translation.Body)
		require.Equal(t, DefaultTranslations[NotificationTypeReaction].ImageURL(), translation.ImageURL)
	})

	t.Run("Returns translation with display name when available", func(t *testing.T) {
		profileData := struct {
			Name        string `json:"name,omitempty"`
			DisplayName string `json:"display_name,omitempty"`
		}{
			Name:        "username",
			DisplayName: "User Display Name",
		}

		content, err := json.Marshal(profileData)
		require.NoError(t, err)

		profileEvent := &model.Event{
			Event: nostr.Event{
				Kind:    nostr.KindProfileMetadata,
				Content: string(content),
			},
		}

		translation := pm.getTranslationWithRelatedInfo(NotificationTypeMentionReply, profileEvent)

		require.Equal(t, DefaultTranslations[NotificationTypeMentionReply].Title(), translation.Title)
		require.Equal(t, DefaultTranslations[NotificationTypeMentionReply].Body(profileEvent), translation.Body)
		require.Equal(t, DefaultTranslations[NotificationTypeMentionReply].ImageURL(), translation.ImageURL)
	})

	t.Run("Returns translation with name when display name not available", func(t *testing.T) {
		profileData := struct {
			Name        string `json:"name,omitempty"`
			DisplayName string `json:"display_name,omitempty"`
		}{
			Name: "username",
		}

		content, err := json.Marshal(profileData)
		require.NoError(t, err)

		profileEvent := &model.Event{
			Event: nostr.Event{
				Kind:    nostr.KindProfileMetadata,
				Content: string(content),
			},
		}

		translation := pm.getTranslationWithRelatedInfo(NotificationTypeNewFollower, profileEvent)

		require.Equal(t, DefaultTranslations[NotificationTypeNewFollower].Title(), translation.Title)
		require.Equal(t, DefaultTranslations[NotificationTypeNewFollower].Body(profileEvent), translation.Body)
		require.Equal(t, DefaultTranslations[NotificationTypeNewFollower].ImageURL(), translation.ImageURL)
	})

	t.Run("Returns default translation with 'Someone' when no name or display name", func(t *testing.T) {
		profileData := struct {
			Name        string `json:"name,omitempty"`
			DisplayName string `json:"display_name,omitempty"`
		}{}

		content, err := json.Marshal(profileData)
		require.NoError(t, err)

		profileEvent := &model.Event{
			Event: nostr.Event{
				Kind:    nostr.KindProfileMetadata,
				Content: string(content),
			},
		}

		translation := pm.getTranslationWithRelatedInfo(NotificationTypeRepost, profileEvent)

		require.Equal(t, DefaultTranslations[NotificationTypeRepost].Title(), translation.Title)
		require.Equal(t, DefaultTranslations[NotificationTypeRepost].Body(), translation.Body)
		require.Equal(t, DefaultTranslations[NotificationTypeRepost].ImageURL(), translation.ImageURL)
	})

	t.Run("Returns default translation when profile event has invalid JSON", func(t *testing.T) {
		profileEvent := &model.Event{
			Event: nostr.Event{
				Kind:    nostr.KindProfileMetadata,
				Content: "invalid json",
			},
		}

		translation := pm.getTranslationWithRelatedInfo(NotificationTypeReaction, profileEvent)

		require.Equal(t, DefaultTranslations[NotificationTypeReaction].Title(), translation.Title)
		require.Equal(t, DefaultTranslations[NotificationTypeReaction].Body(), translation.Body)
		require.Equal(t, DefaultTranslations[NotificationTypeReaction].ImageURL(), translation.ImageURL)
	})

	t.Run("Uses first profile metadata event when multiple provided", func(t *testing.T) {
		profileData1 := struct {
			Name        string `json:"name,omitempty"`
			DisplayName string `json:"display_name,omitempty"`
		}{
			DisplayName: "First User",
		}

		content1, err := json.Marshal(profileData1)
		require.NoError(t, err)

		profileEvent1 := &model.Event{
			Event: nostr.Event{
				Kind:    nostr.KindProfileMetadata,
				Content: string(content1),
			},
		}

		profileData2 := struct {
			Name        string `json:"name,omitempty"`
			DisplayName string `json:"display_name,omitempty"`
		}{
			DisplayName: "Second User",
		}

		content2, err := json.Marshal(profileData2)
		require.NoError(t, err)

		profileEvent2 := &model.Event{
			Event: nostr.Event{
				Kind:    nostr.KindProfileMetadata,
				Content: string(content2),
			},
		}

		otherEvent := &model.Event{
			Event: nostr.Event{
				Kind: nostr.KindTextNote,
			},
		}

		translation := pm.getTranslationWithRelatedInfo(NotificationTypeReaction, otherEvent, profileEvent1, profileEvent2)

		require.Equal(t, DefaultTranslations[NotificationTypeReaction].Title(), translation.Title)
		require.Equal(t, DefaultTranslations[NotificationTypeReaction].Body(profileEvent1), translation.Body)
		require.Equal(t, DefaultTranslations[NotificationTypeReaction].ImageURL(), translation.ImageURL)
	})
}

func TestSortEphemeralEvents(t *testing.T) {
	pm := &PushNotificationManager{
		userDevicesMap: make(map[PublicKey]map[DeviceID]DeviceInfo),
	}

	t.Run("Empty events list returns empty maps", func(t *testing.T) {
		ephemeralEvents, nonEphemeralEvents := pm.sortEphemeralEvents(nil)
		require.Empty(t, ephemeralEvents)
		require.Empty(t, nonEphemeralEvents)
	})

	t.Run("Non-ephemeral events are correctly sorted", func(t *testing.T) {
		events := []*model.Event{
			{
				Event: nostr.Event{
					ID:     "event1",
					Kind:   nostr.KindTextNote,
					PubKey: "pub-key-1",
				},
			},
			{
				Event: nostr.Event{
					ID:     "event2",
					Kind:   nostr.KindFollowList,
					PubKey: "pub-key-2",
				},
			},
		}

		ephemeralEvents, nonEphemeralEvents := pm.sortEphemeralEvents(events)
		require.Empty(t, ephemeralEvents)
		require.Len(t, nonEphemeralEvents, 2)
		require.Equal(t, events, nonEphemeralEvents)
	})

	t.Run("Ephemeral events with e tag are correctly sorted", func(t *testing.T) {
		refID := "ref-event-id"
		ephemeralEvent := &model.Event{
			Event: nostr.Event{
				ID:   "ephemeral-event",
				Kind: model.CustomIONKindEphemeralEmbeddding,
				Tags: nostr.Tags{
					nostr.Tag{"e", refID},
				},
			},
		}
		regularEvent := &model.Event{
			Event: nostr.Event{
				ID:     refID,
				Kind:   nostr.KindTextNote,
				PubKey: "pub-key-1",
			},
		}

		events := []*model.Event{ephemeralEvent, regularEvent}

		ephemeralEvents, nonEphemeralEvents := pm.sortEphemeralEvents(events)
		require.Len(t, ephemeralEvents, 1)
		require.Len(t, nonEphemeralEvents, 1)
		require.Equal(t, []*model.Event{ephemeralEvent}, ephemeralEvents[refID])
		require.Equal(t, []*model.Event{regularEvent}, nonEphemeralEvents)
	})

	t.Run("Ephemeral events with a tag are correctly sorted", func(t *testing.T) {
		pubKey := "pub-key-1"
		regularEvent := &model.Event{
			Event: nostr.Event{
				ID:     "regular-event",
				Kind:   nostr.KindTextNote,
				PubKey: "pub",
				Tags:   nostr.Tags{nostr.Tag{"b", pubKey}},
			},
		}
		ephemeralEvent := &model.Event{
			Event: nostr.Event{
				ID:   "ephemeral-event",
				Kind: model.CustomIONKindEphemeralEmbeddding,
				Tags: nostr.Tags{
					nostr.Tag{"a", "30023:" + pubKey + ":some-other-data"},
				},
			},
		}

		events := []*model.Event{ephemeralEvent, regularEvent}

		ephemeralEvents, nonEphemeralEvents := pm.sortEphemeralEvents(events)
		require.Len(t, ephemeralEvents, 1)
		require.Len(t, nonEphemeralEvents, 1)
		require.Equal(t, []*model.Event{ephemeralEvent}, ephemeralEvents[regularEvent.ID])
		require.Equal(t, []*model.Event{regularEvent}, nonEphemeralEvents)
	})

	t.Run("Multiple ephemeral events for same reference are grouped", func(t *testing.T) {
		refID := "ref-event-id"
		ephemeralEvent1 := &model.Event{
			Event: nostr.Event{
				ID:   "ephemeral-event-1",
				Kind: model.CustomIONKindEphemeralEmbeddding,
				Tags: nostr.Tags{
					nostr.Tag{"e", refID},
				},
			},
		}
		ephemeralEvent2 := &model.Event{
			Event: nostr.Event{
				ID:   "ephemeral-event-2",
				Kind: model.CustomIONKindEphemeralEmbeddding,
				Tags: nostr.Tags{
					nostr.Tag{"e", refID},
				},
			},
		}
		regularEvent := &model.Event{
			Event: nostr.Event{
				ID:     refID,
				Kind:   nostr.KindTextNote,
				PubKey: "pub-key-1",
			},
		}

		events := []*model.Event{ephemeralEvent1, ephemeralEvent2, regularEvent}

		ephemeralEvents, nonEphemeralEvents := pm.sortEphemeralEvents(events)
		require.Len(t, ephemeralEvents, 1)
		require.Len(t, nonEphemeralEvents, 1)
		require.Len(t, ephemeralEvents[refID], 2)
		require.Contains(t, ephemeralEvents[refID], ephemeralEvent1)
		require.Contains(t, ephemeralEvents[refID], ephemeralEvent2)
		require.Equal(t, []*model.Event{regularEvent}, nonEphemeralEvents)
	})

	t.Run("Ephemeral events without valid tags are added to nonEphemeral events", func(t *testing.T) {
		ephemeralEvent := &model.Event{
			Event: nostr.Event{
				ID:   "ephemeral-event",
				Kind: model.CustomIONKindEphemeralEmbeddding,
			},
		}
		regularEvent := &model.Event{
			Event: nostr.Event{
				ID:     "regular-event",
				Kind:   nostr.KindTextNote,
				PubKey: "pub-key-1",
			},
		}

		events := []*model.Event{ephemeralEvent, regularEvent}

		ephemeralEvents, nonEphemeralEvents := pm.sortEphemeralEvents(events)
		require.Empty(t, ephemeralEvents)
		require.Len(t, nonEphemeralEvents, 1)
		require.NotContains(t, nonEphemeralEvents, ephemeralEvent)
		require.Contains(t, nonEphemeralEvents, regularEvent)
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

	notifications := pm.handleEventWithPublicKey(event, NotificationTypeReaction)
	require.NotNil(t, notifications, "Notifications should not be nil when calling handleEventWithPublicKey directly")
	require.Len(t, notifications, 1, "Should create one notification when calling handleEventWithPublicKey directly")

	notification := notifications[0]
	require.Equal(t, DefaultTranslations[NotificationTypeReaction].Title(), notification.Title, "Title should match")
	require.Equal(t, DefaultTranslations[NotificationTypeReaction].Body(), notification.Body, "Body should match")
	require.Equal(t, deviceEvent, notification.Target, "Target should be the device event")
	require.Contains(t, notification.Data, "event", "Data should contain event")
	require.Equal(t, event.String(), notification.Data["event"], "Event in data should match original event")

	notificationsFromProcessEvent, err := pm.processEvent(t.Context(), event)
	require.NoError(t, err)
	require.NotNil(t, notificationsFromProcessEvent, "Notifications should not be nil")
	require.Len(t, notificationsFromProcessEvent, 1, "Should create one notification")

	notificationFromProcessEvent := notificationsFromProcessEvent[0]
	require.Equal(t, DefaultTranslations[NotificationTypeReaction].Title(), notificationFromProcessEvent.Title, "Title should match")
	require.Equal(t, DefaultTranslations[NotificationTypeReaction].Body(), notificationFromProcessEvent.Body, "Body should match")
	require.Equal(t, deviceEvent, notificationFromProcessEvent.Target, "Target should be the device event")
	require.Contains(t, notificationFromProcessEvent.Data, "event", "Data should contain event")
	require.Equal(t, event.String(), notificationFromProcessEvent.Data["event"], "Event in data should match original event")
}
