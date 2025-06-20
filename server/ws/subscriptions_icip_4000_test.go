// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

func TestWhoCanReplySettings_FollowingSettings(t *testing.T) {
	privkeyPostOwner, _ := model.GenerateKeyPair()
	privkeyUser1, pubkeyUser1 := model.GenerateKeyPair()
	privkeyUser2, _ := model.GenerateKeyPair()
	ctx := t.Context()
	RegisterWSSubscriptionListener(query.GetStoredEvents)
	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		require.True(t, len(events) > 0)
		require.NoError(t, query.AcceptEvents(ctx, events...))

		return nil
	})
	relay := helperMustNewRelay(t, pubsubServers[0])

	var post *model.Event
	t.Run("create post with following settings", func(t *testing.T) {
		post = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: nostr.Tags{
				{"settings", model.WhoCanReplySettings, model.FollowingWhoCanReplySettings, strconv.FormatInt(time.Now().Unix(), 10)},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, post, privkeyPostOwner)
		require.NoError(t, relay.Publish(ctx, post.Event))
	})
	t.Run("create followers list for post owner", func(t *testing.T) {
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindFollowList,
			Tags: nostr.Tags{
				{"p", pubkeyUser1, "", "alice"},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyPostOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("create reply for the initial post by user1", func(t *testing.T) {
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: nostr.Tags{
				{"e", post.GetID(), "", model.TagMarkerReply},
				{"e", post.GetID(), "", model.TagMarkerRoot},
				{"p", post.GetMasterPublicKey(), pubkeyUser1},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser1)
		require.NoError(t, relay.PublishMany(ctx, &ev.Event))
	})
	t.Run("create reply for the initial post by user2 that is not in the followers list, forbidden", func(t *testing.T) {
		post = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: nostr.Tags{
				{"e", post.ID, "", model.TagMarkerRoot},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, post, privkeyUser2)
		require.Error(t, relay.Publish(ctx, post.Event))
	})
	helperMustCloseRelay(t, relay)
}

func TestWhoCanReplySettings_MentionedSettings(t *testing.T) {
	privkeyPostOwner, _ := model.GenerateKeyPair()
	privkeyUser1, pubkeyUser1 := model.GenerateKeyPair()
	privkeyUser2, _ := model.GenerateKeyPair()
	ctx := t.Context()
	RegisterWSSubscriptionListener(query.GetStoredEvents)
	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		require.True(t, len(events) > 0)
		require.NoError(t, query.AcceptEvents(ctx, events...))

		return nil
	})
	relay := helperMustNewRelay(t, pubsubServers[0])

	var post *model.Event
	t.Run("create post with mentioned settings", func(t *testing.T) {
		pkey, err := nip19.EncodeProfile(pubkeyUser1, []string{})
		require.NoError(t, err)
		post = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Content:   fmt.Sprintf("hello world: nostr:%v", pkey),
			Tags: nostr.Tags{
				{"settings", model.WhoCanReplySettings, model.MentionWhoCanReplySettings, strconv.FormatInt(time.Now().Unix(), 10)},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, post, privkeyPostOwner)
		require.NoError(t, relay.Publish(ctx, post.Event))
	})
	t.Run("create reply for the initial post by user1", func(t *testing.T) {
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: nostr.Tags{
				{"e", post.GetID(), "", model.TagMarkerRoot},
				{"p", post.GetMasterPublicKey(), pubkeyUser1},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser1)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("create reply for the initial post by user2 that was not mentioned, forbidden", func(t *testing.T) {
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: nostr.Tags{
				{"e", post.GetID(), "", model.TagMarkerReply},
				{"e", post.GetID(), "", model.TagMarkerRoot},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser2)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})

	var richTextPost *model.Event
	t.Run("create post with mentioned settings using rich_text", func(t *testing.T) {
		pkey, err := nip19.EncodeProfile(pubkeyUser1, []string{})
		require.NoError(t, err)
		richTextDelta := fmt.Sprintf(`[{"insert": {"text-editor-profile": "nostr:%v"}}]`, pkey)
		richTextPost = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Content:   "",
			Tags: nostr.Tags{
				{"settings", model.WhoCanReplySettings, model.MentionWhoCanReplySettings, strconv.FormatInt(time.Now().Unix(), 10)},
				{model.CustomIONTagRichText, model.QuillDeltaProtocol, richTextDelta},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, richTextPost, privkeyPostOwner)
		require.NoError(t, relay.Publish(ctx, richTextPost.Event))
	})
	t.Run("create reply for rich_text post by mentioned user1", func(t *testing.T) {
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Content:   "Reply to rich_text mention post",
			Tags: nostr.Tags{
				{"e", richTextPost.GetID(), "", model.TagMarkerReply},
				{"e", richTextPost.GetID(), "", model.TagMarkerRoot},
				{"p", richTextPost.GetMasterPublicKey(), pubkeyUser1},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser1)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("create reply for rich_text post by user2 that was not mentioned, forbidden", func(t *testing.T) {
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Content:   "Reply from non-mentioned user",
			Tags: nostr.Tags{
				{"e", richTextPost.GetID(), "", model.TagMarkerReply},
				{"e", richTextPost.GetID(), "", model.TagMarkerRoot},
				{"p", richTextPost.GetMasterPublicKey()},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser2)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	helperMustCloseRelay(t, relay)
}

func TestWhoCanReplySettings_ModifiableEvent(t *testing.T) {
	privkeyPostOwner, _ := model.GenerateKeyPair()
	privkeyUser1, pubkeyUser1 := model.GenerateKeyPair()
	privkeyUser2, _ := model.GenerateKeyPair()
	ctx := t.Context()
	RegisterWSSubscriptionListener(query.GetStoredEvents)
	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		require.True(t, len(events) > 0)
		require.NoError(t, query.AcceptEvents(ctx, events...))

		return nil
	})
	relay := helperMustNewRelay(t, pubsubServers[0])

	var post *model.Event
	t.Run("create post with following settings", func(t *testing.T) {
		post = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      model.CustomIONKindEditableTextNote,
			Tags: nostr.Tags{
				{"settings", model.WhoCanReplySettings, model.FollowingWhoCanReplySettings, strconv.FormatInt(time.Now().Unix(), 10)},
				{"published_at", "1296962229"},
				{"d", "dummy"},
			},
			Content: "dummy",
		}}
		helperSignWithMinLeadingZeroBits(t, post, privkeyPostOwner)
		require.NoError(t, relay.Publish(ctx, post.Event))
	})
	t.Run("create followers list for post owner", func(t *testing.T) {
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindFollowList,
			Tags: nostr.Tags{
				{"p", pubkeyUser1, "", "alice"},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyPostOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	var reply *model.Event
	t.Run("create reply for the initial post by user1", func(t *testing.T) {
		reply = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      model.CustomIONKindEditableTextNote,
			Tags: nostr.Tags{
				{"a", post.Address(), "", model.TagMarkerRoot},
				{"p", post.GetMasterPublicKey(), pubkeyUser1},
				{"published_at", "1296962229"},
				{"d", "dummy"},
			},
			Content: "dummy reply",
		}}
		helperSignWithMinLeadingZeroBits(t, reply, privkeyUser1)
		require.NoError(t, relay.Publish(ctx, reply.Event))
	})
	t.Run("create reply of reply for the initial post by user1", func(t *testing.T) {
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      model.CustomIONKindEditableTextNote,
			Tags: nostr.Tags{
				{"a", reply.Address(), "", model.TagMarkerReply},
				{"a", post.Address(), "", model.TagMarkerRoot},
				{"p", post.GetMasterPublicKey(), pubkeyUser1},
				{"published_at", "1296962229"},
				{"d", "dummy"},
			},
			Content: "dummy reply of reply",
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser1)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("create reply of reply by user1 with missed `a` root tag: no root tag, we don't know settings, then ok", func(t *testing.T) {
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      model.CustomIONKindEditableTextNote,
			Tags: nostr.Tags{
				{"a", reply.Address(), "", model.TagMarkerReply},
				{"p", post.GetMasterPublicKey(), pubkeyUser1},
				{"published_at", "1296962229"},
				{"d", "dummy"},
			},
			Content: "dummy reply of reply",
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser1)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("create reply for the initial post by user2 that is not in the followers list, forbidden", func(t *testing.T) {
		post = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      model.CustomIONKindEditableTextNote,
			Tags: nostr.Tags{
				{"a", post.Address(), "", model.TagMarkerRoot},
				{"published_at", "1296962229"},
				{"d", "dummy"},
			},
			Content: "dummy reply",
		}}
		helperSignWithMinLeadingZeroBits(t, post, privkeyUser2)
		require.Error(t, relay.Publish(ctx, post.Event))
	})
	helperMustCloseRelay(t, relay)
}

func TestWhoCanReplySettings_SelfReply(t *testing.T) {
	privkeyPostOwner, pubkeyPostOwner := model.GenerateKeyPair()
	privkeyUser1, _ := model.GenerateKeyPair()
	ctx := t.Context()
	RegisterWSSubscriptionListener(query.GetStoredEvents)
	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		require.True(t, len(events) > 0)
		require.NoError(t, query.AcceptEvents(ctx, events...))

		return nil
	})
	relay := helperMustNewRelay(t, pubsubServers[0])

	var badgePost *model.Event
	dBadgeTagVal := "verified"
	t.Run("create post with badge restrictions by post owner", func(t *testing.T) {
		badgePost = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Content:   "This post has badge restrictions",
			Tags: nostr.Tags{
				{"settings", model.WhoCanReplySettings, fmt.Sprintf("%v|%v:%v:%v", model.BadgeWhoCanReplySettingsPrefix, nostr.KindBadgeDefinition, pubkeyPostOwner, dBadgeTagVal), strconv.FormatInt(time.Now().Unix(), 10)},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, badgePost, privkeyPostOwner)
		require.NoError(t, relay.Publish(ctx, badgePost.Event))
	})
	t.Run("post owner can reply to their own badge-restricted post", func(t *testing.T) {
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Content:   "I can reply to my own badge-restricted post",
			Tags: nostr.Tags{
				{"e", badgePost.GetID(), "", model.TagMarkerReply},
				{"e", badgePost.GetID(), "", model.TagMarkerRoot},
				{"p", badgePost.GetMasterPublicKey()},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyPostOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("different user cannot reply to badge-restricted post without badge", func(t *testing.T) {
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Content:   "I cannot reply without badge",
			Tags: nostr.Tags{
				{"e", badgePost.GetID(), "", model.TagMarkerReply},
				{"e", badgePost.GetID(), "", model.TagMarkerRoot},
				{"p", badgePost.GetMasterPublicKey()},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser1)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	var followingPost *model.Event
	t.Run("create post with following restrictions by post owner", func(t *testing.T) {
		followingPost = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Content:   "This post has following restrictions",
			Tags: nostr.Tags{
				{"settings", model.WhoCanReplySettings, model.FollowingWhoCanReplySettings, strconv.FormatInt(time.Now().Unix(), 10)},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, followingPost, privkeyPostOwner)
		require.NoError(t, relay.Publish(ctx, followingPost.Event))
	})
	t.Run("post owner can reply to their own following-restricted post", func(t *testing.T) {
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Content:   "I can reply to my own following-restricted post",
			Tags: nostr.Tags{
				{"e", followingPost.GetID(), "", model.TagMarkerReply},
				{"e", followingPost.GetID(), "", model.TagMarkerRoot},
				{"p", followingPost.GetMasterPublicKey()},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyPostOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("different user cannot reply to following-restricted post without being followed", func(t *testing.T) {
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Content:   "I cannot reply without being followed",
			Tags: nostr.Tags{
				{"e", followingPost.GetID(), "", model.TagMarkerReply},
				{"e", followingPost.GetID(), "", model.TagMarkerRoot},
				{"p", followingPost.GetMasterPublicKey()},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser1)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	var mentionPost *model.Event
	t.Run("create post with mention restrictions by post owner", func(t *testing.T) {
		mentionPost = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Content:   "This post has mention restrictions",
			Tags: nostr.Tags{
				{"settings", model.WhoCanReplySettings, model.MentionWhoCanReplySettings, strconv.FormatInt(time.Now().Unix(), 10)},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, mentionPost, privkeyPostOwner)
		require.NoError(t, relay.Publish(ctx, mentionPost.Event))
	})
	t.Run("post owner can reply to their own mention-restricted post", func(t *testing.T) {
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Content:   "I can reply to my own mention-restricted post",
			Tags: nostr.Tags{
				{"e", mentionPost.GetID(), "", model.TagMarkerReply},
				{"e", mentionPost.GetID(), "", model.TagMarkerRoot},
				{"p", mentionPost.GetMasterPublicKey()},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyPostOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("different user cannot reply to mention-restricted post without being mentioned", func(t *testing.T) {
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Content:   "I cannot reply without being mentioned",
			Tags: nostr.Tags{
				{"e", mentionPost.GetID(), "", model.TagMarkerReply},
				{"e", mentionPost.GetID(), "", model.TagMarkerRoot},
				{"p", mentionPost.GetMasterPublicKey()},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser1)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})

	helperMustCloseRelay(t, relay)
}

func TestWhoCanReplySettings_BadgeSettings(t *testing.T) {
	privkeyPostOwner, pubkeyPostOwner := model.GenerateKeyPair()
	privkeyUser1, pubkeyUser1 := model.GenerateKeyPair()

	privkeyUser2, pubkeyUser2 := model.GenerateKeyPair()
	ctx := t.Context()
	RegisterWSSubscriptionListener(query.GetStoredEvents)
	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		require.True(t, len(events) > 0)
		require.NoError(t, query.AcceptEvents(ctx, events...))

		return nil
	})
	relay := helperMustNewRelay(t, pubsubServers[0])

	var post, post2 *model.Event
	dBadgeTagVal := "verified"
	t.Run("Badge stored in database", func(t *testing.T) {
		t.Run("create post with badge settings", func(t *testing.T) {
			pkey, err := nip19.EncodePublicKey(pubkeyUser1)
			require.NoError(t, err)
			post = &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Content:   fmt.Sprintf("hello world: %v", pkey),
				Tags: nostr.Tags{
					{"settings", model.WhoCanReplySettings, fmt.Sprintf("%v|%v:%v:%v", model.BadgeWhoCanReplySettingsPrefix, nostr.KindBadgeDefinition, pubkeyPostOwner, dBadgeTagVal), strconv.FormatInt(time.Now().Unix(), 10)},
				},
			}}
			helperSignWithMinLeadingZeroBits(t, post, privkeyPostOwner)
			require.NoError(t, relay.Publish(ctx, post.Event))
		})

		t.Run("create badge definition first", func(t *testing.T) {
			badgeDefinition := &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindBadgeDefinition,
				Tags: nostr.Tags{
					{"d", dBadgeTagVal},
					{"name", "Verified Badge"},
					{"description", "Verified user badge"},
				},
			}}
			helperSignWithMinLeadingZeroBits(t, badgeDefinition, privkeyPostOwner)
			require.NoError(t, relay.Publish(ctx, badgeDefinition.Event))
		})

		t.Run("create badge award separately", func(t *testing.T) {
			badgeAward := &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindBadgeAward,
				Tags: nostr.Tags{
					{"a", fmt.Sprintf("%v:%v:%v", nostr.KindBadgeDefinition, pubkeyPostOwner, dBadgeTagVal)},
					{"p", pubkeyUser1, ""},
				},
			}}
			helperSignWithMinLeadingZeroBits(t, badgeAward, privkeyPostOwner)
			require.NoError(t, relay.Publish(ctx, badgeAward.Event))

			t.Run("profile badges event", func(t *testing.T) {
				profileBadges := &model.Event{Event: nostr.Event{
					CreatedAt: nostr.Now(),
					Kind:      nostr.KindProfileBadges,
					Tags: nostr.Tags{
						{"a", fmt.Sprintf("%v:%v:%v", nostr.KindBadgeDefinition, pubkeyPostOwner, dBadgeTagVal)},
						{"e", badgeAward.GetID(), ""},
						{"d", "profile_badges"},
					},
				}}
				helperSignWithMinLeadingZeroBits(t, profileBadges, privkeyUser1)
				require.NoError(t, relay.Publish(ctx, profileBadges.Event))
			})
		})

		t.Run("create reply for the initial post by user1 - should succeed", func(t *testing.T) {
			ev := &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Tags: nostr.Tags{
					{"e", post.GetID(), "", model.TagMarkerReply},
					{"e", post.GetID(), "", model.TagMarkerRoot},
					{"p", post.GetMasterPublicKey()},
				},
			}}
			helperSignWithMinLeadingZeroBits(t, ev, privkeyUser1)
			require.NoError(t, relay.Publish(ctx, ev.Event))
		})

		t.Run("create reply for the initial post by user2 that doesn't have badge - should fail", func(t *testing.T) {
			ev := &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Tags: nostr.Tags{
					{"e", post.GetID(), "", model.TagMarkerReply},
					{"e", post.GetID(), "", model.TagMarkerRoot},
					{"p", post.GetMasterPublicKey()},
				},
			}}
			helperSignWithMinLeadingZeroBits(t, ev, privkeyUser2)
			require.Error(t, relay.Publish(ctx, ev.Event))
		})
	})

	t.Run("Badge provided via ephemeral events", func(t *testing.T) {
		t.Run("create second post with badge settings", func(t *testing.T) {
			post2 = &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Content:   "Second post with badge restrictions",
				Tags: nostr.Tags{
					{"settings", model.WhoCanReplySettings, fmt.Sprintf("%v|%v:%v:%v", model.BadgeWhoCanReplySettingsPrefix, nostr.KindBadgeDefinition, pubkeyPostOwner, dBadgeTagVal), strconv.FormatInt(time.Now().Unix(), 10)},
				},
			}}
			helperSignWithMinLeadingZeroBits(t, post2, privkeyPostOwner)
			require.NoError(t, relay.Publish(ctx, post2.Event))
		})

		t.Run("create reply with badge ephemeral acks - should succeed", func(t *testing.T) {
			replyEvent := &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Tags: nostr.Tags{
					{"e", post2.GetID(), "", model.TagMarkerReply},
					{"e", post2.GetID(), "", model.TagMarkerRoot},
					{"p", post2.GetMasterPublicKey()},
				},
				Content: "Reply with badge acks",
			}}
			helperSignWithMinLeadingZeroBits(t, replyEvent, privkeyUser2)

			badgeDefinition := &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindBadgeDefinition,
				Tags: nostr.Tags{
					{"d", dBadgeTagVal},
					{"name", "Verified Badge"},
				},
			}}
			helperSignWithMinLeadingZeroBits(t, badgeDefinition, privkeyPostOwner)

			badgeDefAckContent, err := badgeDefinition.MarshalJSON()
			require.NoError(t, err)
			badgeDefAck := &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindEphemeralEmbeddding,
				Tags: nostr.Tags{
					{"e", replyEvent.ID},
				},
				Content: string(badgeDefAckContent),
			}}
			helperSignWithMinLeadingZeroBits(t, badgeDefAck, privkeyPostOwner)

			badgeAward := &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindBadgeAward,
				Tags: nostr.Tags{
					{"a", fmt.Sprintf("%d:%s:%s", nostr.KindBadgeDefinition, pubkeyPostOwner, dBadgeTagVal)},
					{"p", pubkeyUser2},
				},
			}}
			helperSignWithMinLeadingZeroBits(t, badgeAward, privkeyPostOwner)

			badgeAwardAckContent, err := badgeAward.MarshalJSON()
			require.NoError(t, err)
			badgeAwardAck := &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindEphemeralEmbeddding,
				Tags: nostr.Tags{
					{"e", replyEvent.ID},
				},
				Content: string(badgeAwardAckContent),
			}}
			helperSignWithMinLeadingZeroBits(t, badgeAwardAck, privkeyPostOwner)
			require.NoError(t, relay.PublishMany(ctx, &replyEvent.Event, &badgeDefAck.Event, &badgeAwardAck.Event))
		})

		t.Run("create reply without badge acks - should fail", func(t *testing.T) {
			ev := &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Tags: nostr.Tags{
					{"e", post2.GetID(), "", model.TagMarkerReply},
					{"e", post2.GetID(), "", model.TagMarkerRoot},
					{"p", post2.GetMasterPublicKey()},
				},
				Content: "Reply without badge acks",
			}}
			helperSignWithMinLeadingZeroBits(t, ev, privkeyUser2)
			require.Error(t, relay.Publish(ctx, ev.Event))
		})
	})

	t.Run("Reply with multiple e-tags (Root + Reply markers)", func(t *testing.T) {
		replyEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: nostr.Tags{
				{"e", post.GetID(), "", model.TagMarkerRoot},
				{"e", post2.GetID(), "", model.TagMarkerReply},
				{"p", post.GetMasterPublicKey()},
			},
			Content: "Reply with multiple e-tags to different posts",
		}}
		helperSignWithMinLeadingZeroBits(t, replyEvent, privkeyUser1)
		require.NoError(t, relay.Publish(ctx, replyEvent.Event))
	})

	t.Run("Self-reply without badge should be allowed", func(t *testing.T) {
		restrictedPost := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Content:   "Post by owner with badge restrictions",
			Tags: nostr.Tags{
				{"settings", model.WhoCanReplySettings, fmt.Sprintf("%v|%v:%v:%v", model.BadgeWhoCanReplySettingsPrefix, nostr.KindBadgeDefinition, pubkeyPostOwner, "verified"), strconv.FormatInt(time.Now().Unix(), 10)},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, restrictedPost, privkeyPostOwner)
		require.NoError(t, relay.Publish(ctx, restrictedPost.Event))

		selfReply := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: nostr.Tags{
				{"e", restrictedPost.GetID(), "", model.TagMarkerReply},
				{"e", restrictedPost.GetID(), "", model.TagMarkerRoot},
				{"p", restrictedPost.GetMasterPublicKey()},
			},
			Content: "Self-reply by post owner without badge",
		}}
		helperSignWithMinLeadingZeroBits(t, selfReply, privkeyPostOwner)
		require.NoError(t, relay.Publish(ctx, selfReply.Event))
	})

	helperMustCloseRelay(t, relay)
}

func TestWhoCanReplySettings_ComplexSettings(t *testing.T) {
	privkeyPostOwner, pubkeyPostOwner := model.GenerateKeyPair()
	privkeyUser1, pubkeyUser1 := model.GenerateKeyPair()
	privkeyUser2, pubkeyUser2 := model.GenerateKeyPair()
	_, pubkeyUser3 := model.GenerateKeyPair()
	privkeyUser4, _ := model.GenerateKeyPair()
	ctx := t.Context()
	RegisterWSSubscriptionListener(query.GetStoredEvents)
	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		require.True(t, len(events) > 0)
		require.NoError(t, query.AcceptEvents(ctx, events...))

		return nil
	})
	relay := helperMustNewRelay(t, pubsubServers[0])

	var post *model.Event
	dBadgeTagVal := "verified"
	t.Run("create post with complex settings", func(t *testing.T) {
		settingsConfiguration := fmt.Sprintf("%v,%v,%v|%v:%v:%v", model.FollowingWhoCanReplySettings, model.MentionWhoCanReplySettings, model.BadgeWhoCanReplySettingsPrefix, nostr.KindBadgeDefinition, pubkeyPostOwner, dBadgeTagVal)
		pkey, err := nip19.EncodePublicKey(pubkeyUser3)
		require.NoError(t, err)
		post = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Content:   fmt.Sprintf("hello world: %v", pkey),
			Tags: nostr.Tags{
				{"settings", model.WhoCanReplySettings, settingsConfiguration, strconv.FormatInt(time.Now().Unix(), 10)},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, post, privkeyPostOwner)
		require.NoError(t, relay.Publish(ctx, post.Event))
	})
	t.Run("create followers list for post owner", func(t *testing.T) {
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindFollowList,
			Tags: nostr.Tags{
				{"p", pubkeyUser1, "", "alice"},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyPostOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	var defineBadgeEv *model.Event
	t.Run("define verified badge", func(t *testing.T) {
		defineBadgeEv = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindBadgeDefinition,
			Tags: nostr.Tags{
				{"d", dBadgeTagVal},
				{"name", "Verified Badge"},
				{"description", "Verified user badge"},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, defineBadgeEv, privkeyPostOwner)
		require.NoError(t, relay.Publish(ctx, defineBadgeEv.Event))
	})
	var awardEvent *model.Event
	t.Run("award user2 verified badge", func(t *testing.T) {
		awardEvent = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindBadgeAward,
			Tags: nostr.Tags{
				{"a", fmt.Sprintf("%v:%v:%v", nostr.KindBadgeDefinition, pubkeyPostOwner, dBadgeTagVal)},
				{"p", pubkeyUser2, ""},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, awardEvent, privkeyPostOwner)
		require.NoError(t, relay.Publish(ctx, awardEvent.Event))
	})
	t.Run("profile badges event", func(t *testing.T) {
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindProfileBadges,
			Tags: nostr.Tags{
				{"a", fmt.Sprintf("%v:%v:%v", nostr.KindBadgeDefinition, pubkeyPostOwner, dBadgeTagVal)},
				{"e", awardEvent.GetID(), ""},
				{"d", "profile_badges"},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser2)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("create reply for the initial post by user1 followed, ok", func(t *testing.T) {
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: nostr.Tags{
				{"e", post.GetID(), "", model.TagMarkerRoot},
				{"p", post.GetMasterPublicKey(), pubkeyUser1},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser1)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("create reply for the initial post by user2 badge awarded, ok", func(t *testing.T) {
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: nostr.Tags{
				{"e", post.GetID(), "", model.TagMarkerRoot},
				{"p", post.GetMasterPublicKey(), pubkeyUser2},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser2)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("create reply for the initial post by user3 mentioned, ok", func(t *testing.T) {
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: nostr.Tags{
				{"e", post.GetID(), "", model.TagMarkerRoot},
				{"p", post.GetMasterPublicKey(), pubkeyUser2},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser2)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("create reply for the initial post by user4, forbidden", func(t *testing.T) {
		post = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: nostr.Tags{
				{"e", post.GetID(), "", model.TagMarkerRoot},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, post, privkeyUser4)
		require.Error(t, relay.Publish(ctx, post.Event))
	})
	helperMustCloseRelay(t, relay)
}

func TestWhoCanReplySettings_MultipleBadgeTypes(t *testing.T) {
	privkeyPostOwner, pubkeyPostOwner := model.GenerateKeyPair()
	privkeyUser1, pubkeyUser1 := model.GenerateKeyPair()
	privkeyUser2, _ := model.GenerateKeyPair()
	privkeyUser3, pubkeyUser3 := model.GenerateKeyPair()
	ctx := t.Context()
	RegisterWSSubscriptionListener(query.GetStoredEvents)
	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		require.True(t, len(events) > 0)
		require.NoError(t, query.AcceptEvents(ctx, events...))

		return nil
	})
	relay := helperMustNewRelay(t, pubsubServers[0])

	var post, post2 *model.Event
	dBadgeTagVal := "verified"

	t.Run("Badge stored in database", func(t *testing.T) {
		t.Run("create post with badge settings", func(t *testing.T) {
			post = &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Content:   "Post restricted to premium users",
				Tags: nostr.Tags{
					{"settings", model.WhoCanReplySettings, fmt.Sprintf("%v|%v:%v:%v", model.BadgeWhoCanReplySettingsPrefix, nostr.KindBadgeDefinition, pubkeyPostOwner, dBadgeTagVal), strconv.FormatInt(time.Now().Unix(), 10)},
				},
			}}
			helperSignWithMinLeadingZeroBits(t, post, privkeyPostOwner)
			require.NoError(t, relay.Publish(ctx, post.Event))
		})

		var premiumBadgeDefinition *model.Event
		t.Run("define premium badge", func(t *testing.T) {
			premiumBadgeDefinition = &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindBadgeDefinition,
				Tags: nostr.Tags{
					{"d", dBadgeTagVal},
					{"name", "Premium Badge"},
					{"description", "Premium user badge"},
					{"thumb", "https://example.com/premium.png"},
				},
			}}
			helperSignWithMinLeadingZeroBits(t, premiumBadgeDefinition, privkeyPostOwner)
			require.NoError(t, relay.Publish(ctx, premiumBadgeDefinition.Event))
		})

		var premiumAward *model.Event
		t.Run("award user1 premium badge", func(t *testing.T) {
			premiumAward = &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindBadgeAward,
				Tags: nostr.Tags{
					{"a", fmt.Sprintf("%v:%v:%v", nostr.KindBadgeDefinition, pubkeyPostOwner, dBadgeTagVal)},
					{"p", pubkeyUser1, ""},
				},
			}}
			helperSignWithMinLeadingZeroBits(t, premiumAward, privkeyPostOwner)
			require.NoError(t, relay.Publish(ctx, premiumAward.Event))
		})

		t.Run("profile badges event for premium user", func(t *testing.T) {
			ev := &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindProfileBadges,
				Tags: nostr.Tags{
					{"a", fmt.Sprintf("%v:%v:%v", nostr.KindBadgeDefinition, pubkeyPostOwner, dBadgeTagVal)},
					{"e", premiumAward.GetID(), ""},
					{"d", "profile_badges"},
				},
			}}
			helperSignWithMinLeadingZeroBits(t, ev, privkeyUser1)
			require.NoError(t, relay.Publish(ctx, ev.Event))
		})

		t.Run("premium user can reply", func(t *testing.T) {
			ev := &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Content:   "Reply from premium user",
				Tags: nostr.Tags{
					{"e", post.GetID(), "", model.TagMarkerRoot},
					{"p", post.GetMasterPublicKey()},
				},
			}}
			helperSignWithMinLeadingZeroBits(t, ev, privkeyUser1)
			require.NoError(t, relay.Publish(ctx, ev.Event))
		})

		t.Run("non-premium user cannot reply", func(t *testing.T) {
			ev := &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Content:   "Reply from non-premium user",
				Tags: nostr.Tags{
					{"e", post.GetID(), "", model.TagMarkerRoot},
					{"p", post.GetMasterPublicKey()},
				},
			}}
			helperSignWithMinLeadingZeroBits(t, ev, privkeyUser2)
			require.Error(t, relay.Publish(ctx, ev.Event))
		})
	})

	t.Run("Developer badge with ephemeral events", func(t *testing.T) {
		var devPost *model.Event
		devBadgeTag := "developer"

		t.Run("create post with developer badge settings", func(t *testing.T) {
			devPost = &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Content:   "Developer-only discussion",
				Tags: nostr.Tags{
					{"settings", model.WhoCanReplySettings, fmt.Sprintf("%v|%v:%v:%v", model.BadgeWhoCanReplySettingsPrefix, nostr.KindBadgeDefinition, pubkeyPostOwner, devBadgeTag), strconv.FormatInt(time.Now().Unix(), 10)},
				},
			}}
			helperSignWithMinLeadingZeroBits(t, devPost, privkeyPostOwner)
			require.NoError(t, relay.Publish(ctx, devPost.Event))
		})

		t.Run("reply with developer badge via ephemeral events", func(t *testing.T) {
			replyEvent := &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Tags: nostr.Tags{
					{"e", devPost.GetID(), "", model.TagMarkerRoot},
					{"p", devPost.GetMasterPublicKey()},
				},
				Content: "Reply with developer badge acks",
			}}
			helperSignWithMinLeadingZeroBits(t, replyEvent, privkeyUser3)

			devBadgeDefinition := &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindBadgeDefinition,
				Tags: nostr.Tags{
					{"d", devBadgeTag},
					{"name", "Developer Badge"},
					{"description", "Software developer badge"},
				},
			}}
			helperSignWithMinLeadingZeroBits(t, devBadgeDefinition, privkeyPostOwner)

			badgeDefAckContent, err := devBadgeDefinition.MarshalJSON()
			require.NoError(t, err)
			badgeDefAck := &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindEphemeralEmbeddding,
				Tags: nostr.Tags{
					{"e", replyEvent.ID},
				},
				Content: string(badgeDefAckContent),
			}}
			helperSignWithMinLeadingZeroBits(t, badgeDefAck, privkeyUser3)

			devBadgeAward := &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindBadgeAward,
				Tags: nostr.Tags{
					{"a", fmt.Sprintf("%d:%s:%s", nostr.KindBadgeDefinition, pubkeyPostOwner, devBadgeTag)},
					{"p", pubkeyUser3},
				},
			}}
			helperSignWithMinLeadingZeroBits(t, devBadgeAward, privkeyPostOwner)

			badgeAwardAckContent, err := devBadgeAward.MarshalJSON()
			require.NoError(t, err)
			badgeAwardAck := &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindEphemeralEmbeddding,
				Tags: nostr.Tags{
					{"e", replyEvent.ID},
				},
				Content: string(badgeAwardAckContent),
			}}
			helperSignWithMinLeadingZeroBits(t, badgeAwardAck, privkeyPostOwner)
			require.NoError(t, relay.PublishMany(ctx, &replyEvent.Event, &badgeDefAck.Event, &badgeAwardAck.Event))
		})
	})

	t.Run("Multiple badge types allowed", func(t *testing.T) {
		t.Run("create post allowing both premium and developer badges", func(t *testing.T) {
			settingsValue := fmt.Sprintf("%v|%v:%v:%v,%v|%v:%v:%v",
				model.BadgeWhoCanReplySettingsPrefix, nostr.KindBadgeDefinition, pubkeyPostOwner, "verified",
				model.BadgeWhoCanReplySettingsPrefix, nostr.KindBadgeDefinition, pubkeyPostOwner, "developer")

			post2 = &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Content:   "Post allowing verified OR developer badges",
				Tags: nostr.Tags{
					{"settings", model.WhoCanReplySettings, settingsValue, strconv.FormatInt(time.Now().Unix(), 10)},
				},
			}}
			helperSignWithMinLeadingZeroBits(t, post2, privkeyPostOwner)
			require.NoError(t, relay.Publish(ctx, post2.Event))
		})

		t.Run("premium user can reply to multi-badge post", func(t *testing.T) {
			ev := &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Content:   "Reply from verified user to multi-badge post",
				Tags: nostr.Tags{
					{"e", post2.GetID(), "", model.TagMarkerRoot},
					{"p", post2.GetMasterPublicKey()},
				},
			}}
			helperSignWithMinLeadingZeroBits(t, ev, privkeyUser1)
			require.NoError(t, relay.Publish(ctx, ev.Event))
		})

		t.Run("non-badge user cannot reply to multi-badge post", func(t *testing.T) {
			ev := &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Content:   "Reply from user without any badges",
				Tags: nostr.Tags{
					{"e", post2.GetID(), "", model.TagMarkerRoot},
					{"p", post2.GetMasterPublicKey()},
				},
			}}
			helperSignWithMinLeadingZeroBits(t, ev, privkeyUser2)
			require.Error(t, relay.Publish(ctx, ev.Event))
		})
	})

	helperMustCloseRelay(t, relay)
}

func TestReplyCannotSetSettings(t *testing.T) {
	privkeyPostOwner, _ := model.GenerateKeyPair()
	privkeyUser1, _ := model.GenerateKeyPair()
	ctx := t.Context()
	RegisterWSSubscriptionListener(query.GetStoredEvents)
	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		require.True(t, len(events) > 0)
		require.NoError(t, query.AcceptEvents(ctx, events...))

		return nil
	})
	relay := helperMustNewRelay(t, pubsubServers[0])
	var rootPost *model.Event
	t.Run("create root post without who_can_reply settings", func(t *testing.T) {
		rootPost = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Content:   "This is a root post without settings",
			Tags:      nostr.Tags{},
		}}
		helperSignWithMinLeadingZeroBits(t, rootPost, privkeyPostOwner)
		require.NoError(t, relay.Publish(ctx, rootPost.Event))
	})
	t.Run("try to create reply with e tag and settings - should fail", func(t *testing.T) {
		replyWithETags := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Content:   "This is a reply that tries to set settings",
			Tags: nostr.Tags{
				{"e", rootPost.GetID(), "", model.TagMarkerRoot},
				{"e", rootPost.GetID(), "", model.TagMarkerReply},
				{"p", rootPost.GetMasterPublicKey()},
				{"settings", model.WhoCanReplySettings, model.MentionWhoCanReplySettings, strconv.FormatInt(time.Now().Unix(), 10)},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, replyWithETags, privkeyUser1)
		require.Error(t, relay.Publish(ctx, replyWithETags.Event))
	})
	t.Run("try to create reply with a tag and settings - should fail", func(t *testing.T) {
		addressableRootPost := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      model.CustomIONKindEditableTextNote,
			Content:   "Addressable root post with settings",
			Tags: nostr.Tags{
				{"d", "test-post"},
				{"published_at", strconv.FormatInt(time.Now().Unix(), 10)},
				{"settings", model.WhoCanReplySettings, model.FollowingWhoCanReplySettings, strconv.FormatInt(time.Now().Unix(), 10)},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, addressableRootPost, privkeyPostOwner)
		require.NoError(t, relay.Publish(ctx, addressableRootPost.Event))

		replyWithATags := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      model.CustomIONKindEditableTextNote,
			Content:   "Reply with a tag trying to set settings",
			Tags: nostr.Tags{
				{"d", "reply-post"},
				{"published_at", strconv.FormatInt(time.Now().Unix(), 10)},
				{"a", addressableRootPost.Address(), "", model.TagMarkerRoot},
				{"a", addressableRootPost.Address(), "", model.TagMarkerReply},
				{"p", addressableRootPost.GetMasterPublicKey()},
				{"settings", model.WhoCanReplySettings, model.MentionWhoCanReplySettings, strconv.FormatInt(time.Now().Unix(), 10)},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, replyWithATags, privkeyUser1)
		require.Error(t, relay.Publish(ctx, replyWithATags.Event))
	})
	t.Run("create normal reply without settings - should succeed", func(t *testing.T) {
		normalReply := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Content:   "Normal reply without settings",
			Tags: nostr.Tags{
				{"e", rootPost.GetID(), "", model.TagMarkerRoot},
				{"e", rootPost.GetID(), "", model.TagMarkerReply},
				{"p", rootPost.GetMasterPublicKey()},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, normalReply, privkeyUser1)
		require.NoError(t, relay.Publish(ctx, normalReply.Event))
	})
	t.Run("create root post with settings - should succeed", func(t *testing.T) {
		newRootPost := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Content:   "New root post with settings",
			Tags: nostr.Tags{
				{"settings", model.WhoCanReplySettings, model.MentionWhoCanReplySettings, strconv.FormatInt(time.Now().Unix(), 10)},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, newRootPost, privkeyUser1)
		require.NoError(t, relay.Publish(ctx, newRootPost.Event))
	})
	t.Run("create another root post with following settings - should succeed", func(t *testing.T) {
		anotherRootPost := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Content:   "Another root post with following settings",
			Tags: nostr.Tags{
				{"settings", model.WhoCanReplySettings, model.FollowingWhoCanReplySettings, strconv.FormatInt(time.Now().Unix(), 10)},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, anotherRootPost, privkeyPostOwner)
		require.NoError(t, relay.Publish(ctx, anotherRootPost.Event))
	})
	helperMustCloseRelay(t, relay)
}
