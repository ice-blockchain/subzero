// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"fmt"
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func TestPublishingNIP92IMetaTag(t *testing.T) {
	privkey := model.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	helperRegisterWSEventListenerProxyWithStorage(t, &storedEvents)
	ctx := t.Context()
	relay := helperMustNewRelay(t, pubsubServers[0])

	var validEvents []*model.Event
	t.Run("kind 1 (text note), imeta (NIP-92): valid imeta tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{
			"imeta",
			"url https://alicerelay.example.com",
			"m image/jpg",
			"dim 3024x4032",
			"alt A scenic photo overlooking the coast of Costa Rica",
			fmt.Sprintf("ox %x", []byte("https://alicerelay.example.com")),
		})
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags:      tags,
			Content:   "dummy",
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relay.Publish(ctx, ev.Event))
		validEvents = append(validEvents, ev)
	})
	t.Run("kind 1 (text note), imeta (NIP-92): invalid imeta key", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{
			"imeta",
			"url https://alicerelay.example.com",
			"m image/jpg",
			"dim 3024x4032",
			"alt A scenic photo overlooking the coast of Costa Rica",
			fmt.Sprintf("ox %x", []byte("https://alicerelay.example.com")),
			"dummy dummy",
		})
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags:      tags,
			Content:   "dummy",
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("kind 1 (text note), imeta (NIP-92): invalid imeta tag: not enough tag values", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{
			"imeta",
		})
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags:      tags,
			Content:   "dummy",
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("kind 1 (text note), imeta (NIP-92): invalid imeta tag: no spaces", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{
			"imeta",
			"url",
		})
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags:      tags,
			Content:   "dummy",
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("kind 1 (text note), imeta (NIP-92): invalid imeta tag: wrong m tag value", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{
			"imeta",
			"url https://alicerelay.example.com",
			"m image/jpg",
			"dim 3024",
			"alt A scenic photo overlooking the coast of Costa Rica",
			fmt.Sprintf("ox %x", []byte("https://alicerelay.example.com")),
		})
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags:      tags,
			Content:   "dummy",
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("kind 1 (text note), imeta (NIP-92): invalid imeta tag: ox not a hash", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{
			"imeta",
			"url https://alicerelay.example.com",
			"m image/jpg",
			"dim 3024x4032",
			"alt A scenic photo overlooking the coast of Costa Rica",
			"ox a",
		})
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags:      tags,
			Content:   "dummy",
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})

	helperMustCloseRelay(t, relay)
	require.Equal(t, validEvents, storedEvents)
}
