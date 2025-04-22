// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"encoding/json"
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func helperCreateRepostEvent(t *testing.T, id string, reposterPubKey string, originalEventID string, originalAuthorPubKey string, content string) *model.Event {
	t.Helper()

	tags := nostr.Tags{
		{"e", originalEventID},
	}

	if originalAuthorPubKey != "" {
		tags = append(tags, nostr.Tag{"p", originalAuthorPubKey})
	}

	return &model.Event{
		Event: nostr.Event{
			ID:      id,
			PubKey:  reposterPubKey,
			Kind:    nostr.KindRepost,
			Content: content,
			Tags:    tags,
		},
	}
}

func TestHandleRepostNotification(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:     make(map[DeviceID]DeviceInfo),
		userDevices: make(map[string][]DeviceID),
	}

	originalAuthorPubKey := "original_pubkey"
	reposterPubKey := "reposter_pubkey"
	originalPostID := "original_id"

	originalEvent := &model.Event{
		Event: nostr.Event{
			ID:      originalPostID,
			PubKey:  originalAuthorPubKey,
			Kind:    nostr.KindTextNote,
			Content: "Original post content",
		},
	}

	originalEventJSON, err := json.Marshal(originalEvent)
	require.NoError(t, err)

	repostEvent := helperCreateRepostEvent(
		t,
		"repost_id",
		reposterPubKey,
		originalPostID,
		originalAuthorPubKey,
		string(originalEventJSON),
	)

	filters := nostr.Filters{
		{
			Kinds: []int{nostr.KindRepost},
		},
	}

	deviceEvent := helperCreateTestDeviceRegistrationEvent(
		t,
		originalAuthorPubKey,
		"device1",
		[]string{"t", "ios", "token", "test_token"},
		filters,
	)

	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent))

	notifications := pm.handleRepostNotification(repostEvent)

	require.NotNil(t, notifications, "Notifications should not be nil")
	require.Len(t, notifications, 1, "Should create one notification")

	notification := notifications[0]
	require.Equal(t, DefaultTranslations[NotificationTypeRepost].Title, notification.Title, "Title should match")
	require.Equal(t, DefaultTranslations[NotificationTypeRepost].Body, notification.Body, "Body should match")
	require.Equal(t, deviceEvent, notification.Target, "Target should match")

	require.Contains(t, notification.Data, "notificationType", "Data should contain notificationType")
	require.Contains(t, notification.Data, "event", "Data should contain event")

	require.Equal(t, repostEvent.String(), notification.Data["event"], "Event should match")
	require.Equal(t, string(NotificationTypeRepost), notification.Data["notificationType"], "NotificationType should match")
}

func TestHandleSelfRepostNotification(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:     make(map[DeviceID]DeviceInfo),
		userDevices: make(map[string][]DeviceID),
	}

	userPubKey := "self_pubkey"
	originalPostID := "original_id"

	originalEvent := &model.Event{
		Event: nostr.Event{
			ID:      originalPostID,
			PubKey:  userPubKey,
			Kind:    nostr.KindTextNote,
			Content: "Original post content",
		},
	}

	originalEventJSON, err := json.Marshal(originalEvent)
	require.NoError(t, err)

	repostEvent := helperCreateRepostEvent(
		t,
		"self_repost_id",
		userPubKey,
		originalPostID,
		userPubKey,
		string(originalEventJSON),
	)

	filters := nostr.Filters{
		{
			Kinds: []int{nostr.KindRepost},
		},
	}

	deviceEvent := helperCreateTestDeviceRegistrationEvent(
		t,
		userPubKey,
		"device1",
		[]string{"t", "ios", "token", "test_token"},
		filters,
	)

	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent))

	notifications := pm.handleRepostNotification(repostEvent)

	require.Empty(t, notifications, "Should not create notifications for self-reposts")
}

func TestHandleRepostNotificationNoAuthor(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:     make(map[DeviceID]DeviceInfo),
		userDevices: make(map[string][]DeviceID),
	}

	repostEvent := helperCreateRepostEvent(
		t,
		"repost_without_author_id",
		"reposter_pubkey",
		"original_post_id",
		"",
		"",
	)

	notifications := pm.handleRepostNotification(repostEvent)

	require.Nil(t, notifications, "Should not create notifications when there's no post author tag")
}

func TestHandleRepostNotificationGenericRepost(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:     make(map[DeviceID]DeviceInfo),
		userDevices: make(map[string][]DeviceID),
	}

	event := &model.Event{
		Event: nostr.Event{
			ID:      "generic_repost_id",
			PubKey:  "reposter_pubkey",
			Kind:    nostr.KindGenericRepost,
			Content: "",
			Tags: nostr.Tags{
				{"e", "original_post_id"},
				{"p", "post_author_pubkey"},
				{"k", "1"},
			},
		},
	}

	filters := nostr.Filters{
		{
			Kinds: []int{nostr.KindGenericRepost},
		},
	}

	deviceEvent := helperCreateTestDeviceRegistrationEvent(
		t,
		"post_author_pubkey",
		"device1",
		[]string{"t", "ios", "token", "test_token"},
		filters,
	)

	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent))

	notifications := pm.handleRepostNotification(event)

	require.Len(t, notifications, 1, "Should not create notifications for generic reposts with current implementation")
	require.Equal(t, string(NotificationTypeRepost), notifications[0].Data["notificationType"], "NotificationType should match")
	require.Equal(t, event.String(), notifications[0].Data["event"], "Event should match")
}

func TestHandleRepostNotificationNoValidDevices(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:     make(map[DeviceID]DeviceInfo),
		userDevices: make(map[string][]DeviceID),
	}

	repostEvent := helperCreateRepostEvent(
		t,
		"repost_id",
		"reposter_pubkey",
		"original_post_id",
		"author_without_devices",
		"",
	)

	notifications := pm.handleRepostNotification(repostEvent)

	require.Empty(t, notifications, "Should not create notifications when original author has no valid devices")
}

func TestHandleRepostNotificationMultipleDevices(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:     make(map[DeviceID]DeviceInfo),
		userDevices: make(map[string][]DeviceID),
	}

	originalAuthorPubKey := "multi_device_author"
	reposterPubKey := "reposter_pubkey"
	originalPostID := "original_post_id"

	repostEvent := helperCreateRepostEvent(
		t,
		"repost_id",
		reposterPubKey,
		originalPostID,
		originalAuthorPubKey,
		"",
	)

	filters := nostr.Filters{
		{
			Kinds: []int{nostr.KindRepost},
		},
	}

	var devices []*model.Event
	deviceEvent1 := helperCreateTestDeviceRegistrationEvent(
		t,
		originalAuthorPubKey,
		"device1",
		[]string{"t", "ios", "token", "token1"},
		filters,
	)
	devices = append(devices, deviceEvent1)

	deviceEvent2 := helperCreateTestDeviceRegistrationEvent(
		t,
		originalAuthorPubKey,
		"device2",
		[]string{"t", "android", "token", "token2"},
		filters,
	)
	devices = append(devices, deviceEvent2)
	deviceEvent3 := helperCreateTestDeviceRegistrationEvent(
		t,
		originalAuthorPubKey,
		"device3",
		[]string{"t", "web", "token", "token3"},
		filters,
	)
	devices = append(devices, deviceEvent3)

	deviceEvent4 := helperCreateTestDeviceRegistrationEvent(
		t,
		originalAuthorPubKey,
		"device4",
		[]string{"t", "web", "token", "token4", "invalid_token", "true"},
		filters,
	)
	devices = append(devices, deviceEvent4)

	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent1))
	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent2))
	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent3))
	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent4))

	deviceInfo := pm.devices[DeviceID("device4")]
	deviceInfo.Event.NotificationTokenInvalid = true
	pm.devices[DeviceID("device4")] = deviceInfo

	notifications := pm.handleRepostNotification(repostEvent)

	require.NotNil(t, notifications, "Notifications should not be nil")
	require.Len(t, notifications, 3, "Should create three notifications for valid devices")

	for ix, notification := range notifications {
		platform := devices[ix].GetTag("t").Value()
		if platform == "ios" || platform == "web" {
			require.Equal(t, DefaultTranslations[NotificationTypeRepost].Title, notification.Title, "Title should match")
			require.Equal(t, DefaultTranslations[NotificationTypeRepost].Body, notification.Body, "Body should match")
		} else {
			require.Equal(t, DefaultTranslations[NotificationTypeRepost].Title, notification.Data["title"], "Title should match")
			require.Equal(t, DefaultTranslations[NotificationTypeRepost].Body, notification.Data["body"], "Body should match")
			require.Equal(t, "", notification.Title, "Title should match")
			require.Equal(t, "", notification.Body, "Body should match")
		}

		require.Contains(t, repostEvent.String(), notification.Data["event"], "Data should contain event")
		require.Contains(t, notification.Data, "notificationType", "Data should contain notificationType")
		require.Equal(t, string(NotificationTypeRepost), notification.Data["notificationType"], "NotificationType should match")
	}

	for _, notification := range notifications {
		require.NotEqual(t, "device4", notification.Target.GetTag("d").Value(), "Device with invalid token should not receive notification")
	}
}
