// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

func TestHandleChannelMessagesNotification(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:         make(map[DeviceID]DeviceInfo),
		userDevices:     make(map[string][]DeviceID),
		filterToDevices: make(map[NotificationType]map[DeviceID]bool),
	}
	pm.filterToDevices[NotificationTypeChannelMessage] = make(map[DeviceID]bool)

	channelID := "channel_id_123"

	event := helperCreateTestEvent(
		t,
		"test_id",
		"user_pubkey",
		nostr.KindChannelMessage,
		"Channel message content",
		nostr.Tags{
			{"e", channelID, "wss://relay.example.com", "root"},
		},
	)
	pm.devices[DeviceID("device1")] = DeviceInfo{
		DeviceID: "device1",
		FCMToken: "test_token",
		PubKey:   channelID,
		Platform: "ios",
		Filters: nostr.Filters{
			{
				Kinds: []int{nostr.KindChannelMessage},
			},
		},
	}

	pm.userDevices[channelID] = []DeviceID{"device1"}
	pm.filterToDevices[NotificationTypeChannelMessage]["device1"] = true

	notifications := pm.handleChannelMessageNotification(&event.Event)

	require.NotNil(t, notifications)

	require.Len(t, notifications, 1, "Should create one single notification")
	notification := notifications[0]
	require.Equal(t, "New channel message", notification.Title, "Title should match")
	require.Equal(t, "New message in channel", notification.Body, "Body should match")
	require.Equal(t, "test_token", notification.Target.Token, "Token should match")
	require.Equal(t, DeviceID("device1"), notification.Target.DeviceID, "DeviceID should match")

	require.Equal(t, "test_id", notification.Data["eventId"], "EventID should match")
	require.Equal(t, "user_pubkey", notification.Data["authorPubKey"], "Author pubkey should match")
	require.Equal(t, string(NotificationTypeChannelMessage), notification.Data["notificationType"], "Notification type should match")
	require.Equal(t, channelID, notification.Data["channelId"], "Channel ID should match")
	require.Equal(t, "", notification.Data["replyToId"], "ReplyToId should be empty")
	require.Equal(t, "Channel message content", notification.Data["content"], "Content should match")
}

func TestHandleChannelMessagesNotificationWithReply(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:         make(map[DeviceID]DeviceInfo),
		userDevices:     make(map[string][]DeviceID),
		filterToDevices: make(map[NotificationType]map[DeviceID]bool),
	}
	pm.filterToDevices[NotificationTypeChannelMessage] = make(map[DeviceID]bool)

	channelID := "channel_id_123"
	replyToID := "reply_id_456"

	event := helperCreateTestEvent(
		t,
		"test_id",
		"user_pubkey",
		nostr.KindChannelMessage,
		"Reply message content",
		nostr.Tags{
			{"e", channelID, "wss://relay.example.com", "root"},
			{"e", replyToID, "wss://relay.example.com", "reply"},
		},
	)
	pm.devices[DeviceID("device1")] = DeviceInfo{
		DeviceID: "device1",
		FCMToken: "test_token",
		PubKey:   channelID,
		Platform: "ios",
		Filters: nostr.Filters{
			{
				Kinds: []int{nostr.KindChannelMessage},
			},
		},
	}

	pm.userDevices[channelID] = []DeviceID{"device1"}
	pm.filterToDevices[NotificationTypeChannelMessage]["device1"] = true

	notifications := pm.handleChannelMessageNotification(&event.Event)

	require.NotNil(t, notifications)
	require.Len(t, notifications, 1, "Should create one single notification")

	notification := notifications[0]
	require.Equal(t, replyToID, notification.Data["replyToId"], "ReplyToId should match")
}

func TestHandleChannelMessagesNotificationMissingChannelID(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:         make(map[DeviceID]DeviceInfo),
		userDevices:     make(map[string][]DeviceID),
		filterToDevices: make(map[NotificationType]map[DeviceID]bool),
	}
	pm.filterToDevices[NotificationTypeChannelMessage] = make(map[DeviceID]bool)

	event := helperCreateTestEvent(
		t,
		"test_id",
		"user_pubkey",
		nostr.KindChannelMessage,
		"Channel message content",
		nostr.Tags{
			{"e", "some_id", "wss://relay.example.com", "other"},
		},
	)
	notifications := pm.handleChannelMessageNotification(&event.Event)
	require.Nil(t, notifications, "Should not create notifications when channelID is missing")
}

func TestHandleChannelMessagesNotificationNoValidDevices(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:         make(map[DeviceID]DeviceInfo),
		userDevices:     make(map[string][]DeviceID),
		filterToDevices: make(map[NotificationType]map[DeviceID]bool),
	}
	pm.filterToDevices[NotificationTypeChannelMessage] = make(map[DeviceID]bool)

	channelID := "channel_id_123"
	event := helperCreateTestEvent(
		t,
		"test_id",
		"user_pubkey",
		nostr.KindChannelMessage,
		"Channel message content",
		nostr.Tags{
			{"e", channelID, "wss://relay.example.com", "root"},
		},
	)
	notifications := pm.handleChannelMessageNotification(&event.Event)
	require.Empty(t, notifications, "Should not create notifications when no valid devices are found")
}
