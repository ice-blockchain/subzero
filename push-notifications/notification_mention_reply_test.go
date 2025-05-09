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
			{"p", "mentioned_pubkey"},
		},
	)
	require.NoError(t, query.AcceptEvents(t.Context(), event2))

	deviceEvent1 := helperCreateTestDeviceRegistrationEvent(
		t,
		"mentioned_pubkey",
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
		"mentioned_pubkey",
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
	require.Equal(t, 2, len(pm.userDevicesMap["mentioned_pubkey"]))

	notifications := pm.handleMentionReplyEvent(event2)

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
}

func TestMention(t *testing.T) {
	t.Parallel()

	nprofileEncoded, err := nip19.EncodeProfile("7e7e9c42a91bfef19fa929e5fda1b72e0ebc1a4c1141673e2794234d86addf4e", []string{"wss://relay.example.com"})
	require.NoError(t, err)

	event := helperCreatePostEvent(
		t,
		"test_id_nprofile_"+uuid.NewString(),
		"author_pubkey",
		nostr.KindTextNote,
		"Post with nprofile mention "+nprofileEncoded,
		nostr.Tags{
			{"e", "event_id", "", model.TagMarkerMention},
			{"p", "0000000000000000000000000000000000000000000000000000000000000001"},
		},
	)
	require.NoError(t, query.AcceptEvents(t.Context(), event))

	pm := &PushNotificationManager{
		userDevicesMap: make(map[PublicKey]map[DeviceID]DeviceInfo),
	}

	deviceEvent := helperCreateTestDeviceRegistrationEvent(
		t,
		"0000000000000000000000000000000000000000000000000000000000000001",
		"device1",
		nostr.Tags{
			{"t", "ios"},
			{"token", "token1"},
		},
		nostr.Filters{
			{
				Kinds: []int{nostr.KindTextNote},
			},
		},
	)

	require.NoError(t, query.AcceptEvents(t.Context(), deviceEvent))
	require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent))

	notifications := pm.handleMentionReplyEvent(event)
	require.NotNil(t, notifications)
	require.Len(t, notifications, 1)
	require.Equal(t, DefaultTranslations[NotificationTypeMentionReply].Title(), notifications[0].Title)
	require.Equal(t, DefaultTranslations[NotificationTypeMentionReply].Body(), notifications[0].Body)
	require.Equal(t, DefaultTranslations[NotificationTypeMentionReply].ImageURL(), notifications[0].ImageURL)
	require.Contains(t, notifications[0].Data["event"], event.String(), "Data should contain event")
}

func TestSelfReplyNotification(t *testing.T) {
	t.Parallel()

	pm := &PushNotificationManager{
		userDevicesMap: make(map[PublicKey]map[DeviceID]DeviceInfo),
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

	require.Empty(t, pm.handleMentionReplyEvent(selfReplyEvent), "Self-reply should not create notifications")
}

func TestHandleMentionReplyEventWithRelevantEvents(t *testing.T) {
	t.Parallel()

	pm := &PushNotificationManager{
		userDevicesMap: make(map[PublicKey]map[DeviceID]DeviceInfo),
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

	notifications := pm.handleMentionReplyEvent(mentionEvent, profileEvent)
	require.Len(t, notifications, 1)
	notification := notifications[0]
	require.Equal(t, DefaultTranslations[NotificationTypeMentionReply].Title(), notification.Title)
	require.Equal(t, DefaultTranslations[NotificationTypeMentionReply].Body(profileEvent), notification.Body)
	require.Equal(t, DefaultTranslations[NotificationTypeMentionReply].ImageURL(), notification.ImageURL)

	require.Contains(t, notification.Data, "event", "Data should contain event")
	require.Contains(t, notification.Data, "relevant_events", "Data should contain relevant events")

	jsonBytes, ok := notification.Data["relevant_events"].([]byte)
	require.True(t, ok, "relevant_events should be a JSON byte array")

	var relevantEvents []string
	err = json.Unmarshal(jsonBytes, &relevantEvents)
	require.NoError(t, err, "Should be able to unmarshal relevant_events")
	require.Len(t, relevantEvents, 1, "Should have one relevant event")
	require.Contains(t, relevantEvents[0], profileEvent.Content, "Relevant event should contain profile event content")
}
