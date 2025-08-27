// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"testing"

	"github.com/google/uuid"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

func TestPublishingNIP23Events(t *testing.T) {
	privkey := model.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	helperRegisterWSEventListenerProxyWithStorage(t, &storedEvents)
	ctx := t.Context()
	relay := helperMustNewRelay(t, pubsubServers[0])

	badgeDefinition, badgeAward, profileMetadata := helperCreateUsernameBadge(t, uuid.NewString(), privkey, relay)
	require.NoError(t, query.AcceptEvents(ctx, badgeDefinition, badgeAward, profileMetadata))

	var validEventKindArticle, validEventKindBlogPost *model.Event
	t.Run("kind 30023 (Article) (NIP-23): valid event", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"a", "30023:a695f6b60119d9521934a691347d9f78e8770b56da16bb255ee286ddf9fda919:ipsum", "wss://relay.nostr.org"})
		tags = append(tags, nostr.Tag{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"})
		tags = append(tags, nostr.Tag{"t", "placeholder"})
		tags = append(tags, nostr.Tag{"published_at", "1296962229"})
		tags = append(tags, nostr.Tag{"title", "Lorem Ipsum"})
		tags = append(tags, nostr.Tag{"d", "lorem-ipsum"})
		validEventKindArticle = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindArticle,
			Tags:      tags,
			Content:   "Lorem [ipsum][nostr:nevent1qqst8cujky046negxgwwm5ynqwn53t8aqjr6afd8g59nfqwxpdhylpcpzamhxue69uhhyetvv9ujuetcv9khqmr99e3k7mg8arnc9] dolor sit amet",
		}}
		helperSignWithMinLeadingZeroBits(t, validEventKindArticle, privkey)
		require.NoError(t, relay.Publish(ctx, validEventKindArticle.Event))
	})
	t.Run("kind 30024 (Blog post) (NIP-23): valid event", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"a", "30023:a695f6b60119d9521934a691347d9f78e8770b56da16bb255ee286ddf9fda919:ipsum", "wss://relay.nostr.org"})
		tags = append(tags, nostr.Tag{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"})
		tags = append(tags, nostr.Tag{"t", "placeholder"})
		tags = append(tags, nostr.Tag{"published_at", "1296962229"})
		tags = append(tags, nostr.Tag{"title", "Lorem Ipsum"})
		tags = append(tags, nostr.Tag{"d", "lorem-ipsum"})
		validEventKindBlogPost = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindDraftArticle,
			Tags:      tags,
			Content:   "Lorem [ipsum][nostr:nevent1qqst8cujky046negxgwwm5ynqwn53t8aqjr6afd8g59nfqwxpdhylpcpzamhxue69uhhyetvv9ujuetcv9khqmr99e3k7mg8arnc9] dolor sit amet",
		}}
		helperSignWithMinLeadingZeroBits(t, validEventKindBlogPost, privkey)
		require.NoError(t, relay.Publish(ctx, validEventKindBlogPost.Event))
	})

	t.Run("kind 30023 (Article) (NIP-23): unsupported tag for this type of event", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"a", "30023:a695f6b60119d9521934a691347d9f78e8770b56da16bb255ee286ddf9fda919:ipsum", "wss://relay.nostr.org"})
		tags = append(tags, nostr.Tag{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"})
		tags = append(tags, nostr.Tag{"t", "placeholder"})
		tags = append(tags, nostr.Tag{"published_at", "1296962229"})
		tags = append(tags, nostr.Tag{"title", "Lorem Ipsum"})
		tags = append(tags, nostr.Tag{"d", "lorem-ipsum"})
		tags = append(tags, nostr.Tag{"p", "pubkey"})
		tags = append(tags, nostr.Tag{"dummy", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindArticle,
			Tags:      tags,
			Content:   "Lorem [ipsum][nostr:nevent1qqst8cujky046negxgwwm5ynqwn53t8aqjr6afd8g59nfqwxpdhylpcpzamhxue69uhhyetvv9ujuetcv9khqmr99e3k7mg8arnc9] dolor sit amet",
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	helperMustCloseRelay(t, relay)

	require.Equal(t, []*model.Event{badgeDefinition, badgeAward, profileMetadata, validEventKindArticle, validEventKindBlogPost}, storedEvents)
}
