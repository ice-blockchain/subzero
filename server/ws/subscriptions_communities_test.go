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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.CustomIONKindCommunityDefinition,
				Tags: model.Tags{
					{"h", communityID},
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
	t.Run("join moderator to the community", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
	t.Run("join by owner to the community", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
	t.Run("join by owner another user to the community", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
	t.Run("join by admin another user to the community", func(t *testing.T) {
		adminAuthorizationEvent := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
	t.Run("join by moderator another user to the community", func(t *testing.T) {
		moderatorAuthorizationEvent := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
	t.Run("join by moderator another user to the community", func(t *testing.T) {
		moderatorAuthorizationEvent := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
	t.Run("try to post to the community by any user, forbidden", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
			},
		}
		helperSignWithMinLeadingZeroBits(t, post, privkeyUser1)
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
			},
		}
		helperSignWithMinLeadingZeroBits(t, post, privkeyUser1)
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    pubkeyCommunityAdmin,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
			},
		}
		helperSignWithMinLeadingZeroBits(t, post, privkeyUser1)
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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

	t.Run("valid open community definition event with anybody posting, comments disabled", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.CustomIONKindCommunityDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"name", "some name"},
					{"description", "some description"},
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
	t.Run("join by admin another user to the community", func(t *testing.T) {
		adminAuthorizationEvent := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
	t.Run("try to post to the community by any user, ok", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.CustomIONKindCommunityDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"name", "some name"},
					{"description", "some description"},
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.CustomIONKindCommunityDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"name", "some name"},
					{"description", "some description"},
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.CustomIONKindCommunityDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"name", "some name"},
					{"description", "some description"},
					{"open"},
					{"settings", model.CommentsEnabledSettings, "false", fmt.Sprint(time.Now().Add(-1 * time.Minute).Unix())},
					{"settings", model.RoleRequiredForPostingSettings, string(model.ModeratorRole), fmt.Sprint(time.Now().Add(-1 * time.Minute).Unix())},
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
	t.Run("join by any user to the community", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
			},
		}
		helperSignWithMinLeadingZeroBits(t, post, privkeyUser1)
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.CustomIONKindCommunityDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"name", "some name"},
					{"description", "some description"},
					{"open"},
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
	t.Run("join by moderator another user to the community", func(t *testing.T) {
		moderatorAuthorizationEvent := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser1},
					{"expiration", fmt.Sprint(time.Now().Add(1 * time.Hour).Unix())},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, moderatorAuthorizationEvent, privkeyModerator)
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser1},
					{"authorization", moderatorAuthorizationEvent.String()},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser1)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("join by moderator another user to the community", func(t *testing.T) {
		moderatorAuthorizationEvent := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser2},
					{"expiration", fmt.Sprint(time.Now().Add(1 * time.Hour).Unix())},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, moderatorAuthorizationEvent, privkeyModerator)
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser2},
					{"authorization", moderatorAuthorizationEvent.String()},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser2)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("join by moderator another user to the community", func(t *testing.T) {
		moderatorAuthorizationEvent := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser2},
					{"expiration", fmt.Sprint(time.Now().Add(1 * time.Hour).Unix())},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, moderatorAuthorizationEvent, privkeyModerator)
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser2},
					{"authorization", moderatorAuthorizationEvent.String()},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser2)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	var post1 *model.Event
	// //---- POST ----
	t.Run("try to post comment to the community by admin, ok", func(t *testing.T) {
		post1 = &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.CustomIONKindCommunityDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"name", "some name"},
					{"description", "some description"},
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
	t.Run("join by admin another user to the community", func(t *testing.T) {
		ownerAuthorizationEvent := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser1},
					{"expiration", fmt.Sprint(time.Now().Add(1 * time.Hour).Unix())},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ownerAuthorizationEvent, privkeyOwner)
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
	// //---- POST ----
	t.Run("try to post to the community by any user, ok", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
