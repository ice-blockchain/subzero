// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

func helperCreatePostEvent(t *testing.T, id, pubKey string, kind int, content string, tags nostr.Tags) *model.Event {
	t.Helper()

	return &model.Event{
		Event: nostr.Event{
			ID:      id,
			PubKey:  pubKey,
			Kind:    kind,
			Content: content,
			Tags:    tags,
		},
	}
}

func TestHandleMentionReplyEvent(t *testing.T) {
	t.Parallel()

	pm := &PushNotificationManager{
		userDevicesMap: make(map[PublicKey]map[DeviceID]DeviceInfo),
		relayURL:       testRelayURL,
	}

	event1 := helperCreatePostEvent(
		t,
		"test_id_1_"+uuid.NewString(),
		"author_pubkey",
		nostr.KindTextNote,
		"Regular post",
		nostr.Tags{},
	)
	require.NoError(t, query.AcceptEvents(t.Context(), event1))

	event2 := helperCreatePostEvent(
		t,
		"test_id_2_"+uuid.NewString(),
		"author_pubkey",
		nostr.KindTextNote,
		"Post with mention user1",
		nostr.Tags{
			{"e", "event_id", "", model.TagMarkerMention},
			{"p", "7e7e9c42a91bfef19fa929e5fda1b72e0ebc1a4c1141673e2794234d86addf4e"},
		},
	)
	require.NoError(t, query.AcceptEvents(t.Context(), event2))

	deviceEvent1 := helperCreateTestDeviceRegistrationEvent(
		t,
		"7e7e9c42a91bfef19fa929e5fda1b72e0ebc1a4c1141673e2794234d86addf4e",
		"device1",
		nostr.Tags{
			{"t", "ios"},
			{"d", "device1"},
			{"relay", "wss://relay.example.com"},
			{"token", "token1"},
		},
		nostr.Filters{
			{
				Kinds: []int{nostr.KindTextNote},
			},
		},
	)
	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent1))

	deviceEvent2 := helperCreateTestDeviceRegistrationEvent(
		t,
		"7e7e9c42a91bfef19fa929e5fda1b72e0ebc1a4c1141673e2794234d86addf4e",
		"device2",
		nostr.Tags{
			{"t", "android"},
			{"d", "device2"},
			{"relay", "wss://relay.example.com"},
			{"token", "token2"},
		},
		nostr.Filters{
			{
				Kinds: []int{nostr.KindTextNote},
			},
		},
	)
	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent2))
	require.Equal(t, 2, len(pm.userDevicesMap["7e7e9c42a91bfef19fa929e5fda1b72e0ebc1a4c1141673e2794234d86addf4e"]))

	t.Run("mention in post", func(t *testing.T) {
		notifications, err := pm.handleMentionReplyEvent(event2)
		require.NoError(t, err)
		require.Len(t, notifications, 2)
		for _, notification := range notifications {
			if notification.Target.GetTag("t").Value() == "ios" {
				require.Equal(t, DefaultTranslations[NotificationTypeMentionReply].Title(), notification.Title)
				require.Equal(t, DefaultTranslations[NotificationTypeMentionReply].Body(), notification.Body)
				require.Equal(t, DefaultTranslations[NotificationTypeMentionReply].ImageURL(), notification.ImageURL)
			} else {
				require.Equal(t, "", notification.Title, "Title should match")
				require.Equal(t, "", notification.Body, "Body should match")
				require.Contains(t, notification.Data, "event", "Data should contain event")
			}
		}
	})

	t.Run("reply to post with mention", func(t *testing.T) {
		mentionedPubkey := "7e7e9c42a91bfef19fa929e5fda1b72e0ebc1a4c1141673e2794234d86addf4e"
		nprofileEncoded, err := nip19.EncodeProfile(mentionedPubkey, []string{"wss://relay.example.com"})
		require.NoError(t, err)
		originalPost := helperCreatePostEvent(
			t,
			"original_post_"+uuid.NewString(),
			"original_author_pubkey",
			nostr.KindTextNote,
			"Original post content",
			nostr.Tags{},
		)
		require.NoError(t, query.AcceptEvents(t.Context(), originalPost))

		replyEvent := helperCreatePostEvent(
			t,
			"reply_id_"+uuid.NewString(),
			"reply_author_pubkey",
			nostr.KindTextNote,
			"Reply with mention: "+nprofileEncoded,
			nostr.Tags{
				{"e", originalPost.GetID(), "", model.TagMarkerReply},
				{"e", originalPost.GetID(), "", model.TagMarkerRoot},
				{"p", originalPost.GetMasterPublicKey()},
			},
		)
		require.NoError(t, query.AcceptEvents(t.Context(), replyEvent))

		notifications, err := pm.handleMentionReplyEvent(replyEvent)
		require.NoError(t, err)
		require.Len(t, notifications, 2, "Should notify mentioned user on both devices")
		for _, notification := range notifications {
			if notification.Target.GetTag("t").Value() == "ios" {
				require.Equal(t, DefaultTranslations[NotificationTypeMentionReply].Title(), notification.Title)
				require.Equal(t, DefaultTranslations[NotificationTypeMentionReply].Body(), notification.Body)
				require.Equal(t, DefaultTranslations[NotificationTypeMentionReply].ImageURL(), notification.ImageURL)
			} else {
				require.Equal(t, "", notification.Title, "Title should match")
				require.Equal(t, "", notification.Body, "Body should match")
				require.Contains(t, notification.Data, "event", "Data should contain event")
			}
		}
	})
}

func TestMention(t *testing.T) {
	t.Parallel()

	pm := &PushNotificationManager{
		userDevicesMap: make(map[PublicKey]map[DeviceID]DeviceInfo),
		relayURL:       testRelayURL,
	}

	nprofileEncoded, err := nip19.EncodeProfile("7e7e9c42a91bfef19fa929e5fda1b72e0ebc1a4c1141673e2794234d86addf4e", []string{"wss://relay.example.com"})
	require.NoError(t, err)

	deviceEvent := helperCreateTestDeviceRegistrationEvent(
		t,
		"7e7e9c42a91bfef19fa929e5fda1b72e0ebc1a4c1141673e2794234d86addf4e",
		"device1",
		nostr.Tags{
			{"t", "ios"},
			{"d", "device1"},
			{"relay", "wss://relay.example.com"},
			{"token", "token1"},
		},
		nostr.Filters{
			{
				Kinds: []int{nostr.KindTextNote},
			},
		},
	)
	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent))

	t.Run("content nprofile mention", func(t *testing.T) {
		event := helperCreatePostEvent(
			t,
			"test_id_content_"+uuid.NewString(),
			"author_pubkey",
			nostr.KindTextNote,
			"Post with nprofile mention in content: "+nprofileEncoded,
			nostr.Tags{},
		)
		require.NoError(t, query.AcceptEvents(t.Context(), event))

		notifications, err := pm.handleMentionReplyEvent(event)
		require.NoError(t, err)
		require.Len(t, notifications, 1)
		require.Equal(t, DefaultTranslations[NotificationTypeMentionReply].Title(), notifications[0].Title)
	})

	t.Run("rich_text nprofile mention", func(t *testing.T) {
		richTextData := []interface{}{
			map[string]interface{}{
				"insert": "Hello ",
			},
			map[string]interface{}{
				"insert": map[string]interface{}{
					"text-editor-profile": nprofileEncoded,
				},
			},
			map[string]interface{}{
				"insert": " and ",
			},
			map[string]interface{}{
				"insert": "@user",
				"attributes": map[string]interface{}{
					"mention": nprofileEncoded,
				},
			},
			map[string]interface{}{
				"insert": " how are you?",
			},
		}
		richTextJSON, err := json.Marshal(richTextData)
		require.NoError(t, err)

		event := helperCreatePostEvent(
			t,
			"test_id_richtext_"+uuid.NewString(),
			"author_pubkey",
			nostr.KindTextNote,
			"",
			nostr.Tags{
				{"rich_text", model.QuillDeltaProtocol, string(richTextJSON)},
			},
		)
		require.NoError(t, query.AcceptEvents(t.Context(), event))

		notifications, err := pm.handleMentionReplyEvent(event)
		require.NoError(t, err)
		require.Len(t, notifications, 1)
		require.Equal(t, DefaultTranslations[NotificationTypeMentionReply].Title(), notifications[0].Title)
	})

	t.Run("both content and p tag mention - no duplicate", func(t *testing.T) {
		event := helperCreatePostEvent(
			t,
			"test_id_both_"+uuid.NewString(),
			"author_pubkey",
			nostr.KindTextNote,
			"Post with nprofile mention in content: "+nprofileEncoded,
			nostr.Tags{
				{"p", "7e7e9c42a91bfef19fa929e5fda1b72e0ebc1a4c1141673e2794234d86addf4e"},
			},
		)
		require.NoError(t, query.AcceptEvents(t.Context(), event))

		notifications, err := pm.handleMentionReplyEvent(event)
		require.NoError(t, err)
		require.Len(t, notifications, 1, "Should not create duplicate notifications for the same user")
		require.Equal(t, DefaultTranslations[NotificationTypeMentionReply].Title(), notifications[0].Title)
	})
}

func TestSelfReplyNotification(t *testing.T) {
	t.Parallel()

	pm := &PushNotificationManager{
		userDevicesMap: make(map[PublicKey]map[DeviceID]DeviceInfo),
		relayURL:       testRelayURL,
	}

	selfReplyEvent := helperCreatePostEvent(
		t,
		"self_reply_id_"+uuid.NewString(),
		"author_pubkey",
		nostr.KindTextNote,
		"Reply to my own post",
		nostr.Tags{
			{"e", "original_event_id", "", model.TagMarkerReply},
			{"p", "author_pubkey"},
		},
	)
	deviceEvent := helperCreateTestDeviceRegistrationEvent(
		t,
		"author_pubkey",
		"device1",
		nostr.Tags{
			{"t", "ios"},
			{"d", "device1"},
			{"relay", "wss://relay.example.com"},
			{"token", "token1"},
		},
		nostr.Filters{
			{
				Kinds: []int{nostr.KindTextNote},
			},
		},
	)
	require.NoError(t, query.AcceptEvents(t.Context(), selfReplyEvent))
	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent))

	notifications, err := pm.handleMentionReplyEvent(selfReplyEvent)
	require.NoError(t, err)
	require.Empty(t, notifications, "Self-reply should not create notifications")
}

func TestHandleMentionReplyEventWithRelevantEvents(t *testing.T) {
	t.Parallel()

	pm := &PushNotificationManager{
		userDevicesMap: make(map[PublicKey]map[DeviceID]DeviceInfo),
		relayURL:       testRelayURL,
	}

	authorPubKey := "author_pubkey_" + uuid.NewString()
	mentionedPubKey := "mentioned_pubkey_" + uuid.NewString()

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

	mentionEvent := helperCreatePostEvent(
		t,
		"mention_id_"+uuid.NewString(),
		authorPubKey,
		nostr.KindTextNote,
		"Post mentioning someone",
		nostr.Tags{
			{"e", "event_id", "", model.TagMarkerMention},
			{"p", mentionedPubKey},
		},
	)

	deviceID := "device1_" + uuid.NewString()
	deviceEvent := helperCreateTestDeviceRegistrationEvent(
		t,
		mentionedPubKey,
		deviceID,
		nostr.Tags{
			{"t", "ios"},
			{"d", deviceID},
			{"token", "token1_" + uuid.NewString()},
		},
		nostr.Filters{
			{
				Kinds: []int{nostr.KindTextNote},
			},
		},
	)

	pm.deviceMutex.Lock()
	pm.userDevicesMap[mentionedPubKey] = map[DeviceID]DeviceInfo{
		DeviceID(deviceID): {
			DeviceID: DeviceID(deviceID),
			Filters: nostr.Filters{
				{
					Kinds: []int{nostr.KindTextNote},
				},
			},
			Event: deviceEvent,
		},
	}
	pm.deviceMutex.Unlock()

	notifications, err := pm.handleMentionReplyEvent(mentionEvent, profileEvent)
	require.NoError(t, err)
	require.Len(t, notifications, 1)
	notification := notifications[0]
	require.Equal(t, DefaultTranslations[NotificationTypeMentionReply].Title(), notification.Title)
	require.Equal(t, DefaultTranslations[NotificationTypeMentionReply].Body(profileEvent), notification.Body)
	require.Equal(t, DefaultTranslations[NotificationTypeMentionReply].ImageURL(), notification.ImageURL)

	require.Contains(t, notification.Data, "event", "Data should contain event")
	require.Contains(t, notification.Data, "relevant_events", "Data should contain relevant events")

	relevantEventsCompressed, ok := notification.Data["relevant_events"].(string)
	require.True(t, ok, "relevant_events should be a string")

	decompressed := helperDecompressZlibAndDecodeBase64(t, relevantEventsCompressed)
	require.Equal(t, `[`+profileEvent.Content+`]`, decompressed, "Decompressed content should match profile event content")
	require.Equal(t, CompressionMethodZlib, notification.Data["compression"], "Compression method should be zlib")
}
