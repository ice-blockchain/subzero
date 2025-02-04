// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

func TestCommunityDefinition_ClosedCommunity_ModeratorPosting_CommentsEnabled(t *testing.T) {
	relay := helperMustNewRelay(t, pubsubServers[0])
	ctx := context.Background()
	RegisterWSSubscriptionListener(func(ctx context.Context, s *model.Subscription) EventIterator {
		return query.GetStoredEvents(ctx, s)
	})
	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		require.True(t, len(events) > 0)
		require.NoError(t, query.AcceptEvents(ctx, events...))

		return nil
	})
	hVal, err := uuid.NewV7()
	require.NoError(t, err)
	communityID := hVal.String()
	privkeyOwner, pubkeyCommunityOwner := model.GenerateKeyPair()
	privkeyAdmin, pubkeyCommunityAdmin := model.GenerateKeyPair()
	privkeyModerator, pubkeyCommunityModerator := model.GenerateKeyPair()
	privkeyUser1, pubkeyUser1 := model.GenerateKeyPair()
	privkeyUser2, pubkeyUser2 := model.GenerateKeyPair()
	privkeyUser3, pubkeyUser3 := model.GenerateKeyPair()
	privkeyUser4, pubkeyUser4 := model.GenerateKeyPair()
	privkeyUser5, pubkeyUser5 := model.GenerateKeyPair()

	t.Run("valid closed community definition event with moderator posting, comments enabled", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"d", "dtagvalue"},
					{"name", "some name"},
					{"description", "some description"},
					{"closed"},
					{"settings", model.CommentsEnabledSettings, "false", fmt.Sprint(time.Now().Add(-3 * time.Hour).Unix())},
					{"settings", model.CommentsEnabledSettings, "true", fmt.Sprint(time.Now().Add(-2 * time.Hour).Unix())},
					{"settings", model.CommentsEnabledSettings, "false", fmt.Sprint(time.Now().Add(-1 * time.Hour).Unix())},
					{"settings", model.CommentsEnabledSettings, "true", fmt.Sprint(time.Now().Unix())},
					{"settings", model.RoleRequiredForPostingSettings, string(model.AdminRole), fmt.Sprint(time.Now().Add(-3 * time.Hour).Unix())},
					{"settings", model.RoleRequiredForPostingSettings, string(model.ModeratorRole), fmt.Sprint(time.Now().Add(-2 * time.Hour).Unix())},
					{"settings", model.RoleRequiredForPostingSettings, string(model.AdminRole), fmt.Sprint(time.Now().Add(-1 * time.Hour).Unix())},
					{"settings", model.RoleRequiredForPostingSettings, string(model.ModeratorRole), fmt.Sprint(time.Now().Unix())},
					{"p", pubkeyCommunityAdmin, "", string(model.AdminRole)},
					{"p", pubkeyCommunityModerator, "", string(model.ModeratorRole)},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	//---- JOIN ----
	ownerAuthorizationEvent := &model.Event{
		Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      model.CustomIONKindCommunityJoin,
			Tags: model.Tags{
				{"h", communityID},
				{"p", pubkeyCommunityOwner},
				{"expiration", fmt.Sprint(time.Now().Add(1 * time.Hour).Unix())},
			},
		},
	}
	helperSignWithMinLeadingZeroBits(t, ownerAuthorizationEvent, privkeyOwner)
	t.Run("join owner to the community", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyCommunityOwner},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("send invitation to admin", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyCommunityAdmin},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("accept invitation by admin to join to the community", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyCommunityAdmin},
					{"authorization", ownerAuthorizationEvent.String()},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyAdmin)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("send invitation to moderator", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyCommunityModerator},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("accept invitation by moderator", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyCommunityModerator},
					{"authorization", ownerAuthorizationEvent.String()},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyModerator)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("send invitation by owner to user1", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser1},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("accept invitation by user1", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser1},
					{"authorization", ownerAuthorizationEvent.String()},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser1)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("send invitation by owner to user2", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser1},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("accept invitation by user2", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser2},
					{"authorization", ownerAuthorizationEvent.String()},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser2)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("send invitation by admin to user3", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser3},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyAdmin)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("accept invitation by user3", func(t *testing.T) {
		adminAuthorizationEvent := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser3},
					{"expiration", fmt.Sprint(time.Now().Add(1 * time.Hour).Unix())},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, adminAuthorizationEvent, privkeyAdmin)
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser3},
					{"authorization", adminAuthorizationEvent.String()},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser3)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("send invitation by moderator to user4", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser4},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyModerator)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("accept invitation from moderator by user4", func(t *testing.T) {
		moderatorAuthorizationEvent := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser4},
					{"expiration", fmt.Sprint(time.Now().Add(1 * time.Hour).Unix())},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, moderatorAuthorizationEvent, privkeyAdmin)
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser4},
					{"authorization", moderatorAuthorizationEvent.String()},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser4)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("send invitation to user5 by moderator", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser5},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyModerator)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("accept invitation by moderator", func(t *testing.T) {
		moderatorAuthorizationEvent := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser5},
					{"expiration", fmt.Sprint(time.Now().Add(1 * time.Hour).Unix())},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, moderatorAuthorizationEvent, privkeyModerator)
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser5},
					{"authorization", moderatorAuthorizationEvent.String()},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser5)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	//---- POST ----
	t.Run("try to post to the community by regular user, forbidden", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
				Tags:      model.Tags{{"h", communityID}},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser1)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("try to post to the community by moderator, ok", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
				Tags:      model.Tags{{"h", communityID}},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyModerator)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("try to post to the community by admin, ok", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
				Tags:      model.Tags{{"h", communityID}},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyAdmin)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("try to post to the community by owner, ok", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
				Tags: model.Tags{
					{"h", communityID},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	//---- COMMENT ----
	t.Run("try to post comment to the community by regular user, forbidden", func(t *testing.T) {
		post := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
				Tags: model.Tags{
					{"h", communityID},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, post, privkeyUser1)
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindRepost,
				Content:   post.String(),
				Tags: model.Tags{
					{"h", communityID},
					{"e", post.GetID()},
					{"p", post.GetMasterPublicKey()},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser1)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("try to post comment to the community by moderator, ok", func(t *testing.T) {
		post := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
			},
		}
		helperSignWithMinLeadingZeroBits(t, post, privkeyUser1)
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindRepost,
				Content:   post.String(),
				Tags: model.Tags{
					{"h", communityID},
					{"e", post.GetID()},
					{"p", post.GetMasterPublicKey()},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyModerator)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("try to post comment to the community by admin, ok", func(t *testing.T) {
		post := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    pubkeyUser1,
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
			},
		}
		helperSignWithMinLeadingZeroBits(t, post, privkeyUser1)
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    pubkeyCommunityAdmin,
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindRepost,
				Content:   post.String(),
				Tags: model.Tags{
					{"h", communityID},
					{"e", post.GetID()},
					{"p", post.GetMasterPublicKey()},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyAdmin)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("try to post comment to the community by owner, ok", func(t *testing.T) {
		post := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
			},
		}
		helperSignWithMinLeadingZeroBits(t, post, privkeyUser1)
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindRepost,
				Content:   post.String(),
				Tags: model.Tags{
					{"h", communityID},
					{"e", post.GetID()},
					{"p", post.GetMasterPublicKey()},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})

	//---- CHANGE DEFINITION ----
	t.Run("promoting user to admin by owner, ok", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Timestamp(time.Now().Add(-1 * time.Minute).Unix()),
				Kind:      model.CustomIONKindCommunityChangeDefinition,
				Content:   "some text",
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser2, "", string(model.AdminRole)},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("try to ban any user by new admin, ok", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityBanUser,
				Content:   "some text",
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser3},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser2)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("demoting admin by owner, ok", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityChangeDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser2, "", string(model.RegularRole)},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("try to ban any user by demoted admin, forbidden", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityBanUser,
				Content:   "some reason",
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser3},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser2)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("try to promote user to admin by moderator, forbidden", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityChangeDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser2, "", string(model.AdminRole)},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyModerator)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("try to demote admin by moderator, forbidden", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityChangeDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyCommunityAdmin, "", ""},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyModerator)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	//---- BAN USER ----
	t.Run("try to ban any user by owner, ok", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityBanUser,
				Content:   "some text",
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser1},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("try to ban any user by admin, ok", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityBanUser,
				Content:   "some text",
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser2},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyAdmin)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("try to ban any user by moderator, ok", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityBanUser,
				Content:   "some text",
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser3},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyModerator)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("try to ban any user by any user, forbidden", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityBanUser,
				Content:   "some text",
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser5},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser4)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("try to ban admin user by moderator, forbidden", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityBanUser,
				Content:   "some text",
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyCommunityAdmin},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyModerator)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("try to ban owner user by moderator, forbidden", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityBanUser,
				Content:   "some text",
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyCommunityOwner},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyModerator)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("try to ban owner user by admin, forbidden", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityBanUser,
				Content:   "some text",
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyCommunityOwner},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyAdmin)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	helperMustCloseRelay(t, relay)
}

func TestCommunityDefinition_OpenPublicCommunity(t *testing.T) {
	relay := helperMustNewRelay(t, pubsubServers[0])
	ctx := context.Background()
	RegisterWSSubscriptionListener(func(ctx context.Context, s *model.Subscription) EventIterator {
		return query.GetStoredEvents(ctx, s)
	})
	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		require.True(t, len(events) > 0)
		require.NoError(t, query.AcceptEvents(ctx, events...))

		return nil
	})
	hVal, err := uuid.NewV7()
	require.NoError(t, err)
	communityID := hVal.String()
	privkeyOwner, pubkeyCommunityOwner := model.GenerateKeyPair()
	privkeyUser1, pubkeyUser1 := model.GenerateKeyPair()
	privkeyUser2, pubkeyUser2 := model.GenerateKeyPair()

	t.Run("valid public open community definition event", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"d", "dtagvalue"},
					{"name", "some name"},
					{"description", "some description"},
					{"public"},
					{"open"},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	//---- JOIN ----
	ownerAuthorizationEvent := &model.Event{
		Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      model.CustomIONKindCommunityJoin,
			Tags: model.Tags{
				{"h", communityID},
				{"p", pubkeyCommunityOwner},
				{"expiration", fmt.Sprint(time.Now().Add(1 * time.Hour).Unix())},
			},
		},
	}
	helperSignWithMinLeadingZeroBits(t, ownerAuthorizationEvent, privkeyOwner)
	t.Run("join owner to the community", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyCommunityOwner},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("self-join user to the community", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser1},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser1)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("join any user to the community with authorization tag as no needed", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser2},
					{"authorization", ownerAuthorizationEvent.String()},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser2)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	helperMustCloseRelay(t, relay)
}

func TestCommunityDefinition_OpenedCommunity_AnyPosting_CommentsDisabled(t *testing.T) {
	relay := helperMustNewRelay(t, pubsubServers[0])
	ctx := context.Background()
	RegisterWSSubscriptionListener(func(ctx context.Context, s *model.Subscription) EventIterator {
		return query.GetStoredEvents(ctx, s)
	})
	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		require.True(t, len(events) > 0)
		require.NoError(t, query.AcceptEvents(ctx, events...))

		return nil
	})
	hVal, err := uuid.NewV7()
	require.NoError(t, err)
	communityID := hVal.String()
	privkeyOwner, pubkeyCommunityOwner := model.GenerateKeyPair()
	privkeyAdmin, pubkeyCommunityAdmin := model.GenerateKeyPair()
	privkeyModerator, pubkeyCommunityModerator := model.GenerateKeyPair()
	privkeyUser1, pubkeyUser1 := model.GenerateKeyPair()
	privkeyUser2, _ := model.GenerateKeyPair()

	t.Run("valid open community definition event with anybody posting, comments disabled", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"name", "some name"},
					{"description", "some description"},
					{"d", "dtagvalue"},
					{"open"},
					{"settings", model.CommentsEnabledSettings, "false", fmt.Sprint(time.Now().Unix())},
					{"p", pubkeyCommunityAdmin, "", string(model.AdminRole)},
					{"p", pubkeyCommunityModerator, "", string(model.ModeratorRole)},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	//---- JOIN ----
	t.Run("join owner to the community", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    pubkeyCommunityOwner,
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyCommunityOwner},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, &ev, privkeyOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("join admin to the community", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyCommunityAdmin},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyAdmin)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("join moderator to the community", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    pubkeyCommunityModerator,
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyCommunityModerator},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyModerator)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("self-join user to the community", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser1},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser1)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	//---- POST ----
	t.Run("try to post to the community by user that not in the community, forbidden", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
				Tags:      model.Tags{{"h", communityID}},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser2)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("try to post to the community by regular role user, ok", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
				Tags:      model.Tags{{"h", communityID}},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser1)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("try to post to the community by moderator, ok", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
				Tags:      model.Tags{{"h", communityID}},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyModerator)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("try to post to the community by admin, ok", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
				Tags:      model.Tags{{"h", communityID}},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyAdmin)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("try to post to the community by owner, ok", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
				Tags: model.Tags{
					{"h", communityID},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	//---- COMMENT ----
	t.Run("try to post comment to the community by any user, forbidden", func(t *testing.T) {
		post := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
				Tags: model.Tags{
					{"h", communityID},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, post, privkeyUser1)
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindRepost,
				Content:   post.String(),
				Tags: model.Tags{
					{"h", communityID},
					{"e", post.GetID()},
					{"p", post.GetMasterPublicKey()},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser1)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("try to post comment to the community by admin, forbidden", func(t *testing.T) {
		post := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
				Tags: model.Tags{
					{"h", communityID},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, post, privkeyUser1)
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindRepost,
				Content:   post.String(),
				Tags: model.Tags{
					{"h", communityID},
					{"e", post.GetID()},
					{"p", post.GetMasterPublicKey()},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyAdmin)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("try to post comment to the community by moderator, forbidden", func(t *testing.T) {
		post := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
				Tags: model.Tags{
					{"h", communityID},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, post, privkeyUser1)
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindRepost,
				Content:   post.String(),
				Tags: model.Tags{
					{"h", communityID},
					{"e", post.GetID()},
					{"p", post.GetMasterPublicKey()},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyModerator)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("try to post comment to the community by owner, forbidden", func(t *testing.T) {
		post := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
				Tags: model.Tags{
					{"h", communityID},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, post, privkeyUser1)
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindRepost,
				Content:   post.String(),
				Tags: model.Tags{
					{"h", communityID},
					{"e", post.GetID()},
					{"p", post.GetMasterPublicKey()},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyOwner)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	helperMustCloseRelay(t, relay)
}

func TestCommunityDefinition_ClosedCommunity_AnybodyPosting(t *testing.T) {
	relay := helperMustNewRelay(t, pubsubServers[0])
	ctx := context.Background()
	RegisterWSSubscriptionListener(func(ctx context.Context, s *model.Subscription) EventIterator {
		return query.GetStoredEvents(ctx, s)
	})
	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		require.True(t, len(events) > 0)
		require.NoError(t, query.AcceptEvents(ctx, events...))

		return nil
	})
	hVal, err := uuid.NewV7()
	require.NoError(t, err)
	communityID := hVal.String()
	privkeyOwner, pubkeyCommunityOwner := model.GenerateKeyPair()
	privkeyAdmin, pubkeyCommunityAdmin := model.GenerateKeyPair()
	privkeyModerator, pubkeyCommunityModerator := model.GenerateKeyPair()
	privkeyUser1, pubkeyUser1 := model.GenerateKeyPair()
	privkeyUser2, _ := model.GenerateKeyPair()

	t.Run("valid open community definition event with anybody posting, comments disabled", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"name", "some name"},
					{"description", "some description"},
					{"d", "dtagvalue"},
					{"closed"},
					{"settings", model.CommentsEnabledSettings, "false", fmt.Sprint(time.Now().Unix())},
					{"p", pubkeyCommunityAdmin, "", string(model.AdminRole)},
					{"p", pubkeyCommunityModerator, "", string(model.ModeratorRole)},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	//---- JOIN ----
	t.Run("join owner to the community", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    pubkeyCommunityOwner,
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyCommunityOwner},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, &ev, privkeyOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("join admin to the community", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyCommunityAdmin},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyAdmin)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("join moderator to the community", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyCommunityModerator},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyModerator)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("invite user1 by admin", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser1},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyAdmin)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("accept invitation by user1", func(t *testing.T) {
		adminAuthorizationEvent := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser1},
					{"expiration", fmt.Sprint(time.Now().Add(1 * time.Hour).Unix())},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, adminAuthorizationEvent, privkeyAdmin)
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser1},
					{"authorization", adminAuthorizationEvent.String()},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser1)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	//---- POST ----
	t.Run("try to post to the community by user that not in the community, forbidden", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
				Tags:      model.Tags{{"h", communityID}},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser2)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("try to post to the community by regular role user, ok", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
				Tags:      model.Tags{{"h", communityID}},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser1)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("try to post to the community by moderator, ok", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
				Tags:      model.Tags{{"h", communityID}},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyModerator)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("try to post to the community by admin, ok", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
				Tags:      model.Tags{{"h", communityID}},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyAdmin)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("try to post to the community by owner, ok", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
				Tags: model.Tags{
					{"h", communityID},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	helperMustCloseRelay(t, relay)
}

func TestCommunityDefinition_ChangeDefinition(t *testing.T) {
	relay := helperMustNewRelay(t, pubsubServers[0])
	ctx := context.Background()
	RegisterWSSubscriptionListener(func(ctx context.Context, s *model.Subscription) EventIterator {
		return query.GetStoredEvents(ctx, s)
	})
	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		require.True(t, len(events) > 0)
		require.NoError(t, query.AcceptEvents(ctx, events...))

		return nil
	})
	hVal, err := uuid.NewV7()
	require.NoError(t, err)
	communityID := hVal.String()
	privkeyOwner, _ := model.GenerateKeyPair()
	_, pubkeyCommunityAdmin := model.GenerateKeyPair()
	privkeyModerator, pubkeyCommunityModerator := model.GenerateKeyPair()
	privkeyUser1, _ := model.GenerateKeyPair()

	t.Run("create community", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"name", "some name"},
					{"description", "some description"},
					{"d", "dtagvalue"},
					{"open"},
					{"settings", model.CommentsEnabledSettings, "false", fmt.Sprint(time.Now().Unix())},
					{"settings", model.RoleRequiredForPostingSettings, "", fmt.Sprint(time.Now().Unix())},
					{"p", pubkeyCommunityAdmin, "", string(model.AdminRole)},
					{"p", pubkeyCommunityModerator, "", string(model.ModeratorRole)},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("try to change definition by any user", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityChangeDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"closed"},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser1)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("try to change community name by moderator", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityChangeDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"name", "new name"},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyModerator)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("try to change community description by moderator", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityChangeDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"description", "new description"},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyModerator)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("try to change community open/closed status definition by moderator", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityChangeDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"closed"},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyModerator)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("try to change community private/public status definition by moderator", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityChangeDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"private"},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyModerator)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("try to change community picture definition by moderator", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityChangeDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"imeta"},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyModerator)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("try to change community settings by moderator", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityChangeDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"settings", model.RoleRequiredForPostingSettings, string(model.ModeratorRole), fmt.Sprint(time.Now().Unix())},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyModerator)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	helperMustCloseRelay(t, relay)
}

func TestCommunity_TransferringOwnership(t *testing.T) {
	relay := helperMustNewRelay(t, pubsubServers[0])
	ctx := context.Background()
	RegisterWSSubscriptionListener(func(ctx context.Context, s *model.Subscription) EventIterator {
		return query.GetStoredEvents(ctx, s)
	})
	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		require.True(t, len(events) > 0)
		require.NoError(t, query.AcceptEvents(ctx, events...))

		return nil
	})
	hVal, err := uuid.NewV7()
	require.NoError(t, err)
	communityID := hVal.String()
	privkeyOwner, pubkeyCommunityOwner := model.GenerateKeyPair()
	privkeyAdmin, pubkeyCommunityAdmin := model.GenerateKeyPair()
	privkeyModerator, pubkeyCommunityModerator := model.GenerateKeyPair()
	_, pubkeyUser1 := model.GenerateKeyPair()
	privkeyUser2, pubkeyUser2 := model.GenerateKeyPair()

	t.Run("valid open community definition ", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"name", "some name"},
					{"description", "some description"},
					{"d", "dtagvalue"},
					{"open"},
					{"p", pubkeyCommunityAdmin, "", string(model.AdminRole)},
					{"p", pubkeyCommunityModerator, "", string(model.ModeratorRole)},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("attempt of transferring ownership of the community by non privileged user", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityOwnershipTransferring,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser2},
					{"a", fmt.Sprintf("%v:%v:%v", model.CustomIONKindCommunityDefinition, pubkeyUser1, communityID)},
				},
				Content: "reason",
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser2)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("attempt of transferring ownership of the community by moderator", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityOwnershipTransferring,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser2},
					{"a", fmt.Sprintf("%v:%v:%v", model.CustomIONKindCommunityDefinition, pubkeyCommunityModerator, communityID)},
				},
				Content: "reason",
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyModerator)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("attempt of transferring ownership of the community by admin", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityOwnershipTransferring,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser2},
					{"a", fmt.Sprintf("%v:%v:%v", model.CustomIONKindCommunityDefinition, pubkeyCommunityAdmin, communityID)},
				},
				Content: "reason",
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyAdmin)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("transferring ownership of community", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityOwnershipTransferring,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser1},
					{"a", fmt.Sprintf("%v:%v:%v", model.CustomIONKindCommunityDefinition, pubkeyCommunityOwner, communityID)},
				},
				Content: "reason",
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyOwner)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	helperMustCloseRelay(t, relay)
}

func TestCommunityChangeDefinitionApplyingPatches(t *testing.T) {
	relay := helperMustNewRelay(t, pubsubServers[0])
	ctx := context.Background()
	RegisterWSSubscriptionListener(func(ctx context.Context, s *model.Subscription) EventIterator {
		return query.GetStoredEvents(ctx, s)
	})
	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		require.True(t, len(events) > 0)
		require.NoError(t, query.AcceptEvents(ctx, events...))

		return nil
	})
	hVal, err := uuid.NewV7()
	require.NoError(t, err)
	communityID := hVal.String()
	privkeyOwner, pubkeyCommunityOwner := model.GenerateKeyPair()
	privkeyAdmin, pubkeyCommunityAdmin := model.GenerateKeyPair()
	privkeyModerator, pubkeyCommunityModerator := model.GenerateKeyPair()
	privkeyUser1, pubkeyUser1 := model.GenerateKeyPair()

	t.Run("valid open community definition event with anybody posting, comments disabled", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"name", "some name"},
					{"description", "some description"},
					{"open"},
					{"settings", model.CommentsEnabledSettings, "false", fmt.Sprint(time.Now().Add(-1 * time.Minute).Unix())},
					{"settings", model.RoleRequiredForPostingSettings, string(model.ModeratorRole), fmt.Sprint(time.Now().Add(-1 * time.Minute).Unix())},
					{"d", "dtagvalue"},
					{"p", pubkeyCommunityAdmin, "", string(model.AdminRole)},
					{"p", pubkeyCommunityModerator, "", string(model.ModeratorRole)},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	//---- JOIN ----
	t.Run("join owner to the community", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyCommunityOwner},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("join admin to the community", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyCommunityAdmin},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyAdmin)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("join moderator to the community", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyCommunityModerator},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyModerator)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("self-join user1 to the community", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser1},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser1)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("try to post to the community by moderator, ok", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
				Tags:      model.Tags{{"h", communityID}},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyModerator)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("try to post comment to the community by any user, forbidden, comments are disabled", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindRepost,
				Content:   "some text",
				Tags: model.Tags{
					{"h", communityID},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser1)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("change community definition event, set commentsEnabled=true, roleRequiredForPosting=admin", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityChangeDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"name", "some name"},
					{"description", "some description"},
					{"open"},
					{"settings", model.CommentsEnabledSettings, "true", fmt.Sprint(time.Now().Unix())},
					{"settings", model.RoleRequiredForPostingSettings, string(model.AdminRole), fmt.Sprint(time.Now().Unix())},
					{"p", pubkeyCommunityAdmin, "", string(model.AdminRole)},
					{"p", pubkeyCommunityModerator, "", string(model.ModeratorRole)},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("try to post to the community by moderator, now forbidden due to patches applying", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
				Tags:      model.Tags{{"h", communityID}},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyModerator)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("try to post comment to the community by admin, ok, comments now enabled", func(t *testing.T) {
		post := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
			},
		}
		helperSignWithMinLeadingZeroBits(t, post, privkeyUser1)
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindRepost,
				Content:   post.String(),
				Tags: model.Tags{
					{"h", communityID},
					{"e", post.GetID()},
					{"p", post.GetMasterPublicKey()},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyAdmin)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	helperMustCloseRelay(t, relay)
}

func TestCommunity_Deletion(t *testing.T) {
	relay := helperMustNewRelay(t, pubsubServers[0])
	ctx := context.Background()
	RegisterWSSubscriptionListener(func(ctx context.Context, s *model.Subscription) EventIterator {
		return query.GetStoredEvents(ctx, s)
	})
	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		require.True(t, len(events) > 0)
		require.NoError(t, query.AcceptEvents(ctx, events...))

		return nil
	})
	hVal, err := uuid.NewV7()
	require.NoError(t, err)
	communityID := hVal.String()
	privkeyOwner, pubkeyCommunityOwner := model.GenerateKeyPair()
	privkeyAdmin, pubkeyCommunityAdmin := model.GenerateKeyPair()
	privkeyModerator, pubkeyCommunityModerator := model.GenerateKeyPair()
	privkeyUser1, pubkeyUser1 := model.GenerateKeyPair()
	privkeyUser2, pubkeyUser2 := model.GenerateKeyPair()

	t.Run("valid open community definition ", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"name", "some name"},
					{"description", "some description"},
					{"open"},
					{"d", "dtagvalue"},
					{"p", pubkeyCommunityAdmin, "", string(model.AdminRole)},
					{"p", pubkeyCommunityModerator, "", string(model.ModeratorRole)},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	//---- JOIN ----
	t.Run("join owner to the community", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyCommunityOwner},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("join admin to the community", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyCommunityAdmin},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyAdmin)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("join moderator to the community", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyCommunityModerator},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyModerator)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("self-join user1 to the community", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser1},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser1)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("self-join user2 to the community", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser2},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser2)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	var post1 *model.Event
	//---- POST ----
	t.Run("try to post comment to the community by admin, ok", func(t *testing.T) {
		post1 = &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
				Tags: model.Tags{
					{"h", communityID},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, post1, privkeyUser1)
		require.NoError(t, relay.Publish(ctx, post1.Event))
	})
	t.Run("try to delete the post by user2, forbidden", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindDeletion,
				Content:   "some text",
				Tags: model.Tags{
					{"e", post1.GetID()},
					{"k", fmt.Sprint(nostr.KindTextNote)},
					{"a", fmt.Sprintf("%v:%v:%v", nostr.KindTextNote, pubkeyUser1, "")},
					{model.CustomIONTagOnBehalfOf, pubkeyUser2},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser2)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("try to delete the post by user1 - author, ok", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindDeletion,
				Tags: model.Tags{
					{"e", post1.GetID()},
					{"k", fmt.Sprint(nostr.KindTextNote)},
					{"a", fmt.Sprintf("%v:%v:%v", nostr.KindTextNote, pubkeyUser1, "")},
					{model.CustomIONTagOnBehalfOf, pubkeyUser1},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser1)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	var post2 *model.Event
	t.Run("try to post to the community by user1, ok", func(t *testing.T) {
		post2 = &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
				Tags:      model.Tags{{"h", communityID}},
			},
		}
		helperSignWithMinLeadingZeroBits(t, post2, privkeyUser1)
		require.NoError(t, relay.Publish(ctx, post2.Event))
	})
	t.Run("try to delete the post by moderator, ok", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindDeletion,
				Tags: model.Tags{
					{"e", post2.GetID()},
					{"k", fmt.Sprint(nostr.KindTextNote)},
					{"a", fmt.Sprintf("%v:%v:%v", nostr.KindTextNote, pubkeyUser1, "")},
					{model.CustomIONTagOnBehalfOf, pubkeyCommunityModerator},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyModerator)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	var post3 *model.Event
	t.Run("try to post to the community by user1, ok", func(t *testing.T) {
		post3 = &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Tags:      model.Tags{{"h", communityID}},
			},
		}
		helperSignWithMinLeadingZeroBits(t, post3, privkeyUser1)
		require.NoError(t, relay.Publish(ctx, post3.Event))
	})
	t.Run("try to delete the post by admin, ok", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindDeletion,
				Tags: model.Tags{
					{"e", post3.GetID()},
					{"k", fmt.Sprint(nostr.KindTextNote)},
					{"a", fmt.Sprintf("%v:%v:%v", nostr.KindTextNote, pubkeyUser1, "")},
					{model.CustomIONTagOnBehalfOf, pubkeyCommunityAdmin},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyAdmin)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	var post4 *model.Event
	t.Run("try to post to the community by user1, ok", func(t *testing.T) {
		post4 = &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
				Tags:      model.Tags{{"h", communityID}},
			},
		}
		helperSignWithMinLeadingZeroBits(t, post4, privkeyUser1)
		require.NoError(t, relay.Publish(ctx, post4.Event))
	})
	t.Run("try to delete the post by community owner, ok", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindDeletion,
				Tags: model.Tags{
					{"e", post4.GetID()},
					{"k", fmt.Sprint(nostr.KindTextNote)},
					{"a", fmt.Sprintf("%v:%v:%v", nostr.KindTextNote, pubkeyUser1, "")},
					{model.CustomIONTagOnBehalfOf, pubkeyCommunityOwner},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	var post5 *model.Event
	t.Run("create post by the community owner, ok", func(t *testing.T) {
		post5 = &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
				Tags:      model.Tags{{"h", communityID}},
			},
		}
		helperSignWithMinLeadingZeroBits(t, post5, privkeyOwner)
		require.NoError(t, relay.Publish(ctx, post5.Event))
	})
	t.Run("try to delete the post by admin, forbidden", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindDeletion,
				Tags: model.Tags{
					{"e", post5.GetID()},
					{"k", fmt.Sprint(nostr.KindTextNote)},
					{"a", fmt.Sprintf("%v:%v:%v", nostr.KindTextNote, pubkeyCommunityOwner, "")},
					{model.CustomIONTagOnBehalfOf, pubkeyCommunityAdmin},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyAdmin)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	helperMustCloseRelay(t, relay)
}

func TestCommunityBanUser(t *testing.T) {
	relay := helperMustNewRelay(t, pubsubServers[0])
	ctx := context.Background()
	RegisterWSSubscriptionListener(func(ctx context.Context, s *model.Subscription) EventIterator {
		return query.GetStoredEvents(ctx, s)
	})
	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		require.True(t, len(events) > 0)
		require.NoError(t, query.AcceptEvents(ctx, events...))

		return nil
	})
	hVal, err := uuid.NewV7()
	require.NoError(t, err)
	communityID := hVal.String()
	privkeyOwner, pubkeyCommunityOwner := model.GenerateKeyPair()
	_, pubkeyCommunityAdmin := model.GenerateKeyPair()
	_, pubkeyCommunityModerator := model.GenerateKeyPair()
	privkeyUser1, pubkeyUser1 := model.GenerateKeyPair()

	t.Run("valid open community definition event with anybody posting, comments disabled", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"name", "some name"},
					{"description", "some description"},
					{"open"},
					{"settings", model.CommentsEnabledSettings, "false", fmt.Sprint(time.Now().Unix())},
					{"d", "dtagvalue"},
					{"p", pubkeyCommunityAdmin, "", string(model.AdminRole)},
					{"p", pubkeyCommunityModerator, "", string(model.ModeratorRole)},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	//---- JOIN ----
	t.Run("join owner to the community", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    pubkeyCommunityOwner,
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyCommunityOwner},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, &ev, privkeyOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("self-join user1 to the community", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser1},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser1)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	//---- POST ----
	t.Run("try to post to the community by any user, ok", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
				Tags:      model.Tags{{"h", communityID}},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser1)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	//---- BAN ----
	t.Run("ban user by owner", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityBanUser,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser1},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("try to post to the community by any user, forbidden", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
				Tags:      model.Tags{{"h", communityID}},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser1)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	helperMustCloseRelay(t, relay)
}

func TestSubscriptionPrivateCommunity(t *testing.T) {
	hVal, err := uuid.NewV7()
	require.NoError(t, err)
	communityID := hVal.String()

	t.Cleanup(func() {
		RegisterEventMustAuthenticate(nil)
	})

	privkeyOwner, pubkeyCommunityOwner := model.GenerateKeyPair()
	privkeyUser1, pubkeyUser1 := model.GenerateKeyPair()

	RegisterWSSubscriptionListener(func(ctx context.Context, s *model.Subscription) EventIterator {
		return query.GetStoredEvents(ctx, s)
	})
	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		require.True(t, len(events) > 0)
		require.NoError(t, query.AcceptEvents(ctx, events...))

		return nil
	})

	RegisterEventMustAuthenticate(func(ctx context.Context, events ...*model.Event) bool {
		for _, event := range events {
			if event.Kind == nostr.KindArticle {
				return true
			}
		}
		return false
	})

	var ev1 model.Event
	relay := helperMustNewRelay(t, pubsubServers[0])
	t.Run("Regular", func(t *testing.T) {
		ev1.Kind = nostr.KindTextNote
		ev1.CreatedAt = 1
		ev1.Content = "test"
		helperSignWithMinLeadingZeroBits(t, &ev1, privkeyUser1)
		require.NoError(t, relay.Publish(context.Background(), ev1.Event))

	})
	t.Run("WithAuth", func(t *testing.T) {
		var ev model.Event

		ev.Kind = nostr.KindArticle
		ev.CreatedAt = 1
		ev.Content = "test"
		ev.Tags = model.Tags{
			{"title", "test"},
			{"d", "foo"},
		}
		helperSignWithMinLeadingZeroBits(t, &ev, privkeyUser1)
		err := relay.Publish(context.Background(), ev.Event)
		t.Logf("publish error: %v", err)
		require.Error(t, err)
		require.Contains(t, err.Error(), errAuthRequired.Error())
	})
	t.Run("DoAuth", func(t *testing.T) {
		err := relay.Auth(context.Background(), func(event *nostr.Event) error {
			subZeroEvent := model.Event{Event: *event}
			if err := subZeroEvent.SignWithAlg(privkeyUser1, model.SignAlgEDDSA, model.KeyAlgCurve25519); err != nil {
				return err
			}
			*event = subZeroEvent.Event

			return nil
		})
		require.NoError(t, err)
	})
	t.Run("PublishAfterAuth", func(t *testing.T) {
		var ev model.Event

		ev.Kind = nostr.KindArticle
		ev.CreatedAt = 2
		ev.Content = "test"
		ev.Tags = model.Tags{
			{"title", "test"},
			{"d", "foo"},
		}
		helperSignWithMinLeadingZeroBits(t, &ev, privkeyUser1)
		require.NoError(t, relay.Publish(context.Background(), ev.Event))

		events, err := relay.QuerySync(context.Background(), model.Filter{Kinds: []int{nostr.KindArticle}})
		require.NoError(t, err)
		require.Len(t, events, 1)
		require.Equal(t, ev.Event, *events[0])
	})

	t.Run("create private community", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"d", "dtagvalue"},
					{"name", "some name"},
					{"description", "some description"},
					{"closed"},
					{"private"},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyOwner)
		require.NoError(t, relay.Publish(context.TODO(), ev.Event))
	})
	t.Run("join owner to the community", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyCommunityOwner},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyOwner)
		require.NoError(t, relay.Publish(context.TODO(), ev.Event))
	})

	var ev2 *model.Event
	t.Run("try to post to the community by owner, ok", func(t *testing.T) {
		ev2 = &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Content:   "some text by owner",
				Tags: model.Tags{
					{"h", communityID},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev2, privkeyOwner)
		require.NoError(t, relay.Publish(context.TODO(), ev2.Event))
	})
	t.Run("select data by authorized user", func(t *testing.T) {
		var ev model.Event

		ev.Kind = nostr.KindArticle
		ev.CreatedAt = 2
		ev.Content = "test 2"
		ev.Tags = model.Tags{
			{"title", "test"},
			{"d", "foo"},
		}
		helperSignWithMinLeadingZeroBits(t, &ev, privkeyUser1)
		require.NoError(t, relay.Publish(context.Background(), ev.Event))

		events, err := relay.QuerySync(context.Background(), model.Filter{Kinds: []int{nostr.KindTextNote}})
		require.NoError(t, err)
		require.Len(t, events, 1)
		require.Equal(t, ev1.Event, *events[0])
	})

	// -- Add user1 to private the community
	ownerAuthorizationEvent := &model.Event{
		Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      model.CustomIONKindCommunityJoin,
			Tags: model.Tags{
				{"h", communityID},
				{"p", pubkeyCommunityOwner},
				{"expiration", fmt.Sprint(time.Now().Add(1 * time.Hour).Unix())},
			},
		},
	}
	helperSignWithMinLeadingZeroBits(t, ownerAuthorizationEvent, privkeyOwner)
	t.Run("send invitation to user1", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser1},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyOwner)
		require.NoError(t, relay.Publish(context.TODO(), ev.Event))
	})
	t.Run("accept invitation by user1 to join to the community", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser1},
					{"authorization", ownerAuthorizationEvent.String()},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser1)
		require.NoError(t, relay.Publish(context.TODO(), ev.Event))
	})

	t.Run("select data by authorized user that is in the community now", func(t *testing.T) {
		events, err := relay.QuerySync(context.Background(), model.Filter{Kinds: []int{nostr.KindTextNote}})
		require.NoError(t, err)
		require.Len(t, events, 2)

		require.Equal(t, ev2.Event, *events[0])
		require.Equal(t, ev1.Event, *events[1])
	})

	helperMustCloseRelay(t, relay)
}
