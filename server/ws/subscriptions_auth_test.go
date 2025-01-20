// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

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
