// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func TestPublishingNIP10Events(t *testing.T) {
	privkey := model.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	helperRegisterWSEventListenerProxyWithStorage(t, &storedEvents)
	ctx := t.Context()
	relay := helperMustNewRelay(t, pubsubServers[0])

	t.Run("kind 1 (NIP-10): e tags required params", func(t *testing.T) {
		inValidKind01Event := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags:      nostr.Tags{[]string{"e"}},
		}}
		helperSignWithMinLeadingZeroBits(t, inValidKind01Event, privkey)
		require.Error(t, relay.Publish(ctx, inValidKind01Event.Event))
	})

	t.Run("kind 1 (NIP-10): invalid reply marker for e tags ", func(t *testing.T) {
		inValidKind01Event := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags:      nostr.Tags{[]string{"e", "", "relay", "invalid marker"}},
		}}
		helperSignWithMinLeadingZeroBits(t, inValidKind01Event, privkey)
		require.Error(t, relay.Publish(ctx, inValidKind01Event.Event))
	})
	t.Run("kind 1 (NIP-10): invalid p tag usage: no e tags", func(t *testing.T) {
		inValidKind01Event := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags:      nostr.Tags{[]string{"p", "pubkey1", "pubkey2"}},
		}}
		helperSignWithMinLeadingZeroBits(t, inValidKind01Event, privkey)
		require.Error(t, relay.Publish(ctx, inValidKind01Event.Event))
	})
	t.Run("kind 1 (NIP-10): invalid p tag usage: empty tag values", func(t *testing.T) {
		inValidKind01Event := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags:      nostr.Tags{[]string{"e", "", "relay", "reply"}, []string{"p"}},
		}}
		helperSignWithMinLeadingZeroBits(t, inValidKind01Event, privkey)
		require.Error(t, relay.Publish(ctx, inValidKind01Event.Event))
	})

	var validKind01NIP10Event *model.Event
	t.Run("kind 1 (NIP-10): valid", func(t *testing.T) {
		validKind01NIP10Event = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags:      nostr.Tags{[]string{"e", "", "relay", "reply"}, []string{"p", "pubkey1", "pubkey2"}},
		}}
		helperSignWithMinLeadingZeroBits(t, validKind01NIP10Event, privkey)
		require.NoError(t, relay.Publish(ctx, validKind01NIP10Event.Event))
	})

	helperMustCloseRelay(t, relay)
	require.Equal(t, []*model.Event{validKind01NIP10Event}, storedEvents)
}
