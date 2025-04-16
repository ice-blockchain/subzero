// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

func TestHandleNewFollowerNotification(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:         make(map[DeviceID]DeviceInfo),
		userDevices:     make(map[string][]DeviceID),
		filterToDevices: make(map[NotificationType]map[DeviceID]bool),
		invalidTokens:   make(map[DeviceID]InvalidTokenInfo),
		translationMgr: &TranslationManager{
			translations: map[NotificationType]map[Language]map[string]string{
				NotificationTypeNewFollower: {
					Language("en"): {
						"title": "New Follower",
						"body":  "You have a new follower",
					},
				},
			},
		},
	}

	pm.filterToDevices[NotificationTypeNewFollower] = make(map[DeviceID]bool)

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

	pm.devices[DeviceID("device1")] = DeviceInfo{
		DeviceID: "device1",
		FCMToken: "test_token",
		PubKey:   "target_pubkey",
		Filters: nostr.Filters{
			{
				Kinds: []int{nostr.KindFollowList},
			},
		},
	}

	pm.userDevices["target_pubkey"] = []DeviceID{"device1"}
	pm.filterToDevices[NotificationTypeNewFollower]["device1"] = true

	notifications := pm.handleNewFollowerNotification(&event.Event, "en")

	require.NotNil(t, notifications)

	require.Len(t, notifications.singleNotifications, 1, "Should create one single notification")
	notification := notifications.singleNotifications[0]
	require.Equal(t, "New Follower", notification.Title, "Title should match")
	require.Equal(t, "You have a new follower", notification.Body, "Body should match")
	require.Equal(t, "test_token", notification.Target.Token, "Token should match")
	require.Equal(t, DeviceID("device1"), notification.Target.DeviceID, "DeviceID should match")
}
