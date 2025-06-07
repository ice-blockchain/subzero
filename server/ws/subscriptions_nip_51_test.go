// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"fmt"
	"testing"

	"github.com/ice-blockchain/subzero/model"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

func TestPublishingNIP51ListsSetsEvents(t *testing.T) {
	privkey := model.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	helperRegisterWSEventListenerProxyWithStorage(t, &storedEvents)
	ctx := t.Context()
	relay := helperMustNewRelay(t, pubsubServers[0])

	var validEvents []*model.Event
	t.Run("Kind 10000 (NIP-51) valid", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"p", "pubkey"})
		tags = append(tags, nostr.Tag{"t", "hash"})
		tags = append(tags, nostr.Tag{"e", "event"})
		tags = append(tags, nostr.Tag{"word", "dummy"})
		validEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindMuteList,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, validEvent, privkey)
		validEvents = append(validEvents, validEvent)
		require.NoError(t, relay.Publish(ctx, validEvent.Event))
	})
	t.Run("Kind 10000 (NIP-51) mute lists: unsupported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"p", "pubkey"})
		tags = append(tags, nostr.Tag{"t", "hash"})
		tags = append(tags, nostr.Tag{"e", "event"})
		tags = append(tags, nostr.Tag{"word", "dummy"})
		tags = append(tags, nostr.Tag{"wrong", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindMuteList,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("Kind 10001 (NIP-51) valid", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "event"})
		validEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindPinList,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, validEvent, privkey)
		validEvents = append(validEvents, validEvent)
		require.NoError(t, relay.Publish(ctx, validEvent.Event))
	})
	t.Run("Kind 10001 (NIP-51) pin lists: unsupported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "event"})
		tags = append(tags, nostr.Tag{"wrong", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindPinList,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})

	t.Run("Kind 10003 (NIP-51) valid", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "event"})
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:dummy:dummy", nostr.KindArticle)})
		tags = append(tags, nostr.Tag{"t", "hash"})
		tags = append(tags, nostr.Tag{"r", "hash"})
		validEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindBookmarkList,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, validEvent, privkey)
		validEvents = append(validEvents, validEvent)
		require.NoError(t, relay.Publish(ctx, validEvent.Event))
	})
	t.Run("Kind 10003 (NIP-51): unsupported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "event"})
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:dummy:dummy", nostr.KindArticle)})
		tags = append(tags, nostr.Tag{"t", "hash"})
		tags = append(tags, nostr.Tag{"r", "hash"})
		tags = append(tags, nostr.Tag{"wrong", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindBookmarkList,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("Kind 10003 (NIP-51): wrong a tag kind", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "event"})
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:dummy:dummy", nostr.KindProfileMetadata)})
		tags = append(tags, nostr.Tag{"t", "hash"})
		tags = append(tags, nostr.Tag{"r", "hash"})
		tags = append(tags, nostr.Tag{"wrong", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindBookmarkList,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("Kind 10004 (NIP-51) valid", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:dummy:dummy", model.CustomIONKindCommunityDefinition)})
		validEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindCommunityList,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, validEvent, privkey)
		validEvents = append(validEvents, validEvent)
		require.NoError(t, relay.Publish(ctx, validEvent.Event))
	})
	t.Run("Kind 10003 (NIP-51): unsupported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:dummy:dummy", model.CustomIONKindCommunityDefinition)})
		tags = append(tags, nostr.Tag{"wrong", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindCommunityList,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("Kind 10003 (NIP-51): wrong a tag kind", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:dummy:dummy", nostr.KindProfileMetadata)})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindCommunityList,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("Kind 10005 (NIP-51) valid", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "dummy"})
		validEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindPublicChatList,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, validEvent, privkey)
		validEvents = append(validEvents, validEvent)
		require.NoError(t, relay.Publish(ctx, validEvent.Event))
	})
	t.Run("Kind 10005 (NIP-51): unsupported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "dummy"})
		tags = append(tags, nostr.Tag{"wrong", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindPublicChatList,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("Kind 10006 (NIP-51) valid", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"relay", "dummy"})
		validEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindBlockedRelayList,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, validEvent, privkey)
		validEvents = append(validEvents, validEvent)
		require.NoError(t, relay.Publish(ctx, validEvent.Event))
	})
	t.Run("Kind 10006 (NIP-51): unsupported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"relay", "dummy"})
		tags = append(tags, nostr.Tag{"wrong", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindBlockedRelayList,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("Kind 10007 (NIP-51) valid", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"relay", "dummy"})
		validEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindSearchRelayList,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, validEvent, privkey)
		validEvents = append(validEvents, validEvent)
		require.NoError(t, relay.Publish(ctx, validEvent.Event))
	})
	t.Run("Kind 10007 (NIP-51): unsupported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"relay", "dummy"})
		tags = append(tags, nostr.Tag{"wrong", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindSearchRelayList,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("Kind 10007 (NIP-51) valid", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"group", "dummy"})
		validEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindSimpleGroupList,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, validEvent, privkey)
		validEvents = append(validEvents, validEvent)
		require.NoError(t, relay.Publish(ctx, validEvent.Event))
	})
	t.Run("Kind 10007 (NIP-51): unsupported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"group", "dummy"})
		tags = append(tags, nostr.Tag{"wrong", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindSimpleGroupList,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("Kind 10015 (NIP-51) valid", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"t", "dummy"})
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:dummy:dummy", nostr.KindInterestSets)})
		validEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindInterestList,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, validEvent, privkey)
		validEvents = append(validEvents, validEvent)
		require.NoError(t, relay.Publish(ctx, validEvent.Event))
	})
	t.Run("Kind 10015 (NIP-51): unsupported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"t", "dummy"})
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:dummy,dummy", nostr.KindInterestSets)})
		tags = append(tags, nostr.Tag{"wrong", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindInterestList,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("Kind 10015 (NIP-51): wrong a tag kind", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"t", "dummy"})
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:dummy:dummy", nostr.KindProfileMetadata)})
		tags = append(tags, nostr.Tag{"wrong", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindInterestList,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})

	t.Run("Kind 10030 (NIP-51) valid", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"emoji", "dummy"})
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:dummy:dummy", nostr.KindEmojiSets)})
		validEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindEmojiList,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, validEvent, privkey)
		validEvents = append(validEvents, validEvent)
		require.NoError(t, relay.Publish(ctx, validEvent.Event))
	})
	t.Run("Kind 10030 (NIP-51) unsupported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"emoji", "dummy"})
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:dummy:dummy", nostr.KindEmojiSets)})
		tags = append(tags, nostr.Tag{"wrong", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindEmojiList,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("Kind 10030 (NIP-51) wrong a tag kind", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"emoji", "dummy"})
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:dummy:dummy", nostr.KindProfileMetadata)})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindEmojiList,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("Kind 10050 (NIP-51) valid", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"relay", "dummy"})
		validEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindDMRelayList,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, validEvent, privkey)
		validEvents = append(validEvents, validEvent)
		require.NoError(t, relay.Publish(ctx, validEvent.Event))
	})
	t.Run("Kind 10050 (NIP-51) unsupported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"relay", "dummy"})
		tags = append(tags, nostr.Tag{"wrong", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindDMRelayList,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("Kind 10101 (NIP-51) valid", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"p", "dummy"})
		validEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindGoodWikiAuthorList,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, validEvent, privkey)
		validEvents = append(validEvents, validEvent)
		require.NoError(t, relay.Publish(ctx, validEvent.Event))
	})
	t.Run("Kind 10101 (NIP-51) unsupported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"p", "dummy"})
		tags = append(tags, nostr.Tag{"wrong", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindGoodWikiAuthorList,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("Kind 10102 (NIP-51) valid", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"relay", "dummy"})
		validEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindGoodWikiRelayList,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, validEvent, privkey)
		validEvents = append(validEvents, validEvent)
		require.NoError(t, relay.Publish(ctx, validEvent.Event))
	})
	t.Run("Kind 10102 (NIP-51) unsupported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"relay", "dummy"})
		tags = append(tags, nostr.Tag{"wrong", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindGoodWikiRelayList,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("Kind 30000 (NIP-51) valid", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"p", "dummy"})
		tags = append(tags, nostr.Tag{"d", "dummy"})
		tags = append(tags, nostr.Tag{"title", "dummy"})
		tags = append(tags, nostr.Tag{"image", "dummy"})
		tags = append(tags, nostr.Tag{"description", "dummy"})
		validEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindCategorizedPeopleList,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, validEvent, privkey)
		validEvents = append(validEvents, validEvent)
		require.NoError(t, relay.Publish(ctx, validEvent.Event))
	})
	t.Run("Kind 30000 (NIP-51) unsupported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"p", "dummy"})
		tags = append(tags, nostr.Tag{"d", "dummy"})
		tags = append(tags, nostr.Tag{"title", "dummy"})
		tags = append(tags, nostr.Tag{"image", "dummy"})
		tags = append(tags, nostr.Tag{"description", "dummy"})
		tags = append(tags, nostr.Tag{"wrong", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindCategorizedPeopleList,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("Kind 30002 (NIP-51) valid", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"relay", "dummy"})
		tags = append(tags, nostr.Tag{"d", "dummy"})
		tags = append(tags, nostr.Tag{"title", "dummy"})
		tags = append(tags, nostr.Tag{"image", "dummy"})
		tags = append(tags, nostr.Tag{"description", "dummy"})
		validEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindRelaySets,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, validEvent, privkey)
		validEvents = append(validEvents, validEvent)
		require.NoError(t, relay.Publish(ctx, validEvent.Event))
	})
	t.Run("Kind 30002 (NIP-51) unsupported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"relay", "dummy"})
		tags = append(tags, nostr.Tag{"d", "dummy"})
		tags = append(tags, nostr.Tag{"title", "dummy"})
		tags = append(tags, nostr.Tag{"image", "dummy"})
		tags = append(tags, nostr.Tag{"description", "dummy"})
		tags = append(tags, nostr.Tag{"wrong", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindCategorizedPeopleList,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("Kind 30003 (NIP-51) valid", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "dummy"})
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:dummy:dummy", nostr.KindArticle)})
		tags = append(tags, nostr.Tag{"t", "dummy"})
		tags = append(tags, nostr.Tag{"r", "dummy"})
		tags = append(tags, nostr.Tag{"d", "dummy"})
		tags = append(tags, nostr.Tag{"title", "dummy"})
		tags = append(tags, nostr.Tag{"image", "dummy"})
		tags = append(tags, nostr.Tag{"description", "dummy"})
		validEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindBookmarkSets,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, validEvent, privkey)
		validEvents = append(validEvents, validEvent)
		require.NoError(t, relay.Publish(ctx, validEvent.Event))
	})
	t.Run("Kind 30003 (NIP-51) unsupported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "dummy"})
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:dummy:dummy", nostr.KindArticle)})
		tags = append(tags, nostr.Tag{"t", "dummy"})
		tags = append(tags, nostr.Tag{"r", "dummy"})
		tags = append(tags, nostr.Tag{"d", "dummy"})
		tags = append(tags, nostr.Tag{"title", "dummy"})
		tags = append(tags, nostr.Tag{"image", "dummy"})
		tags = append(tags, nostr.Tag{"description", "dummy"})
		tags = append(tags, nostr.Tag{"wrong", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindCategorizedPeopleList,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("Kind 30003 (NIP-51) wrong a tag value", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "dummy"})
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:dummy:dummy", nostr.KindProfileMetadata)})
		tags = append(tags, nostr.Tag{"t", "dummy"})
		tags = append(tags, nostr.Tag{"r", "dummy"})
		tags = append(tags, nostr.Tag{"d", "dummy"})
		tags = append(tags, nostr.Tag{"title", "dummy"})
		tags = append(tags, nostr.Tag{"image", "dummy"})
		tags = append(tags, nostr.Tag{"description", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindCategorizedPeopleList,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("Kind 30004 (NIP-51) valid", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:dummy:dummy", nostr.KindTextNote)})
		tags = append(tags, nostr.Tag{"e", "dummy"})
		tags = append(tags, nostr.Tag{"d", "dummy"})
		tags = append(tags, nostr.Tag{"title", "dummy"})
		tags = append(tags, nostr.Tag{"image", "dummy"})
		tags = append(tags, nostr.Tag{"description", "dummy"})
		validEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindCuratedSets,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, validEvent, privkey)
		validEvents = append(validEvents, validEvent)
		require.NoError(t, relay.Publish(ctx, validEvent.Event))
	})
	t.Run("Kind 30004 (NIP-51) unsupported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:dummy:dummy", nostr.KindTextNote)})
		tags = append(tags, nostr.Tag{"e", "dummy"})
		tags = append(tags, nostr.Tag{"d", "dummy"})
		tags = append(tags, nostr.Tag{"title", "dummy"})
		tags = append(tags, nostr.Tag{"image", "dummy"})
		tags = append(tags, nostr.Tag{"description", "dummy"})
		tags = append(tags, nostr.Tag{"wrong", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindCuratedSets,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("Kind 30004 (NIP-51) wrong a tag value", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:dummy:dummy", nostr.KindProfileMetadata)})
		tags = append(tags, nostr.Tag{"e", "dummy"})
		tags = append(tags, nostr.Tag{"d", "dummy"})
		tags = append(tags, nostr.Tag{"title", "dummy"})
		tags = append(tags, nostr.Tag{"image", "dummy"})
		tags = append(tags, nostr.Tag{"description", "dummy"})
		tags = append(tags, nostr.Tag{"wrong", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindCuratedSets,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("Kind 30005 (NIP-51) valid", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:dummy:dummy", nostr.KindVideoEvent)})
		tags = append(tags, nostr.Tag{"d", "dummy"})
		tags = append(tags, nostr.Tag{"title", "dummy"})
		tags = append(tags, nostr.Tag{"image", "dummy"})
		tags = append(tags, nostr.Tag{"description", "dummy"})
		validEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindCuratedVideoSets,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, validEvent, privkey)
		validEvents = append(validEvents, validEvent)
		require.NoError(t, relay.Publish(ctx, validEvent.Event))
	})
	t.Run("Kind 30005 (NIP-51) unsupported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:dummy:dummy", nostr.KindVideoEvent)})
		tags = append(tags, nostr.Tag{"d", "dummy"})
		tags = append(tags, nostr.Tag{"title", "dummy"})
		tags = append(tags, nostr.Tag{"image", "dummy"})
		tags = append(tags, nostr.Tag{"description", "dummy"})
		tags = append(tags, nostr.Tag{"wrong", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindCuratedVideoSets,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("Kind 30005 (NIP-51) wrong a tag value", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:dummy:dummy", nostr.KindProfileMetadata)})
		tags = append(tags, nostr.Tag{"d", "dummy"})
		tags = append(tags, nostr.Tag{"title", "dummy"})
		tags = append(tags, nostr.Tag{"image", "dummy"})
		tags = append(tags, nostr.Tag{"description", "dummy"})
		tags = append(tags, nostr.Tag{"wrong", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindCuratedVideoSets,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("Kind 30007 (NIP-51) valid", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"p", "dummy"})
		tags = append(tags, nostr.Tag{"d", "dummy"})
		tags = append(tags, nostr.Tag{"title", "dummy"})
		tags = append(tags, nostr.Tag{"image", "dummy"})
		tags = append(tags, nostr.Tag{"description", "dummy"})
		validEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindMuteSets,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, validEvent, privkey)
		validEvents = append(validEvents, validEvent)
		require.NoError(t, relay.Publish(ctx, validEvent.Event))
	})
	t.Run("Kind 30007 (NIP-51) unsupported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"p", "dummy"})
		tags = append(tags, nostr.Tag{"d", "dummy"})
		tags = append(tags, nostr.Tag{"title", "dummy"})
		tags = append(tags, nostr.Tag{"image", "dummy"})
		tags = append(tags, nostr.Tag{"description", "dummy"})
		tags = append(tags, nostr.Tag{"wrong", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindMuteSets,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("Kind 30015 (NIP-51) valid", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"t", "dummy"})
		tags = append(tags, nostr.Tag{"d", "dummy"})
		tags = append(tags, nostr.Tag{"title", "dummy"})
		tags = append(tags, nostr.Tag{"image", "dummy"})
		tags = append(tags, nostr.Tag{"description", "dummy"})
		validEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindInterestSets,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, validEvent, privkey)
		validEvents = append(validEvents, validEvent)
		require.NoError(t, relay.Publish(ctx, validEvent.Event))
	})
	t.Run("Kind 30015 (NIP-51) unsupported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"t", "dummy"})
		tags = append(tags, nostr.Tag{"d", "dummy"})
		tags = append(tags, nostr.Tag{"title", "dummy"})
		tags = append(tags, nostr.Tag{"image", "dummy"})
		tags = append(tags, nostr.Tag{"description", "dummy"})
		tags = append(tags, nostr.Tag{"wrong", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindInterestSets,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("Kind 30030 (NIP-51) valid", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"emoji", "dummy"})
		tags = append(tags, nostr.Tag{"d", "dummy"})
		tags = append(tags, nostr.Tag{"title", "dummy"})
		tags = append(tags, nostr.Tag{"image", "dummy"})
		tags = append(tags, nostr.Tag{"description", "dummy"})
		validEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindEmojiSets,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, validEvent, privkey)
		validEvents = append(validEvents, validEvent)
		require.NoError(t, relay.Publish(ctx, validEvent.Event))
	})
	t.Run("Kind 30030 (NIP-51) unsupported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"emoji", "dummy"})
		tags = append(tags, nostr.Tag{"d", "dummy"})
		tags = append(tags, nostr.Tag{"title", "dummy"})
		tags = append(tags, nostr.Tag{"image", "dummy"})
		tags = append(tags, nostr.Tag{"description", "dummy"})
		tags = append(tags, nostr.Tag{"wrong", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindEmojiSets,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("Kind 30063 (NIP-51) valid", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "dummy"})
		tags = append(tags, nostr.Tag{"i", "dummy"})
		tags = append(tags, nostr.Tag{"version", "dummy"})
		tags = append(tags, nostr.Tag{"d", "dummy"})
		tags = append(tags, nostr.Tag{"title", "dummy"})
		tags = append(tags, nostr.Tag{"image", "dummy"})
		tags = append(tags, nostr.Tag{"description", "dummy"})
		validEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindReleaseArtifactSets,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, validEvent, privkey)
		validEvents = append(validEvents, validEvent)
		require.NoError(t, relay.Publish(ctx, validEvent.Event))
	})
	t.Run("Kind 30063 (NIP-51) unsupported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "dummy"})
		tags = append(tags, nostr.Tag{"i", "dummy"})
		tags = append(tags, nostr.Tag{"version", "dummy"})
		tags = append(tags, nostr.Tag{"d", "dummy"})
		tags = append(tags, nostr.Tag{"title", "dummy"})
		tags = append(tags, nostr.Tag{"image", "dummy"})
		tags = append(tags, nostr.Tag{"description", "dummy"})
		tags = append(tags, nostr.Tag{"wrong", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindReleaseArtifactSets,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})

	helperMustCloseRelay(t, relay)
	require.Equal(t, validEvents, storedEvents)
}
