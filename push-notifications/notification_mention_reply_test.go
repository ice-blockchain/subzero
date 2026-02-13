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
		userDevicesMap: make(map[string]map[DeviceID]DeviceInfo),
		relayURL:       testRelayURL,
		compressorPool: helperCreateTestCompressorPool(),
		stats:          newPushStats(),
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
				require.Equal(t, defaultTranslations[NotificationTypeMentionReply].Title, notification.Title)
				require.Equal(t, defaultTranslations[NotificationTypeMentionReply].Body, notification.Body)
				require.Equal(t, defaultTranslations[NotificationTypeMentionReply].ImageURL, notification.ImageURL)
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
				require.Equal(t, defaultTranslations[NotificationTypeMentionReply].Title, notification.Title)
				require.Equal(t, defaultTranslations[NotificationTypeMentionReply].Body, notification.Body)
				require.Equal(t, defaultTranslations[NotificationTypeMentionReply].ImageURL, notification.ImageURL)
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
		userDevicesMap: make(map[string]map[DeviceID]DeviceInfo),
		relayURL:       testRelayURL,
		compressorPool: helperCreateTestCompressorPool(),
		stats:          newPushStats(),
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
		require.Equal(t, defaultTranslations[NotificationTypeMentionReply].Title, notifications[0].Title)
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
		require.Equal(t, defaultTranslations[NotificationTypeMentionReply].Title, notifications[0].Title)
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
		require.Equal(t, defaultTranslations[NotificationTypeMentionReply].Title, notifications[0].Title)
	})
}

func TestSelfReplyNotification(t *testing.T) {
	t.Parallel()

	pm := &PushNotificationManager{
		userDevicesMap: make(map[string]map[DeviceID]DeviceInfo),
		relayURL:       testRelayURL,
		compressorPool: helperCreateTestCompressorPool(),
		stats:          newPushStats(),
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
		userDevicesMap: make(map[string]map[DeviceID]DeviceInfo),
		relayURL:       testRelayURL,
		compressorPool: helperCreateTestCompressorPool(),
		stats:          newPushStats(),
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

func TestMentionWithAuthoritativeEvents(t *testing.T) {
	t.Parallel()

	senderMasterPriv, senderMasterPub := model.GenerateKeyPair()
	senderDevicePriv, senderDevicePub := model.GenerateKeyPair()
	recipientMasterPriv, recipientMasterPub := model.GenerateKeyPair()

	pm := helperNewManager(t)
	pm.relayURL = "wss://test-mention-relay.example.com:8080"

	senderMetadataEvent := &model.Event{
		Event: nostr.Event{
			Kind:      nostr.KindProfileMetadata,
			CreatedAt: nostr.Now(),
			Content:   `{"name":"Mention Sender","display_name":"Mention sender profile"}`,
		},
	}
	require.NoError(t, senderMetadataEvent.SignWithAlg(senderMasterPriv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, query.AcceptEvents(t.Context(), senderMetadataEvent))
	senderAttestationEvent := &model.Event{
		Event: nostr.Event{
			Kind:      model.CustomIONKindAttestation,
			CreatedAt: nostr.Now(),
			Tags: model.Tags{
				{"p", senderDevicePub, "", "active:" + nostr.Now().String() + ":1,7"},
			},
			Content: "",
		},
	}
	require.NoError(t, senderAttestationEvent.SignWithAlg(senderMasterPriv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, query.AcceptEvents(t.Context(), senderAttestationEvent))
	senderRelayListEvent := &model.Event{
		Event: nostr.Event{
			Kind:      nostr.KindRelayListMetadata,
			CreatedAt: nostr.Now(),
			Tags: model.Tags{
				{"r", "wss://test-mention-relay.example.com", "read"},
				{"r", "wss://test-mention-relay.example.com", "write"},
			},
			Content: "",
		},
	}
	require.NoError(t, senderRelayListEvent.SignWithAlg(senderMasterPriv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, query.AcceptEvents(t.Context(), senderRelayListEvent))
	deviceEvent := &model.Event{
		Event: nostr.Event{
			PubKey:    recipientMasterPub,
			Kind:      model.CustomIONKindDeviceRegistration,
			CreatedAt: nostr.Now(),
			Tags: model.Tags{
				{"t", "ios"},
				{"d", "test-device-mention"},
				{"relay", "wss://test-mention-relay.example.com"},
				{"token", "encrypted_token_mention"},
			},
			Content: `[{"kinds":[1]}]`,
		},
	}
	require.NoError(t, deviceEvent.SignWithAlg(recipientMasterPriv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent))
	mentionEvent := &model.Event{
		Event: nostr.Event{
			Kind:      nostr.KindTextNote,
			CreatedAt: nostr.Now(),
			Content:   "Hello @recipient! This is a mention with authoritative events.",
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, senderMasterPub},
				{"p", recipientMasterPub},
			},
		},
	}
	require.NoError(t, mentionEvent.SignWithAlg(senderDevicePriv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	isAuthoritative, profileEvent, attestationEvent, err := pm.getAuthoritativeEvents(t.Context(), mentionEvent)
	require.NoError(t, err)
	require.True(t, isAuthoritative, "Sender should be authoritative on this relay")
	require.NotNil(t, profileEvent, "Should find sender's profile event")
	require.NotNil(t, attestationEvent, "Should find sender's attestation event")

	notifications, err := pm.processEvent(t.Context(), mentionEvent)
	require.NoError(t, err)
	require.NotNil(t, notifications)

	notification := notifications[0]
	require.Contains(t, notification.Data, "relevant_events", "Should contain relevant events")
	relevantEventsData, ok := notification.Data["relevant_events"].(string)
	require.True(t, ok, "relevant_events should be a string")
	require.NotEmpty(t, relevantEventsData, "relevant_events should not be empty")

	decompressed := helperDecompressZlibAndDecodeBase64(t, relevantEventsData)
	var relevantEventsArray []map[string]interface{}
	err = json.Unmarshal([]byte(decompressed), &relevantEventsArray)
	require.NoError(t, err, "Should be able to parse decompressed data as JSON array")
	require.True(t, len(relevantEventsArray) >= 2, "Should contain at least 2 relevant events")

	foundProfile := false
	foundAttestation := false
	for _, event := range relevantEventsArray {
		kind, ok := event["kind"].(float64)
		require.True(t, ok, "Event should have kind field")

		if kind == float64(nostr.KindProfileMetadata) {
			foundProfile = true
			require.Equal(t, senderMasterPub, event["pubkey"], "Profile event should be from sender")
			content, ok := event["content"].(string)
			require.True(t, ok, "Profile event should have content")
			require.Contains(t, content, "Mention Sender", "Profile content should match")
		} else if kind == float64(model.CustomIONKindAttestation) {
			foundAttestation = true
			require.Equal(t, senderMasterPub, event["pubkey"], "Attestation event should be from sender")
		}
	}
	require.True(t, foundProfile, "Should find profile metadata in relevant events")
	require.True(t, foundAttestation, "Should find attestation in relevant events")
}
