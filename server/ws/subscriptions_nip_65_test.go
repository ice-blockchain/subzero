// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"testing"

	"github.com/ice-blockchain/subzero/model"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

func TestPublishingNIP65RelayListMetadataEvents(t *testing.T) {
	privkey := model.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	helperRegisterWSEventListenerProxyWithStorage(t, &storedEvents)
	ctx := t.Context()
	relay := helperMustNewRelay(t, pubsubServers[0])

	var validRelayListEvent *model.Event
	t.Run("kind 10002 (Relay list) (NIP-65): valid relay list", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"r", "wss://alicerelay.example.com"})
		tags = append(tags, nostr.Tag{"r", "wss://brando-relay.com"})
		tags = append(tags, nostr.Tag{"r", "wss://expensive-relay.example2.com", "write"})
		tags = append(tags, nostr.Tag{"r", "wss://nostr-relay.example.com", "read"})
		validRelayListEvent = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindRelayListMetadata,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, validRelayListEvent, privkey)
		require.NoError(t, relay.Publish(ctx, validRelayListEvent.Event))
	})
	t.Run("kind 10002 (Relay list) (NIP-65): unsupported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"r", "wss://alicerelay.example.com"})
		tags = append(tags, nostr.Tag{"r", "wss://brando-relay.com"})
		tags = append(tags, nostr.Tag{"r", "wss://expensive-relay.example2.com", "write"})
		tags = append(tags, nostr.Tag{"r", "wss://nostr-relay.example.com", "read"})
		tags = append(tags, nostr.Tag{"e", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindRelayListMetadata,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 10002 (Relay list) (NIP-65): unsupported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"r", "wss://alicerelay.example.com"})
		tags = append(tags, nostr.Tag{"r", "wss://brando-relay.com"})
		tags = append(tags, nostr.Tag{"r", "wss://expensive-relay.example2.com", "write"})
		tags = append(tags, nostr.Tag{"r", "wss://nostr-relay.example.com", "read"})
		tags = append(tags, nostr.Tag{"e", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindRelayListMetadata,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 10002 (Relay list) (NIP-65): wrong marker", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"r", "wss://alicerelay.example.com"})
		tags = append(tags, nostr.Tag{"r", "wss://brando-relay.com"})
		tags = append(tags, nostr.Tag{"r", "wss://expensive-relay.example2.com", "write"})
		tags = append(tags, nostr.Tag{"r", "wss://nostr-relay.example.com", "read"})
		tags = append(tags, nostr.Tag{"r", "wss://nostr-relay.example.com", "wrong"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindRelayListMetadata,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	helperMustCloseRelay(t, relay)
	require.Equal(t, []*model.Event{validRelayListEvent}, storedEvents)
}
