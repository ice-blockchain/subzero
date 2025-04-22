// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"testing"

	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/validation"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

func helperCreateChannelMessageEvent(t *testing.T, id string, pubKey string, content string, channelID string, replyToID string) *model.Event {
	t.Helper()
	tags := nostr.Tags{
		{"e", channelID, "wss://relay.example.com", "root"},
	}
	if replyToID != "" {
		tags = append(tags, nostr.Tag{"e", replyToID, "wss://relay.example.com", "reply"})
	}

	return &model.Event{
		Event: nostr.Event{
			ID:      id,
			PubKey:  pubKey,
			Kind:    nostr.KindChannelMessage,
			Content: content,
			Tags:    tags,
		},
	}
}

func TestHandleCommunityMessageNotification(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:     make(map[DeviceID]DeviceInfo),
		userDevices: make(map[string][]DeviceID),
	}
	channelID := "channel_id_123"
	event := helperCreateChannelMessageEvent(
		t,
		"test_id",
		"author_pubkey",
		"Hello channel members!",
		channelID,
		"",
	)
	deviceEvent := helperCreateTestDeviceRegistrationEvent(
		t,
		channelID,
		"device1",
		[]string{"t", "ios", "token", "test_token"},
		nostr.Filters{
			{
				Kinds: []int{nostr.KindChannelMessage},
			},
		},
	)

	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent))

	notifications := pm.handleCommunityMessageNotification(event)

	require.NotNil(t, notifications, "Notifications should not be nil")
	require.Len(t, notifications, 1, "Should create one notification")

	notification := notifications[0]
	require.Equal(t, DefaultTranslations[NotificationTypeChannelMessage].Title, notification.Title, "Title should match")
	require.Equal(t, DefaultTranslations[NotificationTypeChannelMessage].Body, notification.Body, "Body should match")
	require.Equal(t, deviceEvent, notification.Target, "Target should match")

	require.Contains(t, notification.Data, "notificationType", "Data should contain notificationType")
	require.Contains(t, notification.Data, "event", "Data should contain channelId")

	require.Equal(t, event.String(), notification.Data["event"], "EventId should match")
	require.Equal(t, string(NotificationTypeChannelMessage), notification.Data["notificationType"], "NotificationType should match")
}

func TestHandleCommunityMessageNotificationWithReply(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:     make(map[DeviceID]DeviceInfo),
		userDevices: make(map[string][]DeviceID),
	}

	channelID := "channel_id_123"
	replyToID := "reply_to_456"

	event := helperCreateChannelMessageEvent(
		t,
		"test_id",
		"author_pubkey",
		"This is a reply message",
		channelID,
		replyToID,
	)
	deviceEvent := helperCreateTestDeviceRegistrationEvent(
		t,
		channelID,
		"device1",
		[]string{"t", "ios", "token", "test_token"},
		nostr.Filters{
			{
				Kinds: []int{nostr.KindChannelMessage},
			},
		},
	)

	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent))
	notifications := pm.handleCommunityMessageNotification(event)

	require.NotNil(t, notifications, "Notifications should not be nil")
	require.Len(t, notifications, 1, "Should create one notification")
	notification := notifications[0]
	require.Contains(t, notification.Data, "event", "Data should contain replyToId")
	require.Equal(t, event.String(), notification.Data["event"], "ReplyToId should match")
}

func TestHandleCommunityMessageNotificationNoValidDevices(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:     make(map[DeviceID]DeviceInfo),
		userDevices: make(map[string][]DeviceID),
	}

	channelID := "channel_id_without_devices"
	event := helperCreateChannelMessageEvent(
		t,
		"test_id",
		"author_pubkey",
		"Message for channel without devices",
		channelID,
		"",
	)
	notifications := pm.handleCommunityMessageNotification(event)
	require.Empty(t, notifications, "Should not create notifications when no valid devices exist")
}

func TestHandleCommunityMessageNotificationMultipleDevices(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:     make(map[DeviceID]DeviceInfo),
		userDevices: make(map[string][]DeviceID),
	}
	channelID := "multi_device_channel"
	event := helperCreateChannelMessageEvent(
		t,
		"test_id",
		"author_pubkey",
		"Message for multiple devices",
		channelID,
		"",
	)
	filters := nostr.Filters{
		{
			Kinds: []int{nostr.KindChannelMessage},
		},
	}
	var deviceEvents []*model.Event
	deviceEvent1 := helperCreateTestDeviceRegistrationEvent(
		t,
		channelID,
		"device1",
		[]string{"t", "ios", "token", "token1"},
		filters,
	)
	deviceEvents = append(deviceEvents, deviceEvent1)

	deviceEvent2 := helperCreateTestDeviceRegistrationEvent(
		t,
		channelID,
		"device2",
		[]string{"t", "android", "token", "token2"},
		filters,
	)
	deviceEvents = append(deviceEvents, deviceEvent2)
	deviceEvent3 := helperCreateTestDeviceRegistrationEvent(
		t,
		channelID,
		"device3",
		[]string{"t", "web", "token", "token3"},
		filters,
	)
	deviceEvents = append(deviceEvents, deviceEvent3)
	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent1))
	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent2))
	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent3))

	notifications := pm.handleCommunityMessageNotification(event)

	require.NotNil(t, notifications, "Notifications should not be nil")
	require.Len(t, notifications, 3, "Should create three notifications")

	for ix, notification := range notifications {
		platform := event.GetTag("t").Value()
		if platform == validation.DeviceTokenOSIOS || platform == validation.DeviceTokenOSWeb {
			require.Equal(t, DefaultTranslations[NotificationTypeChannelMessage].Title, notification.Title, "Title should match")
			require.Equal(t, DefaultTranslations[NotificationTypeChannelMessage].Body, notification.Body, "Body should match")
		} else if platform == validation.DeviceTokenOSAndroid {
			require.Equal(t, DefaultTranslations[NotificationTypeChannelMessage].Title, notification.Data["title"], "Title should match")
			require.Equal(t, DefaultTranslations[NotificationTypeChannelMessage].Body, notification.Data["body"], "Body should match")
			require.Equal(t, "", notification.Title, "Title should match")
			require.Equal(t, "", notification.Body, "Body should match")
		}
		require.Equal(t, event.String(), notification.Data["event"], "event should match")
		require.Equal(t, deviceEvents[ix], notification.Target, "Target should match")
	}
}

func TestHandleCommunityMessageNotificationAuthorDoesNotReceive(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:     make(map[DeviceID]DeviceInfo),
		userDevices: make(map[string][]DeviceID),
	}
	channelID := "self_message_channel"
	authorPubKey := "author_pubkey"
	event := helperCreateChannelMessageEvent(
		t,
		"test_id",
		authorPubKey,
		"Message from author to themself",
		channelID,
		"",
	)
	deviceEvent := helperCreateTestDeviceRegistrationEvent(
		t,
		authorPubKey,
		"device1",
		[]string{"t", "ios", "token", "test_token"},
		nostr.Filters{
			{
				Kinds: []int{nostr.KindChannelMessage},
			},
		},
	)
	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent))
	notifications := pm.handleCommunityMessageNotification(event)
	require.Empty(t, notifications, "Should not create notifications for the author of the message")
}
