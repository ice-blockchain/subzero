// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"encoding/json"
	"testing"

	"github.com/ice-blockchain/subzero/model"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

func TestHandleRepostNotification(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:         make(map[DeviceID]DeviceInfo),
		userDevices:     make(map[string][]DeviceID),
		filterToDevices: make(map[NotificationType]map[DeviceID]bool),
		invalidTokens:   make(map[DeviceID]InvalidTokenInfo),
		translationMgr: &TranslationManager{
			translations: map[NotificationType]map[Language]map[string]string{
				NotificationTypeRepost: {
					Language("en"): {
						"title": "Repost Notification",
						"body":  "User reposted your post",
					},
				},
			},
		},
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

	notifications := pm.handleRepostNotification(&event.Event, "en")

	require.NotNil(t, notifications)

	require.Len(t, notifications.singleNotifications, 1, "There should be one single notification")
	notification := notifications.singleNotifications[0]
	require.Equal(t, "Repost Notification", notification.Title, "Title should match")
	require.Equal(t, "User reposted your post", notification.Body, "Body should match")
	require.Equal(t, "test_token", notification.Target.Token, "Token should match")
	require.Equal(t, DeviceID("device1"), notification.Target.DeviceID, "DeviceID should match")
}
