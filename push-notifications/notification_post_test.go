// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
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
		"Post with mention user1",
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
	require.Equal(t, 0, len(mentionedPubkeys3), "Reply should contain 0 mentioned users")
}

func TestReplyPostNotification(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:     make(map[DeviceID]DeviceInfo),
		userDevices: make(map[string][]DeviceID),
	}

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

	device1 := helperCreateTestDeviceRegistrationEvent(
		t,
		"original_author_pubkey",
		"device1",
		[]string{"t", "ios", "token", "token1"},
		nostr.Filters{
			{
				Kinds: []int{nostr.KindTextNote},
			},
		},
	)
	require.NoError(t, pm.processDeviceRegistrationEvent(device1))
	pm.userDevices["original_author_pubkey"] = []DeviceID{DeviceID(device1.Tags.GetD())}

	replyToPubkey := "original_author_pubkey"
	result := pm.handleReplyPost(&event.Event, replyToPubkey)

	require.Equal(t, 1, len(result), "Notification result should contain one notification")
	require.Equal(t, string(NotificationTypeReply), result[0].Data["notificationType"], "Notification type should be reply")
	require.Equal(t, event.String(), result[0].Data["event"], "Event should match")
}

func TestMentionPostNotification(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:     make(map[DeviceID]DeviceInfo),
		userDevices: make(map[string][]DeviceID),
	}

	event := helperCreateTestEvent(
		t,
		"mention_post_id",
		"author_pubkey",
		nostr.KindTextNote,
		"Post with mention user1 and user2",
		nostr.Tags{
			{"e", "event_id", "", model.TagMarkerMention},
			{"p", "mentioned_pubkey1"},
			{"p", "mentioned_pubkey2"},
		},
	)

	device1 := helperCreateTestDeviceRegistrationEvent(
		t,
		"mentioned_pubkey1",
		"device1",
		[]string{"t", "ios", "token", "token1"},
		nostr.Filters{
			{
				Kinds: []int{nostr.KindTextNote},
			},
		},
	)
	require.NoError(t, pm.processDeviceRegistrationEvent(device1))
	pm.userDevices["mentioned_pubkey1"] = []DeviceID{DeviceID(device1.Tags.GetD())}

	device2 := helperCreateTestDeviceRegistrationEvent(
		t,
		"mentioned_pubkey2",
		"device2",
		[]string{"t", "ios", "token", "token2"},
		nostr.Filters{
			{
				Kinds: []int{nostr.KindTextNote},
			},
		},
	)
	require.NoError(t, pm.processDeviceRegistrationEvent(device2))
	pm.userDevices["mentioned_pubkey2"] = []DeviceID{DeviceID(device2.Tags.GetD())}

	mentionedPubkeys := []string{"mentioned_pubkey1", "mentioned_pubkey2"}
	result := pm.handleMentionPost(&event.Event, mentionedPubkeys)

	require.Len(t, result, 2, "Notification result should contain two notifications")
	require.Equal(t, string(NotificationTypeMention), result[0].Data["notificationType"], "Notification type should be mention")
	require.Equal(t, event.String(), result[0].Data["event"], "Event should match")
	require.Equal(t, string(NotificationTypeMention), result[1].Data["notificationType"], "Notification type should be mention")
	require.Equal(t, event.String(), result[1].Data["event"], "Event should match")
}

func TestHandlePostNotification(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:     make(map[DeviceID]DeviceInfo),
		userDevices: make(map[string][]DeviceID),
	}

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
		"Post with mention user",
		nostr.Tags{
			{"e", "event_id", "", model.TagMarkerMention},
			{"p", "mentioned_pubkey"},
		},
	)

	device1 := helperCreateTestDeviceRegistrationEvent(
		t,
		"original_author_pubkey",
		"device1",
		[]string{"t", "ios", "token", "token1"},
		nostr.Filters{
			{
				Kinds: []int{nostr.KindTextNote},
			},
		},
	)
	require.NoError(t, pm.processDeviceRegistrationEvent(device1))
	pm.userDevices["original_author_pubkey"] = []DeviceID{DeviceID(device1.Tags.GetD())}

	device2 := helperCreateTestDeviceRegistrationEvent(
		t,
		"mentioned_pubkey",
		"device2",
		[]string{"t", "ios", "token", "token2"},
		nostr.Filters{
			{
				Kinds: []int{nostr.KindTextNote},
			},
		},
	)
	require.NoError(t, pm.processDeviceRegistrationEvent(device2))
	pm.userDevices["mentioned_pubkey"] = []DeviceID{DeviceID(device2.Tags.GetD())}

	replyNotifications := pm.handlePostNotification(t.Context(), &replyPost.Event)
	mentionNotifications := pm.handlePostNotification(t.Context(), &mentionPost.Event)

	require.Len(t, replyNotifications, 1, "Notification result should have one notification")
	require.Len(t, mentionNotifications, 1, "Notification result should have one notification")
}

func TestNprofileMentionDetection(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{
		devices:     make(map[DeviceID]DeviceInfo),
		userDevices: make(map[string][]DeviceID),
	}

	nprofileEncoded, err := nip19.EncodeProfile("7e7e9c42a91bfef19fa929e5fda1b72e0ebc1a4c1141673e2794234d86addf4e", []string{"wss://relay.example.com"})
	require.NoError(t, err)

	event := helperCreateTestEvent(
		t,
		"test_id_nprofile",
		"author_pubkey",
		nostr.KindTextNote,
		"Post with nprofile mention "+nprofileEncoded,
		nostr.Tags{
			{"e", "event_id", "", model.TagMarkerMention},
			{"p", "0000000000000000000000000000000000000000000000000000000000000001"},
		},
	)

	isReply, isMention, replyToPubkey, mentionedPubkeys := pm.classifyPostType(&event.Event)
	require.False(t, isReply, "Post with nprofile mention should not be a reply")
	require.True(t, isMention, "Post should be detected as a mention")
	require.Empty(t, replyToPubkey, "replyToPubkey should be empty for a post with nprofile mention")
	require.Equal(t, 2, len(mentionedPubkeys), "There should be one mentioned user")
	require.Equal(t, "7e7e9c42a91bfef19fa929e5fda1b72e0ebc1a4c1141673e2794234d86addf4e", mentionedPubkeys[1], "Mentioned pubkey should match")
	require.Equal(t, "0000000000000000000000000000000000000000000000000000000000000001", mentionedPubkeys[0], "Mentioned pubkey should match")
}

func TestSelfReplyNotification(t *testing.T) {
	t.Parallel()
	pm := &PushNotificationManager{}

	selfReplyEvent := helperCreateTestEvent(
		t,
		"self_reply_id",
		"author_pubkey",
		nostr.KindTextNote,
		"Reply to my own post",
		nostr.Tags{
			{"e", "original_event_id", "", model.TagMarkerReply},
			{"p", "author_pubkey"},
		},
	)

	isReply, isMention, replyToPubkey, mentionedPubkeys := pm.classifyPostType(&selfReplyEvent.Event)
	require.True(t, isReply, "Self-reply should be classified as a reply")
	require.False(t, isMention, "Self-reply should not be classified as a mention")
	require.Equal(t, "author_pubkey", replyToPubkey, "replyToPubkey should match the author's pubkey")
	require.Empty(t, mentionedPubkeys, "There should be no mentioned pubkeys")

	result := pm.handleReplyPost(&selfReplyEvent.Event, "author_pubkey")
	require.Nil(t, result, "No notifications should be created for self-replies")
}
