// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

func TestPublishingICIP3000RelayCustomIONKindCommunityDefinition(t *testing.T) {
	privkey := model.GeneratePrivateKey()
	ctx := context.Background()
	relay := helperMustNewRelay(t, pubsubServers[0])
	hVal, err := uuid.NewV7()
	require.NoError(t, err)
	RegisterWSSubscriptionListener(func(ctx context.Context, s *model.Subscription) EventIterator {
		return query.GetStoredEvents(ctx, s)
	})
	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		require.True(t, len(events) > 0)
		require.NoError(t, query.AcceptEvents(ctx, events...))

		return nil
	})

	var validCommunityDefinitionEvent, validChangeCommunityDefinitionEvent *model.Event
	t.Run("kind 31750 (Community definition) (ICIP-3000): valid", func(t *testing.T) {

		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"name", "Community name"})
		tags = append(tags, nostr.Tag{"description", "Community description"})
		tags = append(tags, nostr.Tag{"h", hVal.String()})
		tags = append(tags, nostr.Tag{"settings", model.CommentsEnabledSettings, "true", fmt.Sprint(time.Now().Unix())})
		tags = append(tags, nostr.Tag{"settings", model.RoleRequiredForPostingSettings, string(model.ModeratorRole), fmt.Sprint(time.Now().Unix())})
		tags = append(tags, nostr.Tag{"public"})
		tags = append(tags, nostr.Tag{"open"})
		tags = append(tags, nostr.Tag{"p", uuid.NewString(), "relay", string(model.ModeratorRole)})
		tags = append(tags, nostr.Tag{"p", uuid.NewString(), "relay", string(model.AdminRole)})
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:%v:%v", model.CustomIONKindCommunityDefinition, uuid.NewString(), "communityDIdentifier1")})
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:%v:%v", model.CustomIONKindCommunityDefinition, uuid.NewString(), "communityDIdentifier2")})
		tags = append(tags, nostr.Tag{
			"imeta",
			"url https://alicerelay.example.com",
			"m image/jpg",
			"i foobar",
			"dim 3024x3024",
			"alt A scenic photo overlooking the coast of Costa Rica",
			fmt.Sprintf("x %x", []byte("https://alicerelay.example.com")),
			fmt.Sprintf("ox %x", []byte("https://alicerelay.example.com")),
		})

		validCommunityDefinitionEvent = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.CustomIONKindCommunityDefinition,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, validCommunityDefinitionEvent, privkey)
		require.NoError(t, relay.Publish(ctx, validCommunityDefinitionEvent.Event))
	})
	t.Run("kind 1753 (Community change definition) (ICIP-3000): valid", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"name", "Community name"})
		tags = append(tags, nostr.Tag{"description", "Community description"})
		tags = append(tags, nostr.Tag{"h", hVal.String()})
		tags = append(tags, nostr.Tag{"settings", model.CommentsEnabledSettings, "true", fmt.Sprint(time.Now().Unix())})
		tags = append(tags, nostr.Tag{"settings", model.RoleRequiredForPostingSettings, string(model.ModeratorRole), fmt.Sprint(time.Now().Unix())})
		tags = append(tags, nostr.Tag{"public"})
		tags = append(tags, nostr.Tag{"open"})
		tags = append(tags, nostr.Tag{"p", uuid.NewString(), "relay", string(model.ModeratorRole)})
		tags = append(tags, nostr.Tag{"p", uuid.NewString(), "relay", string(model.AdminRole)})
		tags = append(tags, nostr.Tag{
			"imeta",
			"url https://alicerelay.example.com",
			"m image/jpg",
			"i foobar",
			"dim 3024x3024",
			"alt A scenic photo overlooking the coast of Costa Rica",
			fmt.Sprintf("x %x", []byte("https://alicerelay.example.com")),
			fmt.Sprintf("ox %x", []byte("https://alicerelay.example.com")),
		})

		validChangeCommunityDefinitionEvent = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.CustomIONKindCommunityChangeDefinition,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, validChangeCommunityDefinitionEvent, privkey)
		require.NoError(t, relay.Publish(ctx, validChangeCommunityDefinitionEvent.Event))
	})
	t.Run("kind 31750 (Community definition) (ICIP-3000): wrong h value", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"name", "Community name"})
		tags = append(tags, nostr.Tag{"description", "Community description"})
		tags = append(tags, nostr.Tag{"h", "aaa"})
		tags = append(tags, nostr.Tag{"settings", model.CommentsEnabledSettings, "true", fmt.Sprint(time.Now().Unix())})
		tags = append(tags, nostr.Tag{"settings", model.RoleRequiredForPostingSettings, string(model.ModeratorRole), fmt.Sprint(time.Now().Unix())})
		tags = append(tags, nostr.Tag{"public"})
		tags = append(tags, nostr.Tag{"open"})
		tags = append(tags, nostr.Tag{"p", uuid.NewString(), "relay", string(model.ModeratorRole)})
		tags = append(tags, nostr.Tag{
			"imeta",
			"url https://alicerelay.example.com",
			"m image/jpg",
			"i foobar",
			"dim 3024x3024",
			"alt A scenic photo overlooking the coast of Costa Rica",
			fmt.Sprintf("x %x", []byte("https://alicerelay.example.com")),
			fmt.Sprintf("ox %x", []byte("https://alicerelay.example.com")),
		})

		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.CustomIONKindCommunityDefinition,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 1753 (Community change definition) (ICIP-3000): wrong h value", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"name", "Community name"})
		tags = append(tags, nostr.Tag{"description", "Community description"})
		tags = append(tags, nostr.Tag{"h", "aaa"})
		tags = append(tags, nostr.Tag{"settings", model.CommentsEnabledSettings, "true", fmt.Sprint(time.Now().Unix())})
		tags = append(tags, nostr.Tag{"settings", model.RoleRequiredForPostingSettings, string(model.ModeratorRole), fmt.Sprint(time.Now().Unix())})
		tags = append(tags, nostr.Tag{"public"})
		tags = append(tags, nostr.Tag{"open"})
		tags = append(tags, nostr.Tag{"p", uuid.NewString(), "relay", string(model.ModeratorRole)})
		tags = append(tags, nostr.Tag{
			"imeta",
			"url https://alicerelay.example.com",
			"m image/jpg",
			"i foobar",
			"dim 3024x3024",
			"alt A scenic photo overlooking the coast of Costa Rica",
			fmt.Sprintf("x %x", []byte("https://alicerelay.example.com")),
			fmt.Sprintf("ox %x", []byte("https://alicerelay.example.com")),
		})

		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.CustomIONKindCommunityChangeDefinition,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 31750 (Community definition) (ICIP-3000): wrong settings comments_enabled value", func(t *testing.T) {
		hVal, err := uuid.NewV7()
		require.NoError(t, err)

		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"name", "Community name"})
		tags = append(tags, nostr.Tag{"description", "Community description"})
		tags = append(tags, nostr.Tag{"h", hVal.String()})
		tags = append(tags, nostr.Tag{"settings", model.CommentsEnabledSettings, "bogus", fmt.Sprint(time.Now().Unix())})
		tags = append(tags, nostr.Tag{"settings", model.RoleRequiredForPostingSettings, string(model.ModeratorRole), fmt.Sprint(time.Now().Unix())})
		tags = append(tags, nostr.Tag{"public"})
		tags = append(tags, nostr.Tag{"open"})
		tags = append(tags, nostr.Tag{"p", uuid.NewString(), "relay", string(model.ModeratorRole)})
		tags = append(tags, nostr.Tag{
			"imeta",
			"url https://alicerelay.example.com",
			"m image/jpg",
			"i foobar",
			"dim 3024x3024",
			"alt A scenic photo overlooking the coast of Costa Rica",
			fmt.Sprintf("x %x", []byte("https://alicerelay.example.com")),
			fmt.Sprintf("ox %x", []byte("https://alicerelay.example.com")),
		})

		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.CustomIONKindCommunityDefinition,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 1753 (Community change definition) (ICIP-3000): wrong settings comments_enabled value", func(t *testing.T) {
		hVal, err := uuid.NewV7()
		require.NoError(t, err)

		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"name", "Community name"})
		tags = append(tags, nostr.Tag{"description", "Community description"})
		tags = append(tags, nostr.Tag{"h", hVal.String()})
		tags = append(tags, nostr.Tag{"settings", model.CommentsEnabledSettings, "bogus", fmt.Sprint(time.Now().Unix())})
		tags = append(tags, nostr.Tag{"settings", model.RoleRequiredForPostingSettings, string(model.ModeratorRole), fmt.Sprint(time.Now().Unix())})
		tags = append(tags, nostr.Tag{"public"})
		tags = append(tags, nostr.Tag{"open"})
		tags = append(tags, nostr.Tag{"p", uuid.NewString(), "relay", string(model.ModeratorRole)})
		tags = append(tags, nostr.Tag{
			"imeta",
			"url https://alicerelay.example.com",
			"m image/jpg",
			"i foobar",
			"dim 3024x3024",
			"alt A scenic photo overlooking the coast of Costa Rica",
			fmt.Sprintf("x %x", []byte("https://alicerelay.example.com")),
			fmt.Sprintf("ox %x", []byte("https://alicerelay.example.com")),
		})

		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.CustomIONKindCommunityChangeDefinition,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 31750 (Community definition) (ICIP-3000): wrong settings role_required_for_posting value", func(t *testing.T) {
		hVal, err := uuid.NewV7()
		require.NoError(t, err)

		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"name", "Community name"})
		tags = append(tags, nostr.Tag{"description", "Community description"})
		tags = append(tags, nostr.Tag{"h", hVal.String()})
		tags = append(tags, nostr.Tag{"settings", model.CommentsEnabledSettings, "true", fmt.Sprint(time.Now().Unix())})
		tags = append(tags, nostr.Tag{"settings", model.RoleRequiredForPostingSettings, "dummy", fmt.Sprint(time.Now().Unix())})
		tags = append(tags, nostr.Tag{"public"})
		tags = append(tags, nostr.Tag{"open"})
		tags = append(tags, nostr.Tag{"p", uuid.NewString(), "relay", string(model.ModeratorRole)})
		tags = append(tags, nostr.Tag{
			"imeta",
			"url https://alicerelay.example.com",
			"m image/jpg",
			"i foobar",
			"dim 3024x3024",
			"alt A scenic photo overlooking the coast of Costa Rica",
			fmt.Sprintf("x %x", []byte("https://alicerelay.example.com")),
			fmt.Sprintf("ox %x", []byte("https://alicerelay.example.com")),
		})

		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.CustomIONKindCommunityDefinition,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 1753 (Community change definition) (ICIP-3000): wrong settings role_required_for_posting value", func(t *testing.T) {
		hVal, err := uuid.NewV7()
		require.NoError(t, err)

		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"name", "Community name"})
		tags = append(tags, nostr.Tag{"description", "Community description"})
		tags = append(tags, nostr.Tag{"h", hVal.String()})
		tags = append(tags, nostr.Tag{"settings", model.CommentsEnabledSettings, "true", fmt.Sprint(time.Now().Unix())})
		tags = append(tags, nostr.Tag{"settings", model.RoleRequiredForPostingSettings, "dummy", fmt.Sprint(time.Now().Unix())})
		tags = append(tags, nostr.Tag{"public"})
		tags = append(tags, nostr.Tag{"open"})
		tags = append(tags, nostr.Tag{"p", uuid.NewString(), "relay", string(model.ModeratorRole)})
		tags = append(tags, nostr.Tag{
			"imeta",
			"url https://alicerelay.example.com",
			"m image/jpg",
			"i foobar",
			"dim 3024x3024",
			"alt A scenic photo overlooking the coast of Costa Rica",
			fmt.Sprintf("x %x", []byte("https://alicerelay.example.com")),
			fmt.Sprintf("ox %x", []byte("https://alicerelay.example.com")),
		})

		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.CustomIONKindCommunityChangeDefinition,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 31750 (Community definition) (ICIP-3000): wrong public/private tags: both exists in the event", func(t *testing.T) {
		hVal, err := uuid.NewV7()
		require.NoError(t, err)

		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"name", "Community name"})
		tags = append(tags, nostr.Tag{"description", "Community description"})
		tags = append(tags, nostr.Tag{"h", hVal.String()})
		tags = append(tags, nostr.Tag{"settings", model.CommentsEnabledSettings, "true", fmt.Sprint(time.Now().Unix())})
		tags = append(tags, nostr.Tag{"settings", model.RoleRequiredForPostingSettings, string(model.ModeratorRole), fmt.Sprint(time.Now().Unix())})
		tags = append(tags, nostr.Tag{"public"})
		tags = append(tags, nostr.Tag{"private"})
		tags = append(tags, nostr.Tag{"open"})
		tags = append(tags, nostr.Tag{"p", uuid.NewString(), "relay", string(model.ModeratorRole)})
		tags = append(tags, nostr.Tag{
			"imeta",
			"url https://alicerelay.example.com",
			"m image/jpg",
			"i foobar",
			"dim 3024x3024",
			"alt A scenic photo overlooking the coast of Costa Rica",
			fmt.Sprintf("x %x", []byte("https://alicerelay.example.com")),
			fmt.Sprintf("ox %x", []byte("https://alicerelay.example.com")),
		})

		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.CustomIONKindCommunityDefinition,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 1753 (Community change definition) (ICIP-3000): wrong public/private tags: both exists in the event", func(t *testing.T) {
		hVal, err := uuid.NewV7()
		require.NoError(t, err)

		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"name", "Community name"})
		tags = append(tags, nostr.Tag{"description", "Community description"})
		tags = append(tags, nostr.Tag{"h", hVal.String()})
		tags = append(tags, nostr.Tag{"settings", model.CommentsEnabledSettings, "true", fmt.Sprint(time.Now().Unix())})
		tags = append(tags, nostr.Tag{"settings", model.RoleRequiredForPostingSettings, string(model.ModeratorRole), fmt.Sprint(time.Now().Unix())})
		tags = append(tags, nostr.Tag{"public"})
		tags = append(tags, nostr.Tag{"private"})
		tags = append(tags, nostr.Tag{"open"})
		tags = append(tags, nostr.Tag{"p", uuid.NewString(), "relay", string(model.ModeratorRole)})
		tags = append(tags, nostr.Tag{
			"imeta",
			"url https://alicerelay.example.com",
			"m image/jpg",
			"i foobar",
			"dim 3024x3024",
			"alt A scenic photo overlooking the coast of Costa Rica",
			fmt.Sprintf("x %x", []byte("https://alicerelay.example.com")),
			fmt.Sprintf("ox %x", []byte("https://alicerelay.example.com")),
		})

		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.CustomIONKindCommunityChangeDefinition,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 31750 (Community definition) (ICIP-3000): wrong role for p tag", func(t *testing.T) {
		hVal, err := uuid.NewV7()
		require.NoError(t, err)

		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"name", "Community name"})
		tags = append(tags, nostr.Tag{"description", "Community description"})
		tags = append(tags, nostr.Tag{"h", hVal.String()})
		tags = append(tags, nostr.Tag{"settings", model.CommentsEnabledSettings, "true", fmt.Sprint(time.Now().Unix())})
		tags = append(tags, nostr.Tag{"settings", model.RoleRequiredForPostingSettings, string(model.ModeratorRole), fmt.Sprint(time.Now().Unix())})
		tags = append(tags, nostr.Tag{"public"})
		tags = append(tags, nostr.Tag{"open"})
		tags = append(tags, nostr.Tag{"p", uuid.NewString(), "relay", "dummy"})
		tags = append(tags, nostr.Tag{
			"imeta",
			"url https://alicerelay.example.com",
			"m image/jpg",
			"i foobar",
			"dim 3024x3024",
			"alt A scenic photo overlooking the coast of Costa Rica",
			fmt.Sprintf("x %x", []byte("https://alicerelay.example.com")),
			fmt.Sprintf("ox %x", []byte("https://alicerelay.example.com")),
		})

		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.CustomIONKindCommunityDefinition,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 1753 (Community change definition) (ICIP-3000): wrong role for p tag", func(t *testing.T) {
		hVal, err := uuid.NewV7()
		require.NoError(t, err)

		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"name", "Community name"})
		tags = append(tags, nostr.Tag{"description", "Community description"})
		tags = append(tags, nostr.Tag{"h", hVal.String()})
		tags = append(tags, nostr.Tag{"settings", model.CommentsEnabledSettings, "true", fmt.Sprint(time.Now().Unix())})
		tags = append(tags, nostr.Tag{"settings", model.RoleRequiredForPostingSettings, string(model.ModeratorRole), fmt.Sprint(time.Now().Unix())})
		tags = append(tags, nostr.Tag{"public"})
		tags = append(tags, nostr.Tag{"open"})
		tags = append(tags, nostr.Tag{"p", uuid.NewString(), "relay", "dummy"})
		tags = append(tags, nostr.Tag{
			"imeta",
			"url https://alicerelay.example.com",
			"m image/jpg",
			"i foobar",
			"dim 3024x3024",
			"alt A scenic photo overlooking the coast of Costa Rica",
			fmt.Sprintf("x %x", []byte("https://alicerelay.example.com")),
			fmt.Sprintf("ox %x", []byte("https://alicerelay.example.com")),
		})

		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.CustomIONKindCommunityChangeDefinition,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})

	helperMustCloseRelay(t, relay)
}

func TestPublishingICIP3000RelayCustomIONKindCommunityJoin(t *testing.T) {
	hVal, err := uuid.NewV7()
	require.NoError(t, err)
	privkey := model.GeneratePrivateKey()
	RegisterWSSubscriptionListener(func(ctx context.Context, s *model.Subscription) EventIterator {
		return query.GetStoredEvents(ctx, s)
	})
	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		require.True(t, len(events) > 0)
		require.NoError(t, query.AcceptEvents(ctx, events...))

		return nil
	})
	ctx := context.Background()
	relay := helperMustNewRelay(t, pubsubServers[0])

	var validJoinCommunityEvent *model.Event
	t.Run("kind 31750 (Community definition) (ICIP-3000): valid", func(t *testing.T) {

		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"name", "Community name"})
		tags = append(tags, nostr.Tag{"description", "Community description"})
		tags = append(tags, nostr.Tag{"h", hVal.String()})
		tags = append(tags, nostr.Tag{"settings", model.CommentsEnabledSettings, "true", fmt.Sprint(time.Now().Unix())})
		tags = append(tags, nostr.Tag{"settings", model.RoleRequiredForPostingSettings, string(model.ModeratorRole), fmt.Sprint(time.Now().Unix())})
		tags = append(tags, nostr.Tag{"public"})
		tags = append(tags, nostr.Tag{"open"})
		tags = append(tags, nostr.Tag{"p", uuid.NewString(), "relay", string(model.ModeratorRole)})
		tags = append(tags, nostr.Tag{"p", uuid.NewString(), "relay", string(model.AdminRole)})
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:%v:%v", model.CustomIONKindCommunityDefinition, uuid.NewString(), "communityDIdentifier1")})
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:%v:%v", model.CustomIONKindCommunityDefinition, uuid.NewString(), "communityDIdentifier2")})
		tags = append(tags, nostr.Tag{
			"imeta",
			"url https://alicerelay.example.com",
			"m image/jpg",
			"i foobar",
			"dim 3024x3024",
			"alt A scenic photo overlooking the coast of Costa Rica",
			fmt.Sprintf("x %x", []byte("https://alicerelay.example.com")),
			fmt.Sprintf("ox %x", []byte("https://alicerelay.example.com")),
		})

		validCommunityDefinitionEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.CustomIONKindCommunityDefinition,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, validCommunityDefinitionEvent, privkey)
		require.NoError(t, relay.Publish(ctx, validCommunityDefinitionEvent.Event))
	})
	t.Run("kind 1750 (Community Join) (ICIP-3000): valid", func(t *testing.T) {
		authorizationEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.CustomIONKindCommunityJoin,
			Tags: nostr.Tags{
				{"h", hVal.String()},
				{"expiration", fmt.Sprint(time.Now().Add(1 * time.Minute).Unix())},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, authorizationEvent, privkey)

		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"h", hVal.String()})
		tags = append(tags, nostr.Tag{"p", uuid.NewString()})
		tags = append(tags, nostr.Tag{"authorization", authorizationEvent.String()})

		validJoinCommunityEvent = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.CustomIONKindCommunityJoin,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, validJoinCommunityEvent, privkey)
		require.NoError(t, relay.Publish(ctx, validJoinCommunityEvent.Event))
	})
	t.Run("kind 1750 (Community Join) (ICIP-3000): no h tag", func(t *testing.T) {
		authorizationEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.CustomIONKindCommunityJoin,
			Tags: nostr.Tags{
				{"expiration", fmt.Sprint(time.Now().Add(1 * time.Minute).Unix())},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, authorizationEvent, privkey)

		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"p", uuid.NewString()})
		tags = append(tags, nostr.Tag{"authorization", authorizationEvent.String()})

		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.CustomIONKindCommunityJoin,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 1750 (Community Join) (ICIP-3000): wrong kind in authorization event", func(t *testing.T) {
		authorizationEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.CustomIONKindCommunityBanUser,
			Tags: nostr.Tags{
				{"expiration", fmt.Sprint(time.Now().Add(1 * time.Minute).Unix())},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, authorizationEvent, privkey)

		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"h", hVal.String()})
		tags = append(tags, nostr.Tag{"p", uuid.NewString()})
		tags = append(tags, nostr.Tag{"authorization", authorizationEvent.String()})

		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.CustomIONKindCommunityJoin,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 1750 (Community Join) (ICIP-3000): authorization event was expired", func(t *testing.T) {
		authorizationEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.CustomIONKindCommunityJoin,
			Tags: nostr.Tags{
				{"expiration", fmt.Sprint(time.Now().Add(-1 * time.Minute).Unix())},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, authorizationEvent, privkey)

		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"h", hVal.String()})
		tags = append(tags, nostr.Tag{"p", uuid.NewString()})
		tags = append(tags, nostr.Tag{"authorization", authorizationEvent.String()})

		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.CustomIONKindCommunityJoin,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 1750 (Community Join) (ICIP-3000): wrong authorization event", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"h", hVal.String()})
		tags = append(tags, nostr.Tag{"p", uuid.NewString()})
		tags = append(tags, nostr.Tag{"authorization", "dummy"})

		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.CustomIONKindCommunityJoin,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})

	helperMustCloseRelay(t, relay)
}

func TestPublishingICIP3000RelayKindTransferCommunityMembership(t *testing.T) {
	hVal, err := uuid.NewV7()
	require.NoError(t, err)
	privkey := model.GeneratePrivateKey()
	RegisterWSSubscriptionListener(func(ctx context.Context, s *model.Subscription) EventIterator {
		return query.GetStoredEvents(ctx, s)
	})
	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		require.True(t, len(events) > 0)
		require.NoError(t, query.AcceptEvents(ctx, events...))

		return nil
	})
	ctx := context.Background()
	relay := helperMustNewRelay(t, pubsubServers[0])

	eventAuthorPubkey := uuid.NewString()

	t.Run("kind 31750 (Community definition) (ICIP-3000): valid", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"name", "Community name"})
		tags = append(tags, nostr.Tag{"description", "Community description"})
		tags = append(tags, nostr.Tag{"h", hVal.String()})
		tags = append(tags, nostr.Tag{"settings", model.CommentsEnabledSettings, "true", fmt.Sprint(time.Now().Unix())})
		tags = append(tags, nostr.Tag{"settings", model.RoleRequiredForPostingSettings, string(model.ModeratorRole), fmt.Sprint(time.Now().Unix())})
		tags = append(tags, nostr.Tag{"public"})
		tags = append(tags, nostr.Tag{"open"})
		tags = append(tags, nostr.Tag{"p", uuid.NewString(), "relay", string(model.ModeratorRole)})
		tags = append(tags, nostr.Tag{"p", uuid.NewString(), "relay", string(model.AdminRole)})
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:%v:%v", model.CustomIONKindCommunityDefinition, uuid.NewString(), "communityDIdentifier1")})
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:%v:%v", model.CustomIONKindCommunityDefinition, uuid.NewString(), "communityDIdentifier2")})
		tags = append(tags, nostr.Tag{
			"imeta",
			"url https://alicerelay.example.com",
			"m image/jpg",
			"i foobar",
			"dim 3024x3024",
			"alt A scenic photo overlooking the coast of Costa Rica",
			fmt.Sprintf("x %x", []byte("https://alicerelay.example.com")),
			fmt.Sprintf("ox %x", []byte("https://alicerelay.example.com")),
		})

		validCommunityDefinitionEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.CustomIONKindCommunityDefinition,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, validCommunityDefinitionEvent, privkey)
		require.NoError(t, relay.Publish(ctx, validCommunityDefinitionEvent.Event))
	})
	t.Run("kind 1751 (Community transferring ownership) (ICIP-3000): valid", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"h", hVal.String()})
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:%v:%v", model.CustomIONKindCommunityDefinition, eventAuthorPubkey, "communityDIdentifier")})
		tags = append(tags, nostr.Tag{"expiration", fmt.Sprint(time.Now().Add(1 * time.Minute).Unix())})
		tags = append(tags, nostr.Tag{"p", uuid.NewString()})

		validCommunityTransferOwnershipEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.CustomIONKindCommunityOwnershipTransferring,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, validCommunityTransferOwnershipEvent, privkey)
		require.NoError(t, relay.Publish(ctx, validCommunityTransferOwnershipEvent.Event))
	})
	t.Run("kind 1751 (Community transferring ownership) (ICIP-3000): no expiration", func(t *testing.T) {
		hVal, err := uuid.NewV7()
		require.NoError(t, err)

		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"h", hVal.String()})
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:%v:%v", model.CustomIONKindCommunityDefinition, eventAuthorPubkey, "communityDIdentifier")})
		tags = append(tags, nostr.Tag{"p", uuid.NewString()})

		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.CustomIONKindCommunityOwnershipTransferring,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 1751 (Community transferring ownership) (ICIP-3000): expired", func(t *testing.T) {
		hVal, err := uuid.NewV7()
		require.NoError(t, err)

		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"h", hVal.String()})
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:%v:%v", model.CustomIONKindCommunityDefinition, eventAuthorPubkey, "communityDIdentifier")})
		tags = append(tags, nostr.Tag{"p", uuid.NewString()})
		tags = append(tags, nostr.Tag{"expiration", fmt.Sprint(time.Now().Add(-1 * time.Minute).Unix())})

		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.CustomIONKindCommunityOwnershipTransferring,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 1751 (Community transferring ownership) (ICIP-3000): wrong a tag: wrong kind", func(t *testing.T) {
		hVal, err := uuid.NewV7()
		require.NoError(t, err)

		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"h", hVal.String()})
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:%v:%v", model.CustomIONKindCommunityJoin, eventAuthorPubkey, "communityDIdentifier")})
		tags = append(tags, nostr.Tag{"p", uuid.NewString()})
		tags = append(tags, nostr.Tag{"expiration", fmt.Sprint(time.Now().Add(1 * time.Minute).Unix())})

		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.CustomIONKindCommunityOwnershipTransferring,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 1751 (Community transferring ownership) (ICIP-3000): wrong a tag", func(t *testing.T) {
		hVal, err := uuid.NewV7()
		require.NoError(t, err)

		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"h", hVal.String()})
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:%v", model.CustomIONKindCommunityDefinition, eventAuthorPubkey)})
		tags = append(tags, nostr.Tag{"p", uuid.NewString()})
		tags = append(tags, nostr.Tag{"expiration", fmt.Sprint(time.Now().Add(1 * time.Minute).Unix())})

		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.CustomIONKindCommunityOwnershipTransferring,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})

	helperMustCloseRelay(t, relay)
}

func TestPublishingICIP3000RelayKindBanUser(t *testing.T) {
	privkey := model.GeneratePrivateKey()
	RegisterWSSubscriptionListener(func(ctx context.Context, s *model.Subscription) EventIterator {
		return query.GetStoredEvents(ctx, s)
	})
	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		require.True(t, len(events) > 0)
		require.NoError(t, query.AcceptEvents(ctx, events...))

		return nil
	})
	ctx := context.Background()
	relay := helperMustNewRelay(t, pubsubServers[0])
	hVal, err := uuid.NewV7()
	require.NoError(t, err)

	t.Run("kind 31750 (Community definition) (ICIP-3000): valid", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"name", "Community name"})
		tags = append(tags, nostr.Tag{"description", "Community description"})
		tags = append(tags, nostr.Tag{"h", hVal.String()})
		tags = append(tags, nostr.Tag{"settings", model.CommentsEnabledSettings, "true", fmt.Sprint(time.Now().Unix())})
		tags = append(tags, nostr.Tag{"settings", model.RoleRequiredForPostingSettings, string(model.ModeratorRole), fmt.Sprint(time.Now().Unix())})
		tags = append(tags, nostr.Tag{"public"})
		tags = append(tags, nostr.Tag{"open"})
		tags = append(tags, nostr.Tag{"p", uuid.NewString(), "relay", string(model.ModeratorRole)})
		tags = append(tags, nostr.Tag{"p", uuid.NewString(), "relay", string(model.AdminRole)})
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:%v:%v", model.CustomIONKindCommunityDefinition, uuid.NewString(), "communityDIdentifier1")})
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:%v:%v", model.CustomIONKindCommunityDefinition, uuid.NewString(), "communityDIdentifier2")})
		tags = append(tags, nostr.Tag{
			"imeta",
			"url https://alicerelay.example.com",
			"m image/jpg",
			"i foobar",
			"dim 3024x3024",
			"alt A scenic photo overlooking the coast of Costa Rica",
			fmt.Sprintf("x %x", []byte("https://alicerelay.example.com")),
			fmt.Sprintf("ox %x", []byte("https://alicerelay.example.com")),
		})

		validCommunityDefinitionEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.CustomIONKindCommunityDefinition,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, validCommunityDefinitionEvent, privkey)
		require.NoError(t, relay.Publish(ctx, validCommunityDefinitionEvent.Event))
	})
	t.Run("kind 1752 (Community ban user) (ICIP-3000): valid", func(t *testing.T) {
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.CustomIONKindCommunityBanUser,
			Tags: model.Tags{
				{"h", hVal.String()},
				{"p", uuid.NewString()},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("kind 1752 (Community ban user) (ICIP-3000): wrong h tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"h", "dummy"})
		tags = append(tags, nostr.Tag{"p", uuid.NewString()})

		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.CustomIONKindCommunityBanUser,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	helperMustCloseRelay(t, relay)
}
