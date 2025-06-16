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
	RegisterWSSubscriptionListener(func(context.Context, ...model.Filter) EventIterator {
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
		sub, err := relay.Subscribe(t.Context(), []model.Filter{
			{
				Kinds: []int{nostr.KindRepost},
			},
		})
		require.NoError(t, err)
		helperDrainSub(t, sub)
	})
	t.Run("WithAuth", func(t *testing.T) {
		sub, err := relay.Subscribe(t.Context(), []model.Filter{
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
			err := relay.Auth(t.Context(), func(event *nostr.Event) error {
				event.Sig = "random-sig" // Want to see an error.

				return nil
			})
			t.Logf("auth error: %v", err)
			require.Error(t, err)
			helperDoAuth(t, relay.Relay, model.GeneratePrivateKey())
		})
		t.Run("SubscribeAfterAuth", func(t *testing.T) {
			sub, err := relay.Subscribe(t.Context(), []model.Filter{
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
				master, pk, authenticated, _ := model.GetUserDataFromContext(ctx)
				t.Logf("ctx data: user=%v/%v, auth=%v", master, pk, authenticated)
				require.True(t, authenticated)
				require.Equal(t, pubKey, pk)
				require.Equal(t, pk, master)
			}
		}
		return nil
	})

	RegisterWSSubscriptionListener(func(context.Context, ...model.Filter) EventIterator {
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
		require.NoError(t, relay.Publish(t.Context(), ev.Event))

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
		err := relay.Publish(t.Context(), ev.Event)
		t.Logf("publish error: %v", err)
		require.Error(t, err)
		require.Contains(t, err.Error(), errAuthRequired.Error())
	})
	t.Run("DoAuth", func(t *testing.T) {
		helperDoAuth(t, relay.Relay, privKey)
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
		require.NoError(t, relay.Publish(t.Context(), ev.Event))

		events, err := relay.QuerySync(t.Context(), model.Filter{Kinds: []int{nostr.KindArticle}})
		require.NoError(t, err)
		require.Len(t, events, 1)
		require.Equal(t, ev.Event, *events[0])
	})
	helperMustCloseRelay(t, relay)
}

func TestSubscriptionEventAuthWithEmbeddedAttesttion(t *testing.T) {
	var storedEvents []*model.Event

	t.Cleanup(func() {
		RegisterEventMustAuthenticate(nil)
	})

	masterPrivKey, masterPubKey := model.GenerateKeyPair()
	privKey, pubKey := model.GenerateKeyPair()

	var attestation model.Event
	attestation.Kind = model.CustomIONKindAttestation
	attestation.CreatedAt = nostr.Now()
	attestation.Tags = model.Tags{
		{model.TagAttestationName, pubKey, "", model.CustomIONAttestationKindActive + ":1"},
	}
	helperSignWithMinLeadingZeroBits(t, &attestation, masterPrivKey)

	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		for _, event := range events {
			require.NotNil(t, event)

			if event.IsEphemeral() {
				continue
			}

			t.Logf("received event: %v", event)
			storedEvents = append(storedEvents, event)
			if event.Kind == nostr.KindArticle {
				master, pk, authenticated, _ := model.GetUserDataFromContext(ctx)
				t.Logf("ctx data: user=%q/%q, auth=%v", master, pk, authenticated)
				require.True(t, authenticated)
				require.Equal(t, pubKey, pk)
				require.Equal(t, masterPubKey, master)
			}
		}
		return nil
	})

	RegisterWSSubscriptionListener(func(context.Context, ...model.Filter) EventIterator {
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
		err := relay.Publish(t.Context(), ev.Event)
		t.Logf("publish error: %v", err)
		require.Error(t, err)
		require.Contains(t, err.Error(), errAuthRequired.Error())
	})
	t.Run("DoAuth", func(t *testing.T) {
		err := relay.Auth(t.Context(), func(event *nostr.Event) error {
			subZeroEvent := model.Event{Event: *event}
			subZeroEvent.Tags = append(subZeroEvent.Tags, model.Tag{model.CustomIONTagOnBehalfOf, masterPubKey})
			subZeroEvent.Tags = append(subZeroEvent.Tags, model.Tag{"attestation", attestation.String()})
			require.NoError(t, subZeroEvent.SignWithAlg(privKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

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
		require.NoError(t, relay.Publish(t.Context(), ev.Event))

		events, err := relay.QuerySync(t.Context(), model.Filter{Kinds: []int{nostr.KindArticle}})
		require.NoError(t, err)
		require.Len(t, events, 1)
		require.Equal(t, ev.Event, *events[0])
	})
	helperMustCloseRelay(t, relay)
}
