// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"context"
	"testing"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

func TestCountEvents(t *testing.T) {
	privkey, pubkey := model.GenerateKeyPair()
	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		t.Logf("received events: %v", events)
		return query.AcceptEvents(ctx, events...)
	})

	ctx := t.Context()
	relay := helperMustNewRelay(t, pubsubServers[0])

	t.Run("SaveEvent", func(t *testing.T) {
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			PubKey:    pubkey,
			Tags:      nil,
			Content:   "validEvent",
		}}

		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("CountEvents", func(t *testing.T) {
		c, err := relay.Count(ctx, nostr.Filters{{Kinds: []int{nostr.KindTextNote}, Search: "test", Authors: []string{pubkey}}})
		require.NoError(t, err)
		require.Equal(t, int64(1), c)
	})
	helperMustCloseRelay(t, relay)
}
