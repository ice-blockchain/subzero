// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/validation"
)

func TestCommunityDefinitionHijack(t *testing.T) {
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
	privkeyOwner, pub1 := model.GenerateKeyPair()
	privkeyOwner2, _ := model.GenerateKeyPair()

	_, pubkeyCommunityAdmin := model.GenerateKeyPair()
	_, pubkeyCommunityModerator := model.GenerateKeyPair()

	t.Run("Original", func(t *testing.T) {
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
					{"p", pubkeyCommunityAdmin, "", string(model.AdminRole)},
					{"p", pubkeyCommunityModerator, "", string(model.ModeratorRole)},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))

		def, err := validation.GetCommunityDefinition(ctx, communityID)
		require.NoError(t, err)
		require.Equal(t, pub1, def.PubKey)
	})

	t.Run("Hijack", func(t *testing.T) {
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
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyOwner2)
		require.Error(t, relay.Publish(ctx, ev.Event))

		def, err := validation.GetCommunityDefinition(ctx, communityID)
		require.NoError(t, err)
		require.Equal(t, pub1, def.PubKey)
	})
	helperMustCloseRelay(t, relay)
}
