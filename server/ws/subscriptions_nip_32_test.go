// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func TestPublishingNIP32LabelingEvents(t *testing.T) {
	privkey := model.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	helperRegisterWSEventListenerProxyWithStorage(t, &storedEvents)
	ctx := t.Context()
	relay := helperMustNewRelay(t, pubsubServers[0])

	var validLabelingEvent, validUGCLabelingEvent *model.Event
	t.Run("kind 1985 (Labeling) (NIP-32): valid labeling event", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"})
		tags = append(tags, nostr.Tag{"L", "#t"})
		tags = append(tags, nostr.Tag{"l", "permies", "#t"})
		validLabelingEvent = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindLabel,
			Tags:      tags,
			Content:   "Some label long description",
		}}
		helperSignWithMinLeadingZeroBits(t, validLabelingEvent, privkey)
		require.NoError(t, relay.Publish(ctx, validLabelingEvent.Event))
	})
	t.Run("kind 1985 (Labeling) (NIP-32): valid labeling event, no label namespace tag L, but l refers to ugc namespace", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"})
		tags = append(tags, nostr.Tag{"l", "permies", "ugc"})
		validUGCLabelingEvent = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindLabel,
			Tags:      tags,
			Content:   "Some label long description",
		}}
		helperSignWithMinLeadingZeroBits(t, validUGCLabelingEvent, privkey)
		require.NoError(t, relay.Publish(ctx, validUGCLabelingEvent.Event))
	})
	t.Run("kind 1985 (Labeling) (NIP-32): invalid labeling event, no one of required (e,p,a,t,r) tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"L", "#t"})
		tags = append(tags, nostr.Tag{"l", "permies", "#t"})
		invalidLabelingEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindLabel,
			Tags:      tags,
			Content:   "Some label long description",
		}}
		helperSignWithMinLeadingZeroBits(t, invalidLabelingEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidLabelingEvent.Event))
	})
	t.Run("kind 1985 (Labeling) (NIP-32): invalid labeling event, no label tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"})
		tags = append(tags, nostr.Tag{"L", "#t"})
		invalidLabelingEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindLabel,
			Tags:      tags,
			Content:   "Some label long description",
		}}
		helperSignWithMinLeadingZeroBits(t, invalidLabelingEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidLabelingEvent.Event))
	})
	t.Run("kind 1985 (Labeling) (NIP-32): invalid labeling event, no namespace specified at the label tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"})
		tags = append(tags, nostr.Tag{"L", "#t"})
		tags = append(tags, nostr.Tag{"l", "permies"})
		invalidLabelingEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindLabel,
			Tags:      tags,
			Content:   "Some label long description",
		}}
		helperSignWithMinLeadingZeroBits(t, invalidLabelingEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidLabelingEvent.Event))
	})
	t.Run("kind 1985 (Labeling) (NIP-32): invalid labeling event, exceeds max label symbols", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"})
		tags = append(tags, nostr.Tag{"L", "#t"})
		tags = append(tags, nostr.Tag{"l", "permies permies permies permies permies permies permies permies permies permies permies permies permies", "#t"})
		invalidLabelingEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindLabel,
			Tags:      tags,
			Content:   "Some label long description",
		}}
		helperSignWithMinLeadingZeroBits(t, invalidLabelingEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidLabelingEvent.Event))
	})
	t.Run("kind 1985 (Labeling) (NIP-32): invalid labeling event, no label namespace tag L, l doesn't refer to ugc namespace", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"})
		tags = append(tags, nostr.Tag{"l", "permies", "#t"})
		invalidLabelingEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindLabel,
			Tags:      tags,
			Content:   "Some label long description",
		}}
		helperSignWithMinLeadingZeroBits(t, invalidLabelingEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidLabelingEvent.Event))
	})
	t.Run("kind 1985 (Labeling) (NIP-32): invalid labeling event, l -> L values mismatch", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"L", "#a"})
		tags = append(tags, nostr.Tag{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"})
		tags = append(tags, nostr.Tag{"l", "permies", "#t"})
		invalidLabelingEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindLabel,
			Tags:      tags,
			Content:   "Some label long description",
		}}
		helperSignWithMinLeadingZeroBits(t, invalidLabelingEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidLabelingEvent.Event))
	})
	t.Run("kind 1 (NIP-32): invalid label", func(t *testing.T) {
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags:      nostr.Tags{[]string{"e", "", "relay", "reply"}, []string{"p", "pubkey1", "pubkey2"}, []string{"l", "permies", "#t"}, []string{"L", "#a"}},
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	var validKind01EventWithLabels *model.Event
	t.Run("kind 1 (NIP-32): valid with label", func(t *testing.T) {
		validKind01EventWithLabels = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags:      nostr.Tags{[]string{"e", "", "relay", "reply"}, []string{"p", "pubkey1", "pubkey2"}, []string{"l", "permies", "#t"}, []string{"L", "#t"}},
		}}
		helperSignWithMinLeadingZeroBits(t, validKind01EventWithLabels, privkey)
		require.NoError(t, relay.Publish(ctx, validKind01EventWithLabels.Event))
	})

	helperMustCloseRelay(t, relay)
	require.Equal(t, []*model.Event{validLabelingEvent, validUGCLabelingEvent, validKind01EventWithLabels}, storedEvents)
}
