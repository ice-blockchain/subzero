// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ice-blockchain/subzero/model"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

func TestCommunityDefinition_ClosedCommunity_ModeratorPosting_CommentsEnabled(t *testing.T) {
	t.Parallel()
	db := helperNewDatabase(t)
	defer db.Close()

	communityID := "community1"
	ownerCommunityPubkey := "owner"
	adminCommunityPubkey := "admin"
	moderatorCommunityPubkey := "moderator"
	anyUserPubKey1 := "user1"
	anyUserPubKey2 := "user2"
	anyUserPubKey3 := "user3"
	anyUserPubKey4 := "user4"
	anyUserPubKey5 := "user5"

	t.Run("valid closed community definition event with moderator posting, comments enabled", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    ownerCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"name", "some name"},
					{"description", "some description"},
					{"closed"},
					{"settings", "comments_enabled", "false", fmt.Sprint(time.Now().Add(-3 * time.Hour).Unix())},
					{"settings", "comments_enabled", "true", fmt.Sprint(time.Now().Add(-2 * time.Hour).Unix())},
					{"settings", "comments_enabled", "false", fmt.Sprint(time.Now().Add(-1 * time.Hour).Unix())},
					{"settings", "comments_enabled", "true", fmt.Sprint(time.Now().Unix())},
					{"settings", "role_required_for_posting", "admin", fmt.Sprint(time.Now().Add(-3 * time.Hour).Unix())},
					{"settings", "role_required_for_posting", "moderator", fmt.Sprint(time.Now().Add(-2 * time.Hour).Unix())},
					{"settings", "role_required_for_posting", "admin", fmt.Sprint(time.Now().Add(-1 * time.Hour).Unix())},
					{"settings", "role_required_for_posting", "moderator", fmt.Sprint(time.Now().Unix())},
					{"p", adminCommunityPubkey, "", "admin"},
					{"p", moderatorCommunityPubkey, "", "moderator"},
				},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
		require.Len(t, helperSelectEvents(t, db, model.Filter{
			Kinds: []int{model.KindCommunityDefinition},
			Tags:  model.TagMap{}.SetLiterals("h", communityID),
		}), 1)
	})
	ownerAuthorizationEvent := model.Event{
		Event: nostr.Event{
			ID:        uuid.NewString(),
			PubKey:    ownerCommunityPubkey,
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindCommunityJoin,
			Tags: model.Tags{
				{"h", communityID},
				{"p", anyUserPubKey1},
			},
		},
	}
	//---- JOIN ----
	t.Run("join owner to the community", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    ownerCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", ownerCommunityPubkey},
				},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
		require.Len(t, helperSelectEvents(t, db, model.Filter{
			Kinds: []int{model.KindCommunityJoin},
			Tags:  model.TagMap{}.SetLiterals("p", ownerCommunityPubkey),
		}), 1)
	})
	t.Run("join admin to the community", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    adminCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", adminCommunityPubkey},
				},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
		require.Len(t, helperSelectEvents(t, db, model.Filter{
			Kinds: []int{model.KindCommunityJoin},
			Tags:  model.TagMap{}.SetLiterals("p", adminCommunityPubkey),
		}), 1)
	})
	t.Run("join moderator to the community", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    moderatorCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", moderatorCommunityPubkey},
				},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
		require.Len(t, helperSelectEvents(t, db, model.Filter{
			Kinds: []int{model.KindCommunityJoin},
			Tags:  model.TagMap{}.SetLiterals("p", adminCommunityPubkey),
		}), 1)
	})
	t.Run("join by owner to the community", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    anyUserPubKey1,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", anyUserPubKey1},
					{"authorization", ownerAuthorizationEvent.String()},
				},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
		require.Len(t, helperSelectEvents(t, db, model.Filter{
			Kinds: []int{model.KindCommunityJoin},
			Tags:  model.TagMap{}.SetLiterals("p", anyUserPubKey1),
		}), 1)
	})
	t.Run("join by owner another user to the community", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    anyUserPubKey2,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", anyUserPubKey2},
					{"authorization", ownerAuthorizationEvent.String()},
				},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
		require.Len(t, helperSelectEvents(t, db, model.Filter{
			Kinds: []int{model.KindCommunityJoin},
			Tags:  model.TagMap{}.SetLiterals("p", anyUserPubKey2),
		}), 1)
	})
	t.Run("join by admin another user to the community", func(t *testing.T) {
		adminAuthorizationEvent := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    adminCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", anyUserPubKey3},
				},
			},
		}
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    anyUserPubKey3,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", anyUserPubKey3},
					{"authorization", adminAuthorizationEvent.String()},
				},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
		require.Len(t, helperSelectEvents(t, db, model.Filter{
			Kinds: []int{model.KindCommunityJoin},
			Tags:  model.TagMap{}.SetLiterals("p", anyUserPubKey2),
		}), 1)
	})
	t.Run("join by moderator another user to the community", func(t *testing.T) {
		moderatorAuthorizationEvent := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    adminCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", anyUserPubKey4},
				},
			},
		}
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    anyUserPubKey4,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", anyUserPubKey4},
					{"authorization", moderatorAuthorizationEvent.String()},
				},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
		require.Len(t, helperSelectEvents(t, db, model.Filter{
			Kinds: []int{model.KindCommunityJoin},
			Tags:  model.TagMap{}.SetLiterals("p", anyUserPubKey2),
		}), 1)
	})
	t.Run("join by moderator another user to the community", func(t *testing.T) {
		moderatorAuthorizationEvent := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    adminCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", anyUserPubKey5},
				},
			},
		}
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    anyUserPubKey5,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", anyUserPubKey5},
					{"authorization", moderatorAuthorizationEvent.String()},
				},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
		require.Len(t, helperSelectEvents(t, db, model.Filter{
			Kinds: []int{model.KindCommunityJoin},
			Tags:  model.TagMap{}.SetLiterals("p", anyUserPubKey2),
		}), 1)
	})
	//---- POST ----
	t.Run("try to post to the community by any user, forbidden", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    anyUserPubKey1,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
				Tags:      model.Tags{{"h", communityID}},
			},
		}
		require.Error(t, db.AcceptEvents(context.Background(), &ev))
	})
	t.Run("try to post to the community by moderator, ok", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    moderatorCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
				Tags:      model.Tags{{"h", communityID}},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
	})
	t.Run("try to post to the community by admin, ok", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    adminCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
				Tags:      model.Tags{{"h", communityID}},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
	})
	t.Run("try to post to the community by owner, ok", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    ownerCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
				Tags: model.Tags{
					{"h", communityID},
				},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
	})
	//---- COMMENT ----
	t.Run("try to post comment to the community by any user, ok", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    anyUserPubKey1,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindComment,
				Content:   "some text",
				Tags: model.Tags{
					{"h", communityID},
				},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
	})
	t.Run("try to post comment to the community by moderator, ok", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    moderatorCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindComment,
				Content:   "some text",
				Tags: model.Tags{
					{"h", communityID},
				},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
	})
	t.Run("try to post comment to the community by admin, ok", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    adminCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindComment,
				Content:   "some text",
				Tags: model.Tags{
					{"h", communityID},
				},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
	})
	t.Run("try to post comment to the community by owner, ok", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    ownerCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindComment,
				Content:   "some text",
				Tags: model.Tags{
					{"h", communityID},
				},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
	})

	//---- CHANGE DEFINITION ----
	t.Run("promoting user to admin by owner, ok", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    ownerCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityChangeDefinition,
				Content:   "some text",
				Tags: model.Tags{
					{"h", communityID},
					{"p", anyUserPubKey2, "", "admin"},
				},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
	})
	t.Run("try to ban any user by new admin, ok as owner should update the community definition first by 31750 event", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    anyUserPubKey2,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityBanUser,
				Content:   "some text",
				Tags: model.Tags{
					{"h", communityID},
					{"p", anyUserPubKey3},
				},
			},
		}
		require.Error(t, db.AcceptEvents(context.Background(), &ev))
	})
	t.Run("demoting admin by owner, ok", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    ownerCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityChangeDefinition,
				Content:   "some text",
				Tags: model.Tags{
					{"h", communityID},
					{"p", anyUserPubKey2, "", ""},
				},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
	})
	t.Run("try to ban any user by denoted admin, forbidden", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    anyUserPubKey2,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityBanUser,
				Content:   "some text",
				Tags: model.Tags{
					{"h", communityID},
					{"p", anyUserPubKey3},
				},
			},
		}
		require.Error(t, db.AcceptEvents(context.Background(), &ev))
	})
	t.Run("try to promote user to admin by moderator, forbidden", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    moderatorCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityChangeDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"p", anyUserPubKey2, "", "admin"},
				},
			},
		}
		require.Error(t, db.AcceptEvents(context.Background(), &ev))
	})
	t.Run("try to demote admin by moderator, forbidden", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    moderatorCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityChangeDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"p", adminCommunityPubkey, "", ""},
				},
			},
		}
		require.Error(t, db.AcceptEvents(context.Background(), &ev))
	})
	//---- BAN USER ----
	t.Run("try to ban any user by owner, ok", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    ownerCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityBanUser,
				Content:   "some text",
				Tags: model.Tags{
					{"h", communityID},
					{"p", anyUserPubKey1},
				},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
	})
	t.Run("try to ban any user by admin, ok", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    adminCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityBanUser,
				Content:   "some text",
				Tags: model.Tags{
					{"h", communityID},
					{"p", anyUserPubKey2},
				},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
	})
	t.Run("try to ban any user by moderator, ok", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    moderatorCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityBanUser,
				Content:   "some text",
				Tags: model.Tags{
					{"h", communityID},
					{"p", anyUserPubKey3},
				},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
	})
	t.Run("try to ban any user by any user, forbidden", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    anyUserPubKey4,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityBanUser,
				Content:   "some text",
				Tags: model.Tags{
					{"h", communityID},
					{"p", anyUserPubKey5},
				},
			},
		}
		require.Error(t, db.AcceptEvents(context.Background(), &ev))
	})
	t.Run("try to ban admin user by moderator, forbidden", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    moderatorCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityBanUser,
				Content:   "some text",
				Tags: model.Tags{
					{"h", communityID},
					{"p", adminCommunityPubkey},
				},
			},
		}
		require.Error(t, db.AcceptEvents(context.Background(), &ev))
	})
	t.Run("try to ban owner user by moderator, forbidden", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    moderatorCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityBanUser,
				Content:   "some text",
				Tags: model.Tags{
					{"h", communityID},
					{"p", ownerCommunityPubkey},
				},
			},
		}
		require.Error(t, db.AcceptEvents(context.Background(), &ev))
	})
	t.Run("try to ban owner user by admin, forbidden", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    adminCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityBanUser,
				Content:   "some text",
				Tags: model.Tags{
					{"h", communityID},
					{"p", ownerCommunityPubkey},
				},
			},
		}
		require.Error(t, db.AcceptEvents(context.Background(), &ev))
	})
}

func TestCommunityDefinition_OpenedCommunity_AnyPosting_CommentsDisabled(t *testing.T) {
	t.Parallel()
	db := helperNewDatabase(t)
	defer db.Close()

	communityID := "community1"
	ownerCommunityPubkey := "owner"
	adminCommunityPubkey := "admin"
	moderatorCommunityPubkey := "moderator"
	anyUserPubKey1 := "user1"

	t.Run("valid open community definition event with anybody posting, comments disabled", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    ownerCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"name", "some name"},
					{"description", "some description"},
					{"open"},
					{"settings", "comments_enabled", "false", fmt.Sprint(time.Now().Unix())},
					{"p", adminCommunityPubkey, "", "admin"},
					{"p", moderatorCommunityPubkey, "", "moderator"},
				},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
		require.Len(t, helperSelectEvents(t, db, model.Filter{
			Kinds: []int{model.KindCommunityDefinition},
			Tags:  model.TagMap{}.SetLiterals("h", communityID),
		}), 1)
	})
	//---- JOIN ----
	t.Run("join owner to the community", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    ownerCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", ownerCommunityPubkey},
				},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
		require.Len(t, helperSelectEvents(t, db, model.Filter{
			Kinds: []int{model.KindCommunityJoin},
			Tags:  model.TagMap{}.SetLiterals("p", ownerCommunityPubkey),
		}), 1)
	})
	t.Run("join admin to the community", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    adminCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", adminCommunityPubkey},
				},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
		require.Len(t, helperSelectEvents(t, db, model.Filter{
			Kinds: []int{model.KindCommunityJoin},
			Tags:  model.TagMap{}.SetLiterals("p", adminCommunityPubkey),
		}), 1)
	})
	t.Run("join moderator to the community", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    moderatorCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", moderatorCommunityPubkey},
				},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
		require.Len(t, helperSelectEvents(t, db, model.Filter{
			Kinds: []int{model.KindCommunityJoin},
			Tags:  model.TagMap{}.SetLiterals("p", adminCommunityPubkey),
		}), 1)
	})
	t.Run("join by owner to the community", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    anyUserPubKey1,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", anyUserPubKey1},
				},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
		require.Len(t, helperSelectEvents(t, db, model.Filter{
			Kinds: []int{model.KindCommunityJoin},
			Tags:  model.TagMap{}.SetLiterals("p", anyUserPubKey1),
		}), 1)
	})
	//---- POST ----
	t.Run("try to post to the community by any user, ok", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    anyUserPubKey1,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
				Tags:      model.Tags{{"h", communityID}},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
	})
	t.Run("try to post to the community by moderator, ok", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    moderatorCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
				Tags:      model.Tags{{"h", communityID}},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
	})
	t.Run("try to post to the community by admin, ok", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    adminCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
				Tags:      model.Tags{{"h", communityID}},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
	})
	t.Run("try to post to the community by owner, ok", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    ownerCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
				Tags: model.Tags{
					{"h", communityID},
				},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
	})
	//---- COMMENT ----
	t.Run("try to post comment to the community by any user, forbidden, comments are disabled", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    anyUserPubKey1,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindComment,
				Content:   "some text",
				Tags: model.Tags{
					{"h", communityID},
				},
			},
		}
		require.Error(t, db.AcceptEvents(context.Background(), &ev))
	})
	t.Run("try to post comment to the community by moderator, forbidden, comments are disabled", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    moderatorCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindComment,
				Content:   "some text",
				Tags: model.Tags{
					{"h", communityID},
				},
			},
		}
		require.Error(t, db.AcceptEvents(context.Background(), &ev))
	})
	t.Run("try to post comment to the community by admin, forbidden, comments are disabled", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    adminCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindComment,
				Content:   "some text",
				Tags: model.Tags{
					{"h", communityID},
				},
			},
		}
		require.Error(t, db.AcceptEvents(context.Background(), &ev))
	})
	t.Run("try to post comment to the community by owner, forbidden, comments are disabled", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    ownerCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindComment,
				Content:   "some text",
				Tags: model.Tags{
					{"h", communityID},
				},
			},
		}
		require.Error(t, db.AcceptEvents(context.Background(), &ev))
	})
}

func TestCommunityDefinition_ChangeDefinition(t *testing.T) {
	t.Parallel()
	db := helperNewDatabase(t)
	defer db.Close()

	communityID := "community1"
	ownerCommunityPubkey := "owner"
	adminCommunityPubkey := "admin"
	moderatorCommunityPubkey := "moderator"
	anyUserPubKey1 := "user1"

	t.Run("create community", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    ownerCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"name", "some name"},
					{"description", "some description"},
					{"open"},
					{"settings", "comments_enabled", "false", fmt.Sprint(time.Now().Unix())},
					{"settings", "role_required_for_posting", "", fmt.Sprint(time.Now().Unix())},
					{"p", adminCommunityPubkey, "", "admin"},
					{"p", moderatorCommunityPubkey, "", "moderator"},
				},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
		require.Len(t, helperSelectEvents(t, db, model.Filter{
			Kinds: []int{model.KindCommunityDefinition},
			Tags:  model.TagMap{}.SetLiterals("h", communityID),
		}), 1)
	})
	t.Run("try to change definition by any user", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    anyUserPubKey1,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityChangeDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"closed"},
				},
			},
		}
		require.Error(t, db.AcceptEvents(context.Background(), &ev))
	})
	t.Run("try to change community name by moderator", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    moderatorCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityChangeDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"name", "new name"},
				},
			},
		}
		require.Error(t, db.AcceptEvents(context.Background(), &ev))
	})
	t.Run("try to change community description by moderator", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    moderatorCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityChangeDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"description", "new description"},
				},
			},
		}
		require.Error(t, db.AcceptEvents(context.Background(), &ev))
	})
	t.Run("try to change community open/closed status definition by moderator", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    moderatorCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityChangeDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"closed"},
				},
			},
		}
		require.Error(t, db.AcceptEvents(context.Background(), &ev))
	})
	t.Run("try to change community private/public status definition by moderator", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    moderatorCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityChangeDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"private"},
				},
			},
		}
		require.Error(t, db.AcceptEvents(context.Background(), &ev))
	})
	t.Run("try to change community picture definition by moderator", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    moderatorCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityChangeDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"imeta"},
				},
			},
		}
		require.Error(t, db.AcceptEvents(context.Background(), &ev))
	})
	t.Run("try to change community picture definition by moderator", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    moderatorCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityChangeDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"settings", "role_required_for_posting", "moderator", fmt.Sprint(time.Now().Unix())},
				},
			},
		}
		require.Error(t, db.AcceptEvents(context.Background(), &ev))
	})
}

func TestCommunity_Deletion(t *testing.T) {
	t.Parallel()
	db := helperNewDatabase(t)
	defer db.Close()

	communityID := "community1"
	ownerCommunityPubkey := "owner"
	adminCommunityPubkey := "admin"
	moderatorCommunityPubkey := "moderator"
	anyUserPubKey1 := "user1"
	anyUserPubKey2 := "user2"
	anyUserPubKey3 := "user3"

	t.Run("valid open community definition ", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    ownerCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"name", "some name"},
					{"description", "some description"},
					{"open"},
					{"p", adminCommunityPubkey, "", "admin"},
					{"p", moderatorCommunityPubkey, "", "moderator"},
				},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
		require.Len(t, helperSelectEvents(t, db, model.Filter{
			Kinds: []int{model.KindCommunityDefinition},
			Tags:  model.TagMap{}.SetLiterals("h", communityID),
		}), 1)
	})
	//---- JOIN ----
	t.Run("join owner to the community", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    ownerCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", ownerCommunityPubkey},
				},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
		require.Len(t, helperSelectEvents(t, db, model.Filter{
			Kinds: []int{model.KindCommunityJoin},
			Tags:  model.TagMap{}.SetLiterals("p", ownerCommunityPubkey),
		}), 1)
	})
	t.Run("join admin to the community", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    adminCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", adminCommunityPubkey},
				},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
		require.Len(t, helperSelectEvents(t, db, model.Filter{
			Kinds: []int{model.KindCommunityJoin},
			Tags:  model.TagMap{}.SetLiterals("p", adminCommunityPubkey),
		}), 1)
	})
	t.Run("join moderator to the community", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    moderatorCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", moderatorCommunityPubkey},
				},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
		require.Len(t, helperSelectEvents(t, db, model.Filter{
			Kinds: []int{model.KindCommunityJoin},
			Tags:  model.TagMap{}.SetLiterals("p", adminCommunityPubkey),
		}), 1)
	})
	t.Run("join user1 to the community", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    anyUserPubKey1,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", anyUserPubKey1},
				},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
		require.Len(t, helperSelectEvents(t, db, model.Filter{
			Kinds: []int{model.KindCommunityJoin},
			Tags:  model.TagMap{}.SetLiterals("p", anyUserPubKey1),
		}), 1)
	})
	t.Run("join user2 to the community", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    anyUserPubKey2,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", anyUserPubKey2},
				},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
		require.Len(t, helperSelectEvents(t, db, model.Filter{
			Kinds: []int{model.KindCommunityJoin},
			Tags:  model.TagMap{}.SetLiterals("p", anyUserPubKey2),
		}), 1)
	})
	t.Run("join user3 to the community", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    anyUserPubKey3,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", anyUserPubKey3},
				},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
		require.Len(t, helperSelectEvents(t, db, model.Filter{
			Kinds: []int{model.KindCommunityJoin},
			Tags:  model.TagMap{}.SetLiterals("p", anyUserPubKey2),
		}), 1)
	})
	postID1 := "post1"
	//---- POST ----
	t.Run("try to post to the community by any user, ok", func(t *testing.T) {

		ev := model.Event{
			Event: nostr.Event{
				ID:        postID1,
				PubKey:    anyUserPubKey1,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
				Tags:      model.Tags{{"h", communityID}},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
	})
	t.Run("try to delete the post by user2, forbidden", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    anyUserPubKey2,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindDeletion,
				Content:   "some text",
				Tags:      model.Tags{{"e", postID1}, {"k", fmt.Sprint(nostr.KindTextNote)}, {"a", fmt.Sprintf("%v:%v:%v", nostr.KindTextNote, anyUserPubKey1, "d")}},
			},
		}
		require.Error(t, db.AcceptEvents(context.Background(), &ev))
	})
	t.Run("try to delete the post by user1 - author, ok", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    anyUserPubKey1,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindDeletion,
				Content:   "some text",
				Tags:      model.Tags{{"e", postID1}, {"k", fmt.Sprint(nostr.KindTextNote)}, {"a", fmt.Sprintf("%v:%v:%v", nostr.KindTextNote, anyUserPubKey1, "d")}},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
		require.Len(t, helperSelectEvents(t, db, model.Filter{
			Kinds: []int{nostr.KindTextNote},
			Tags:  model.TagMap{}.SetLiterals("h", communityID),
			IDs:   []string{postID1},
		}), 0)
	})
	postID2 := "post2"
	t.Run("try to post to the community by user1, ok", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        postID2,
				PubKey:    anyUserPubKey1,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
				Tags:      model.Tags{{"h", communityID}},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
	})
	t.Run("try to delete the post by moderator, ok", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    moderatorCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindDeletion,
				Content:   "some text",
				Tags:      model.Tags{{"e", postID2}, {"k", fmt.Sprint(nostr.KindTextNote)}},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
		require.Len(t, helperSelectEvents(t, db, model.Filter{
			Kinds: []int{nostr.KindTextNote},
			Tags:  model.TagMap{}.SetLiterals("h", communityID),
			IDs:   []string{postID2},
		}), 0)
	})
	postID3 := "post3"
	t.Run("try to post to the community by user1, ok", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        postID3,
				PubKey:    anyUserPubKey1,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
				Tags:      model.Tags{{"h", communityID}},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
	})
	t.Run("try to delete the post by admin, ok", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    adminCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindDeletion,
				Content:   "some text",
				Tags:      model.Tags{{"e", postID3}, {"k", fmt.Sprint(nostr.KindTextNote)}},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
		require.Len(t, helperSelectEvents(t, db, model.Filter{
			Kinds: []int{nostr.KindTextNote},
			Tags:  model.TagMap{}.SetLiterals("h", communityID),
			IDs:   []string{postID3},
		}), 0)
	})
	postID4 := "post4"
	t.Run("try to post to the community by user1, ok", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        postID4,
				PubKey:    anyUserPubKey1,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindTextNote,
				Content:   "some text",
				Tags:      model.Tags{{"h", communityID}},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
	})
	t.Run("try to delete the post by community owner, ok", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    ownerCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindDeletion,
				Content:   "some text",
				Tags:      model.Tags{{"e", postID4}, {"k", fmt.Sprint(nostr.KindTextNote)}},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
		require.Len(t, helperSelectEvents(t, db, model.Filter{
			Kinds: []int{nostr.KindTextNote},
			Tags:  model.TagMap{}.SetLiterals("h", communityID),
			IDs:   []string{postID4},
		}), 0)
	})
}

func TestCommunity_TransferringOwnership(t *testing.T) {
	t.Parallel()
	db := helperNewDatabase(t)
	defer db.Close()

	communityID := "community1"
	ownerCommunityPubkey := "owner"
	adminCommunityPubkey := "admin"
	moderatorCommunityPubkey := "moderator"
	anyUserPubKey1 := "user1"
	anyUserPubKey2 := "user2"

	t.Run("valid open community definition ", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    ownerCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"name", "some name"},
					{"description", "some description"},
					{"open"},
					{"p", adminCommunityPubkey, "", "admin"},
					{"p", moderatorCommunityPubkey, "", "moderator"},
				},
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
		require.Len(t, helperSelectEvents(t, db, model.Filter{
			Kinds: []int{model.KindCommunityDefinition},
			Tags:  model.TagMap{}.SetLiterals("h", communityID),
		}), 1)
	})
	t.Run("attempt of transferring ownership of the community by non privileged user", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    anyUserPubKey1,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityOwnershipTransferring,
				Tags: model.Tags{
					{"h", communityID},
					{"p", anyUserPubKey2},
					{"a", fmt.Sprintf("%v:%v:%v", model.KindCommunityDefinition, anyUserPubKey1, communityID)},
				},
				Content: "reason",
			},
		}
		require.Error(t, db.AcceptEvents(context.Background(), &ev))
	})
	t.Run("attempt of transferring ownership of the community by moderator", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    moderatorCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityOwnershipTransferring,
				Tags: model.Tags{
					{"h", communityID},
					{"p", anyUserPubKey2},
					{"a", fmt.Sprintf("%v:%v:%v", model.KindCommunityDefinition, moderatorCommunityPubkey, communityID)},
				},
				Content: "reason",
			},
		}
		require.Error(t, db.AcceptEvents(context.Background(), &ev))
	})
	t.Run("attempt of transferring ownership of the community by admin", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    adminCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityOwnershipTransferring,
				Tags: model.Tags{
					{"h", communityID},
					{"p", anyUserPubKey2},
					{"a", fmt.Sprintf("%v:%v:%v", model.KindCommunityDefinition, adminCommunityPubkey, communityID)},
				},
				Content: "reason",
			},
		}
		require.Error(t, db.AcceptEvents(context.Background(), &ev))
	})
	t.Run("transferring ownership of community", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    ownerCommunityPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindCommunityOwnershipTransferring,
				Tags: model.Tags{
					{"h", communityID},
					{"p", anyUserPubKey1},
					{"a", fmt.Sprintf("%v:%v:%v", model.KindCommunityDefinition, ownerCommunityPubkey, communityID)},
				},
				Content: "reason",
			},
		}
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
	})
}

func TestGetCommunityRoleByPubkey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		pubkey   string
		defEvent model.Event
		wantRole string
	}{
		{
			name:     "no author",
			pubkey:   "foo",
			wantRole: "",
		},
		{
			name:   "author",
			pubkey: "foo",
			defEvent: model.Event{
				Event: nostr.Event{
					PubKey: "foo",
				},
			},
			wantRole: "owner",
		},
		{
			name:   "moderator",
			pubkey: "foo",
			defEvent: model.Event{
				Event: nostr.Event{
					PubKey: "bar",
					Tags:   model.Tags{{"p", "foo", "", "moderator"}},
				},
			},
			wantRole: "moderator",
		},
		{
			name:   "admin",
			pubkey: "foo",
			defEvent: model.Event{
				Event: nostr.Event{
					PubKey: "bar",
					Tags:   model.Tags{{"p", "foo", "", "admin"}},
				},
			},
			wantRole: "admin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			role := getCommunityRoleByPubkey(tt.pubkey, &tt.defEvent)

			if role != tt.wantRole {
				t.Errorf("getCommunityRoleByPubkey() = %q, want %q", role, tt.wantRole)
			}
		})
	}
}

func TestGetLatestSettingsTag(t *testing.T) {
	t.Parallel()
	now := time.Now()

	tests := []struct {
		name        string
		settingsTag string
		community   model.Event
		wantTag     *model.Tag
	}{
		{
			name:        "no settings tag",
			settingsTag: "foo",
			community: model.Event{
				Event: nostr.Event{
					Tags: model.Tags{},
				},
			},
			wantTag: nil,
		},
		{
			name:        "settings tag",
			settingsTag: "foo",
			community: model.Event{
				Event: nostr.Event{
					Tags: model.Tags{
						{"settings", "foo", "0"},
					},
				},
			},
			wantTag: nil,
		},
		{
			name:        "multiple settings tags",
			settingsTag: "foo",
			community: model.Event{
				Event: nostr.Event{
					Tags: model.Tags{
						{"settings", "foo", "0", fmt.Sprint(now.Add(-5 * time.Minute).Unix())},
						{"settings", "foo", "1", fmt.Sprint(now.Add(-1 * time.Minute).Unix())},
						{"settings", "foo", "3", fmt.Sprint(now.Add(-3 * time.Minute).Unix())},
						{"settings", "foo", "2", fmt.Sprint(now.Add(-4 * time.Minute).Unix())},
					},
				},
			},
			wantTag: &model.Tag{"settings", "foo", "1", fmt.Sprint(now.Add(-1 * time.Minute).Unix())},
		},
		{
			name:        "wrong unix timestamps",
			settingsTag: "foo",
			community: model.Event{
				Event: nostr.Event{
					Tags: model.Tags{
						{"settings", "foo", "0", "x"},
						{"settings", "foo", "1", "y"},
						{"settings", "foo", "3", "z"},
						{"settings", "foo", "2", "w"},
					},
				},
			},
			wantTag: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tag := getLatestSettingsTag(&tt.community, tt.settingsTag)

			require.EqualValues(t, tag, tt.wantTag)
		})
	}
}

func TestHandleChangeCommunityDefinitionEvent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		event    *model.Event
		defEvent *model.Event
		wantErr  bool
	}{
		{
			name:  "change community definition by owner",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityChangeDefinition, ID: "id", PubKey: "owner pubkey"}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}, {"name", "new name"}},
				},
			},
			wantErr: false,
		},
		{
			name:  "change community definition description by owner",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityChangeDefinition, ID: "id", PubKey: "owner pubkey"}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}, {"description", "new description"}},
				},
			},
			wantErr: false,
		},
		{
			name:  "change community definition closed status by owner",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityChangeDefinition, ID: "id", PubKey: "owner pubkey"}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}, {"closed"}},
				},
			},
			wantErr: false,
		},
		{
			name:  "change community definition open status by owner",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityChangeDefinition, ID: "id", PubKey: "owner pubkey"}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}, {"open"}},
				},
			},
			wantErr: false,
		},
		{
			name:  "change community definition public status by owner",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityChangeDefinition, ID: "id", PubKey: "owner pubkey"}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}, {"public"}},
				},
			},
			wantErr: false,
		},
		{
			name:  "change community definition private status by owner",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityChangeDefinition, ID: "id", PubKey: "owner pubkey"}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}, {"private"}},
				},
			},
			wantErr: false,
		},
		{
			name:  "change community definition picture by owner",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityChangeDefinition, ID: "id", PubKey: "owner pubkey"}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}, {"imeta"}},
				},
			},
			wantErr: false,
		},
		{
			name:  "change community definition settings by owner",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityChangeDefinition, ID: "id", PubKey: "owner pubkey"}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}, {"settings"}},
				},
			},
			wantErr: false,
		},
		{
			name:  "change community definition by admin",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityChangeDefinition, ID: "id", PubKey: "admin pubkey"}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}, {"name", "new name"}},
				},
			},
			wantErr: false,
		},
		{
			name:  "change community definition description by admin",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityChangeDefinition, ID: "id", PubKey: "admin pubkey"}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}, {"description", "new description"}},
				},
			},
			wantErr: false,
		},
		{
			name:  "change community definition closed status by admin",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityChangeDefinition, ID: "id", PubKey: "admin pubkey"}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}, {"closed"}},
				},
			},
			wantErr: false,
		},
		{
			name:  "change community definition open status by admin",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityChangeDefinition, ID: "id", PubKey: "admin pubkey"}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}, {"open"}},
				},
			},
			wantErr: false,
		},
		{
			name:  "change community definition public status by admin",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityChangeDefinition, ID: "id", PubKey: "admin pubkey"}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}, {"public"}},
				},
			},
			wantErr: false,
		},
		{
			name:  "change community definition private status by admin",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityChangeDefinition, ID: "id", PubKey: "admin pubkey"}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}, {"private"}},
				},
			},
			wantErr: false,
		},
		{
			name:  "change community definition picture by admin",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityChangeDefinition, ID: "id", PubKey: "admin pubkey"}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}, {"imeta"}},
				},
			},
			wantErr: false,
		},
		{
			name:  "change community definition settings by admin",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityChangeDefinition, ID: "id", PubKey: "admin pubkey"}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}, {"settings"}},
				},
			},
			wantErr: false,
		},
		{
			name:  "change community definition name by moderator",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityChangeDefinition, ID: "id", PubKey: "moderator pubkey"}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}, {"name", "new name"}},
				},
			},
			wantErr: true,
		},
		{
			name:  "change community definition description by moderator",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityChangeDefinition, ID: "id", PubKey: "moderator pubkey"}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}, {"description", "new description"}},
				},
			},
			wantErr: true,
		},
		{
			name:  "change community definition closed status by moderator",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityChangeDefinition, ID: "id", PubKey: "moderator pubkey"}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}, {"closed"}},
				},
			},
			wantErr: true,
		},
		{
			name:  "change community definition open status by moderator",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityChangeDefinition, ID: "id", PubKey: "moderator pubkey"}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}, {"open"}},
				},
			},
			wantErr: true,
		},
		{
			name:  "change community definition public status by moderator",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityChangeDefinition, ID: "id", PubKey: "moderator pubkey"}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}, {"public"}},
				},
			},
			wantErr: true,
		},
		{
			name:  "change community definition private status by moderator",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityChangeDefinition, ID: "id", PubKey: "moderator pubkey"}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}, {"private"}},
				},
			},
			wantErr: true,
		},
		{
			name:  "change community definition picture by moderator",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityChangeDefinition, ID: "id", PubKey: "moderator pubkey"}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}, {"imeta"}},
				},
			},
			wantErr: true,
		},
		{
			name:  "change community definition settings by moderator",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityChangeDefinition, ID: "id", PubKey: "moderator pubkey"}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}, {"settings"}},
				},
			},
			wantErr: true,
		},

		{
			name:  "change community definition name by any non-privileged user",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityChangeDefinition, ID: "id", PubKey: "any user pubkey"}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}, {"name", "new name"}},
				},
			},
			wantErr: true,
		},
		{
			name:  "change community definition description by any non-privileged user",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityChangeDefinition, ID: "id", PubKey: "any user pubkey"}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}, {"description", "new description"}},
				},
			},
			wantErr: true,
		},
		{
			name:  "change community definition closed status by any non-privileged user",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityChangeDefinition, ID: "id", PubKey: "any user pubkey"}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}, {"closed"}},
				},
			},
			wantErr: true,
		},
		{
			name:  "change community definition open status by any non-privileged user",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityChangeDefinition, ID: "id", PubKey: "any user pubkey"}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}, {"open"}},
				},
			},
			wantErr: true,
		},
		{
			name:  "change community definition public status by any non-privileged user",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityChangeDefinition, ID: "id", PubKey: "any user pubkey"}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}, {"public"}},
				},
			},
			wantErr: true,
		},
		{
			name:  "change community definition private status by any non-privileged user",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityChangeDefinition, ID: "id", PubKey: "any user pubkey"}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}, {"private"}},
				},
			},
			wantErr: true,
		},
		{
			name:  "change community definition picture by any non-privileged user",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityChangeDefinition, ID: "id", PubKey: "any user pubkey"}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}, {"imeta"}},
				},
			},
			wantErr: true,
		},
		{
			name:  "change community definition settings by any non-privileged user",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityChangeDefinition, ID: "id", PubKey: "any user pubkey"}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}, {"settings"}},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handleChangeCommunityDefinitionEvent(tt.event, tt.defEvent)

			if (err != nil) != tt.wantErr {
				t.Errorf("handleChangeCommunityDefinitionEvent() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHandleBanUserEvent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		event    *model.Event
		defEvent *model.Event
		wantErr  bool
	}{
		{
			name:  "ban non privileged user by owner",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityBanUser, ID: "id", PubKey: "owner pubkey", Tags: model.Tags{{"p", "any user pubkey"}}}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}},
				},
			},
			wantErr: false,
		},
		{
			name:  "ban non privileged user by admin",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityBanUser, ID: "id", PubKey: "admin pubkey", Tags: model.Tags{{"p", "any user pubkey"}}}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}},
				},
			},
			wantErr: false,
		},
		{
			name:  "ban non privileged user by moderator",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityBanUser, ID: "id", PubKey: "moderator pubkey", Tags: model.Tags{{"p", "any user pubkey"}}}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}},
				},
			},
			wantErr: false,
		},
		{
			name:  "ban admin by owner",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityBanUser, ID: "id", PubKey: "owner pubkey", Tags: model.Tags{{"p", "admin pubkey"}}}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}},
				},
			},
			wantErr: false,
		},
		{
			name:  "ban moderator by owner",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityBanUser, ID: "id", PubKey: "owner pubkey", Tags: model.Tags{{"p", "moderator pubkey"}}}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}},
				},
			},
			wantErr: false,
		},
		{
			name:  "ban moderator by admin",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityBanUser, ID: "id", PubKey: "admin pubkey", Tags: model.Tags{{"p", "moderator pubkey"}}}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}},
				},
			},
			wantErr: false,
		},
		{
			name:  "ban admin by non privileged user",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityBanUser, ID: "id", PubKey: "any pubkey", Tags: model.Tags{{"p", "admin pubkey"}}}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}},
				},
			},
			wantErr: true,
		},
		{
			name:  "ban owner by non privileged user",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityBanUser, ID: "id", PubKey: "any pubkey", Tags: model.Tags{{"p", "owner pubkey"}}}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}},
				},
			},
			wantErr: true,
		},
		{
			name:  "ban moderator by non privileged user",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityBanUser, ID: "id", PubKey: "any pubkey", Tags: model.Tags{{"p", "moderator pubkey"}}}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}},
				},
			},
			wantErr: true,
		},
		{
			name:  "ban owner by admin",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityBanUser, ID: "id", PubKey: "admin pubkey", Tags: model.Tags{{"p", "owner pubkey"}}}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}},
				},
			},
			wantErr: true,
		},
		{
			name:  "ban owner by moderator",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityBanUser, ID: "id", PubKey: "moderator pubkey", Tags: model.Tags{{"p", "owner pubkey"}}}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}},
				},
			},
			wantErr: true,
		},
		{
			name:  "ban admin by moderator",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityBanUser, ID: "id", PubKey: "moderator pubkey", Tags: model.Tags{{"p", "admin pubkey"}}}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handleBanUserEvent(tt.event, tt.defEvent)

			if (err != nil) != tt.wantErr {
				t.Errorf("handleBanUserEvent() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHandleCommunityJoinEvent(t *testing.T) {
	t.Parallel()

	authorizationEventModerator := model.Event{
		Event: nostr.Event{
			Kind:   model.KindCommunityJoin,
			PubKey: "moderator pubkey",
		},
	}
	authorizationEventAdmin := model.Event{
		Event: nostr.Event{
			Kind:   model.KindCommunityJoin,
			PubKey: "admin pubkey",
		},
	}
	authorizationEventOwner := model.Event{
		Event: nostr.Event{
			Kind:   model.KindCommunityJoin,
			PubKey: "owner pubkey",
		},
	}
	authorizationEventNotAuthorized := model.Event{
		Event: nostr.Event{
			Kind:   model.KindCommunityJoin,
			PubKey: "any pubkey",
		},
	}

	tests := []struct {
		name     string
		event    *model.Event
		defEvent *model.Event
		wantErr  bool
	}{
		{
			name:  "join to open community",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityJoin, ID: "id", PubKey: "any user pubkey"}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}, {"open"}},
				},
			},
			wantErr: false,
		},
		{
			name:  "join to closed community with authorization tag by moderator",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityJoin, ID: "id", PubKey: "any user pubkey", Tags: model.Tags{{"authorization", authorizationEventModerator.String()}}}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}, {"closed"}},
				},
			},
			wantErr: false,
		},
		{
			name:  "join to closed community with authorization tag by admin",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityJoin, ID: "id", PubKey: "any user pubkey", Tags: model.Tags{{"authorization", authorizationEventAdmin.String()}}}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}, {"closed"}},
				},
			},
			wantErr: false,
		},
		{
			name:  "join to closed community with authorization tag by owner",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityJoin, ID: "id", PubKey: "any user pubkey", Tags: model.Tags{{"authorization", authorizationEventOwner.String()}}}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}, {"closed"}},
				},
			},
			wantErr: false,
		},
		{
			name:  "join to closed community with non authorized user event",
			event: &model.Event{Event: nostr.Event{Kind: model.KindCommunityJoin, ID: "id", PubKey: "any user pubkey", Tags: model.Tags{{"authorization", authorizationEventNotAuthorized.String()}}}},
			defEvent: &model.Event{
				Event: nostr.Event{
					PubKey: "owner pubkey",
					Tags:   model.Tags{{"p", "admin pubkey", "", "admin"}, {"p", "moderator pubkey", "", "moderator"}, {"closed"}},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handleCommunityJoinEvent(tt.event, tt.defEvent)

			if (err != nil) != tt.wantErr {
				t.Errorf("handleCommunityJoinEvent() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRoleRequiredForPosting(t *testing.T) {
	t.Parallel()
	now := time.Now()

	tests := []struct {
		name string
		def  *model.Event
		want string
	}{
		{
			name: "role_required_for_posting settings is admin",
			def:  &model.Event{Event: nostr.Event{Tags: model.Tags{{"settings", "role_required_for_posting", "admin", fmt.Sprint(now.Unix())}}}},
			want: "admin",
		},
		{
			name: "role_required_for_posting settings is moderator",
			def:  &model.Event{Event: nostr.Event{Tags: model.Tags{{"settings", "role_required_for_posting", "moderator", fmt.Sprint(now.Unix())}}}},
			want: "moderator",
		},
		{
			name: "role_required_for_posting settings is wrong",
			def:  &model.Event{Event: nostr.Event{Tags: model.Tags{{"settings", "role_required_for_posting", "wrong", fmt.Sprint(now.Unix())}}}},
			want: "",
		},
		{
			name: "several role_required_for_posting settings is moderator",
			def: &model.Event{
				Event: nostr.Event{
					Tags: model.Tags{
						{"settings", "role_required_for_posting", "moderator", fmt.Sprint(now.Add(-1 * time.Hour).Unix())},
						{"settings", "role_required_for_posting", "admin", fmt.Sprint(now.Unix())},
					},
				},
			},
			want: "admin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := roleRequiredForPosting(tt.def)

			if got != tt.want {
				t.Errorf("roleRequiredForPosting() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsCommunityCommentsEnabled(t *testing.T) {
	t.Parallel()
	now := time.Now()

	tests := []struct {
		name string
		def  *model.Event
		want bool
	}{
		{
			name: "comments_enabled settings is true",
			def:  &model.Event{Event: nostr.Event{Tags: model.Tags{{"settings", "comments_enabled", "true", fmt.Sprint(now.Unix())}}}},
			want: true,
		},
		{
			name: "comments_enabled settings is false",
			def:  &model.Event{Event: nostr.Event{Tags: model.Tags{{"settings", "comments_enabled", "false", fmt.Sprint(now.Unix())}}}},
			want: false,
		},
		{
			name: "several comments_enabled settings",
			def: &model.Event{
				Event: nostr.Event{
					Tags: model.Tags{
						{"settings", "comments_enabled", "false", fmt.Sprint(now.Add(-1 * time.Hour).Unix())},
						{"settings", "comments_enabled", "true", fmt.Sprint(now.Unix())},
					},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isCommunityCommentsEnabled(tt.def)

			if got != tt.want {
				t.Errorf("isCommunityCommentsEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}
