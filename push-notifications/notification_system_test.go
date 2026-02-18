// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func helperCreateSystemNotificationEvent(t *testing.T, id string, pubKey string, content string, notificationType string, targetPubKey string) *model.Event {
	t.Helper()

	tags := model.Tags{
		{"type", notificationType},
		{"title", "System " + notificationType},
		{"body", content},
	}

	if targetPubKey != "" {
		tags = append(tags, model.Tag{"p", targetPubKey})
	} else {
		tags = append(tags, model.Tag{"topic", "all_users"})
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

func TestHandleSystemEvent(t *testing.T) {
	t.Parallel()

	pm := helperNewManager(t)

	t.Run("Basic system event", func(t *testing.T) {
		t.Parallel()

		event := &model.Event{
			Event: nostr.Event{
				ID:      "test_id",
				PubKey:  "system_pubkey",
				Kind:    model.CustomIONSystemMessage,
				Content: "System notification",
				Tags:    model.Tags{{"type", "system_announcement"}},
			},
		}

		expectedTopics := []string{"system_en", "system_zh", "system_es", "system_fr", "system_de", "system_ru"}
		expectedCount := 6

		notifications := pm.handleSystemEvent(event)

		require.NotNil(t, notifications, "Notifications should not be nil")
		require.Len(t, notifications, expectedCount, "Should have correct number of notifications")

		for _, notification := range notifications {
			topicName := string(notification.Target)
			require.Contains(t, expectedTopics, topicName, "Topic name should be in expected list")
			require.Contains(t, notification.Data, "event", "Data should contain event")
			require.NotEmpty(t, notification.Data["event"], "Event data should not be empty")
		}
	})

	t.Run("System event with custom content", func(t *testing.T) {
		t.Parallel()

		event := helperCreateSystemNotificationEvent(
			t,
			"custom_id",
			"system_pubkey",
			"Important update",
			"maintenance",
			"",
		)

		expectedTopics := []string{"system_en", "system_zh", "system_es", "system_fr", "system_de", "system_ru"}
		expectedCount := 6

		notifications := pm.handleSystemEvent(event)

		require.NotNil(t, notifications, "Notifications should not be nil")
		require.Len(t, notifications, expectedCount, "Should have correct number of notifications")

		for _, notification := range notifications {
			topicName := string(notification.Target)
			require.Contains(t, expectedTopics, topicName, "Topic name should be in expected list")
			require.Contains(t, notification.Data, "event", "Data should contain event")
			require.NotEmpty(t, notification.Data["event"], "Event data should not be empty")
		}
	})
}

func TestHandleSystemEventDataFormat(t *testing.T) {
	t.Parallel()

	pm := helperNewManager(t)

	event := &model.Event{
		Event: nostr.Event{
			ID:      "data_format_test",
			PubKey:  "system_pubkey",
			Kind:    model.CustomIONSystemMessage,
			Content: "Test data format",
			Tags:    model.Tags{{"type", "data_format_test"}},
		},
	}

	notifications := pm.handleSystemEvent(event)

	require.NotNil(t, notifications, "Notifications should not be nil")
	require.Len(t, notifications, 6, "Should have 6 notifications")

	for _, notification := range notifications {
		require.Len(t, notification.Data, 1, "Data should contain exactly one entry")

		require.Empty(t, notification.Title, "Title should be empty")
		require.Empty(t, notification.Body, "Body should be empty")
		require.Empty(t, notification.ImageURL, "ImageURL should be empty")
	}
}
