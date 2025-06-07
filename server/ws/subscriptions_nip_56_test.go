// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"testing"

	"github.com/ice-blockchain/subzero/model"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

func TestPublishingNIP56(t *testing.T) {
	privkey := model.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	helperRegisterWSEventListenerProxyWithStorage(t, &storedEvents)
	ctx := t.Context()
	relay := helperMustNewRelay(t, pubsubServers[0])

	var validReportEventWithPTagOnly *model.Event
	t.Run("kind 1984 (Report) (NIP-56): valid report event with p tag only", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"p", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", model.TagReportTypeNudity})
		validReportEventWithPTagOnly = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindReporting,
			Tags:      tags,
			Content:   "Report description",
		}}
		helperSignWithMinLeadingZeroBits(t, validReportEventWithPTagOnly, privkey)
		require.NoError(t, relay.Publish(ctx, validReportEventWithPTagOnly.Event))
	})
	var validReportEventWithBothTags *model.Event
	t.Run("kind 1984 (Report) (NIP-56): valid report event with both e and p tags", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"p", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87"})
		tags = append(tags, nostr.Tag{"e", "event id", model.TagReportTypeNudity})
		validReportEventWithBothTags = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindReporting,
			Tags:      tags,
			Content:   "Report description",
		}}
		helperSignWithMinLeadingZeroBits(t, validReportEventWithBothTags, privkey)
		require.NoError(t, relay.Publish(ctx, validReportEventWithBothTags.Event))
	})
	var validReportEventWithLabel *model.Event
	t.Run("kind 1984 (Report) (NIP-56): valid report event with labels", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"p", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87"})
		tags = append(tags, nostr.Tag{"e", "event id", model.TagReportTypeNudity})
		tags = append(tags, nostr.Tag{"l", "permies", "#t"})
		tags = append(tags, nostr.Tag{"L", "#t"})
		validReportEventWithLabel = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindReporting,
			Tags:      tags,
			Content:   "Report description",
		}}
		helperSignWithMinLeadingZeroBits(t, validReportEventWithLabel, privkey)
		require.NoError(t, relay.Publish(ctx, validReportEventWithLabel.Event))
	})
	t.Run("kind 1984 (Report) (NIP-56): invalid report with no p tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "event id", model.TagReportTypeNudity})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindReporting,
			Tags:      tags,
			Content:   "Report description",
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 1984 (Report) (NIP-56): invalid report with wrong p tag while e tag is added", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"p", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", model.TagReportTypeNudity})
		tags = append(tags, nostr.Tag{"e", "event id", model.TagReportTypeNudity})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindReporting,
			Tags:      tags,
			Content:   "Report description",
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 1984 (Report) (NIP-56): invalid report with wrong p tag when no e tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"p", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindReporting,
			Tags:      tags,
			Content:   "Report description",
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 1984 (Report) (NIP-56): invalid report with e tag when both tags represented", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"p", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87"})
		tags = append(tags, nostr.Tag{"e", "event id"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindReporting,
			Tags:      tags,
			Content:   "Report description",
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 1984 (Report) (NIP-56): invalid report with not supported report type", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"p", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87"})
		tags = append(tags, nostr.Tag{"e", "event id", "unsupported report type"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindReporting,
			Tags:      tags,
			Content:   "Report description",
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})

	helperMustCloseRelay(t, relay)
	require.Equal(t, []*model.Event{validReportEventWithPTagOnly, validReportEventWithBothTags, validReportEventWithLabel}, storedEvents)
}
