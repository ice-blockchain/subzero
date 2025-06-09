// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"fmt"
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func TestPublishingNIP58Badges(t *testing.T) {
	privkey := model.GeneratePrivateKey()
	pubkey, _ := model.GetPublicKey(privkey)
	storedEvents := []*model.Event{}
	helperRegisterWSEventListenerProxyWithStorage(t, &storedEvents)
	ctx := t.Context()
	relay := helperMustNewRelay(t, pubsubServers[0])

	var validBadgeDefinitionEvent, validBadgeAwardEvent, validProfileBadgesEvent *model.Event

	t.Run("kind 30009 (Badge definition) (NIP-56): valid badge definition event", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"d", "bravery"})
		tags = append(tags, nostr.Tag{"name", "Medal of Bravery"})
		tags = append(tags, nostr.Tag{"description", "Awarded to users demonstrating bravery"})
		tags = append(tags, nostr.Tag{"image", "https://nostr.academy/awards/bravery.png", "1024x1024"})
		tags = append(tags, nostr.Tag{"thumb", "https://nostr.academy/awards/bravery_256x256.png", "256x256"})
		validBadgeDefinitionEvent = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindBadgeDefinition,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, validBadgeDefinitionEvent, privkey)
	})

	t.Run("kind 8 (Badge award) (NIP-56): valid badge award event", func(t *testing.T) {
		var tags nostr.Tags
		badgeRef := fmt.Sprintf("30009:%s:bravery", pubkey)
		tags = append(tags, nostr.Tag{"a", badgeRef})
		tags = append(tags, nostr.Tag{"p", pubkey, "wss://relay"})
		validBadgeAwardEvent = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindBadgeAward,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, validBadgeAwardEvent, privkey)
	})

	t.Run("kind 3008 (Profile badges) (NIP-56): valid profile badges event", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"d", model.ProfileBadgesIdentifier})
		badgeRef := fmt.Sprintf("30009:%s:bravery", pubkey)
		tags = append(tags, nostr.Tag{"a", badgeRef})
		tags = append(tags, nostr.Tag{"e", validBadgeAwardEvent.ID, "wss://nostr.academy"})
		validProfileBadgesEvent = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindProfileBadges,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, validProfileBadgesEvent, privkey)

		require.NoError(t, relay.PublishMany(ctx, &validBadgeDefinitionEvent.Event, &validBadgeAwardEvent.Event, &validProfileBadgesEvent.Event))
	})

	t.Run("kind 30009 (Badge defenition) (NIP-56): invalid, no d tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"name", "Medal of Bravery"})
		tags = append(tags, nostr.Tag{"description", "Awarded to users demonstrating bravery"})
		tags = append(tags, nostr.Tag{"image", "https://nostr.academy/awards/bravery.png", "1024x1024"})
		tags = append(tags, nostr.Tag{"thumb", "https://nostr.academy/awards/bravery_256x256.png", "256x256"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindBadgeDefinition,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 30009 (Badge defenition) (NIP-56): not supported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"profile", "bogus"})
		tags = append(tags, nostr.Tag{"name", "Medal of Bravery"})
		tags = append(tags, nostr.Tag{"description", "Awarded to users demonstrating bravery"})
		tags = append(tags, nostr.Tag{"image", "https://nostr.academy/awards/bravery.png", "1024x1024"})
		tags = append(tags, nostr.Tag{"thumb", "https://nostr.academy/awards/bravery_256x256.png", "256x256"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindBadgeDefinition,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 8 (Badge award) (NIP-56): invalid, no a tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"p", "bob", "wss://relay"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindBadgeAward,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 8 (Badge award) (NIP-56): invalid, a tag refers to wrong kind", func(t *testing.T) {
		var tags nostr.Tags
		badgeRef := fmt.Sprintf("1:%s:bravery", pubkey)
		tags = append(tags, nostr.Tag{"a", badgeRef})
		tags = append(tags, nostr.Tag{"p", "bob", "wss://relay"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindBadgeAward,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 8 (Badge award) (NIP-56): invalid, no at least one p tag", func(t *testing.T) {
		var tags nostr.Tags
		badgeRef := fmt.Sprintf("30009:%s:bravery", pubkey)
		tags = append(tags, nostr.Tag{"a", badgeRef})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindBadgeAward,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 3008 (Profile badges) (NIP-56): invalid d tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"d", "bogus"})
		badgeRef := fmt.Sprintf("30009:%s:bravery", pubkey)
		tags = append(tags, nostr.Tag{"a", badgeRef})
		tags = append(tags, nostr.Tag{"e", "<bravery badge award event id>", "wss://nostr.academy"})
		badgeRef2 := fmt.Sprintf("30009:%s:honor", pubkey)
		tags = append(tags, nostr.Tag{"a", badgeRef2})
		tags = append(tags, nostr.Tag{"e", "<honor badge award event id>", "wss://nostr.academy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindProfileBadges,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 3008 (Profile badges) (NIP-56): invalid a tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"d", model.ProfileBadgesIdentifier})
		badgeRef := fmt.Sprintf("1:%s:bravery", pubkey)
		tags = append(tags, nostr.Tag{"a", badgeRef})
		tags = append(tags, nostr.Tag{"e", "<bravery badge award event id>", "wss://nostr.academy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindProfileBadges,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 3008 (Profile badges) (NIP-56): e/a tags mismatch", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"d", model.ProfileBadgesIdentifier})
		badgeRef := fmt.Sprintf("30009:%s:bravery", pubkey)
		tags = append(tags, nostr.Tag{"a", badgeRef})
		tags = append(tags, nostr.Tag{"e", "<bravery badge award event id>", "wss://nostr.academy"})
		badgeRef2 := fmt.Sprintf("30009:%s:honor", pubkey)
		tags = append(tags, nostr.Tag{"a", badgeRef2})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindProfileBadges,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})

	helperMustCloseRelay(t, relay)
	require.Equal(t, []*model.Event{validBadgeDefinitionEvent, validBadgeAwardEvent, validProfileBadgesEvent}, storedEvents)
}
