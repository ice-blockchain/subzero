// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func TestPublishingNIP01NIP24Events(t *testing.T) {
	privkey := model.GeneratePrivateKey()
	badgeIssuerPrivKey, badgeIssuerPubkey := model.GenerateKeyPair()
	userPrivKey, _ := model.GenerateKeyPair()
	storedEvents := []*model.Event{}
	helperRegisterWSEventListenerProxyWithStorage(t, &storedEvents)
	ctx := t.Context()
	relay := helperMustNewRelay(t, pubsubServers[0])

	var validEventNIP01, validEventNIP24, badgeDefinition, badgeAward *model.Event

	t.Run("create username proof badges for testuser", func(t *testing.T) {
		username := "testuser"
		badgeDefinition = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindBadgeDefinition,
			Tags: nostr.Tags{
				{"d", fmt.Sprintf("username_proof_of_ownership~%s", username)},
				{"name", "Username Proof Badge"},
				{"description", "Proof of ownership for username"},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, badgeDefinition, badgeIssuerPrivKey)
	})

	t.Run("kind 0 (ProfileMetadata) (NIP-01): valid event", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"a", "30023:a695f6b60119d9521934a691347d9f78e8770b56da16bb255ee286ddf9fda919:ipsum", "wss://relay.nostr.org"})
		tags = append(tags, nostr.Tag{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"})
		tags = append(tags, nostr.Tag{"p", "pubkey1"})
		tags = append(tags, nostr.Tag{"alt", "reply"})
		validEventNIP01 = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindProfileMetadata,
			Tags:      tags,
			Content:   `{"name":"testuser","display_name":"Test User","about":"me is bot","picture":"https://example.com/pic.jpg"}`,
		}}
		helperSignWithMinLeadingZeroBits(t, validEventNIP01, userPrivKey)

		badgeAward = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindBadgeAward,
			Tags: nostr.Tags{
				{"a", fmt.Sprintf("%d:%s:%s", nostr.KindBadgeDefinition, badgeDefinition.PubKey, badgeDefinition.GetTag("d").Value())},
				{"p", validEventNIP01.PubKey},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, badgeAward, badgeIssuerPrivKey)
		require.NoError(t, relay.PublishMany(ctx, &badgeDefinition.Event, &badgeAward.Event, &validEventNIP01.Event))
	})

	t.Run("create username proof badges for testuser2", func(t *testing.T) {
		username := "testuser2"
		badgeDefinition2 := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindBadgeDefinition,
			Tags: nostr.Tags{
				{"d", fmt.Sprintf("username_proof_of_ownership~%s", username)},
				{"name", "Username Proof Badge 2"},
				{"description", "Proof of ownership for username 2"},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, badgeDefinition2, badgeIssuerPrivKey)

		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"a", "30023:a695f6b60119d9521934a691347d9f78e8770b56da16bb255ee286ddf9fda919:ipsum", "wss://relay.nostr.org"})
		tags = append(tags, nostr.Tag{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"})
		tags = append(tags, nostr.Tag{"p", "pubkey1"})
		tags = append(tags, nostr.Tag{"alt", "reply"})
		validEventNIP24 = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindProfileMetadata,
			Tags:      tags,
			Content:   `{"name":"testuser2","display_name":"qqq","about":"me is bot","picture":"https://example.com/pic.jpg","website":"https://ice.io","banner":"https://example.com/banner.jpg","bot":true}`,
		}}
		helperSignWithMinLeadingZeroBits(t, validEventNIP24, userPrivKey)

		badgeAward2 := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindBadgeAward,
			Tags: nostr.Tags{
				{"a", fmt.Sprintf("%d:%s:%s", nostr.KindBadgeDefinition, badgeDefinition2.PubKey, badgeDefinition2.GetTag("d").Value())},
				{"p", validEventNIP24.PubKey},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, badgeAward2, badgeIssuerPrivKey)
		require.NoError(t, relay.PublishMany(ctx, &badgeDefinition2.Event, &badgeAward2.Event, &validEventNIP24.Event))
	})

	t.Run("text note with badge restrictions using ephemeral events", func(t *testing.T) {
		restrictedPost := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Content:   "Badge restricted post",
			Tags: nostr.Tags{
				{"settings", model.WhoCanReplySettings, fmt.Sprintf("%s|%d:%s:verified", model.BadgeWhoCanReplySettingsPrefix, nostr.KindBadgeDefinition, badgeIssuerPubkey), strconv.FormatInt(time.Now().Unix(), 10)},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, restrictedPost, privkey)
		require.NoError(t, relay.Publish(ctx, restrictedPost.Event))

		replyEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: nostr.Tags{
				{"e", restrictedPost.ID, "", model.TagMarkerRoot},
				{"p", restrictedPost.GetMasterPublicKey()},
			},
			Content: "Reply with valid badge acks",
		}}
		helperSignWithMinLeadingZeroBits(t, replyEvent, userPrivKey)

		badgeDefAck := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      model.CustomIONKindEphemeralEmbedding,
			Tags: nostr.Tags{
				{"e", replyEvent.ID},
			},
			Content: badgeDefinition.String(),
		}}
		helperSignWithMinLeadingZeroBits(t, badgeDefAck, userPrivKey)

		badgeAwardAck := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      model.CustomIONKindEphemeralEmbedding,
			Tags: nostr.Tags{
				{"e", replyEvent.ID},
			},
			Content: badgeAward.String(),
		}}
		helperSignWithMinLeadingZeroBits(t, badgeAwardAck, userPrivKey)

		require.NoError(t, relay.PublishMany(ctx, &replyEvent.Event, &badgeDefAck.Event, &badgeAwardAck.Event))
	})

	t.Run("kind 0 (ProfileMetadata) (NIP-24): invalid event: empty obligatory fields", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"a", "30023:a695f6b60119d9521934a691347d9f78e8770b56da16bb255ee286ddf9fda919:ipsum", "wss://relay.nostr.org"})
		tags = append(tags, nostr.Tag{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"})
		tags = append(tags, nostr.Tag{"p", "pubkey1"})
		tags = append(tags, nostr.Tag{"alt", "reply"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindProfileMetadata,
			Tags:      tags,
			Content:   `{"name":"","display_name":"","website":"https://ice.io","banner":"https://example.com/banner.jpg","bot":true}`,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 0 (ProfileMetadata) (NIP-24): invalid event: content is not JSON", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"a", "30023:a695f6b60119d9521934a691347d9f78e8770b56da16bb255ee286ddf9fda919:ipsum", "wss://relay.nostr.org"})
		tags = append(tags, nostr.Tag{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"})
		tags = append(tags, nostr.Tag{"p", "pubkey1"})
		tags = append(tags, nostr.Tag{"alt", "reply"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindProfileMetadata,
			Tags:      tags,
			Content:   `plain text`,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 0 (ProfileMetadata) (NIP-24): invalid event: unsupported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"a", "30023:a695f6b60119d9521934a691347d9f78e8770b56da16bb255ee286ddf9fda919:ipsum", "wss://relay.nostr.org"})
		tags = append(tags, nostr.Tag{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"})
		tags = append(tags, nostr.Tag{"p", "pubkey1"})
		tags = append(tags, nostr.Tag{"alt", "reply"})
		tags = append(tags, nostr.Tag{"unsupported", "value"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindProfileMetadata,
			Tags:      tags,
			Content:   `{"name":"testuser4","display_name":"Test User","about":"me is bot","picture":"https://example.com/pic.jpg"}`,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})

	helperMustCloseRelay(t, relay)
	require.GreaterOrEqual(t, len(storedEvents), 4)
}
