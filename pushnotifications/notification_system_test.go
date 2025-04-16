// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"sync"
	"testing"

	"github.com/ice-blockchain/subzero/model"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

func TestHandleSystemNotification(t *testing.T) {
	t.Parallel()
	customTranslationMgr := &TranslationManager{
		mu: sync.RWMutex{},
		translations: map[NotificationType]map[Language]map[string]string{
			NotificationTypeSystem: {
				Language("en"): {
					"title": "System Notification",
					"body":  "System message",
				},
				Language("fr"): {
					"title": "Notification système",
					"body":  "Message système",
				},
			},
		},
	}

	pm := &PushNotificationManager{
		devices:         make(map[DeviceID]DeviceInfo),
		userDevices:     make(map[string][]DeviceID),
		filterToDevices: make(map[NotificationType]map[DeviceID]bool),
		invalidTokens:   make(map[DeviceID]InvalidTokenInfo),
		translationMgr:  customTranslationMgr,
	}

	event := helperCreateTestEvent(
		t,
		"test_id",
		"system_pubkey",
		model.CustomIONSystemMessage,
		"Important system message",
		nostr.Tags{},
	)

	notifications := pm.handleSystemNotification(&event.Event)

	require.NotNil(t, notifications)
	require.Equal(t, 2, len(notifications.topicNotifications))

	foundMessages := make(map[string]bool)

	for _, notification := range notifications.topicNotifications {
		key := notification.Title + "|" + notification.Body
		foundMessages[key] = true
	}

	require.True(t, foundMessages["System Notification|System message"], "There should be an English notification")
	require.True(t, foundMessages["Notification système|Message système"], "There should be a French notification")
}
