// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
)

func helperCreateCommunityMessageEvent(t *testing.T, id string, pubKey string, content string, communityID string, recipientPubKey string) *model.Event {
	t.Helper()

	tags := nostr.Tags{
		{model.CustomIONTagCommunity, communityID},
	}

	if recipientPubKey != "" {
		tags = append(tags, nostr.Tag{"p", recipientPubKey})
	}

	return &model.Event{
		Event: nostr.Event{
			ID:      id,
			PubKey:  pubKey,
			Kind:    nostr.KindTextNote,
			Content: content,
			Tags:    tags,
		},
	}
}

func helperCreateCommunityDefinitionEvent(t *testing.T, id string, pubKey string, communityID string, commentsEnabled bool) *model.Event {
	t.Helper()

	tags := nostr.Tags{
		{model.CustomIONTagCommunity, communityID},
	}
	if commentsEnabled {
		tags = append(tags, nostr.Tag{"settings", model.CommentsEnabledSettings, "true", "1000"})
	} else {
		tags = append(tags, nostr.Tag{"settings", model.CommentsEnabledSettings, "false", "1000"})
	}

	return &model.Event{
		Event: nostr.Event{
			ID:      id,
			PubKey:  pubKey,
			Kind:    model.CustomIONKindCommunityDefinition,
			Content: "Community definition",
			Tags:    tags,
		},
	}
}

func TestGetCommunityNotificationType(t *testing.T) {
	t.Parallel()

	t.Run("community with comments enabled", func(t *testing.T) {
		t.Parallel()

		ownerPubKey := "owner_pubkey1" + uuid.NewString()
		communityID := "community_id_1_test" + uuid.NewString()

		communityDefEvent := helperCreateCommunityDefinitionEvent(
			t,
			"community_def_id_1_test",
			ownerPubKey,
			communityID,
			true,
		)

		require.NoError(t, query.AcceptEvents(t.Context(), communityDefEvent))
		messageEvent := helperCreateCommunityMessageEvent(
			t,
			"message_id_1_test",
			"author_pubkey",
			"Message content",
			communityID,
			"recipient_pubkey",
		)

		notificationType, err := getCommunityNotificationType(t.Context(), messageEvent)
		require.NoError(t, err, "Should not return error")
		assert.Equal(t, NotificationTypeGroupChatMessage, notificationType, "Should return group chat message type")
	})

	t.Run("community with comments disabled", func(t *testing.T) {
		t.Parallel()

		ownerPubKey := "owner_pubkey2" + uuid.NewString()
		communityID := "community_id_2_test" + uuid.NewString()

		communityDefEvent := helperCreateCommunityDefinitionEvent(
			t,
			"community_def_id_2_test",
			ownerPubKey,
			communityID,
			false,
		)

		require.NoError(t, query.AcceptEvents(t.Context(), communityDefEvent))

		messageEvent := helperCreateCommunityMessageEvent(
			t,
			"message_id_2_test",
			"author_pubkey",
			"Message content",
			communityID,
			"recipient_pubkey",
		)

		notificationType, err := getCommunityNotificationType(t.Context(), messageEvent)
		require.NoError(t, err, "Should not return error")
		assert.Equal(t, NotificationTypeChannelMessage, notificationType, "Should return channel message type")
	})

	t.Run("community without comments settings", func(t *testing.T) {
		t.Parallel()

		ownerPubKey := "owner_pubkey3" + uuid.NewString()
		communityID := "community_id_3_test" + uuid.NewString()

		tags := nostr.Tags{
			{model.CustomIONTagCommunity, communityID},
		}

		communityDefEvent := &model.Event{
			Event: nostr.Event{
				ID:      "community_def_id_3_test",
				PubKey:  ownerPubKey,
				Kind:    model.CustomIONKindCommunityDefinition,
				Content: "Community definition",
				Tags:    tags,
			},
		}

		require.NoError(t, query.AcceptEvents(t.Context(), communityDefEvent))

		messageEvent := helperCreateCommunityMessageEvent(
			t,
			"message_id_3_test",
			"author_pubkey",
			"Message content",
			communityID,
			"recipient_pubkey",
		)

		notificationType, err := getCommunityNotificationType(t.Context(), messageEvent)
		require.NoError(t, err, "Should not return error")
		assert.Equal(t, NotificationTypeChannelMessage, notificationType, "Should return channel message type")
	})

	t.Run("non-existent community", func(t *testing.T) {
		t.Parallel()

		messageEvent := helperCreateCommunityMessageEvent(
			t,
			"message_id_4_test",
			"author_pubkey",
			"Message content",
			"non_existent_community_id_test",
			"recipient_pubkey",
		)

		_, err := getCommunityNotificationType(t.Context(), messageEvent)
		require.Error(t, err, "Should return error for non-existent community")
	})
}

func TestHandleCommunityMessageEvent(t *testing.T) {
	t.Parallel()

	t.Run("message with p-tag", func(t *testing.T) {
		t.Parallel()

		pm := &PushNotificationManager{
			userDevicesMap: make(map[PublicKey]map[DeviceID]DeviceInfo),
			compressorPool: helperCreateTestCompressorPool(),
			stats:          newPushStats(),
		}

		ownerPubKey := "owner_pubkey1" + uuid.NewString()
		authorPubKey := "author_pubkey1" + uuid.NewString()
		recipientPubKey := "recipient_pubkey1" + uuid.NewString()
		communityID := "community_id_5" + uuid.NewString()

		communityDefEvent := helperCreateCommunityDefinitionEvent(
			t,
			"community_def_id_5",
			ownerPubKey,
			communityID,
			true,
		)

		require.NoError(t, query.AcceptEvents(t.Context(), communityDefEvent))

		messageEvent := helperCreateCommunityMessageEvent(
			t,
			"message_id_5",
			authorPubKey,
			"Message content",
			communityID,
			recipientPubKey,
		)

		deviceEvent := helperCreateTestDeviceRegistrationEvent(
			t,
			recipientPubKey,
			"device1",
			nostr.Tags{
				{"t", "ios"},
				{"token", "test_token1"},
			},
			nostr.Filters{
				{
					Kinds: []int{nostr.KindTextNote},
				},
			},
		)

		require.NoError(t, query.AcceptEvents(t.Context(), messageEvent))
		require.NoError(t, query.AcceptEvents(t.Context(), deviceEvent))

		require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent))

		notifications, err := pm.handleCommunityMessageEvent(t.Context(), messageEvent)

		require.NoError(t, err, "Should not return error")
		require.NotNil(t, notifications, "Notifications should not be nil")
		require.Len(t, notifications, 1, "Should create one notification")

		notification := notifications[0]
		assert.Equal(t, defaultTranslations[NotificationTypeGroupChatMessage].Title, notification.Title, "Title should match")
		assert.Equal(t, defaultTranslations[NotificationTypeGroupChatMessage].Body, notification.Body, "Body should match")
		assert.Equal(t, deviceEvent, notification.Target, "Target should match")
		assert.Contains(t, notification.Data, "event", "Data should contain event")
	})

	t.Run("message without p-tag", func(t *testing.T) {
		t.Parallel()

		pm := &PushNotificationManager{
			userDevicesMap: make(map[PublicKey]map[DeviceID]DeviceInfo),
			compressorPool: helperCreateTestCompressorPool(),
			stats:          newPushStats(),
		}

		ownerPubKey := "owner_pubkey2" + uuid.NewString()
		authorPubKey := "author_pubkey2" + uuid.NewString()
		communityID := "community_id_6" + uuid.NewString()

		communityDefEvent := helperCreateCommunityDefinitionEvent(
			t,
			"community_def_id_6",
			ownerPubKey,
			communityID,
			true,
		)

		require.NoError(t, query.AcceptEvents(t.Context(), communityDefEvent))

		messageEvent := helperCreateCommunityMessageEvent(
			t,
			"message_id_6",
			authorPubKey,
			"Message content",
			communityID,
			"",
		)

		notifications, err := pm.handleCommunityMessageEvent(t.Context(), messageEvent)

		require.NoError(t, err, "Should not return error")
		require.Nil(t, notifications, "Notifications should be nil")
	})

	t.Run("message with p-tag containing sender's pubkey", func(t *testing.T) {
		t.Parallel()

		pm := &PushNotificationManager{
			userDevicesMap: make(map[PublicKey]map[DeviceID]DeviceInfo),
			compressorPool: helperCreateTestCompressorPool(),
			stats:          newPushStats(),
		}

		ownerPubKey := "owner_pubkey3" + uuid.NewString()
		authorPubKey := "author_pubkey3" + uuid.NewString()
		communityID := "community_id_7" + uuid.NewString()

		communityDefEvent := helperCreateCommunityDefinitionEvent(
			t,
			"community_def_id_7",
			ownerPubKey,
			communityID,
			true,
		)

		require.NoError(t, query.AcceptEvents(t.Context(), communityDefEvent))

		messageEvent := helperCreateCommunityMessageEvent(
			t,
			"message_id_7",
			authorPubKey,
			"Message content",
			communityID,
			authorPubKey,
		)

		notifications, err := pm.handleCommunityMessageEvent(t.Context(), messageEvent)

		require.NoError(t, err, "Should not return error")
		require.Nil(t, notifications, "Notifications should be nil")
	})

	t.Run("message with non-existent recipient", func(t *testing.T) {
		t.Parallel()

		pm := &PushNotificationManager{
			userDevicesMap: make(map[PublicKey]map[DeviceID]DeviceInfo),
			compressorPool: helperCreateTestCompressorPool(),
			stats:          newPushStats(),
		}

		ownerPubKey := "owner_pubkey4" + uuid.NewString()
		authorPubKey := "author_pubkey4" + uuid.NewString()
		nonExistentRecipientPubKey := "non_existent_recipient" + uuid.NewString()
		communityID := "community_id_8" + uuid.NewString()

		communityDefEvent := helperCreateCommunityDefinitionEvent(
			t,
			"community_def_id_8",
			ownerPubKey,
			communityID,
			true,
		)

		require.NoError(t, query.AcceptEvents(t.Context(), communityDefEvent))

		messageEvent := helperCreateCommunityMessageEvent(
			t,
			"message_id_8",
			authorPubKey,
			"Message content",
			communityID,
			nonExistentRecipientPubKey,
		)

		notifications, err := pm.handleCommunityMessageEvent(t.Context(), messageEvent)

		require.NoError(t, err, "Should not return error")
		require.Empty(t, notifications, "Notifications should be empty for non-existent recipient")
	})

	t.Run("message with recipient having no devices", func(t *testing.T) {
		t.Parallel()

		pm := &PushNotificationManager{
			userDevicesMap: make(map[PublicKey]map[DeviceID]DeviceInfo),
			compressorPool: helperCreateTestCompressorPool(),
			stats:          newPushStats(),
		}

		ownerPubKey := "owner_pubkey5" + uuid.NewString()
		authorPubKey := "author_pubkey5" + uuid.NewString()
		recipientPubKey := "recipient_without_devices" + uuid.NewString()
		communityID := "community_id_9" + uuid.NewString()

		communityDefEvent := helperCreateCommunityDefinitionEvent(
			t,
			"community_def_id_9",
			ownerPubKey,
			communityID,
			true,
		)
		require.NoError(t, query.AcceptEvents(t.Context(), communityDefEvent))

		messageEvent := helperCreateCommunityMessageEvent(
			t,
			"message_id_9",
			authorPubKey,
			"Message content",
			communityID,
			recipientPubKey,
		)

		notifications, err := pm.handleCommunityMessageEvent(t.Context(), messageEvent)

		require.NoError(t, err, "Should not return error")
		require.Empty(t, notifications, "Notifications should be empty for recipient without devices")
	})

	t.Run("message_with_recipient_having_multiple_devices", func(t *testing.T) {
		t.Parallel()

		testSuffix := uuid.NewString()

		pm := &PushNotificationManager{
			userDevicesMap: make(map[PublicKey]map[DeviceID]DeviceInfo),
			compressorPool: helperCreateTestCompressorPool(),
			stats:          newPushStats(),
		}

		communityID := "community_" + testSuffix
		senderPubKey := "sender_pubkey_" + testSuffix
		recipientPubKey := "multi_device_recipient_" + testSuffix

		definitionEvent := helperCreateCommunityDefinitionEvent(t, "definition_id_"+testSuffix, senderPubKey, communityID, true)
		require.NoError(t, query.AcceptEvents(t.Context(), definitionEvent))

		messageEvent := helperCreateCommunityMessageEvent(
			t,
			"message_id_"+testSuffix,
			senderPubKey,
			"Test message content for multi device",
			communityID,
			recipientPubKey,
		)

		filters := nostr.Filters{
			{
				Kinds: []int{nostr.KindTextNote},
			},
		}

		deviceID1 := "device1_" + testSuffix
		deviceID2 := "device2_" + testSuffix
		deviceID3 := "device3_" + testSuffix

		deviceEvent1 := helperCreateTestDeviceRegistrationEvent(
			t,
			recipientPubKey,
			deviceID1,
			nostr.Tags{
				{"t", "ios"},
				{"d", deviceID1},
				{"token", "token1_" + testSuffix},
			},
			filters,
		)

		deviceEvent2 := helperCreateTestDeviceRegistrationEvent(
			t,
			recipientPubKey,
			deviceID2,
			nostr.Tags{
				{"t", "android"},
				{"d", deviceID2},
				{"token", "token2_" + testSuffix},
			},
			filters,
		)

		deviceEvent3 := helperCreateTestDeviceRegistrationEvent(
			t,
			recipientPubKey,
			deviceID3,
			nostr.Tags{
				{"t", "web"},
				{"d", deviceID3},
				{"token", "token3_" + testSuffix},
			},
			filters,
		)

		deviceEvents := []*model.Event{deviceEvent1, deviceEvent2, deviceEvent3}

		for _, device := range deviceEvents {
			require.NoError(t, query.AcceptEvents(t.Context(), device))
			require.NoError(t, pm.processDeviceRegistrationEvent(device))
		}

		require.Len(t, pm.userDevicesMap, 1, "Should have one user in cache")
		require.Contains(t, pm.userDevicesMap, recipientPubKey, "User should be in cache")
		require.Len(t, pm.userDevicesMap[recipientPubKey], 3, "User should have 3 devices")

		require.Contains(t, pm.userDevicesMap[recipientPubKey], DeviceID(deviceID1), "Device 1 should be in cache")
		require.Contains(t, pm.userDevicesMap[recipientPubKey], DeviceID(deviceID2), "Device 2 should be in cache")
		require.Contains(t, pm.userDevicesMap[recipientPubKey], DeviceID(deviceID3), "Device 3 should be in cache")

		require.NoError(t, query.AcceptEvents(t.Context(), messageEvent))

		notifications, err := pm.handleCommunityMessageEvent(t.Context(), messageEvent)

		require.NoError(t, err, "Should not return error")
		require.NotNil(t, notifications, "Notifications should not be nil")
		require.Len(t, notifications, 3, "Should create three notifications (one for each device)")

		deviceTypeMap := make(map[string]*pn.Notification[*DeviceRegistrationEvent])
		for _, notification := range notifications {
			deviceType := notification.Target.GetTag("t").Value()
			deviceTypeMap[deviceType] = notification
		}

		require.Len(t, deviceTypeMap, 3, "Should have notifications for all device types")
		require.Contains(t, deviceTypeMap, "ios", "Should have iOS notification")
		require.Contains(t, deviceTypeMap, "android", "Should have Android notification")
		require.Contains(t, deviceTypeMap, "web", "Should have web notification")

		for _, platform := range []string{"ios", "android", "web"} {
			notification := deviceTypeMap[platform]
			require.Contains(t, notification.Data, "event", "Data should contain event")

			switch platform {
			case model.DeviceTokenOSIOS, model.DeviceTokenOSWeb:
				require.Equal(t, defaultTranslations[NotificationTypeGroupChatMessage].Title, notification.Title, "Title should match")
				require.Equal(t, defaultTranslations[NotificationTypeGroupChatMessage].Body, notification.Body, "Body should match")
			case model.DeviceTokenOSAndroid:
				require.Equal(t, "", notification.Title, "Title should be empty for Android devices")
				require.Equal(t, "", notification.Body, "Body should be empty for Android devices")
			}
		}
	})
}

func TestHandleCommunityMessageEventWithRelevantEvents(t *testing.T) {
	t.Parallel()

	pm := &PushNotificationManager{
		userDevicesMap: make(map[PublicKey]map[DeviceID]DeviceInfo),
		compressorPool: helperCreateTestCompressorPool(),
		stats:          newPushStats(),
	}

	authorPubKey := "author_pubkey_" + uuid.NewString()
	recipientPubKey := "recipient_pubkey_" + uuid.NewString()
	communityID := "community_id_" + uuid.NewString()

	profileData := struct {
		Name        string `json:"name,omitempty"`
		DisplayName string `json:"display_name,omitempty"`
	}{
		Name:        "AuthorUsername",
		DisplayName: "Author Display Name",
	}

	profileJSON, err := json.Marshal(profileData)
	require.NoError(t, err)

	profileEvent := &model.Event{
		Event: nostr.Event{
			ID:      "profile_id_" + uuid.NewString(),
			PubKey:  authorPubKey,
			Kind:    nostr.KindProfileMetadata,
			Content: string(profileJSON),
		},
	}

	messageEvent := helperCreateCommunityMessageEvent(
		t,
		"message_id_"+uuid.NewString(),
		authorPubKey,
		"Message with recipient",
		communityID,
		recipientPubKey,
	)

	deviceID := "device1_" + uuid.NewString()
	deviceEvent := helperCreateTestDeviceRegistrationEvent(
		t,
		recipientPubKey,
		deviceID,
		nostr.Tags{
			{"t", "ios"},
			{"token", "test_token_" + uuid.NewString()},
		},
		nostr.Filters{
			{
				Kinds: []int{nostr.KindTextNote},
				Tags:  nostr.TagMap{"p": []nostr.TagValues{{&recipientPubKey}}},
			},
		},
	)

	pm.deviceMutex.Lock()
	pm.userDevicesMap[recipientPubKey] = map[DeviceID]DeviceInfo{
		DeviceID(deviceID): {
			Filters: nostr.Filters{
				{
					Kinds: []int{nostr.KindTextNote},
					Tags:  nostr.TagMap{"p": []nostr.TagValues{{&recipientPubKey}}},
				},
			},
			Event: deviceEvent,
		},
	}
	pm.deviceMutex.Unlock()

	notifications, err := pm.createNotifications(
		[]*DeviceRegistrationEvent{deviceEvent},
		NotificationTypeMentionReply,
		messageEvent,
		profileEvent,
	)

	require.NoError(t, err)
	require.NotNil(t, notifications)
	require.Len(t, notifications, 1)

	notification := notifications[0]
	require.Equal(t, defaultTranslations[NotificationTypeMentionReply].Title, notification.Title)
	require.Equal(t, defaultTranslations[NotificationTypeMentionReply].Body, notification.Body)
	require.Equal(t, defaultTranslations[NotificationTypeMentionReply].ImageURL, notification.ImageURL)

	require.Contains(t, notification.Data, "event", "Data should contain event")
	require.Contains(t, notification.Data, "relevant_events", "Data should contain relevant events")

	relevantEventsCompressed, ok := notification.Data["relevant_events"].(string)
	require.True(t, ok, "relevant_events should be a string")

	decompressed := helperDecompressZlibAndDecodeBase64(t, relevantEventsCompressed)
	require.Equal(t, `[`+profileEvent.Content+`]`, decompressed, "Decompressed content should match profile event content")
	require.Equal(t, CompressionMethodZlib, notification.Data["compression"], "Compression method should be zlib")
}
