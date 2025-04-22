// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
)

func helperCreateSystemNotificationEvent(t *testing.T, id string, pubKey string, content string, notificationType string, targetPubKey string) *model.Event {
	t.Helper()

	tags := nostr.Tags{
		{"type", notificationType},
		{"title", "System " + notificationType},
		{"body", content},
	}

	if targetPubKey != "" {
		tags = append(tags, nostr.Tag{"p", targetPubKey})
	} else {
		tags = append(tags, nostr.Tag{"topic", "all_users"})
	}

	return &model.Event{
		Event: nostr.Event{
			ID:      id,
			PubKey:  pubKey,
			Kind:    model.CustomIONSystemMessage,
			Content: content,
			Tags:    tags,
		},
	}
}

func TestHandleSystemNotification(t *testing.T) {
	t.Parallel()

	pm := &PushNotificationManager{
		devices:     make(map[DeviceID]DeviceInfo),
		userDevices: make(map[string][]DeviceID),
	}

	event := helperCreateSystemNotificationEvent(
		t,
		"test_id",
		"system_pubkey",
		"System announcement",
		"announcement",
		"",
	)

	notifications := pm.handleSystemNotification(event)

	require.Len(t, notifications, 6, "Should return 1 item for system notification")
	require.Equal(t, event.String(), notifications[0].Data["event"], "Event should match")
}

func TestHandleSystemNotificationWithPubKey(t *testing.T) {
	t.Parallel()

	pm := &PushNotificationManager{
		devices:     make(map[DeviceID]DeviceInfo),
		userDevices: make(map[string][]DeviceID),
	}

	targetPubKey := "target_pubkey"
	deviceID := "device1"

	filters := nostr.Filters{
		{
			Kinds: []int{model.CustomIONSystemMessage},
		},
	}

	deviceEvent := helperCreateTestDeviceRegistrationEvent(
		t,
		targetPubKey,
		deviceID,
		[]string{"t", "ios", "token", "test_token"},
		filters,
	)

	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent))

	require.Contains(t, pm.devices, DeviceID(deviceID), "Device should be in devices map")
	require.Contains(t, pm.userDevices[targetPubKey], DeviceID(deviceID), "Device should be in user's devices list")

	event := helperCreateSystemNotificationEvent(
		t,
		"test_id",
		"system_pubkey",
		"Targeted announcement",
		"announcement",
		targetPubKey,
	)

	notifications := pm.handleSystemNotification(event)

	languages := getAvailableLanguages()
	require.Len(t, notifications, len(languages), "Should return notifications for all supported languages")

	for i, lang := range languages {
		require.Equal(t, pn.SubscriptionTopic("system_"+lang), notifications[i].Target, "Target should be system_"+lang)
		require.Equal(t, NotificationTypeSystem, NotificationType(notifications[i].Data["notificationType"].(string)), "Type should be system")
		require.Equal(t, event.String(), notifications[i].Data["event"], "Event should match")
	}
}

func TestHandleSystemNotificationDifferentTypes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		notifType string
	}{
		{
			name:      "Type: announcement",
			notifType: "announcement",
		},
		{
			name:      "Type: maintenance",
			notifType: "maintenance",
		},
		{
			name:      "Type: warning",
			notifType: "warning",
		},
		{
			name:      "Type: info",
			notifType: "info",
		},
		{
			name:      "Type: update",
			notifType: "update",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			pm := &PushNotificationManager{
				devices:     make(map[DeviceID]DeviceInfo),
				userDevices: make(map[string][]DeviceID),
			}

			event := helperCreateSystemNotificationEvent(
				t,
				"test_id_"+tc.notifType,
				"system_pubkey",
				"Message for "+tc.notifType,
				tc.notifType,
				"",
			)

			notifications := pm.handleSystemNotification(event)

			languages := getAvailableLanguages()
			require.Len(t, notifications, len(languages),
				"Should return notifications for all supported languages for type %s", tc.notifType)

			for i, lang := range languages {
				require.Equal(t, pn.SubscriptionTopic("system_"+lang), notifications[i].Target,
					"Target topic should be system_%s for language %s", lang, lang)

				require.Equal(t, NotificationTypeSystem, NotificationType(notifications[i].Data["notificationType"].(string)),
					"Type should be system for notification type %s", tc.notifType)

				require.Equal(t, event.String(), notifications[i].Data["event"],
					"Event string representation should match for notification type %s", tc.notifType)
			}
		})
	}
}
