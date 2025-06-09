// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"context"
	"strconv"
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

func TestPublishingNIP05Events(t *testing.T) {
	privkey := model.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	helperRegisterWSEventListenerProxyWithStorage(t, &storedEvents)
	ctx := t.Context()
	relay := helperMustNewRelay(t, pubsubServers[0])

	var validEventNIP09WithEKTags, validEventNIP09AllTags, validEventAccountDelete *model.Event
	t.Run("kind 5 (Deletion) (NIP-05): valid event with e/k tag", func(t *testing.T) {
		validEventNIP09WithEKTags = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindDeletion,
			Tags: model.Tags{
				{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"},
				{"k", "1"},
			},
			Content: "Deletion reason",
		}}
		helperSignWithMinLeadingZeroBits(t, validEventNIP09WithEKTags, privkey)
		require.NoError(t, relay.Publish(ctx, validEventNIP09WithEKTags.Event))
	})
	t.Run("kind 5 (Deletion) (NIP-05): valid event with all tag", func(t *testing.T) {
		validEventNIP09AllTags = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindDeletion,
			Tags: model.Tags{
				{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"},
				{"k", "1"},
				{"a", "1:foo:"},
			},
			Content: "Deletion reason",
		}}
		helperSignWithMinLeadingZeroBits(t, validEventNIP09AllTags, privkey)
		require.NoError(t, relay.Publish(ctx, validEventNIP09AllTags.Event))
	})
	t.Run("kind 5 (Deletion) (NIP-05): account deletion", func(t *testing.T) {
		validEventAccountDelete = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindDeletion,
			Content:   "account deletion reason",
		}}
		helperSignWithMinLeadingZeroBits(t, validEventAccountDelete, privkey)
		require.NoError(t, relay.Publish(ctx, validEventAccountDelete.Event))
	})
	t.Run("kind 5 (Deletion) (NIP-05): invalid event, mismatch e -> k tags", func(t *testing.T) {
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindDeletion,
			Tags: model.Tags{
				{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"},
			},
			Content: "Deletion reason",
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 5 (Deletion) (NIP-05): unsupported tag", func(t *testing.T) {
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindDeletion,
			Tags: model.Tags{
				{"r", "wss://relay.example.com"},
			},
			Content: "Deletion reason",
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})

	helperMustCloseRelay(t, relay)
	require.ElementsMatch(t, []*model.Event{validEventNIP09WithEKTags, validEventAccountDelete, validEventNIP09AllTags}, storedEvents)
}

func TestPublishingNIP05Events_NoEvent(t *testing.T) {
	privkey, pubkey := model.GenerateKeyPair()

	RegisterWSSubscriptionListener(func(ctx context.Context, s *model.Subscription) EventIterator {
		return query.GetStoredEvents(ctx, s)
	})
	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		require.True(t, len(events) > 0)
		require.NoError(t, query.AcceptEvents(ctx, events...))

		return nil
	})
	ctx := t.Context()
	relay := helperMustNewRelay(t, pubsubServers[0])

	var deletionEvent *model.Event
	t.Run("kind 5 (Deletion) (NIP-05): no such event", func(t *testing.T) {
		deletionEvent = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindDeletion,
			Tags: model.Tags{
				{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"},
				{"k", "1"},
				{model.CustomIONTagOnBehalfOf, pubkey},
			},
			Content: "Deletion reason",
		}}
		helperSignWithMinLeadingZeroBits(t, deletionEvent, privkey)
		require.NoError(t, relay.Publish(ctx, deletionEvent.Event))
	})
	var validKind01NIP10Event *model.Event
	t.Run("kind 1 (NIP-10): valid", func(t *testing.T) {
		validKind01NIP10Event = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: model.Tags{
				{"e", "", "relay", "reply"},
				{"p", "pubkey1", "pubkey2"},
				{model.CustomIONTagOnBehalfOf, pubkey},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, validKind01NIP10Event, privkey)
		require.NoError(t, relay.Publish(ctx, validKind01NIP10Event.Event))
	})
	t.Run("kind 5 (Deletion) (NIP-05): delete event that exists", func(t *testing.T) {
		deletionEvent = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindDeletion,
			Tags: model.Tags{
				{"e", validKind01NIP10Event.ID, "wss://relay.example.com"},
				{"k", strconv.Itoa(validKind01NIP10Event.Kind)},
				{model.CustomIONTagOnBehalfOf, pubkey},
			},
			Content: "Deletion reason",
		}}
		helperSignWithMinLeadingZeroBits(t, deletionEvent, privkey)
		require.NoError(t, relay.Publish(ctx, deletionEvent.Event))
	})
	t.Run("kind 5 (Deletion) (NIP-05): delete event one more time again", func(t *testing.T) {
		deletionEvent = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindDeletion,
			Tags: model.Tags{
				{"e", validKind01NIP10Event.ID, "wss://relay.example.com"},
				{"k", strconv.Itoa(validKind01NIP10Event.Kind)},
				{model.CustomIONTagOnBehalfOf, pubkey},
			},
			Content: "Deletion reason",
		}}
		helperSignWithMinLeadingZeroBits(t, deletionEvent, privkey)
		require.NoError(t, relay.Publish(ctx, deletionEvent.Event))
	})
	t.Run("kind 5 (Deletion) (NIP-05): delete user account", func(t *testing.T) {
		deletionEvent = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindDeletion,
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, pubkey},
			},
			Content: "Remebmer me",
		}}
		helperSignWithMinLeadingZeroBits(t, deletionEvent, privkey)
		require.NoError(t, relay.Publish(ctx, deletionEvent.Event))
	})

	helperMustCloseRelay(t, relay)
}
