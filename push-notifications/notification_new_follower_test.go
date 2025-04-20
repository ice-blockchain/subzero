// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"testing"

	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

type followerNotifier interface {
	getOldFollowListEvent(event *model.Event) *model.Event
	shouldSendNewFollowerNotification(event, oldEvent *model.Event) (bool, string)
	createNewFollowerNotification(event *model.Event, recipientPubKey string) []*pn.Notification[pn.DeviceToken]
}

type mockFollowerNotifier struct {
	shouldSendResult bool
	recipientPubKey  string
	validDevices     bool
}

func (m *mockFollowerNotifier) getOldFollowListEvent(event *model.Event) *model.Event {
	return nil
}

func (m *mockFollowerNotifier) shouldSendNewFollowerNotification(event, oldEvent *model.Event) (bool, string) {
	return m.shouldSendResult, m.recipientPubKey
}

func (m *mockFollowerNotifier) createNewFollowerNotification(event *model.Event, recipientPubKey string) []*pn.Notification[pn.DeviceToken] {
	if !m.validDevices {
		return nil
	}

	data := map[string]interface{}{
		"eventId":          event.ID,
		"authorPubKey":     event.GetMasterPublicKey(),
		"notificationType": string(NotificationTypeNewFollower),
		"content":          event.Content,
	}

	return []*pn.Notification[pn.DeviceToken]{
		{
			Title: "New follower",
			Body:  "Someone is now following you",
			Data:  data,
			Target: pn.DeviceToken{
				Token:    "test_token",
				DeviceID: "device1",
			},
		},
	}
}

func createTestHandleNewFollowerNotification(mock *mockFollowerNotifier) func(*model.Event) []*pn.Notification[pn.DeviceToken] {
	return func(event *model.Event) []*pn.Notification[pn.DeviceToken] {
		oldEvent := mock.getOldFollowListEvent(event)
		shouldSend, recipientPubKey := mock.shouldSendNewFollowerNotification(event, oldEvent)
		if !shouldSend {
			return nil
		}

		return mock.createNewFollowerNotification(event, recipientPubKey)
	}
}

func TestHandleNewFollowerNotification(t *testing.T) {
	t.Parallel()

	event := helperCreateTestEvent(
		t,
		"test_id",
		"follower_pubkey",
		nostr.KindFollowList,
		"Follow list",
		nostr.Tags{
			{"p", "pubkey1"},
			{"p", "pubkey2"},
			{"p", "target_pubkey"},
		},
	)

	mock := &mockFollowerNotifier{
		shouldSendResult: true,
		recipientPubKey:  "target_pubkey",
		validDevices:     true,
	}

	handleNotification := createTestHandleNewFollowerNotification(mock)
	notifications := handleNotification(&event.Event)

	require.NotNil(t, notifications)
	require.Len(t, notifications, 1, "Should create one single notification")

	notification := notifications[0]
	require.Equal(t, "New follower", notification.Title, "Title should match")
	require.Equal(t, "Someone is now following you", notification.Body, "Body should match")
	require.Equal(t, "test_token", notification.Target.Token, "Token should match")
	require.Equal(t, DeviceID("device1"), notification.Target.DeviceID, "DeviceID should match")

	require.Equal(t, "test_id", notification.Data["eventId"], "EventID should match")
	require.Equal(t, "follower_pubkey", notification.Data["authorPubKey"], "Author pubkey should match")
	require.Equal(t, string(NotificationTypeNewFollower), notification.Data["notificationType"], "Notification type should match")
	require.Equal(t, "Follow list", notification.Data["content"], "Content should match")
}

func TestHandleNewFollowerNotificationShouldNotSend(t *testing.T) {
	t.Parallel()

	event := helperCreateTestEvent(
		t,
		"test_id",
		"follower_pubkey",
		nostr.KindFollowList,
		"Follow list",
		nostr.Tags{
			{"p", "pubkey1"},
			{"p", "pubkey2"},
			{"p", "target_pubkey"},
		},
	)

	mock := &mockFollowerNotifier{
		shouldSendResult: false,
		recipientPubKey:  "",
	}

	handleNotification := createTestHandleNewFollowerNotification(mock)
	notifications := handleNotification(&event.Event)

	require.Nil(t, notifications, "Should not create notifications when shouldSendNewFollowerNotification returns false")
}

func TestShouldSendNewFollowerNotification(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{}

	emptyTagsEvent := helperCreateTestEvent(
		t,
		"test_id",
		"follower_pubkey",
		nostr.KindFollowList,
		"Follow list",
		nostr.Tags{},
	)

	shouldSend, recipient := pm.shouldSendNewFollowerNotification(&emptyTagsEvent.Event, nil)
	require.False(t, shouldSend, "Should not send notification when no p-tags")
	require.Empty(t, recipient, "Recipient should be empty when no p-tags")

	selfFollowEvent := helperCreateTestEvent(
		t,
		"test_id",
		"follower_pubkey",
		nostr.KindFollowList,
		"Follow list",
		nostr.Tags{
			{"p", "follower_pubkey"},
		},
	)

	shouldSend, recipient = pm.shouldSendNewFollowerNotification(&selfFollowEvent.Event, nil)
	require.False(t, shouldSend, "Should not send notification when author follows self")
	require.Empty(t, recipient, "Recipient should be empty when author follows self")

	oldEvent := helperCreateTestEvent(
		t,
		"old_id",
		"follower_pubkey",
		nostr.KindFollowList,
		"Old follow list",
		nostr.Tags{
			{"p", "pubkey1"},
			{"p", "pubkey2"},
			{"p", "pubkey3"},
		},
	)

	newEvent := helperCreateTestEvent(
		t,
		"new_id",
		"follower_pubkey",
		nostr.KindFollowList,
		"New follow list",
		nostr.Tags{
			{"p", "pubkey1"},
			{"p", "pubkey2"},
		},
	)

	shouldSend, recipient = pm.shouldSendNewFollowerNotification(&newEvent.Event, &oldEvent.Event)
	require.False(t, shouldSend, "Should not send notification when follow list reduced")
	require.Empty(t, recipient, "Recipient should be empty when follow list reduced")

	validEvent := helperCreateTestEvent(
		t,
		"new_id",
		"follower_pubkey",
		nostr.KindFollowList,
		"New follow list",
		nostr.Tags{
			{"p", "pubkey1"},
			{"p", "pubkey2"},
			{"p", "pubkey3"},
			{"p", "new_pubkey"},
		},
	)

	shouldSend, recipient = pm.shouldSendNewFollowerNotification(&validEvent.Event, &oldEvent.Event)
	require.True(t, shouldSend, "Should send notification when new follower added")
	require.Equal(t, "new_pubkey", recipient, "Recipient should be the last p-tag")
}

func TestHandleNewFollowerNotificationNoValidDevices(t *testing.T) {
	t.Parallel()

	event := helperCreateTestEvent(
		t,
		"test_id",
		"follower_pubkey",
		nostr.KindFollowList,
		"Follow list",
		nostr.Tags{
			{"p", "pubkey1"},
			{"p", "pubkey2"},
			{"p", "target_pubkey"},
		},
	)

	mock := &mockFollowerNotifier{
		shouldSendResult: true,
		recipientPubKey:  "target_pubkey",
		validDevices:     false,
	}

	handleNotification := createTestHandleNewFollowerNotification(mock)
	notifications := handleNotification(&event.Event)

	require.Empty(t, notifications, "Should not create notifications when no valid devices are found")
}
