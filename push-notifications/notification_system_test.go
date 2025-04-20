// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"strings"
	"testing"

	"github.com/ice-blockchain/subzero/model"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

func TestHandleSystemNotification(t *testing.T) {
	t.Parallel()

	pm := &PushNotificationManager{
		devices:         make(map[DeviceID]DeviceInfo),
		userDevices:     make(map[string][]DeviceID),
		filterToDevices: make(map[NotificationType]map[DeviceID]bool),
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

	availableLanguages := getAvailableLanguages()
	require.Equal(t, len(availableLanguages), len(notifications), "Should have notification for each supported language")

	topicSeen := make(map[string]bool)

	for _, notification := range notifications {
		topicStr := string(notification.Target)
		require.True(t, strings.HasPrefix(topicStr, "system_"), "Topic should start with 'system_'")

		lang := strings.TrimPrefix(topicStr, "system_")
		topicSeen[lang] = true
	}

	for _, lang := range availableLanguages {
		require.True(t, topicSeen[lang], "Should have notification for language "+lang)
	}
}
