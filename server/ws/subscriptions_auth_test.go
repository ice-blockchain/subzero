// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

func helperDrainSub(t *testing.T, sub *nostr.Subscription) {
	t.Helper()

loop:
	for {
		select {
		case r := <-sub.ClosedReason:
			t.Logf("closed reason: %v", r)
		case <-sub.Events:
			break loop
		case <-sub.EndOfStoredEvents:
			break loop
		case <-time.After(5 * time.Second):
			t.Fatalf("timeout waiting for EOS")
		}
	}
	sub.Close()
}

func TestSubscriptionReqWithAuth(t *testing.T) {
	t.Cleanup(func() {
		RegisterReqMustAuthenticate(nil)
	})

	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		t.Logf("received events: %v", events)

		return nil
	})
	RegisterWSSubscriptionListener(func(ctx context.Context, subscription *model.Subscription) EventIterator {
		t.Logf("received subscription: %v", subscription)

		return helperNewIterator(t, []*model.Event{})
	})
	RegisterReqMustAuthenticate(func(ctx context.Context, subscription *model.Subscription) bool {
		if len(subscription.Filters) > 0 && slices.Contains(subscription.Filters[0].Kinds, nostr.KindTextNote) {
			return true
		}

		return false
	})

	relay := helperMustNewRelay(t, pubsubServers[0])
	t.Run("Regular", func(t *testing.T) {
		sub, err := relay.Subscribe(context.Background(), []model.Filter{
			{
				Kinds: []int{nostr.KindRepost},
			},
		})
		require.NoError(t, err)
		helperDrainSub(t, sub)
	})
	t.Run("WithAuth", func(t *testing.T) {
		sub, err := relay.Subscribe(context.Background(), []model.Filter{
			{
				Kinds: []int{nostr.KindTextNote},
			},
		})
		require.NoError(t, err)
		reason := <-sub.ClosedReason
		t.Logf("closed reason: %v", reason)
		require.True(t, strings.HasPrefix(reason, "auth-required:"))
		sub.Close()
		t.Run("DoAuth", func(t *testing.T) {
			err := relay.Auth(context.Background(), func(event *nostr.Event) error {
				event.Sig = "random-sig" // Want to see an error.

				return nil
			})
			t.Logf("auth error: %v", err)
			require.Error(t, err)

			err = relay.Auth(context.Background(), func(event *nostr.Event) error {
				subZeroEvent := model.Event{Event: *event}
				if err := subZeroEvent.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519); err != nil {
					return err
				}
				*event = subZeroEvent.Event

				return nil
			})
			require.NoError(t, err)
		})
		t.Run("SubscribeAfterAuth", func(t *testing.T) {
			sub, err := relay.Subscribe(context.Background(), []model.Filter{
				{
					Kinds: []int{nostr.KindTextNote},
				},
			})
			require.NoError(t, err)
			helperDrainSub(t, sub)
		})
	})
	helperMustCloseRelay(t, relay)
}

func TestSubscriptionEventAuth(t *testing.T) {
	var storedEvents []*model.Event

	t.Cleanup(func() {
		RegisterEventMustAuthenticate(nil)
	})

	privKey, pubKey := model.GenerateKeyPair()

	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		for _, event := range events {
			require.NotNil(t, event)

			if event.IsEphemeral() {
				continue
			}

			t.Logf("received event: %v", event)
			storedEvents = append(storedEvents, event)
			if event.Kind == nostr.KindArticle {
				master, pk, authenticated := model.GetUserDataFromContext(ctx)
				t.Logf("ctx data: user=%v/%v, auth=%v", master, pk, authenticated)
				require.True(t, authenticated)
				require.Equal(t, pubKey, pk)
				require.Equal(t, pk, master)
			}
		}
		return nil
	})

	RegisterWSSubscriptionListener(func(ctx context.Context, subscription *model.Subscription) EventIterator {
		return helperNewIterator(t, storedEvents)
	})
	RegisterEventMustAuthenticate(func(ctx context.Context, events ...*model.Event) bool {
		for _, event := range events {
			if event.Kind == nostr.KindArticle {
				return true
			}
		}
		return false
	})

	relay := helperMustNewRelay(t, pubsubServers[0])
	t.Run("Regular", func(t *testing.T) {
		var ev model.Event

		ev.Kind = nostr.KindTextNote
		ev.CreatedAt = 1
		ev.Content = "test"
		helperSignWithMinLeadingZeroBits(t, &ev, privKey)
		require.NoError(t, relay.Publish(context.Background(), ev.Event))

	})
	t.Run("WithAuth", func(t *testing.T) {
		var ev model.Event

		ev.Kind = nostr.KindArticle
		ev.CreatedAt = 1
		ev.Content = "test"
		ev.Tags = model.Tags{
			{"title", "test"},
			{"d", "foo"},
		}
		helperSignWithMinLeadingZeroBits(t, &ev, privKey)
		err := relay.Publish(context.Background(), ev.Event)
		t.Logf("publish error: %v", err)
		require.Error(t, err)
		require.Contains(t, err.Error(), errAuthRequired.Error())
	})
	t.Run("DoAuth", func(t *testing.T) {
		err := relay.Auth(context.Background(), func(event *nostr.Event) error {
			subZeroEvent := model.Event{Event: *event}
			if err := subZeroEvent.SignWithAlg(privKey, model.SignAlgEDDSA, model.KeyAlgCurve25519); err != nil {
				return err
			}
			*event = subZeroEvent.Event

			return nil
		})
		require.NoError(t, err)
	})
	t.Run("PublishAfterAuth", func(t *testing.T) {
		var ev model.Event

		ev.Kind = nostr.KindArticle
		ev.CreatedAt = 2
		ev.Content = "test"
		ev.Tags = model.Tags{
			{"title", "test"},
			{"d", "foo"},
		}
		helperSignWithMinLeadingZeroBits(t, &ev, privKey)
		require.NoError(t, relay.Publish(context.Background(), ev.Event))

		events, err := relay.QuerySync(context.Background(), model.Filter{Kinds: []int{nostr.KindArticle}})
		require.NoError(t, err)
		require.Len(t, events, 1)
		require.Equal(t, ev.Event, *events[0])
	})
	helperMustCloseRelay(t, relay)
}

func TestSubscriptionPrivateCommunity(t *testing.T) {
	t.Cleanup(func() {
		RegisterEventMustAuthenticate(nil)
		RegisterWSSubscriptionListener(nil)
		RegisterWSEventListener(nil)
	})

	hVal, err := uuid.NewV7()
	require.NoError(t, err)
	communityID := hVal.String()
	ctx := context.Background()

	privkeyOwner, pubkeyCommunityOwner := model.GenerateKeyPair()
	privkeyUser1, pubkeyUser1 := model.GenerateKeyPair()

	RegisterWSSubscriptionListener(func(ctx context.Context, s *model.Subscription) EventIterator {
		return query.GetStoredEvents(ctx, s)
	})
	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		require.True(t, len(events) > 0)
		require.NoError(t, query.AcceptEvents(ctx, events...))

		return nil
	})

	RegisterEventMustAuthenticate(func(ctx context.Context, events ...*model.Event) bool {
		for _, event := range events {
			if event.Kind == nostr.KindArticle {
				return true
			}
		}
		return false
	})

	var nonCommunityEvent model.Event
	relay := helperMustNewRelay(t, pubsubServers[0])
	t.Run("Regular", func(t *testing.T) {
		nonCommunityEvent.Kind = nostr.KindTextNote
		nonCommunityEvent.CreatedAt = nostr.Timestamp(time.Now().Add(-1 * time.Hour).Unix())
		nonCommunityEvent.Content = "test"
		helperSignWithMinLeadingZeroBits(t, &nonCommunityEvent, privkeyUser1)
		require.NoError(t, relay.Publish(context.Background(), nonCommunityEvent.Event))
	})
	t.Run("WithAuth", func(t *testing.T) {
		var ev model.Event

		ev.Kind = nostr.KindArticle
		ev.CreatedAt = 1
		ev.Content = "test"
		ev.Tags = model.Tags{
			{"title", "test"},
			{"d", "foo"},
		}
		helperSignWithMinLeadingZeroBits(t, &ev, privkeyUser1)
		err := relay.Publish(context.Background(), ev.Event)
		t.Logf("publish error: %v", err)
		require.Error(t, err)
		require.Contains(t, err.Error(), errAuthRequired.Error())
	})
	t.Run("DoAuth", func(t *testing.T) {
		err := relay.Auth(ctx, func(event *nostr.Event) error {
			subZeroEvent := model.Event{Event: *event}
			if err := subZeroEvent.SignWithAlg(privkeyUser1, model.SignAlgEDDSA, model.KeyAlgCurve25519); err != nil {
				return err
			}
			*event = subZeroEvent.Event

			return nil
		})
		require.NoError(t, err)
	})

	t.Run("create private community", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"d", "dtagvalue"},
					{"name", "some name"},
					{"description", "some description"},
					{"closed"},
					{"private"},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("join owner to the community", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyCommunityOwner},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})

	//---- POST BY OWNER ----
	var communityPostEvent *model.Event
	t.Run("try to post to the community by owner, ok", func(t *testing.T) {
		communityPostEvent = &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Timestamp(time.Now().Add(-30 * time.Minute).Unix()),
				Kind:      nostr.KindTextNote,
				Content:   "some text by owner",
				Tags: model.Tags{
					{"h", communityID},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, communityPostEvent, privkeyOwner)
		require.NoError(t, relay.Publish(ctx, communityPostEvent.Event))
	})
	t.Run("select data by authorized user, post from private community is not available", func(t *testing.T) {
		var ev model.Event

		ev.Kind = nostr.KindArticle
		ev.CreatedAt = 2
		ev.Content = "test 2"
		ev.Tags = model.Tags{
			{"title", "test"},
			{"d", "foo"},
		}
		helperSignWithMinLeadingZeroBits(t, &ev, privkeyUser1)
		require.NoError(t, relay.Publish(ctx, ev.Event))

		events, err := relay.QuerySync(ctx, model.Filter{Kinds: []int{nostr.KindTextNote}})
		require.NoError(t, err)
		require.Len(t, events, 1)
		require.Equal(t, nonCommunityEvent.Event, *events[0])
	})

	//---- JOIN user1 ----
	t.Run("send invitation to user1", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser1},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("accept invitation by user1 to join to the community", func(t *testing.T) {
		ownerAuthorizationEvent := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyCommunityOwner},
					{"expiration", strconv.FormatInt(time.Now().Add(1*time.Hour).Unix(), 10)},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ownerAuthorizationEvent, privkeyOwner)
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser1},
					{"authorization", ownerAuthorizationEvent.String()},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser1)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})

	t.Run("select data by authorized user that is in the community now", func(t *testing.T) {
		events, err := relay.QuerySync(ctx, model.Filter{Kinds: []int{nostr.KindTextNote}})
		require.NoError(t, err)
		require.Len(t, events, 2)

		require.Contains(t, events, &nonCommunityEvent.Event)
		require.Contains(t, events, &communityPostEvent.Event)
	})

	//---- BAN USER 1 ----
	t.Run("ban user by owner", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityBanUser,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser1},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})

	t.Run("select data by authorized user that is in the community now", func(t *testing.T) {
		events, err := relay.QuerySync(ctx, model.Filter{Kinds: []int{nostr.KindTextNote}})
		require.NoError(t, err)
		require.Len(t, events, 1)
		require.Equal(t, nonCommunityEvent.Event, *events[0])
	})

	helperMustCloseRelay(t, relay)

	newRelay := helperMustNewRelay(t, pubsubServers[0])

	t.Run("select data by non authorized user from another relay", func(t *testing.T) {
		events, err := newRelay.QuerySync(ctx, model.Filter{Kinds: []int{nostr.KindTextNote}})
		require.NoError(t, err)
		require.Len(t, events, 1)
		require.Equal(t, nonCommunityEvent.Event, *events[0])
	})

	helperMustCloseRelay(t, newRelay)
}
