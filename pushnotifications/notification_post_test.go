// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"testing"

	"github.com/ice-blockchain/subzero/model"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

func TestPostTypeClassification(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{}

	event1 := helperCreateTestEvent(
		t,
		"test_id_1",
		"author_pubkey",
		nostr.KindTextNote,
		"Regular post",
		nostr.Tags{},
	)
	isReply1, isMention1, replyToPubkey1, mentionedPubkeys1 := pm.classifyPostType(&event1.Event)
	require.False(t, isReply1, "Regular post should not be a reply")
	require.False(t, isMention1, "Regular post should not be a mention")
	require.Empty(t, replyToPubkey1, "replyToPubkey should be empty for a regular post")
	require.Empty(t, mentionedPubkeys1, "There should be no mentioned users in a regular post")

	event2 := helperCreateTestEvent(
		t,
		"test_id_2",
		"author_pubkey",
		nostr.KindTextNote,
		"Post with mention @user1",
		nostr.Tags{
			{"e", "event_id", "", model.TagMarkerMention},
			{"p", "mentioned_pubkey"},
		},
	)
	isReply2, isMention2, replyToPubkey2, mentionedPubkeys2 := pm.classifyPostType(&event2.Event)
	require.False(t, isReply2, "Post with mention should not be a reply")
	require.True(t, isMention2, "Post should be a mention")
	require.Empty(t, replyToPubkey2, "replyToPubkey should be empty for a post with mention")
	require.Equal(t, 1, len(mentionedPubkeys2), "There should be one mentioned user")
	require.Equal(t, "mentioned_pubkey", mentionedPubkeys2[0], "Mentioned pubkey should match")

	event3 := helperCreateTestEvent(
		t,
		"test_id_3",
		"author_pubkey",
		nostr.KindTextNote,
		"Reply to post",
		nostr.Tags{
			{"e", "original_event_id", "", model.TagMarkerReply},
			{"p", "original_author_pubkey"},
		},
	)
	isReply3, isMention3, replyToPubkey3, mentionedPubkeys3 := pm.classifyPostType(&event3.Event)
	require.True(t, isReply3, "Post should be a reply")
	require.False(t, isMention3, "Reply post should not be a mention")
	require.Equal(t, "original_author_pubkey", replyToPubkey3, "replyToPubkey should match the original post author")
	require.Equal(t, 1, len(mentionedPubkeys3), "Reply should mention the original post author")
}

func TestReplyPostNotification(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:         make(map[DeviceID]DeviceInfo),
		userDevices:     make(map[string][]DeviceID),
		filterToDevices: make(map[NotificationType]map[DeviceID]bool),
		invalidTokens:   make(map[DeviceID]InvalidTokenInfo),
		translationMgr: &TranslationManager{
			translations: map[NotificationType]map[Language]map[string]string{
				NotificationTypeReply: {
					Language("en"): {
						"title": "New Reply",
						"body":  "You have a new reply from {{pubkey}}",
					},
				},
			},
		},
	}

	pm.filterToDevices[NotificationTypeReply] = make(map[DeviceID]bool)

	event := helperCreateTestEvent(
		t,
		"reply_post_id",
		"replier_pubkey",
		nostr.KindTextNote,
		"Reply to post",
		nostr.Tags{
			{"e", "original_event_id", "", model.TagMarkerReply},
			{"p", "original_author_pubkey"},
		},
	)

	pm.devices[DeviceID("device1")] = DeviceInfo{
		DeviceID: "device1",
		FCMToken: "token1",
		PubKey:   "original_author_pubkey",
		Filters: nostr.Filters{
			{
				Kinds: []int{nostr.KindTextNote},
			},
		},
	}
	pm.userDevices["original_author_pubkey"] = []DeviceID{"device1"}
	pm.filterToDevices[NotificationTypeReply]["device1"] = true

	replyToPubkey := "original_author_pubkey"
	notifications := pm.handleReplyPost(&event.Event, replyToPubkey, "en")

	require.NotNil(t, notifications, "Notification package should not be nil")
	require.Len(t, notifications.singleNotifications, 1, "There should be one single notification")

	notification := notifications.singleNotifications[0]
	require.Equal(t, "New Reply", notification.Title, "Title should match")
	require.Contains(t, notification.Body, "replier_pubkey", "Body should contain sender's pubkey")
	require.Equal(t, "token1", notification.Target.Token, "Token should match")
	require.Equal(t, DeviceID("device1"), notification.Target.DeviceID, "DeviceID should match")
}

func TestMentionPostNotification(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:         make(map[DeviceID]DeviceInfo),
		userDevices:     make(map[string][]DeviceID),
		filterToDevices: make(map[NotificationType]map[DeviceID]bool),
		invalidTokens:   make(map[DeviceID]InvalidTokenInfo),
		translationMgr: &TranslationManager{
			translations: map[NotificationType]map[Language]map[string]string{
				NotificationTypeMention: {
					Language("en"): {
						"title": "You were mentioned",
						"body":  "{{pubkey}} mentioned you: {{message}}",
					},
				},
			},
		},
	}

	pm.filterToDevices[NotificationTypeMention] = make(map[DeviceID]bool)

	event := helperCreateTestEvent(
		t,
		"mention_post_id",
		"author_pubkey",
		nostr.KindTextNote,
		"Post with mention @user1 and @user2",
		nostr.Tags{
			{"e", "event_id", "", model.TagMarkerMention},
			{"p", "mentioned_pubkey1"},
			{"p", "mentioned_pubkey2"},
		},
	)

	pm.devices[DeviceID("device1")] = DeviceInfo{
		DeviceID: "device1",
		FCMToken: "token1",
		PubKey:   "mentioned_pubkey1",
		Filters: nostr.Filters{
			{
				Kinds: []int{nostr.KindTextNote},
			},
		},
	}
	pm.userDevices["mentioned_pubkey1"] = []DeviceID{"device1"}
	pm.filterToDevices[NotificationTypeMention]["device1"] = true

	pm.devices[DeviceID("device2")] = DeviceInfo{
		DeviceID: "device2",
		FCMToken: "token2",
		PubKey:   "mentioned_pubkey2",
		Filters: nostr.Filters{
			{
				Kinds: []int{nostr.KindTextNote},
			},
		},
	}
	pm.userDevices["mentioned_pubkey2"] = []DeviceID{"device2"}
	pm.filterToDevices[NotificationTypeMention]["device2"] = true

	mentionedPubkeys := []string{"mentioned_pubkey1", "mentioned_pubkey2"}

	notifications := pm.handleMentionPost(&event.Event, mentionedPubkeys, "en")

	require.NotNil(t, notifications, "Notification package should not be nil")
	require.Len(t, notifications.singleNotifications, 2, "There should be two single notifications")

	notificationsByDevice := make(map[DeviceID]bool)

	for _, notification := range notifications.singleNotifications {
		notificationsByDevice[notification.Target.DeviceID] = true

		require.Equal(t, "You were mentioned", notification.Title, "Title should match")
		require.Contains(t, notification.Body, "author_pubkey", "Body should contain author's pubkey")
		require.Contains(t, notification.Body, "mentioned", "Body should contain part of the message")

		if notification.Target.DeviceID == DeviceID("device1") {
			require.Equal(t, "token1", notification.Target.Token, "Token should match")
		} else if notification.Target.DeviceID == DeviceID("device2") {
			require.Equal(t, "token2", notification.Target.Token, "Token should match")
		}
	}

	require.True(t, notificationsByDevice[DeviceID("device1")], "There should be a notification for device1")
	require.True(t, notificationsByDevice[DeviceID("device2")], "There should be a notification for device2")
}

func TestHandlePostNotification(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:         make(map[DeviceID]DeviceInfo),
		userDevices:     make(map[string][]DeviceID),
		filterToDevices: make(map[NotificationType]map[DeviceID]bool),
		invalidTokens:   make(map[DeviceID]InvalidTokenInfo),
		translationMgr: &TranslationManager{
			translations: map[NotificationType]map[Language]map[string]string{
				NotificationTypePost: {
					Language("en"): {
						"title": "New Post",
						"body":  "You have a new post from {{pubkey}}",
					},
				},
				NotificationTypeReply: {
					Language("en"): {
						"title": "New Reply",
						"body":  "You have a new reply from {{pubkey}}",
					},
				},
				NotificationTypeMention: {
					Language("en"): {
						"title": "New Mention",
						"body":  "You were mentioned by {{pubkey}}",
					},
				},
			},
		},
	}

	pm.filterToDevices[NotificationTypePost] = make(map[DeviceID]bool)
	pm.filterToDevices[NotificationTypeReply] = make(map[DeviceID]bool)
	pm.filterToDevices[NotificationTypeMention] = make(map[DeviceID]bool)

	replyPost := helperCreateTestEvent(
		t,
		"reply_post_id",
		"replier_pubkey",
		nostr.KindTextNote,
		"Reply to post",
		nostr.Tags{
			{"e", "original_event_id", "", model.TagMarkerReply},
			{"p", "original_author_pubkey"},
		},
	)

	mentionPost := helperCreateTestEvent(
		t,
		"mention_post_id",
		"author_pubkey",
		nostr.KindTextNote,
		"Post with mention @user",
		nostr.Tags{
			{"e", "event_id", "", model.TagMarkerMention},
			{"p", "mentioned_pubkey"},
		},
	)

	pm.devices[DeviceID("device1")] = DeviceInfo{
		DeviceID: "device1",
		FCMToken: "token1",
		PubKey:   "original_author_pubkey",
		Filters: nostr.Filters{
			{
				Kinds: []int{nostr.KindTextNote},
			},
		},
	}
	pm.userDevices["original_author_pubkey"] = []DeviceID{"device1"}
	pm.filterToDevices[NotificationTypeReply]["device1"] = true

	pm.devices[DeviceID("device2")] = DeviceInfo{
		DeviceID: "device2",
		FCMToken: "token2",
		PubKey:   "mentioned_pubkey",
		Filters: nostr.Filters{
			{
				Kinds: []int{nostr.KindTextNote},
			},
		},
	}
	pm.userDevices["mentioned_pubkey"] = []DeviceID{"device2"}
	pm.filterToDevices[NotificationTypeMention]["device2"] = true

	replyBatch := pm.handlePostNotification(t.Context(), &replyPost.Event, "en")
	mentionBatch := pm.handlePostNotification(t.Context(), &mentionPost.Event, "en")

	require.NotNil(t, replyBatch, "Notification package for reply should not be nil")
	require.NotEmpty(t, replyBatch.singleNotifications, "There should be notifications for reply")

	require.NotNil(t, mentionBatch, "Notification package for mention should not be nil")
	require.NotEmpty(t, mentionBatch.singleNotifications, "There should be notifications for mention")
}
