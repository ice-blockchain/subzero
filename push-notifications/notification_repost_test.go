// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"encoding/json"
	"testing"

	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/validation"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

func TestHandleRepostNotification(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:         make(map[DeviceID]DeviceInfo),
		userDevices:     make(map[string][]DeviceID),
		filterToDevices: make(map[NotificationType]map[DeviceID]bool),
	}

	pm.filterToDevices[NotificationTypeRepost] = make(map[DeviceID]bool)

	repostedEvent := model.Event{
		Event: nostr.Event{
			ID:      "original_id",
			PubKey:  "original_pubkey",
			Kind:    nostr.KindTextNote,
			Content: "Original post content",
		},
	}

	repostedEventJSON, _ := json.Marshal(repostedEvent)

	event := helperCreateTestEvent(
		t,
		"test_id",
		"reposter_pubkey",
		nostr.KindRepost,
		string(repostedEventJSON),
		nostr.Tags{
			{"e", "original_id"},
			{"p", "original_pubkey"},
		},
	)

	pm.devices[DeviceID("device1")] = DeviceInfo{
		Platform: validation.DeviceTokenOSIOS,
		DeviceID: "device1",
		FCMToken: "test_token",
		PubKey:   "original_pubkey",
		Filters: nostr.Filters{
			{
				Kinds: []int{nostr.KindRepost},
			},
		},
	}

	pm.userDevices["original_pubkey"] = []DeviceID{"device1"}
	pm.filterToDevices[NotificationTypeRepost]["device1"] = true

	notifications := pm.handleRepostNotification(&event.Event)

	require.NotNil(t, notifications)

	require.Len(t, notifications, 1, "There should be one single notification")
	notification := notifications[0]
	require.Equal(t, "New repost", notification.Title, "Title should match")
	require.Equal(t, "Someone reposted your post", notification.Body, "Body should match")
	require.Equal(t, "test_token", notification.Target.Token, "Token should match")
	require.Equal(t, DeviceID("device1"), notification.Target.DeviceID, "DeviceID should match")

	require.NotNil(t, notification.Data, "Notification should have data")
	require.Equal(t, "test_id", notification.Data["eventId"], "EventId should match")
	require.Equal(t, "reposter_pubkey", notification.Data["authorPubKey"], "AuthorPubKey should match")
	require.Equal(t, string(NotificationTypeRepost), notification.Data["notificationType"], "NotificationType should match")
	require.Equal(t, "original_id", notification.Data["repostedEventId"], "RepostedEventId should match")
}

func TestHandleSelfRepostNotification(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:         make(map[DeviceID]DeviceInfo),
		userDevices:     make(map[string][]DeviceID),
		filterToDevices: make(map[NotificationType]map[DeviceID]bool),
	}

	pm.filterToDevices[NotificationTypeRepost] = make(map[DeviceID]bool)

	repostedEvent := model.Event{
		Event: nostr.Event{
			ID:      "original_id",
			PubKey:  "self_pubkey",
			Kind:    nostr.KindTextNote,
			Content: "Original post content",
		},
	}

	repostedEventJSON, _ := json.Marshal(repostedEvent)

	event := helperCreateTestEvent(
		t,
		"test_id_self_repost",
		"self_pubkey",
		nostr.KindRepost,
		string(repostedEventJSON),
		nostr.Tags{
			{"e", "original_id"},
			{"p", "self_pubkey"},
		},
	)

	pm.devices[DeviceID("device1")] = DeviceInfo{
		DeviceID: "device1",
		FCMToken: "test_token",
		PubKey:   "self_pubkey",
		Filters: nostr.Filters{
			{
				Kinds: []int{nostr.KindRepost},
			},
		},
	}

	pm.userDevices["self_pubkey"] = []DeviceID{"device1"}
	pm.filterToDevices[NotificationTypeRepost]["device1"] = true

	notifications := pm.handleRepostNotification(&event.Event)

	require.Nil(t, notifications, "Self-reposts should not generate notifications")
}
