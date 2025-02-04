// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

func TestSelfChat(t *testing.T) {
	t.Cleanup(func() {
		RegisterReqMustAuthenticate(nil)
		RegisterEventMustAuthenticate(nil)
	})

	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		for _, event := range events {
			if event.Kind == nostr.KindGiftWrap {
				_, _, authenticated, _ := model.GetUserDataFromContext(ctx)
				if authenticated {
					return fmt.Errorf("%v: authenticated user is not allowed to send gift wrap events", event.ID)
				}
			}
		}

		t.Logf("received events: %v", events)

		return nil
	})
	RegisterWSSubscriptionListener(func(ctx context.Context, subscription *model.Subscription) EventIterator {
		t.Logf("received subscription: %v", subscription)

		return helperNewIterator(t, []*model.Event{})
	})
	RegisterReqMustAuthenticate(func(ctx context.Context, subscription *model.Subscription) bool {
		return true
	})
	RegisterEventMustAuthenticate(func(ctx context.Context, events ...*model.Event) bool {
		for _, event := range events {
			if event.Kind != nostr.KindGiftWrap {
				return true
			}
		}
		return false
	})

	priv, pub := model.GenerateKeyPair()
	masterPriv, masterPub := model.GenerateKeyPair()

	var attestation model.Event
	attestation.Kind = model.CustomIONKindAttestation
	attestation.CreatedAt = 1
	attestation.Tags = model.Tags{
		{model.TagAttestationName, pub, "", model.CustomIONAttestationKindActive + ":1"},
	}
	helperSignWithMinLeadingZeroBits(t, &attestation, masterPriv)
	require.NoError(t, query.AcceptEvents(context.Background(), &attestation))

	receiver := helperMustNewRelay(t, pubsubServers[0])
	t.Run("Auth", func(t *testing.T) {
		var note model.Event

		note.Kind = nostr.KindTextNote
		note.CreatedAt = 1
		note.Content = "test"
		helperSignWithMinLeadingZeroBits(t, &note, priv)
		err := receiver.Publish(context.Background(), note.Event)
		if err != nil {
			require.NoError(t, receiver.Auth(context.Background(), func(event *nostr.Event) error {
				sEvent := model.Event{Event: *event}
				sEvent.Tags = append(sEvent.Tags, model.Tag{model.CustomIONTagOnBehalfOf, masterPub})

				require.NoError(t, sEvent.SignWithAlg(priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
				*event = sEvent.Event

				return nil
			}))
		}
	})

	sub, err := receiver.Subscribe(context.Background(), []model.Filter{
		{
			Kinds: []int{nostr.KindGiftWrap},
			Tags: model.TagMap{}.
				Append("k", model.PointerOf("1")).
				Append("k", model.PointerOf("42")),
		},
	})
	require.NoError(t, err)

	tSend, tFinish := time.After(time.Second), time.After(time.Second*2)
	var received int
loop:
	for {
		select {
		case <-tFinish:
			break loop

		case <-tSend:
			var evUser, evMaster, evRandom model.Event

			evUser.Kind = nostr.KindGiftWrap
			evUser.CreatedAt = 2
			evUser.Content = "test"
			evUser.Tags = model.Tags{
				{"p", pub},
				{"k", "1"},
				{"expiration", strconv.FormatInt(time.Now().Add(time.Minute).Unix(), 10)},
			}
			evMaster.Kind = nostr.KindGiftWrap
			evMaster.CreatedAt = 2
			evMaster.Content = "test master"
			evMaster.Tags = model.Tags{
				{"p", masterPub},
				{"k", "1"},
				{"expiration", strconv.FormatInt(time.Now().Add(time.Minute).Unix(), 10)},
			}
			evRandom.Kind = nostr.KindGiftWrap
			evRandom.CreatedAt = 3
			evRandom.Content = "test random"
			evRandom.Tags = model.Tags{
				{"p", "some_random_pub"},
				{"k", "1"},
				{"expiration", strconv.FormatInt(time.Now().Add(time.Minute).Unix(), 10)},
			}
			helperSignWithMinLeadingZeroBits(t, &evUser, model.GeneratePrivateKey())
			helperSignWithMinLeadingZeroBits(t, &evRandom, model.GeneratePrivateKey())
			helperSignWithMinLeadingZeroBits(t, &evMaster, model.GeneratePrivateKey())

			sender := helperMustNewRelay(t, pubsubServers[0])
			require.NoError(t, sender.PublishMany(context.Background(), &evUser.Event, &evMaster.Event, &evRandom.Event))
			helperMustCloseRelay(t, sender)

		case ev := <-sub.Events:
			t.Logf("received event in subscription: %v", ev)
			received++
			require.True(t, ev.Tags.ContainsAny("p", []string{pub, masterPub}))

		case <-sub.EndOfStoredEvents:
			t.Logf("end of stored events")

		case reason := <-sub.ClosedReason:
			t.Fatalf("subscription closed: %v", reason)
		}
	}
	require.Equal(t, 2, received)
	helperMustCloseRelay(t, receiver)
}
