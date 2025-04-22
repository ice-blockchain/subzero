// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/validation"
)

func helperCreateFollowListEvent(t *testing.T, id string, pubKey string, followedPubKeys []string) *model.Event {
	t.Helper()

	tags := nostr.Tags{}
	for _, followedPubKey := range followedPubKeys {
		tags = append(tags, nostr.Tag{"p", followedPubKey})
	}

	return &model.Event{
		Event: nostr.Event{
			ID:      id,
			PubKey:  pubKey,
			Kind:    nostr.KindFollowList,
			Content: "Follow list",
			Tags:    tags,
		},
	}
}

func TestShouldSendNewFollowerNotification(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{}

	emptyTagsEvent := helperCreateFollowListEvent(
		t,
		"test_id",
		"follower_pubkey",
		[]string{},
	)

	shouldSend, recipient := pm.shouldSendNewFollowerNotification(emptyTagsEvent, nil)
	require.False(t, shouldSend, "Should not send notification when no p-tags")
	require.Empty(t, recipient, "Recipient should be empty when no p-tags")

	selfFollowEvent := helperCreateFollowListEvent(
		t,
		"test_id",
		"follower_pubkey",
		[]string{"follower_pubkey"},
	)

	shouldSend, recipient = pm.shouldSendNewFollowerNotification(selfFollowEvent, nil)
	require.False(t, shouldSend, "Should not send notification when author follows self")
	require.Empty(t, recipient, "Recipient should be empty when author follows self")

	oldEvent := helperCreateFollowListEvent(
		t,
		"old_id",
		"follower_pubkey",
		[]string{"pubkey1", "pubkey2", "pubkey3"},
	)

	newReducedEvent := helperCreateFollowListEvent(
		t,
		"new_id",
		"follower_pubkey",
		[]string{"pubkey1", "pubkey2"},
	)

	shouldSend, recipient = pm.shouldSendNewFollowerNotification(newReducedEvent, oldEvent)
	require.False(t, shouldSend, "Should not send notification when follow list reduced")
	require.Empty(t, recipient, "Recipient should be empty when follow list reduced")

	newExtendedEvent := helperCreateFollowListEvent(
		t,
		"new_id",
		"follower_pubkey",
		[]string{"pubkey1", "pubkey2", "pubkey3", "new_pubkey"},
	)

	shouldSend, recipient = pm.shouldSendNewFollowerNotification(newExtendedEvent, oldEvent)
	require.True(t, shouldSend, "Should send notification when new follower added")
	require.Equal(t, "new_pubkey", recipient, "Recipient should be the last p-tag")
}

func TestCreateNewFollowerNotification(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:     make(map[DeviceID]DeviceInfo),
		userDevices: make(map[string][]DeviceID),
	}

	followerPubKey := "follower_pubkey"
	targetPubKey := "target_pubkey"

	followListEvent := helperCreateFollowListEvent(
		t,
		"test_id",
		followerPubKey,
		[]string{"pubkey1", "pubkey2", targetPubKey},
	)

	filters := nostr.Filters{
		{
			Kinds: []int{nostr.KindFollowList},
		},
	}

	deviceEvent := helperCreateTestDeviceRegistrationEvent(
		t,
		targetPubKey,
		"device1",
		[]string{"t", "ios", "token", "test_token"},
		filters,
	)

	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent))

	notifications := pm.createNewFollowerNotification(followListEvent, targetPubKey)

	require.NotNil(t, notifications, "Notifications should not be nil")
	require.Len(t, notifications, 1, "Should create one notification")

	notification := notifications[0]
	require.Equal(t, DefaultTranslations[NotificationTypeNewFollower].Title, notification.Title, "Title should match")
	require.Equal(t, DefaultTranslations[NotificationTypeNewFollower].Body, notification.Body, "Body should match")
	require.Equal(t, deviceEvent, notification.Target, "Target should match")

	require.Contains(t, notification.Data, "event", "Data should contain event")
	require.Contains(t, notification.Data, "notificationType", "Data should contain notificationType")
	require.Equal(t, string(NotificationTypeNewFollower), notification.Data["notificationType"], "NotificationType should match")
}

func TestCreateNewFollowerNotificationMultipleDevices(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:     make(map[DeviceID]DeviceInfo),
		userDevices: make(map[string][]DeviceID),
	}

	followerPubKey := "follower_pubkey"
	targetPubKey := "multi_device_target"

	followListEvent := helperCreateFollowListEvent(
		t,
		"test_id",
		followerPubKey,
		[]string{"pubkey1", "pubkey2", targetPubKey},
	)

	filters := nostr.Filters{
		{
			Kinds: []int{nostr.KindFollowList},
		},
	}

	var deviceEvents []*model.Event
	deviceEvent1 := helperCreateTestDeviceRegistrationEvent(
		t,
		targetPubKey,
		"device1",
		[]string{"t", "ios", "token", "token1"},
		filters,
	)
	deviceEvents = append(deviceEvents, deviceEvent1)
	deviceEvent2 := helperCreateTestDeviceRegistrationEvent(
		t,
		targetPubKey,
		"device2",
		[]string{"t", "android", "token", "token2"},
		filters,
	)
	deviceEvents = append(deviceEvents, deviceEvent2)
	deviceEvent3 := helperCreateTestDeviceRegistrationEvent(
		t,
		targetPubKey,
		"device3",
		[]string{"t", "web", "token", "token3"},
		filters,
	)
	deviceEvents = append(deviceEvents, deviceEvent3)
	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent1))
	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent2))
	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent3))

	notifications := pm.createNewFollowerNotification(followListEvent, targetPubKey)

	require.NotNil(t, notifications, "Notifications should not be nil")
	require.Len(t, notifications, 3, "Should create three notifications")

	for ix, notification := range notifications {
		platform := deviceEvents[ix].GetTag("t").Value()
		require.Contains(t, notification.Data, "event", "Data should contain event")
		require.Contains(t, notification.Data, "notificationType", "Data should contain notificationType")
		require.Equal(t, string(NotificationTypeNewFollower), notification.Data["notificationType"], "NotificationType should match")
		require.Equal(t, followListEvent.String(), notification.Data["event"], "Event should match")

		if platform == validation.DeviceTokenOSIOS || platform == validation.DeviceTokenOSWeb {
			require.Equal(t, DefaultTranslations[NotificationTypeNewFollower].Title, notification.Title, "Title should match")
			require.Equal(t, DefaultTranslations[NotificationTypeNewFollower].Body, notification.Body, "Body should match")
		} else if platform == validation.DeviceTokenOSAndroid {
			require.Equal(t, "", notification.Title, "Title should match")
			require.Equal(t, "", notification.Body, "Body should match")
			require.Equal(t, DefaultTranslations[NotificationTypeNewFollower].Title, notification.Data["title"], "Title should match")
			require.Equal(t, DefaultTranslations[NotificationTypeNewFollower].Body, notification.Data["body"], "Body should match")
		}
	}
}

func TestCreateNewFollowerNotificationNoDevices(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:     make(map[DeviceID]DeviceInfo),
		userDevices: make(map[string][]DeviceID),
	}

	followerPubKey := "follower_pubkey"
	targetPubKey := "target_without_devices"

	followListEvent := helperCreateFollowListEvent(
		t,
		"test_id",
		followerPubKey,
		[]string{"pubkey1", "pubkey2", targetPubKey},
	)
	notifications := pm.createNewFollowerNotification(followListEvent, targetPubKey)
	require.Empty(t, notifications, "Should not create notifications when user has no devices")
}
