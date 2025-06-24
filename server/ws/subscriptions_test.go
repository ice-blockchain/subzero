// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"context"
	"log"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/google/uuid"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip13"
	"github.com/schollz/progressbar/v3"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/server/ws/fixture"
)

type nostrRelay struct {
	*nostr.Relay
	service *fixture.MockService
}

func TestRelayEventsBroadcastMultipleSubs(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), testDeadline)
	defer cancel()
	privkey := model.GeneratePrivateKey()
	storedEvents := []*model.Event{{Event: nostr.Event{
		CreatedAt: nostr.Now(),
		Kind:      nostr.KindTextNote,
		Content:   "db event",
	}}}
	RegisterWSSubscriptionListener(func(context.Context, ...model.Filter) EventIterator {
		return helperNewIterator(t, storedEvents)
	})
	helperSignWithMinLeadingZeroBits(t, storedEvents[len(storedEvents)-1], privkey)
	helperRegisterWSEventListenerProxyWithStorage(t, &storedEvents)
	pubsubServers[0].Reset()
	connsCount := 10
	subsPerConnectionCount := 10
	subs := make(map[*nostr.Relay]map[*nostr.Subscription]struct{}, 0)
	subCtx, subCancel := context.WithTimeout(ctx, 5*time.Second)
	defer subCancel()
	filters := []nostr.Filter{{
		Kinds: []int{nostr.KindTextNote},
		Limit: 1,
	}}
	for connIdx := 0; connIdx < connsCount; connIdx++ {
		relay, err := fixture.NewRelayClient(ctx, pubsubServers[0].Endpoint())
		if err != nil {
			log.Panic(err)
		}
		subsForConn, ok := subs[relay]
		if !ok {
			subsForConn = make(map[*nostr.Subscription]struct{})
			subs[relay] = subsForConn
		}
		for subIdx := 0; subIdx < subsPerConnectionCount; subIdx++ {
			sub, err := relay.Subscribe(subCtx, filters)
			if err != nil {
				log.Panic(err)
			}
			subsForConn[sub] = struct{}{}
		}
	}
	newRealtimeEvent := model.Event{
		Event: nostr.Event{
			Kind:    nostr.KindTextNote,
			Content: "new realtime event",
		},
	}
	require.NoError(t, newRealtimeEvent.SignWithAlg(privkey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	tag, err := nip13.DoWork(ctx, newRealtimeEvent.Event, NIP13MinLeadingZeroBits)
	require.NoError(t, err)
	newRealtimeEvent.Tags = append(newRealtimeEvent.Tags, tag)
	require.NoError(t, newRealtimeEvent.SignWithAlg(privkey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	var wg sync.WaitGroup
	eosCh := make(chan struct{})
	for _, subsForConn := range subs {
		for s := range subsForConn {
			wg.Add(1)
			go func(sub *nostr.Subscription) {
				defer wg.Done()
				var ev *nostr.Event
				select {
				case ev = <-sub.Events:
				case <-ctx.Done():
					log.Panic(errors.New("timeout waiting for the event"))
				}
				require.Equal(t, storedEvents[0].ID, ev.ID)
				require.Equal(t, storedEvents[0].Tags, ev.Tags)
				require.Equal(t, storedEvents[0].CreatedAt, ev.CreatedAt)
				require.Equal(t, storedEvents[0].Sig, ev.Sig)
				require.Equal(t, storedEvents[0].Kind, ev.Kind)
				require.Equal(t, storedEvents[0].PubKey, ev.PubKey)
				require.Equal(t, storedEvents[0].Content, ev.Content)
				select {
				case <-eosCh:
				case <-ctx.Done():
					log.Panic(errors.New("timeout waiting for EOS"))
				}
				select {
				case ev = <-sub.Events:
				case <-ctx.Done():
					log.Panic(errors.New("timeout waiting for the event"))
				}
				require.NotNil(t, ev)
				require.Equal(t, storedEvents[1].ID, ev.ID)
				require.Equal(t, storedEvents[1].Tags, ev.Tags)
				require.Equal(t, storedEvents[1].CreatedAt, ev.CreatedAt)
				require.Equal(t, storedEvents[1].Sig, ev.Sig)
				require.Equal(t, storedEvents[1].Kind, ev.Kind)
				require.Equal(t, storedEvents[1].PubKey, ev.PubKey)
				require.Equal(t, storedEvents[1].Content, ev.Content)

				require.Equal(t, newRealtimeEvent.ID, ev.ID)
				require.Equal(t, newRealtimeEvent.Tags, ev.Tags)
				require.Equal(t, newRealtimeEvent.CreatedAt, ev.CreatedAt)
				require.Equal(t, newRealtimeEvent.Sig, ev.Sig)
				require.Equal(t, newRealtimeEvent.Kind, ev.Kind)
				require.Equal(t, newRealtimeEvent.PubKey, ev.PubKey)
				require.Equal(t, newRealtimeEvent.Content, ev.Content)
				sub.Close()
			}(s)
		}
	}
	var randomRelay *nostr.Relay
	for r, subsForRelay := range subs {
		randomRelay = r
		for s := range subsForRelay {
			select {
			case <-s.EndOfStoredEvents:
			case <-ctx.Done():
				log.Panic(errors.New("timeout waiting for EOS"))
			}
		}
	}
	close(eosCh)
	require.NoError(t, randomRelay.Publish(ctx, newRealtimeEvent.Event))
	wg.Wait()
	for r := range subs {
		require.NoError(t, r.Close())
	}
	require.NoError(t, pubsubServers[0].WaitForReaders(testDeadline))
}

func TestPublishingEvents(t *testing.T) {
	privkey := model.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	helperRegisterWSEventListenerProxyWithStorage(t, &storedEvents)
	ctx := t.Context()
	relay := helperMustNewRelay(t, pubsubServers[0])
	validEvent := &model.Event{Event: nostr.Event{
		CreatedAt: nostr.Now(),
		Kind:      nostr.KindTextNote,
		Tags:      nil,
		Content:   "validEvent",
	}}

	t.Run("valid event", func(t *testing.T) {
		helperSignWithMinLeadingZeroBits(t, validEvent, privkey)
		require.NoError(t, relay.Publish(ctx, validEvent.Event))
	})
	t.Run("invalid event kind", func(t *testing.T) {
		invalidKindEvent := model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      -1,
			Tags:      nil,
			Content:   "invalid kind id event",
		}}
		helperSignWithMinLeadingZeroBits(t, &invalidKindEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidKindEvent.Event))

		invalidKindEvent.Kind = 65536
		helperSignWithMinLeadingZeroBits(t, &invalidKindEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidKindEvent.Event))
	})
	t.Run("invalid event id", func(t *testing.T) {
		invalidID := model.Event{Event: nostr.Event{
			ID:        uuid.NewString(),
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags:      nil,
			Content:   "invalidID",
		}}
		helperSignWithMinLeadingZeroBits(t, &invalidID, privkey)
		invalidID.ID = uuid.NewString()
		require.Error(t, relay.Publish(ctx, invalidID.Event))
	})
	t.Run("invalid event signature", func(t *testing.T) {
		invalidSignature := model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags:      nil,
			Content:   "invalidSignature",
			Sig:       uuid.NewString(),
			PubKey:    uuid.NewString(),
		}}
		require.NoError(t, invalidSignature.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.Error(t, relay.Publish(ctx, invalidSignature.Event))
	})
	t.Run("duplicated event", func(t *testing.T) {
		require.NoError(t, relay.Publish(ctx, validEvent.Event))
	})
	t.Run("ephemeral event", func(t *testing.T) {
		ephemeralEvent := model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindClientAuthentication,
				Content:   "bogus",
			},
		}
		require.NoError(t, ephemeralEvent.SignWithAlg(privkey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		tag, err := nip13.DoWork(ctx, ephemeralEvent.Event, NIP13MinLeadingZeroBits)
		require.NoError(t, err)
		ephemeralEvent.Tags = append(ephemeralEvent.Tags, tag)
		require.NoError(t, ephemeralEvent.SignWithAlg(privkey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, relay.Publish(ctx, ephemeralEvent.Event))
	})
	var emptyKind03Event *model.Event
	t.Run("empty kind 03 follow list tag parameters", func(t *testing.T) {
		emptyKind03Event = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindFollowList,
		}}
		helperSignWithMinLeadingZeroBits(t, emptyKind03Event, privkey)
		require.NoError(t, relay.Publish(ctx, emptyKind03Event.Event))
	})
	t.Run("wrong kind 03 follow list content", func(t *testing.T) {
		inValidKind03Event := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindFollowList,
			Tags:      nostr.Tags{[]string{"p"}, []string{"p"}},
			Content:   "invalidEvent",
		}}
		helperSignWithMinLeadingZeroBits(t, inValidKind03Event, privkey)
		require.Error(t, relay.Publish(ctx, inValidKind03Event.Event))
	})
	var validKind03Event *model.Event
	t.Run("valid kind 03 follow list event", func(t *testing.T) {
		validKind03Event = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindFollowList,
			Tags:      nostr.Tags{[]string{"p", "wss://alicerelay.com/", "alice"}, []string{"p", "wss://bobrelay.com/nostr", "bob"}},
		}}
		helperSignWithMinLeadingZeroBits(t, validKind03Event, privkey)
		require.NoError(t, relay.Publish(ctx, validKind03Event.Event))
	})
	master := model.GeneratePrivateKey()
	userPubKey, _ := model.GetPublicKey(privkey)
	masterPubKey, _ := model.GetPublicKey(master)
	attestationEvent := &model.Event{Event: nostr.Event{
		Kind:      model.CustomIONKindAttestation,
		CreatedAt: 1,
		Tags: model.Tags{
			{model.TagAttestationName, userPubKey, "", model.CustomIONAttestationKindActive + ":" + strconv.Itoa(int(time.Now().Unix()-10))},
		},
	}}
	t.Run("create on-behalf attestations", func(t *testing.T) {
		helperSignWithMinLeadingZeroBits(t, attestationEvent, privkey)
		require.NoError(t, relay.Publish(ctx, attestationEvent.Event))
	})
	onBehalfEvent := *validEvent
	onBehalfEvent.Tags = nostr.Tags{
		{model.CustomIONTagOnBehalfOf, masterPubKey},
	}
	t.Run("valid on behalf event", func(t *testing.T) {
		helperSignWithMinLeadingZeroBits(t, &onBehalfEvent, privkey)
		require.NoError(t, onBehalfEvent.SignWithAlg(privkey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, relay.Publish(ctx, onBehalfEvent.Event))
	})
	helperMustCloseRelay(t, relay)
	require.Equal(t, []*model.Event{validEvent, emptyKind03Event, validKind03Event, attestationEvent, &onBehalfEvent}, storedEvents)
}

func TestRelayMultiEventsAndFilter(t *testing.T) {
	var generatedEvents []*nostr.Event

	privkey := model.GeneratePrivateKey()
	t.Run("Generate", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: 1,
				Kind:      nostr.KindTextNote,
				Tags: nostr.Tags{
					{"e", "bar", "wss://example.com", "reply"},
				},
				Content: "content",
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		generatedEvents = append(generatedEvents, &ev.Event)

		ev = &model.Event{
			Event: nostr.Event{
				CreatedAt: 2,
				Kind:      nostr.KindTextNote,
				Tags: nostr.Tags{
					{"e", "bar", "wss://example.com", "root"},
				},
				Content: "content",
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		generatedEvents = append(generatedEvents, &ev.Event)
	})

	var storedEvents []*model.Event
	helperRegisterWSEventListenerProxyWithStorage(t, &storedEvents)
	helperRegisterWSSubscriptionListenerWithStorage(t, &storedEvents)

	relay := helperMustNewRelay(t, pubsubServers[0])

	t.Run("Publish", func(t *testing.T) {
		t.Logf("publishing %v event(s)", len(generatedEvents))
		err := relay.PublishMany(t.Context(), generatedEvents...)
		require.NoError(t, err)
	})

	receivedEvents, err := relay.QuerySync(t.Context(),
		model.Filter{
			Kinds: []int{nostr.KindTextNote},
			Tags: model.TagMap{}.
				Set("e", model.PointerOf("bar"), nil, model.PointerOf("reply")),
		})
	require.NoError(t, err)

	// Want only one event that matches the filter.
	require.Len(t, receivedEvents, 1)
	require.Equal(t, generatedEvents[0], receivedEvents[0])

	helperMustCloseRelay(t, relay)
}

func TestCanForwardEvent(t *testing.T) {
	t.Parallel()

	t.Run("Regular", func(t *testing.T) {
		require.True(t, canForwardEvent(&model.Event{Event: nostr.Event{Kind: nostr.KindTextNote}}, nil, "", ""))
	})
	t.Run("Protected", func(t *testing.T) {
		user1Priv, user1Pub := model.GenerateKeyPair()
		_, user2Pub := model.GenerateKeyPair()

		var ev model.Event
		ev.Kind = nostr.KindGiftWrap
		ev.Content = "content"
		helperSignWithMinLeadingZeroBits(t, &ev, user1Priv)

		require.False(t, canForwardEvent(&ev, nil, "", user1Pub)) // user1 cannot see it's own event.
		require.False(t, canForwardEvent(&ev, nil, "", user2Pub)) // user2 is not included in the event yet.
		require.False(t, canForwardEvent(&ev, nil, "", ""))

		ev.Tags = append(ev.Tags,
			model.Tag{"p", "", "", user2Pub},
		)
		helperSignWithMinLeadingZeroBits(t, &ev, user1Priv)
		require.True(t, canForwardEvent(&ev, nil, "", user2Pub))
	})
	t.Run("Not allowed", func(t *testing.T) {
		user1Priv, user1Pub := model.GenerateKeyPair()

		var ev model.Event
		ev.Kind = nostr.KindTextNote
		ev.Content = "content"
		helperSignWithMinLeadingZeroBits(t, &ev, user1Priv)

		require.True(t, canForwardEvent(&ev, nil, "", user1Pub))
		require.True(t, canForwardEvent(&ev, map[int]struct{}{
			nostr.KindTextNote: {},
		}, "", user1Pub))
		require.False(t, canForwardEvent(&ev, map[int]struct{}{
			nostr.KindArticle: {},
		}, "", user1Pub))
	})
}

func TestSubscriptionMostRelevantFollowers(t *testing.T) {
	t.Cleanup(func() {
		RegisterReqMustAuthenticate(nil)
		RegisterEventMustAuthenticate(nil)
	})

	privKey, pubKey := model.GenerateKeyPair()
	RegisterWSEventListener(func(context.Context, ...*model.Event) error {
		return nil
	})
	RegisterWSSubscriptionListener(func(ctx context.Context, filters ...model.Filter) EventIterator {
		require.Len(t, filters, 1)
		require.Len(t, filters[0].Kinds, 1)
		require.Equal(t, nostr.KindFollowList, filters[0].Kinds[0])
		require.Contains(t, filters[0].Authors, pubKey)
		require.Equal(t, `include:dependencies:kind3>kind0+p+|foo,bar|`, filters[0].Search)

		return query.GetStoredEvents(ctx, filters...)
	})
	RegisterReqMustAuthenticate(func(context.Context, *model.Subscription) bool {
		return false
	})
	RegisterEventMustAuthenticate(func(context.Context, ...*model.Event) bool {
		return true
	})

	relay := helperMustNewRelay(t, pubsubServers[0])
	t.Run("DoAuth", func(t *testing.T) {
		var ev model.Event
		ev.Kind = nostr.KindTextNote
		ev.CreatedAt = 1
		ev.Content = "test"
		helperSignWithMinLeadingZeroBits(t, &ev, privKey)
		err := relay.Publish(t.Context(), ev.Event)
		require.Error(t, err)
		require.Contains(t, err.Error(), errAuthRequired.Error())
		helperDoAuth(t, relay.Relay, privKey)
	})
	t.Run("Request", func(t *testing.T) {
		helperQueryEventsWithOptions(t, relay.Relay, []nostr.SubscriptionOption{nostr.WithDoNotCheckFilters()}, model.Filter{
			Search: model.ExtensionTextMRF,
			Tags:   model.TagMap{}.Set("p", model.PointerOf("foo")).Append("p", model.PointerOf("bar")),
		})
	})
	helperMustCloseRelay(t, relay)
}

func helperSignWithMinLeadingZeroBits(t testing.TB, event *model.Event, privkey string) {
	t.Helper()
	require.NoError(t, event.SignWithAlg(privkey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, event.GenerateNIP13(t.Context(), NIP13MinLeadingZeroBits))
	require.NoError(t, event.SignWithAlg(privkey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
}

func helperNewIterator[T any](t *testing.T, data []T) func(func(T, error) bool) {
	t.Helper()

	return func(yield func(T, error) bool) {
		for i := range data {
			if !yield(data[i], nil) {
				return
			}
		}
	}
}

func helperDoAuth(t *testing.T, relay *nostr.Relay, privateKey string, masterKey ...string) {
	t.Helper()

	err := relay.Auth(t.Context(), func(event *nostr.Event) error {
		subZeroEvent := model.Event{Event: *event}
		if len(masterKey) > 0 {
			subZeroEvent.Tags = append(subZeroEvent.Tags, model.Tag{model.CustomIONTagOnBehalfOf, masterKey[0]})
		}
		if err := subZeroEvent.SignWithAlg(privateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519); err != nil {
			return err
		}
		*event = subZeroEvent.Event

		return nil
	})
	require.NoError(t, err)
}

func helperQueryEventsWithOptions(t *testing.T, relay *nostr.Relay, options []nostr.SubscriptionOption, filters ...model.Filter) (events []*model.Event) {
	t.Helper()

	sub, err := relay.Subscribe(t.Context(), filters, options...)
	require.NoError(t, err)

	go func() {
		select {
		case r := <-sub.ClosedReason:
			t.Log("subscription closed: ", r)
		case <-sub.EndOfStoredEvents:
		case <-t.Context().Done():
		case <-relay.Context().Done():
		}
		sub.Unsub()
	}()

	for evt := range sub.Events {
		events = append(events, &model.Event{Event: *evt})
	}

	return events
}

func helperRegisterWSEventListenerProxy(t *testing.T, f func(*model.Event)) {
	t.Helper()

	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		for i := range events {
			f(events[i])
		}

		return nil
	})
}

func helperRegisterWSEventListenerProxyWithStorage(t *testing.T, storedEvents *[]*model.Event) {
	t.Helper()

	helperRegisterWSEventListenerProxy(t, func(event *model.Event) {
		require.NotNil(t, event)

		if event.IsEphemeral() {
			return
		}

		for i := range *storedEvents {
			if (*storedEvents)[i].ID == event.ID {
				return
			}
		}

		*storedEvents = append(*storedEvents, event)
	})
}

func helperRegisterWSSubscriptionListenerWithStorage(t *testing.T, storedEvents *[]*model.Event) {
	t.Helper()

	RegisterWSSubscriptionListener(func(ctx context.Context, filters ...model.Filter) EventIterator {
		if len(filters) == 0 {
			return helperNewIterator(t, *storedEvents)
		}

		var filteredEvents []*model.Event
		for i := range *storedEvents {
			ev := (*storedEvents)[i]
			if model.FiltersMatch(filters, ev, "", "") {
				filteredEvents = append(filteredEvents, (*storedEvents)[i])
			}
		}
		return helperNewIterator(t, filteredEvents)
	})
}

func helperMustNewRelay(t testing.TB, service *fixture.MockService) *nostrRelay {
	t.Helper()

	service.Reset()
	relay, err := fixture.NewRelayClient(t.Context(), service.Endpoint())
	require.NoError(t, err)
	require.NotNil(t, relay)

	return &nostrRelay{Relay: relay, service: service}
}

func helperMustCloseRelay(t *testing.T, relay *nostrRelay) {
	t.Helper()

	if relay != nil {
		err := relay.Close()
		if err != nil {
			if !(strings.Contains(err.Error(), "relay not connected") || strings.Contains(err.Error(), "relay already closed")) {
				require.NoError(t, err)
			}
		}
		require.NoError(t, relay.service.WaitForReaders(testDeadline))
	}
}

func TestStreamGiftWrapEvents(t *testing.T) {
	const giftWrapCount = 3_000

	masterPriv, masterPub := model.GenerateKeyPair()
	userPriv, userPub := model.GenerateKeyPair()

	t.Cleanup(func() {
		RegisterReqMustAuthenticate(nil)
		RegisterEventMustAuthenticate(nil)
	})

	t.Run("Create attestation", func(t *testing.T) {
		attestationEvent := &model.Event{Event: nostr.Event{
			Kind:      model.CustomIONKindAttestation,
			CreatedAt: 1,
			Tags: model.Tags{
				{model.TagAttestationName, userPub, "", model.CustomIONAttestationKindActive + ":1"},
			},
		}}
		require.NoError(t, attestationEvent.SignWithAlg(masterPriv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, query.AcceptEvents(t.Context(), attestationEvent))
	})
	published := make([]*model.Event, 0, giftWrapCount)
	t.Run("Create gift wrap events", func(t *testing.T) {
		bar := progressbar.Default(int64(giftWrapCount), "generating events")
		for i := range giftWrapCount {
			var ev model.Event
			ev.Kind = nostr.KindGiftWrap
			ev.CreatedAt = model.Timestamp(time.Now().UnixNano())
			ev.Content = "Gift wrap event " + strconv.Itoa(i+1)
			ev.Tags = model.Tags{
				{"p", masterPub, "", userPub},
			}
			require.NoError(t, ev.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.NoError(t, query.AcceptEvents(t.Context(), &ev))
			published = append(published, &ev)
			bar.Add(1)
		}
	})

	RegisterReqMustAuthenticate(func(context.Context, *model.Subscription) bool { return true })
	RegisterEventMustAuthenticate(func(context.Context, ...*model.Event) bool { return true })
	RegisterWSSubscriptionListener(query.GetStoredEvents)
	RegisterWSEventListener(query.AcceptEvents)

	relay := helperMustNewRelay(t, pubsubServers[0])

	t.Run("WithAuth", func(t *testing.T) {
		var ev model.Event
		ev.Kind = nostr.KindNostrConnect
		ev.CreatedAt = nostr.Now()
		helperSignWithMinLeadingZeroBits(t, &ev, userPriv)
		err := relay.Publish(t.Context(), ev.Event)
		require.Error(t, err)
		require.Contains(t, err.Error(), errAuthRequired.Error())
	})
	t.Run("DoAuth", func(t *testing.T) {
		helperDoAuth(t, relay.Relay, userPriv, masterPub)
	})

	helperCompareEvents := func(t *testing.T, received []*model.Event) {
		t.Helper()

		t.Logf("received %d events", len(received))
		expected := make(map[string]struct{}, len(published))
		for _, ev := range published {
			expected[ev.ID] = struct{}{}
		}
		for i := range received {
			delete(expected, received[i].ID)
		}
		require.Emptyf(t, expected, "not all events were received, missing: %#v", expected)
	}

	t.Run("Fetch as single filter", func(t *testing.T) {
		start := time.Now()
		received := helperQueryEvents(t, t.Context(), relay, model.Filter{
			Kinds: []int{nostr.KindGiftWrap},
			Tags:  model.TagMap{}.SetLiterals("p", masterPub, "", userPub),
		})
		t.Logf("received %d events in %s", len(published), time.Since(start))
		helperCompareEvents(t, received)
	})
	t.Run("Fetch as part of the filters in the beginning", func(t *testing.T) {
		received := helperQueryEvents(t, t.Context(), relay,
			model.Filter{
				Kinds: []int{nostr.KindGiftWrap},
				Tags:  model.TagMap{}.SetLiterals("p", masterPub, "", userPub),
			},
			model.Filter{
				Kinds: []int{nostr.KindTextNote},
			},
		)
		helperCompareEvents(t, received)
	})
	t.Run("Fetch as part of the filters in the end", func(t *testing.T) {
		received := helperQueryEvents(t, t.Context(), relay,
			model.Filter{
				Kinds: []int{nostr.KindTextNote},
			},
			model.Filter{
				Kinds: []int{nostr.KindGiftWrap},
				Tags:  model.TagMap{}.SetLiterals("p", masterPub, "", userPub),
			},
		)
		helperCompareEvents(t, received)
	})
	helperMustCloseRelay(t, relay)
}

func TestBufferAndStremEvents(t *testing.T) {
	waitChannel := make(chan struct{})

	RegisterReqMustAuthenticate(nil)
	RegisterEventMustAuthenticate(nil)

	var note model.Event
	note.Kind = nostr.KindTextNote
	note.CreatedAt = nostr.Now()
	note.Content = "test note initial"
	helperSignWithMinLeadingZeroBits(t, &note, model.GeneratePrivateKey())

	RegisterWSSubscriptionListener(func(ctx context.Context, f ...model.Filter) EventIterator {
		t.Logf("DB request with filters: %v", model.Filters(f).String())
		<-waitChannel
		t.Logf("DB request unblocked, returning stored events")
		return helperNewIterator(t, []*model.Event{&note})
	})
	RegisterWSEventListener(func(context.Context, ...*model.Event) error {
		return nil
	})

	relay := helperMustNewRelay(t, pubsubServers[0])

	sub, err := relay.Subscribe(t.Context(), model.Filters{{Kinds: []int{nostr.KindTextNote}}})
	require.NoError(t, err)

	received := make([]*model.Event, 0, 10)
	sent := make([]*model.Event, 0, 10)
	signal := make(chan struct{}, 1)
	signal <- struct{}{} // Start the loop immediately.

loop:
	for {
		select {
		case reason := <-sub.ClosedReason:
			t.Fatalf("subscription %s closed unexpectedly: %s", sub.GetID(), reason)

		case <-sub.EndOfStoredEvents:
			t.Logf("subscription %s reached end of stored events", sub.GetID())
			// Must be the event from the database.
			require.NotEmpty(t, received)
			require.True(t,
				slices.ContainsFunc(received, func(ev *model.Event) bool {
					return ev.ID == note.ID
				}))

		case <-signal:
			t.Logf("sending %d events to relay", cap(sent))
			// Send for event that must be buffered.
			for range cap(sent) {
				var ev model.Event

				ev.Kind = nostr.KindTextNote
				ev.CreatedAt = nostr.Now()
				ev.Content = "test note " + strconv.Itoa(len(sent)+1)
				helperSignWithMinLeadingZeroBits(t, &ev, model.GeneratePrivateKey())
				require.NoError(t, relay.Publish(t.Context(), ev.Event))
				t.Logf("sending event %s / %s", ev.ID, ev.Content)
				sent = append(sent, &ev)
			}
			// Unblock the subscription to start receiving events.
			close(waitChannel)
			t.Logf("unblocked subscription %s", sub.GetID())

		case ev := <-sub.Events:
			t.Logf("received event %s / %s from subscription %s", ev.ID, ev.Content, sub.GetID())
			received = append(received, &model.Event{Event: *ev})
			if len(received) == cap(sent)+1 { // +1 for the event from the database.
				t.Logf("received all %d events", len(received))
				sub.Unsub()
				break loop
			}
		}
	}
	t.Logf("subscription %s finished with %d events", sub.GetID(), len(received))

	require.Len(t, received, len(sent)+1) // +1 for the event from the database.
	require.ElementsMatch(t, append(sent, &note), received)

	helperMustCloseRelay(t, relay)
}
