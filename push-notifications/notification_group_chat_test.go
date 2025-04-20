// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

func TestHandleGroupChatMessagesNotification(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:         make(map[DeviceID]DeviceInfo),
		userDevices:     make(map[string][]DeviceID),
		filterToDevices: make(map[NotificationType]map[DeviceID]bool),
	}

	pm.filterToDevices[NotificationTypeGroupChatMessage] = make(map[DeviceID]bool)

	groupID := "group_id_123"

	event := helperCreateTestEvent(
		t,
		"test_id",
		"user_pubkey",
		nostr.KindSimpleGroupChatMessage,
		"Group chat message content",
		nostr.Tags{
			{"e", groupID, "wss://relay.example.com", "root"},
		},
	)

	pm.devices[DeviceID("device1")] = DeviceInfo{
		DeviceID: "device1",
		FCMToken: "test_token",
		PubKey:   groupID,
		Platform: "ios",
		Filters: nostr.Filters{
			{
				Kinds: []int{nostr.KindSimpleGroupChatMessage},
			},
		},
	}

	pm.userDevices[groupID] = []DeviceID{"device1"}
	pm.filterToDevices[NotificationTypeGroupChatMessage]["device1"] = true

	notifications := pm.handleGroupChatMessageNotification(&event.Event)

	require.NotNil(t, notifications)

	require.Len(t, notifications, 1, "Should create one single notification")
	notification := notifications[0]
	require.Equal(t, "New group message", notification.Title, "Title should match")
	require.Equal(t, "New message in group", notification.Body, "Body should match")
	require.Equal(t, "test_token", notification.Target.Token, "Token should match")
	require.Equal(t, DeviceID("device1"), notification.Target.DeviceID, "DeviceID should match")

	require.Equal(t, "test_id", notification.Data["eventId"], "EventID should match")
	require.Equal(t, "user_pubkey", notification.Data["authorPubKey"], "Author pubkey should match")
	require.Equal(t, string(NotificationTypeGroupChatMessage), notification.Data["notificationType"], "Notification type should match")
	require.Equal(t, groupID, notification.Data["groupId"], "Group ID should match")
	require.Equal(t, "Group chat message content", notification.Data["content"], "Content should match")
	_, hasReplyTo := notification.Data["replyToId"]
	require.False(t, hasReplyTo, "ReplyToId should not be present")
}

func TestHandleGroupChatMessagesNotificationWithReply(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:         make(map[DeviceID]DeviceInfo),
		userDevices:     make(map[string][]DeviceID),
		filterToDevices: make(map[NotificationType]map[DeviceID]bool),
	}

	pm.filterToDevices[NotificationTypeGroupChatMessage] = make(map[DeviceID]bool)

	groupID := "group_id_123"
	replyToID := "reply_id_456"

	event := helperCreateTestEvent(
		t,
		"test_id",
		"user_pubkey",
		nostr.KindSimpleGroupChatMessage,
		"Reply message content",
		nostr.Tags{
			{"e", groupID, "wss://relay.example.com", "root"},
			{"e", replyToID, "wss://relay.example.com", "reply"},
		},
	)

	pm.devices[DeviceID("device1")] = DeviceInfo{
		DeviceID: "device1",
		FCMToken: "test_token",
		PubKey:   groupID,
		Platform: "ios",
		Filters: nostr.Filters{
			{
				Kinds: []int{nostr.KindSimpleGroupChatMessage},
			},
		},
	}

	pm.userDevices[groupID] = []DeviceID{"device1"}
	pm.filterToDevices[NotificationTypeGroupChatMessage]["device1"] = true

	notifications := pm.handleGroupChatMessageNotification(&event.Event)

	require.NotNil(t, notifications)
	require.Len(t, notifications, 1, "Should create one single notification")

	notification := notifications[0]
	require.Equal(t, replyToID, notification.Data["replyToId"], "ReplyToId should match")
}

func TestHandleGroupChatMessagesNotificationMissingGroupID(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:         make(map[DeviceID]DeviceInfo),
		userDevices:     make(map[string][]DeviceID),
		filterToDevices: make(map[NotificationType]map[DeviceID]bool),
	}

	pm.filterToDevices[NotificationTypeGroupChatMessage] = make(map[DeviceID]bool)

	event := helperCreateTestEvent(
		t,
		"test_id",
		"user_pubkey",
		nostr.KindSimpleGroupChatMessage,
		"Group chat message content",
		nostr.Tags{
			{"e", "some_id", "wss://relay.example.com", "other"},
		},
	)

	notifications := pm.handleGroupChatMessageNotification(&event.Event)

	require.Nil(t, notifications, "Should not create notifications when groupID is missing")
}

func TestHandleGroupChatMessagesNotificationNoValidDevices(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:         make(map[DeviceID]DeviceInfo),
		userDevices:     make(map[string][]DeviceID),
		filterToDevices: make(map[NotificationType]map[DeviceID]bool),
	}

	pm.filterToDevices[NotificationTypeGroupChatMessage] = make(map[DeviceID]bool)

	groupID := "group_id_123"

	event := helperCreateTestEvent(
		t,
		"test_id",
		"user_pubkey",
		nostr.KindSimpleGroupChatMessage,
		"Group chat message content",
		nostr.Tags{
			{"e", groupID, "wss://relay.example.com", "root"},
		},
	)

	notifications := pm.handleGroupChatMessageNotification(&event.Event)

	require.Empty(t, notifications, "Should not create notifications when no valid devices are found")
}
