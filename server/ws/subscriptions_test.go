// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/google/uuid"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip13"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/server/ws/fixture"
)

type nostrRelay struct {
	*nostr.Relay
	service *fixture.MockService
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

func helperMustNewRelay(t *testing.T, service *fixture.MockService) *nostrRelay {
	t.Helper()

	service.Reset()
	relay, err := fixture.NewRelayClient(context.Background(), service.Endpoint())
	require.NoError(t, err)
	require.NotNil(t, relay)

	return &nostrRelay{Relay: relay, service: service}
}

func helperMustCloseRelay(t *testing.T, relay *nostrRelay) {
	t.Helper()

	if relay != nil {
		require.NoError(t, relay.Close())
		require.NoError(t, relay.service.WaitForReaders(testDeadline))
	}
}

func TestRelaySubscription(t *testing.T) {
	var eventsQueue []*model.Event

	privkey := model.GeneratePrivateKey()
	ev := &model.Event{
		Event: nostr.Event{
			ID:        uuid.NewString(),
			PubKey:    uuid.NewString(),
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindTextNote,
			Tags:      nostr.Tags{},
			Content:   uuid.NewString(),
			Sig:       uuid.NewString(),
		},
	}
	helperSignWithMinLeadingZeroBits(t, ev, privkey)
	eventsQueue = append(eventsQueue, ev)

	RegisterWSSubscriptionListener(func(ctx context.Context, subscription *model.Subscription) query.EventIterator {
		events := make([]*model.Event, 0, len(eventsQueue))
		for _, ev := range eventsQueue {
			for _, f := range subscription.Filters {
				if f.Matches(&ev.Event) {
					events = append(events, ev)
				}
			}
		}
		return helperNewIterator(t, events)
	})

	storedEvents := []*model.Event{eventsQueue[len(eventsQueue)-1]}
	helperRegisterWSEventListenerProxyWithStorage(t, &storedEvents)

	ctx := context.Background()
	relay := helperMustNewRelay(t, pubsubServers[0])
	filters := []nostr.Filter{{
		Kinds: []int{nostr.KindTextNote},
		Limit: 1,
	}}

	subCtx, subCancel := context.WithTimeout(ctx, 5*time.Second)
	defer subCancel()

	sub, err := relay.Subscribe(subCtx, filters)
	require.NoError(t, err)

	var receivedEvents []*model.Event
	var wg sync.WaitGroup
	{
		t.Logf("subscribed to %v", sub.GetID())
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ev := range sub.Events {
				t.Logf("received event %v", ev)
				receivedEvents = append(receivedEvents, &model.Event{Event: *ev})
			}
		}()
	}

	select {
	case <-sub.EndOfStoredEvents:
		t.Logf("received EOS")
	case <-ctx.Done():
		t.Fatalf("EOS not received: %v", ctx.Err())
	}

	eventsQueue = append(eventsQueue, &model.Event{
		Event: nostr.Event{
			ID:        uuid.NewString(),
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindTextNote,
			Tags:      nostr.Tags{},
			PubKey:    uuid.NewString(),
			Content:   "realtime event matching filter" + uuid.NewString(),
		},
	})
	helperSignWithMinLeadingZeroBits(t, eventsQueue[len(eventsQueue)-1], privkey)
	require.NoError(t, relay.Publish(ctx, eventsQueue[len(eventsQueue)-1].Event))

	eventBy3rdParty := &model.Event{Event: nostr.Event{
		ID:        uuid.NewString(),
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      nostr.KindTextNote,
		Tags:      nostr.Tags{},
		Content:   "eventBy3rdParty" + uuid.NewString(),
	}}
	eventsQueue = append(eventsQueue, eventBy3rdParty)
	storedEvents = append(storedEvents, eventBy3rdParty)
	require.NoError(t, eventsQueue[len(eventsQueue)-1].SignWithAlg(privkey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, eventsQueue[len(eventsQueue)-1].GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
	require.NoError(t, eventsQueue[len(eventsQueue)-1].SignWithAlg(privkey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, notifySubscriptions(eventBy3rdParty))

	repostedPubkey := "pubkey1"
	repostedID := uuid.NewString()
	notMatchingEvent := &model.Event{Event: nostr.Event{
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      nostr.KindRepost,
		Tags:      nostr.Tags{[]string{"e", repostedID, "relay"}, []string{"p", repostedPubkey}},
		Content:   fmt.Sprintf(`{"kind":1,"id":"%v","pubkey":"%v"}`, repostedID, repostedPubkey),
	}}
	require.NoError(t, notMatchingEvent.SignWithAlg(privkey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, notMatchingEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
	require.NoError(t, notMatchingEvent.SignWithAlg(privkey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, relay.Publish(ctx, notMatchingEvent.Event))

	// Replacing of subscription by another subscription with another filter: smth broken from go-nostr v0.36.0 to write the message to change filters directly.
	sub.Close()
	require.Empty(t, <-sub.ClosedReason)

	sub, err = relay.Subscribe(subCtx, []nostr.Filter{{
		Kinds: []int{nostr.KindArticle},
		Limit: 1,
	}})
	require.NoError(t, err)
	{
		t.Logf("subscribed to %v", sub.GetID())
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ev := range sub.Events {
				t.Logf("received event %v", ev)
				receivedEvents = append(receivedEvents, &model.Event{Event: *ev})
			}
		}()
	}

	select {
	case <-sub.EndOfStoredEvents:
		t.Logf("received EOS")
	case <-ctx.Done():
		t.Fatalf("EOS not received: %v", ctx.Err())
	}

	eventMatchingReplacedSub := &model.Event{Event: nostr.Event{
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      nostr.KindArticle,
		Tags:      nostr.Tags{},
		Content:   "event matching replaced filter" + uuid.NewString(),
	}}
	helperSignWithMinLeadingZeroBits(t, eventMatchingReplacedSub, privkey)
	require.NoError(t, relay.Publish(ctx, eventMatchingReplacedSub.Event))
	eventsQueue = append(eventsQueue, eventMatchingReplacedSub)

	sub.Close()
	require.Empty(t, <-sub.ClosedReason)

	helperMustCloseRelay(t, relay)
	wg.Wait()

	if len(receivedEvents) > len(eventsQueue) {
		t.Logf("FIXME: received more events than expected")
		receivedEvents = receivedEvents[:len(eventsQueue)]
	}
	require.Equal(t, eventsQueue, receivedEvents)
}

func TestRelayEventsBroadcastMultipleSubs(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
	privkey := model.GeneratePrivateKey()
	storedEvents := []*model.Event{{Event: nostr.Event{
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      nostr.KindTextNote,
		Content:   "db event",
	}}}
	RegisterWSSubscriptionListener(func(context.Context, *model.Subscription) query.EventIterator {
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
				assert.Equal(t, storedEvents[0].ID, ev.ID)
				assert.Equal(t, storedEvents[0].Tags, ev.Tags)
				assert.Equal(t, storedEvents[0].CreatedAt, ev.CreatedAt)
				assert.Equal(t, storedEvents[0].Sig, ev.Sig)
				assert.Equal(t, storedEvents[0].Kind, ev.Kind)
				assert.Equal(t, storedEvents[0].PubKey, ev.PubKey)
				assert.Equal(t, storedEvents[0].Content, ev.Content)
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
				assert.Equal(t, storedEvents[1].ID, ev.ID)
				assert.Equal(t, storedEvents[1].Tags, ev.Tags)
				assert.Equal(t, storedEvents[1].CreatedAt, ev.CreatedAt)
				assert.Equal(t, storedEvents[1].Sig, ev.Sig)
				assert.Equal(t, storedEvents[1].Kind, ev.Kind)
				assert.Equal(t, storedEvents[1].PubKey, ev.PubKey)
				assert.Equal(t, storedEvents[1].Content, ev.Content)

				assert.Equal(t, newRealtimeEvent.ID, ev.ID)
				assert.Equal(t, newRealtimeEvent.Tags, ev.Tags)
				assert.Equal(t, newRealtimeEvent.CreatedAt, ev.CreatedAt)
				assert.Equal(t, newRealtimeEvent.Sig, ev.Sig)
				assert.Equal(t, newRealtimeEvent.Kind, ev.Kind)
				assert.Equal(t, newRealtimeEvent.PubKey, ev.PubKey)
				assert.Equal(t, newRealtimeEvent.Content, ev.Content)
				sub.Close()
				assert.Empty(t, <-sub.ClosedReason)
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
	ctx := context.Background()
	relay := helperMustNewRelay(t, pubsubServers[0])
	validEvent := &model.Event{Event: nostr.Event{
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindFollowList,
		}}
		helperSignWithMinLeadingZeroBits(t, emptyKind03Event, privkey)
		require.NoError(t, relay.Publish(ctx, emptyKind03Event.Event))
	})
	t.Run("wrong kind 03 follow list content", func(t *testing.T) {
		inValidKind03Event := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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

func TestPublishingNIP09Events(t *testing.T) {
	privkey := model.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	helperRegisterWSEventListenerProxyWithStorage(t, &storedEvents)
	ctx := context.Background()
	relay := helperMustNewRelay(t, pubsubServers[0])

	var validEventNIP09WithEKTags, validEventNIP09AllTags *model.Event
	t.Run("kind 5 (Deletion) (NIP-05): valid event with e/k tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"})
		tags = append(tags, nostr.Tag{"k", "1"})
		validEventNIP09WithEKTags = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindDeletion,
			Tags:      tags,
			Content:   "Deletion reason",
		}}
		helperSignWithMinLeadingZeroBits(t, validEventNIP09WithEKTags, privkey)
		require.NoError(t, relay.Publish(ctx, validEventNIP09WithEKTags.Event))
	})
	t.Run("kind 5 (Deletion) (NIP-05): valid event with all tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"})
		tags = append(tags, nostr.Tag{"a", "30023:a695f6b60119d9521934a691347d9f78e8770b56da16bb255ee286ddf9fda919:ipsum", "wss://relay.nostr.org"})
		tags = append(tags, nostr.Tag{"k", "1"})
		validEventNIP09AllTags = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindDeletion,
			Tags:      tags,
			Content:   "Deletion reason",
		}}
		helperSignWithMinLeadingZeroBits(t, validEventNIP09AllTags, privkey)
		require.NoError(t, relay.Publish(ctx, validEventNIP09AllTags.Event))
	})
	t.Run("kind 5 (Deletion) (NIP-05): invalid event, no required tags", func(t *testing.T) {
		var tags nostr.Tags
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindDeletion,
			Tags:      tags,
			Content:   "Deletion reason",
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 5 (Deletion) (NIP-05): invalid event, mismatch e -> k tags", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindDeletion,
			Tags:      tags,
			Content:   "Deletion reason",
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 5 (Deletion) (NIP-05): unsupported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"r", "1"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindDeletion,
			Tags:      tags,
			Content:   "Deletion reason",
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})

	helperMustCloseRelay(t, relay)
	require.Equal(t, []*model.Event{validEventNIP09WithEKTags, validEventNIP09AllTags}, storedEvents)
}

func TestPublishingNIP10Events(t *testing.T) {
	privkey := model.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	helperRegisterWSEventListenerProxyWithStorage(t, &storedEvents)
	ctx := context.Background()
	relay := helperMustNewRelay(t, pubsubServers[0])

	t.Run("kind 1 (NIP-10): e tags required params", func(t *testing.T) {
		inValidKind01Event := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindTextNote,
			Tags:      nostr.Tags{[]string{"e"}},
		}}
		helperSignWithMinLeadingZeroBits(t, inValidKind01Event, privkey)
		require.Error(t, relay.Publish(ctx, inValidKind01Event.Event))
	})

	t.Run("kind 1 (NIP-10): invalid reply marker for e tags ", func(t *testing.T) {
		inValidKind01Event := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindTextNote,
			Tags:      nostr.Tags{[]string{"e", "", "relay", "invalid marker"}},
		}}
		helperSignWithMinLeadingZeroBits(t, inValidKind01Event, privkey)
		require.Error(t, relay.Publish(ctx, inValidKind01Event.Event))
	})
	t.Run("kind 1 (NIP-10): invalid p tag usage: no e tags", func(t *testing.T) {
		inValidKind01Event := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindTextNote,
			Tags:      nostr.Tags{[]string{"p", "pubkey1", "pubkey2"}},
		}}
		helperSignWithMinLeadingZeroBits(t, inValidKind01Event, privkey)
		require.Error(t, relay.Publish(ctx, inValidKind01Event.Event))
	})
	t.Run("kind 1 (NIP-10): invalid p tag usage: empty tag values", func(t *testing.T) {
		inValidKind01Event := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindTextNote,
			Tags:      nostr.Tags{[]string{"e", "", "relay", "reply"}, []string{"p"}},
		}}
		helperSignWithMinLeadingZeroBits(t, inValidKind01Event, privkey)
		require.Error(t, relay.Publish(ctx, inValidKind01Event.Event))
	})

	var validKind01NIP10Event *model.Event
	t.Run("kind 1 (NIP-10): valid", func(t *testing.T) {
		validKind01NIP10Event = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindTextNote,
			Tags:      nostr.Tags{[]string{"e", "", "relay", "reply"}, []string{"p", "pubkey1", "pubkey2"}},
		}}
		helperSignWithMinLeadingZeroBits(t, validKind01NIP10Event, privkey)
		require.NoError(t, relay.Publish(ctx, validKind01NIP10Event.Event))
	})

	helperMustCloseRelay(t, relay)
	require.Equal(t, []*model.Event{validKind01NIP10Event}, storedEvents)
}

func TestPublishingNIP18Events(t *testing.T) {
	privkey := model.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	helperRegisterWSEventListenerProxyWithStorage(t, &storedEvents)
	ctx := context.Background()
	relay := helperMustNewRelay(t, pubsubServers[0])

	var originalEvent model.Event
	originalEvent.Kind = nostr.KindTextNote
	originalEvent.CreatedAt = 1
	originalEvent.Content = "hello world"
	helperSignWithMinLeadingZeroBits(t, &originalEvent, privkey)

	var validKind06NIP18Event *model.Event
	t.Run("kind 6 (NIP-18): valid event", func(t *testing.T) {
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: model.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindRepost,
			Tags:      model.Tags{model.Tag{"e", originalEvent.ID, "relay"}, model.Tag{"p", originalEvent.GetMasterPublicKey()}},
			Content:   originalEvent.String(),
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relay.Publish(ctx, ev.Event))
		validKind06NIP18Event = ev
	})
	t.Run("kind 6 (NIP-18): invalid event, no e tags", func(t *testing.T) {
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: model.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindRepost,
			Tags:      model.Tags{model.Tag{"p", originalEvent.GetMasterPublicKey()}},
			Content:   originalEvent.String(),
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("kind 6 (NIP-18): invalid event, no p tags", func(t *testing.T) {
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: model.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindRepost,
			Tags:      model.Tags{model.Tag{"e", originalEvent.ID, "relay"}},
			Content:   originalEvent.String(),
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("kind 6 (NIP-18): invalid event, no enough e tag parameters", func(t *testing.T) {
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: model.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindRepost,
			Tags:      model.Tags{model.Tag{"e", originalEvent.ID}, model.Tag{"p", originalEvent.GetMasterPublicKey()}},
			Content:   originalEvent.String(),
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("kind 6 (NIP-18): invalid event, no enough p tag parameters", func(t *testing.T) {
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: model.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindRepost,
			Tags:      model.Tags{model.Tag{"e", originalEvent.ID, "relay"}, model.Tag{"p"}},
			Content:   originalEvent.String(),
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("kind 6 (NIP-18): invalid event, wrong p tag pubkey != reposted note pubkey", func(t *testing.T) {
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: model.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindRepost,
			Tags:      model.Tags{model.Tag{"e", originalEvent.ID, "relay"}, model.Tag{"p", "foo"}},
			Content:   originalEvent.String(),
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("kind 6 (NIP-18): invalid event, wrong content value: != 1", func(t *testing.T) {
		var subEv model.Event
		subEv.Kind = nostr.KindArticle
		subEv.CreatedAt = 1
		subEv.Content = "hello world"
		helperSignWithMinLeadingZeroBits(t, &subEv, privkey)

		ev := &model.Event{Event: nostr.Event{
			CreatedAt: model.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindRepost,
			Tags:      model.Tags{model.Tag{"e", subEv.ID, "relay"}, model.Tag{"p", subEv.GetMasterPublicKey()}},
			Content:   subEv.String(),
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})

	var validKind16NIP18GenericRepostEvent *model.Event
	t.Run("kind 16 (NIP-18): valid generic repost event", func(t *testing.T) {
		var subEv model.Event
		subEv.Kind = nostr.KindArticle
		subEv.CreatedAt = 1
		subEv.Content = "hello article"
		helperSignWithMinLeadingZeroBits(t, &subEv, privkey)

		ev := &model.Event{Event: nostr.Event{
			CreatedAt: model.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindGenericRepost,
			Tags: model.Tags{
				model.Tag{"e", subEv.ID, "relay"},
				model.Tag{"p", subEv.GetMasterPublicKey()},
				model.Tag{"k", strconv.Itoa(subEv.Kind)},
			},
			Content: subEv.String(),
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relay.Publish(ctx, ev.Event))
		validKind16NIP18GenericRepostEvent = ev
	})

	t.Run("kind 16 (NIP-18): invalid generic repost event: wrong k tag", func(t *testing.T) {
		var subEv model.Event
		subEv.Kind = nostr.KindArticle
		subEv.CreatedAt = 1
		subEv.Content = "hello article 2"
		helperSignWithMinLeadingZeroBits(t, &subEv, privkey)

		ev := &model.Event{Event: nostr.Event{
			CreatedAt: model.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindGenericRepost,
			Tags: model.Tags{
				model.Tag{"e", subEv.ID, "relay"},
				model.Tag{"p", subEv.GetMasterPublicKey()},
				model.Tag{"k", "foo"},
			},
			Content: subEv.String(),
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})

	var validKind16NIP18GenericRepostEventWithBTag *model.Event
	t.Run("kind 16 (NIP-18): valid repost with b tag", func(t *testing.T) {
		_, pub := model.GenerateKeyPair()

		var subEv model.Event
		subEv.Kind = nostr.KindArticle
		subEv.CreatedAt = 1
		subEv.Content = "hello article with B"
		subEv.Tags = model.Tags{
			model.Tag{model.CustomIONTagOnBehalfOf, pub},
		}
		helperSignWithMinLeadingZeroBits(t, &subEv, privkey)
		require.Equal(t, subEv.GetMasterPublicKey(), pub)

		ev := &model.Event{Event: nostr.Event{
			CreatedAt: model.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindGenericRepost,
			Tags: model.Tags{
				model.Tag{"e", subEv.ID, "relay"},
				model.Tag{"p", pub},
				model.Tag{"k", strconv.Itoa(subEv.Kind)},
			},
			Content: subEv.String(),
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relay.Publish(ctx, ev.Event))
		validKind16NIP18GenericRepostEventWithBTag = ev
	})

	helperMustCloseRelay(t, relay)
	require.Equal(t,
		[]*model.Event{
			validKind06NIP18Event,
			validKind16NIP18GenericRepostEvent,
			validKind16NIP18GenericRepostEventWithBTag,
		},
		storedEvents,
	)
}

func TestPublishingNIP23Events(t *testing.T) {
	privkey := model.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	helperRegisterWSEventListenerProxyWithStorage(t, &storedEvents)
	ctx := context.Background()
	relay := helperMustNewRelay(t, pubsubServers[0])

	var validEventKindArticle, validEventKindBlogPost, validEventNoTagsKindArticle *model.Event
	t.Run("kind 30023 (Article) (NIP-23): valid event", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"a", "30023:a695f6b60119d9521934a691347d9f78e8770b56da16bb255ee286ddf9fda919:ipsum", "wss://relay.nostr.org"})
		tags = append(tags, nostr.Tag{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"})
		tags = append(tags, nostr.Tag{"t", "placeholder"})
		tags = append(tags, nostr.Tag{"published_at", "1296962229"})
		tags = append(tags, nostr.Tag{"title", "Lorem Ipsum"})
		tags = append(tags, nostr.Tag{"d", "lorem-ipsum"})
		validEventKindArticle = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindDraftArticle,
			Tags:      tags,
			Content:   "Lorem [ipsum][nostr:nevent1qqst8cujky046negxgwwm5ynqwn53t8aqjr6afd8g59nfqwxpdhylpcpzamhxue69uhhyetvv9ujuetcv9khqmr99e3k7mg8arnc9] dolor sit amet",
		}}
		helperSignWithMinLeadingZeroBits(t, validEventKindBlogPost, privkey)
		require.NoError(t, relay.Publish(ctx, validEventKindBlogPost.Event))
	})

	t.Run("kind 30023 (Article) (NIP-23): valid event no tags", func(t *testing.T) {
		var tags nostr.Tags
		validEventNoTagsKindArticle = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindArticle,
			Tags:      tags,
			Content:   "Lorem [ipsum][nostr:nevent1qqst8cujky046negxgwwm5ynqwn53t8aqjr6afd8g59nfqwxpdhylpcpzamhxue69uhhyetvv9ujuetcv9khqmr99e3k7mg8arnc9] dolor sit amet",
		}}
		helperSignWithMinLeadingZeroBits(t, validEventNoTagsKindArticle, privkey)
		require.NoError(t, relay.Publish(ctx, validEventNoTagsKindArticle.Event))
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindArticle,
			Tags:      tags,
			Content:   "Lorem [ipsum][nostr:nevent1qqst8cujky046negxgwwm5ynqwn53t8aqjr6afd8g59nfqwxpdhylpcpzamhxue69uhhyetvv9ujuetcv9khqmr99e3k7mg8arnc9] dolor sit amet",
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	helperMustCloseRelay(t, relay)
	require.Equal(t, []*model.Event{validEventKindArticle, validEventKindBlogPost, validEventNoTagsKindArticle}, storedEvents)
}

func TestPublishingNIP01NIP24Events(t *testing.T) {
	privkey := model.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	helperRegisterWSEventListenerProxyWithStorage(t, &storedEvents)
	ctx := context.Background()
	relay := helperMustNewRelay(t, pubsubServers[0])

	var validEventNIP01, validEventNIP24 *model.Event
	t.Run("kind 0 (ProfileMetadata) (NIP-01): valid event", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"a", "30023:a695f6b60119d9521934a691347d9f78e8770b56da16bb255ee286ddf9fda919:ipsum", "wss://relay.nostr.org"})
		tags = append(tags, nostr.Tag{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"})
		tags = append(tags, nostr.Tag{"p", "pubkey1"})
		tags = append(tags, nostr.Tag{"alt", "reply"})
		validEventNIP01 = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindProfileMetadata,
			Tags:      tags,
			Content:   `{"name":"qwerty","display_name":"qwerty","about":"me is bot","picture":"https://example.com/pic.jpg"}`,
		}}
		helperSignWithMinLeadingZeroBits(t, validEventNIP01, privkey)
		require.NoError(t, relay.Publish(ctx, validEventNIP01.Event))
	})
	t.Run("kind 0 (ProfileMetadata) (NIP-24): valid event with NIP-24 extra fields", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"a", "30023:a695f6b60119d9521934a691347d9f78e8770b56da16bb255ee286ddf9fda919:ipsum", "wss://relay.nostr.org"})
		tags = append(tags, nostr.Tag{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"})
		tags = append(tags, nostr.Tag{"p", "pubkey1"})
		tags = append(tags, nostr.Tag{"alt", "reply"})
		validEventNIP24 = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindProfileMetadata,
			Tags:      tags,
			Content:   `{"name":"qwerty","about":"me is bot","picture":"https://example.com/pic.jpg","display_name":"qqq","website":"https://ice.io","banner":"https://example.com/banner.jpg","bot":true}`,
		}}
		helperSignWithMinLeadingZeroBits(t, validEventNIP24, privkey)
		require.NoError(t, relay.Publish(ctx, validEventNIP24.Event))
	})
	t.Run("kind 0 (ProfileMetadata) (NIP-24): invalid event: empty obligatory fields", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"a", "30023:a695f6b60119d9521934a691347d9f78e8770b56da16bb255ee286ddf9fda919:ipsum", "wss://relay.nostr.org"})
		tags = append(tags, nostr.Tag{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"})
		tags = append(tags, nostr.Tag{"p", "pubkey1"})
		tags = append(tags, nostr.Tag{"alt", "reply"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindProfileMetadata,
			Tags:      tags,
			Content:   `{"display_name":"","website":"https://ice.io","banner":"https://example.com/banner.jpg","bot":true}`,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 0 (ProfileMetadata) (NIP-24): invalid event: content is not JSON", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"a", "30023:a695f6b60119d9521934a691347d9f78e8770b56da16bb255ee286ddf9fda919:ipsum", "wss://relay.nostr.org"})
		tags = append(tags, nostr.Tag{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"})
		tags = append(tags, nostr.Tag{"p", "pubkey1"})
		tags = append(tags, nostr.Tag{"alt", "reply"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindProfileMetadata,
			Tags:      tags,
			Content:   `plain text`,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 0 (ProfileMetadata) (NIP-24): invalid event: unsupported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"a", "30023:a695f6b60119d9521934a691347d9f78e8770b56da16bb255ee286ddf9fda919:ipsum", "wss://relay.nostr.org"})
		tags = append(tags, nostr.Tag{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"})
		tags = append(tags, nostr.Tag{"p", "pubkey1"})
		tags = append(tags, nostr.Tag{"alt", "reply"})
		tags = append(tags, nostr.Tag{"unsupported", "value"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindProfileMetadata,
			Tags:      tags,
			Content:   `{"name":"qwerty","about":"me is bot","picture":"https://example.com/pic.jpg"}`,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})

	helperMustCloseRelay(t, relay)
	require.Equal(t, []*model.Event{validEventNIP01, validEventNIP24}, storedEvents)
}

func TestPublishingNIP24ReactionEvents(t *testing.T) {
	privkey := model.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	helperRegisterWSEventListenerProxyWithStorage(t, &storedEvents)
	ctx := context.Background()
	relay := helperMustNewRelay(t, pubsubServers[0])

	var validUpvoteEvent, validReactionToWebsiteEvent, validUpvoteEmptyContentEvent, validDownvoteEvent *model.Event
	t.Run("kind 7 (Reactions) (NIP-25): valid upvote event", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"})
		tags = append(tags, nostr.Tag{"p", "pubkey1"})
		tags = append(tags, nostr.Tag{"a", "1:b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87:dummyDTag"})
		tags = append(tags, nostr.Tag{"k", "1"})
		validUpvoteEvent = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindReaction,
			Tags:      tags,
			Content:   "+",
		}}
		helperSignWithMinLeadingZeroBits(t, validUpvoteEvent, privkey)
		require.NoError(t, relay.Publish(ctx, validUpvoteEvent.Event))
	})
	t.Run("kind 7 (Reactions) (NIP-25): valid upvote with empty content event", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"})
		tags = append(tags, nostr.Tag{"p", "pubkey1"})
		tags = append(tags, nostr.Tag{"a", "1:b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87:dummyDTag"})
		tags = append(tags, nostr.Tag{"k", "1"})
		validUpvoteEmptyContentEvent = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindReaction,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, validUpvoteEmptyContentEvent, privkey)
		require.NoError(t, relay.Publish(ctx, validUpvoteEmptyContentEvent.Event))
	})
	t.Run("kind 7 (Reactions) (NIP-25): valid downvote event", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"})
		tags = append(tags, nostr.Tag{"p", "pubkey1"})
		tags = append(tags, nostr.Tag{"a", "1:b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87:dummyDTag"})
		tags = append(tags, nostr.Tag{"k", "1"})
		validDownvoteEvent = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindReaction,
			Tags:      tags,
			Content:   "-",
		}}
		helperSignWithMinLeadingZeroBits(t, validDownvoteEvent, privkey)
		require.NoError(t, relay.Publish(ctx, validDownvoteEvent.Event))
	})
	t.Run("kind 7 (Reactions) (NIP-25): valid upvote reaction to website event", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"r", "https://example.com/"})
		validReactionToWebsiteEvent = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindReactionToWebsite,
			Tags:      tags,
			Content:   "+",
		}}
		helperSignWithMinLeadingZeroBits(t, validReactionToWebsiteEvent, privkey)
		require.NoError(t, relay.Publish(ctx, validReactionToWebsiteEvent.Event))
	})
	t.Run("kind 7 (Reactions) (NIP-25): invalid event, wrong e tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"p", "pubkey1"})
		tags = append(tags, nostr.Tag{"a", "1:b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87:dummyDTag"})
		tags = append(tags, nostr.Tag{"k", "1"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindReaction,
			Tags:      tags,
			Content:   "+",
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 7 (Reactions) (NIP-25): invalid event, wrong p tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"})
		tags = append(tags, nostr.Tag{"a", "1:b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87:dummyDTag"})
		tags = append(tags, nostr.Tag{"k", "1"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindReaction,
			Tags:      tags,
			Content:   "+",
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 7 (Reactions) (NIP-25): invalid event, wrong a tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"})
		tags = append(tags, nostr.Tag{"p", "pubkey1"})
		tags = append(tags, nostr.Tag{"a", "1:b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87"})
		tags = append(tags, nostr.Tag{"k", "1"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindReaction,
			Tags:      tags,
			Content:   "+",
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})

	helperMustCloseRelay(t, relay)
	require.Equal(t, []*model.Event{validUpvoteEvent, validUpvoteEmptyContentEvent, validDownvoteEvent, validReactionToWebsiteEvent}, storedEvents)
}

func TestPublishingNIP32LabelingEvents(t *testing.T) {
	privkey := model.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	helperRegisterWSEventListenerProxyWithStorage(t, &storedEvents)
	ctx := context.Background()
	relay := helperMustNewRelay(t, pubsubServers[0])

	var validLabelingEvent, validUGCLabelingEvent *model.Event
	t.Run("kind 1985 (Labeling) (NIP-32): valid labeling event", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"})
		tags = append(tags, nostr.Tag{"L", "#t"})
		tags = append(tags, nostr.Tag{"l", "permies", "#t"})
		validLabelingEvent = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindLabel,
			Tags:      tags,
			Content:   "Some label long description",
		}}
		helperSignWithMinLeadingZeroBits(t, invalidLabelingEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidLabelingEvent.Event))
	})
	t.Run("kind 1 (NIP-32): invalid label", func(t *testing.T) {
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindTextNote,
			Tags:      nostr.Tags{[]string{"e", "", "relay", "reply"}, []string{"p", "pubkey1", "pubkey2"}, []string{"l", "permies", "#t"}, []string{"L", "#a"}},
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	var validKind01EventWithLabels *model.Event
	t.Run("kind 1 (NIP-32): valid with label", func(t *testing.T) {
		validKind01EventWithLabels = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindTextNote,
			Tags:      nostr.Tags{[]string{"e", "", "relay", "reply"}, []string{"p", "pubkey1", "pubkey2"}, []string{"l", "permies", "#t"}, []string{"L", "#t"}},
		}}
		helperSignWithMinLeadingZeroBits(t, validKind01EventWithLabels, privkey)
		require.NoError(t, relay.Publish(ctx, validKind01EventWithLabels.Event))
	})

	helperMustCloseRelay(t, relay)
	require.Equal(t, []*model.Event{validLabelingEvent, validUGCLabelingEvent, validKind01EventWithLabels}, storedEvents)
}

func TestPublishingNIP56(t *testing.T) {
	privkey := model.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	helperRegisterWSEventListenerProxyWithStorage(t, &storedEvents)
	ctx := context.Background()
	relay := helperMustNewRelay(t, pubsubServers[0])

	var validReportEventWithPTagOnly *model.Event
	t.Run("kind 1984 (Report) (NIP-56): valid report event with p tag only", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"p", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", model.TagReportTypeNudity})
		validReportEventWithPTagOnly = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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

func TestPublishingNIP58Badges(t *testing.T) {
	privkey := model.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	helperRegisterWSEventListenerProxyWithStorage(t, &storedEvents)
	ctx := context.Background()
	relay := helperMustNewRelay(t, pubsubServers[0])

	var validBadgeDefinitionEvent, validBadgeAwardEvent, validProfileBadgesEvent *model.Event
	t.Run("kind 30009 (Badge defenition) (NIP-56): valid badge definition event", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"d", "bravery"})
		tags = append(tags, nostr.Tag{"name", "Medal of Bravery"})
		tags = append(tags, nostr.Tag{"description", "Awarded to users demonstrating bravery"})
		tags = append(tags, nostr.Tag{"image", "https://nostr.academy/awards/bravery.png", "1024x1024"})
		tags = append(tags, nostr.Tag{"thumb", "https://nostr.academy/awards/bravery_256x256.png", "256x256"})
		validBadgeDefinitionEvent = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindBadgeDefinition,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, validBadgeDefinitionEvent, privkey)
		require.NoError(t, relay.Publish(ctx, validBadgeDefinitionEvent.Event))
	})
	t.Run("kind 8 (Badge award) (NIP-56): valid badge award event", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"a", "30009:alice:bravery"})
		tags = append(tags, nostr.Tag{"p", "bob", "wss://relay"})
		tags = append(tags, nostr.Tag{"p", "charlie", "wss://relay"})
		validBadgeAwardEvent = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindBadgeAward,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, validBadgeAwardEvent, privkey)
		require.NoError(t, relay.Publish(ctx, validBadgeAwardEvent.Event))
	})
	t.Run("kind 3008 (Profile badges) (NIP-56): valid profile badges event", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"d", model.ProfileBadgesIdentifier})
		tags = append(tags, nostr.Tag{"a", "30009:alice:bravery"})
		tags = append(tags, nostr.Tag{"e", "<bravery badge award event id>", "wss://nostr.academy"})
		tags = append(tags, nostr.Tag{"a", "30009:alice:honor"})
		tags = append(tags, nostr.Tag{"e", "<honor badge award event id>", "wss://nostr.academy"})
		validProfileBadgesEvent = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindProfileBadges,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, validProfileBadgesEvent, privkey)
		require.NoError(t, relay.Publish(ctx, validProfileBadgesEvent.Event))
	})

	t.Run("kind 30009 (Badge defenition) (NIP-56): invalid, no d tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"name", "Medal of Bravery"})
		tags = append(tags, nostr.Tag{"description", "Awarded to users demonstrating bravery"})
		tags = append(tags, nostr.Tag{"image", "https://nostr.academy/awards/bravery.png", "1024x1024"})
		tags = append(tags, nostr.Tag{"thumb", "https://nostr.academy/awards/bravery_256x256.png", "256x256"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindBadgeDefinition,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 30009 (Badge defenition) (NIP-56): not supported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"profile", "bogus"})
		tags = append(tags, nostr.Tag{"name", "Medal of Bravery"})
		tags = append(tags, nostr.Tag{"description", "Awarded to users demonstrating bravery"})
		tags = append(tags, nostr.Tag{"image", "https://nostr.academy/awards/bravery.png", "1024x1024"})
		tags = append(tags, nostr.Tag{"thumb", "https://nostr.academy/awards/bravery_256x256.png", "256x256"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindBadgeDefinition,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 8 (Badge award) (NIP-56): invalid, no a tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"p", "bob", "wss://relay"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindBadgeAward,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 8 (Badge award) (NIP-56): invalid, a tag refers to wrong kind", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"a", "1:alice:bravery"})
		tags = append(tags, nostr.Tag{"p", "bob", "wss://relay"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindBadgeAward,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 8 (Badge award) (NIP-56): invalid, no at least one p tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"a", "3009:alice:bravery"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindBadgeAward,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 3008 (Profile badges) (NIP-56): invalid d tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"d", "bogus"})
		tags = append(tags, nostr.Tag{"a", "30009:alice:bravery"})
		tags = append(tags, nostr.Tag{"e", "<bravery badge award event id>", "wss://nostr.academy"})
		tags = append(tags, nostr.Tag{"a", "30009:alice:honor"})
		tags = append(tags, nostr.Tag{"e", "<honor badge award event id>", "wss://nostr.academy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindProfileBadges,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 3008 (Profile badges) (NIP-56): invalid a tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"d", model.ProfileBadgesIdentifier})
		tags = append(tags, nostr.Tag{"a", "1:alice:bravery"})
		tags = append(tags, nostr.Tag{"e", "<bravery badge award event id>", "wss://nostr.academy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindProfileBadges,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 3008 (Profile badges) (NIP-56): e/a tags mismatch", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"d", model.ProfileBadgesIdentifier})
		tags = append(tags, nostr.Tag{"a", "3009:alice:bravery"})
		tags = append(tags, nostr.Tag{"e", "<bravery badge award event id>", "wss://nostr.academy"})
		tags = append(tags, nostr.Tag{"a", "3009:alice:honor"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindProfileBadges,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})

	helperMustCloseRelay(t, relay)
	require.Equal(t, []*model.Event{validBadgeDefinitionEvent, validBadgeAwardEvent, validProfileBadgesEvent}, storedEvents)
}

func TestPublishingNIP65RelayListMetadataEvents(t *testing.T) {
	privkey := model.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	helperRegisterWSEventListenerProxyWithStorage(t, &storedEvents)
	ctx := context.Background()
	relay := helperMustNewRelay(t, pubsubServers[0])

	var validRelayListEvent *model.Event
	t.Run("kind 10002 (Relay list) (NIP-65): valid relay list", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"r", "wss://alicerelay.example.com"})
		tags = append(tags, nostr.Tag{"r", "wss://brando-relay.com"})
		tags = append(tags, nostr.Tag{"r", "wss://expensive-relay.example2.com", "write"})
		tags = append(tags, nostr.Tag{"r", "wss://nostr-relay.example.com", "read"})
		validRelayListEvent = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindRelayListMetadata,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, validRelayListEvent, privkey)
		require.NoError(t, relay.Publish(ctx, validRelayListEvent.Event))
	})
	t.Run("kind 10002 (Relay list) (NIP-65): unsupported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"r", "wss://alicerelay.example.com"})
		tags = append(tags, nostr.Tag{"r", "wss://brando-relay.com"})
		tags = append(tags, nostr.Tag{"r", "wss://expensive-relay.example2.com", "write"})
		tags = append(tags, nostr.Tag{"r", "wss://nostr-relay.example.com", "read"})
		tags = append(tags, nostr.Tag{"e", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindRelayListMetadata,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 10002 (Relay list) (NIP-65): unsupported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"r", "wss://alicerelay.example.com"})
		tags = append(tags, nostr.Tag{"r", "wss://brando-relay.com"})
		tags = append(tags, nostr.Tag{"r", "wss://expensive-relay.example2.com", "write"})
		tags = append(tags, nostr.Tag{"r", "wss://nostr-relay.example.com", "read"})
		tags = append(tags, nostr.Tag{"e", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindRelayListMetadata,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 10002 (Relay list) (NIP-65): wrong marker", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"r", "wss://alicerelay.example.com"})
		tags = append(tags, nostr.Tag{"r", "wss://brando-relay.com"})
		tags = append(tags, nostr.Tag{"r", "wss://expensive-relay.example2.com", "write"})
		tags = append(tags, nostr.Tag{"r", "wss://nostr-relay.example.com", "read"})
		tags = append(tags, nostr.Tag{"r", "wss://nostr-relay.example.com", "wrong"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindRelayListMetadata,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	helperMustCloseRelay(t, relay)
	require.Equal(t, []*model.Event{validRelayListEvent}, storedEvents)
}

func TestPublishingNIP51ListsSetsEvents(t *testing.T) {
	privkey := model.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	helperRegisterWSEventListenerProxyWithStorage(t, &storedEvents)
	ctx := context.Background()
	relay := helperMustNewRelay(t, pubsubServers[0])

	var validEvents []*model.Event
	t.Run("Kind 10000 (NIP-51) valid", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"p", "pubkey"})
		tags = append(tags, nostr.Tag{"t", "hash"})
		tags = append(tags, nostr.Tag{"e", "event"})
		tags = append(tags, nostr.Tag{"word", "dummy"})
		validEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindBookmarkList,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("Kind 10004 (NIP-51) valid", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:dummy:dummy", nostr.KindCommunityDefinition)})
		validEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindCommunityList,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, validEvent, privkey)
		validEvents = append(validEvents, validEvent)
		require.NoError(t, relay.Publish(ctx, validEvent.Event))
	})
	t.Run("Kind 10003 (NIP-51): unsupported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:dummy:dummy", nostr.KindCommunityDefinition)})
		tags = append(tags, nostr.Tag{"wrong", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindReleaseArtifactSets,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})

	helperMustCloseRelay(t, relay)
	require.Equal(t, validEvents, storedEvents)
}

func TestCountEvents(t *testing.T) {
	privkey := model.GeneratePrivateKey()
	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		t.Logf("received events: %v", events)
		return query.AcceptEvents(ctx, events...)
	})

	ctx := context.Background()
	relay := helperMustNewRelay(t, pubsubServers[0])

	t.Run("SaveEvent", func(t *testing.T) {
		pk, err := model.GetPublicKey(privkey)
		require.NoError(t, err)

		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindTextNote,
			PubKey:    pk,
			Tags:      nil,
			Content:   "validEvent",
		}}

		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("CountEvents", func(t *testing.T) {
		c, err := relay.Count(ctx, nostr.Filters{{Kinds: []int{nostr.KindTextNote}, Search: "test"}})
		require.NoError(t, err)
		require.Equal(t, int64(1), c)
	})
	helperMustCloseRelay(t, relay)
}

func helperSignWithMinLeadingZeroBits(t *testing.T, event *model.Event, privkey string) {
	t.Helper()
	require.NoError(t, event.SignWithAlg(privkey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, event.GenerateNIP13(context.Background(), NIP13MinLeadingZeroBits))
	require.NoError(t, event.SignWithAlg(privkey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
}

func TestPublishingNIP92IMetaTag(t *testing.T) {
	privkey := model.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	helperRegisterWSEventListenerProxyWithStorage(t, &storedEvents)
	ctx := context.Background()
	relay := helperMustNewRelay(t, pubsubServers[0])

	var validEvents []*model.Event
	t.Run("kind 1 (text note), imeta (NIP-92): valid imeta tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{
			"imeta",
			"url https://alicerelay.example.com",
			"m image/jpg",
			"dim 3024x4032",
			"i foobar",
			"alt A scenic photo overlooking the coast of Costa Rica",
			fmt.Sprintf("x %x", []byte("https://alicerelay.example.com")),
			fmt.Sprintf("ox %x", []byte("https://alicerelay.example.com")),
		})
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindTextNote,
			Tags:      tags,
			Content:   "dummy",
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relay.Publish(ctx, ev.Event))
		validEvents = append(validEvents, ev)
	})
	t.Run("kind 1 (text note), imeta (NIP-92): invalid imeta key", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{
			"imeta",
			"url https://alicerelay.example.com",
			"m image/jpg",
			"i foobar",
			"dim 3024x4032",
			"alt A scenic photo overlooking the coast of Costa Rica",
			fmt.Sprintf("x %x", []byte("https://alicerelay.example.com")),
			fmt.Sprintf("ox %x", []byte("https://alicerelay.example.com")),
			"dummy dummy",
		})
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindTextNote,
			Tags:      tags,
			Content:   "dummy",
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("kind 1 (text note), imeta (NIP-92): invalid imeta tag: not enough tag values", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{
			"imeta",
		})
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindTextNote,
			Tags:      tags,
			Content:   "dummy",
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("kind 1 (text note), imeta (NIP-92): invalid imeta tag: no spaces", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{
			"imeta",
			"url",
		})
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindTextNote,
			Tags:      tags,
			Content:   "dummy",
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("kind 1 (text note), imeta (NIP-92): invalid imeta tag: wrong m tag value", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{
			"imeta",
			"url https://alicerelay.example.com",
			"m image/jpg",
			"i foobar",
			"dim 3024",
			"alt A scenic photo overlooking the coast of Costa Rica",
			fmt.Sprintf("x %x", []byte("https://alicerelay.example.com")),
			fmt.Sprintf("ox %x", []byte("https://alicerelay.example.com")),
		})
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindTextNote,
			Tags:      tags,
			Content:   "dummy",
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("kind 1 (text note), imeta (NIP-92): invalid imeta tag: x not a hash", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{
			"imeta",
			"url https://alicerelay.example.com",
			"m image/jpg",
			"dim 3024x4032",
			"i foobar",
			"alt A scenic photo overlooking the coast of Costa Rica",
			"x a",
			fmt.Sprintf("ox %v", hex.EncodeToString([]byte("https://alicerelay.example.com"))),
		})
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindTextNote,
			Tags:      tags,
			Content:   "dummy",
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("kind 1 (text note), imeta (NIP-92): invalid imeta tag: ox not a hash", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{
			"imeta",
			"url https://alicerelay.example.com",
			"m image/jpg",
			"dim 3024x4032",
			"i foobar",
			"alt A scenic photo overlooking the coast of Costa Rica",
			"ox a",
		})
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindTextNote,
			Tags:      tags,
			Content:   "dummy",
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.Error(t, relay.Publish(ctx, ev.Event))
	})

	helperMustCloseRelay(t, relay)
	require.Equal(t, validEvents, storedEvents)
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

	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		t.Logf("received events: %v", events)
		err := query.AcceptEvents(ctx, events...)
		require.NoError(t, err)
		return err
	})

	RegisterWSSubscriptionListener(func(ctx context.Context, subscription *model.Subscription) query.EventIterator {
		t.Logf("received subscription: %v", subscription)
		return query.GetStoredEvents(ctx, subscription)
	})

	relay := helperMustNewRelay(t, pubsubServers[0])

	t.Run("Publish", func(t *testing.T) {
		t.Logf("publishing %v event(s)", len(generatedEvents))
		err := relay.PublishMany(context.Background(), generatedEvents...)
		require.NoError(t, err)
	})

	sub, err := relay.Subscribe(context.Background(), []model.Filter{
		{
			Kinds: []int{nostr.KindTextNote},
			Tags: model.TagMap{}.
				Set("e", model.PointerOf("bar"), nil, model.PointerOf("reply")),
		},
	})
	require.NoError(t, err)

	var wg sync.WaitGroup
	var receivedEvents []*model.Event
	{
		t.Logf("subscribed to %v", sub.GetID())
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ev := range sub.Events {
				t.Logf("received event %v via sub", ev)
				receivedEvents = append(receivedEvents, &model.Event{Event: *ev})
			}
		}()
	}

	select {
	case <-sub.EndOfStoredEvents:
		t.Logf("received EOS")

	case <-time.After(5 * time.Second):
		t.Fatalf("timeout waiting for EOS")
	}

	sub.Close()
	require.Empty(t, <-sub.ClosedReason)

	// Want only one event that matches the filter.
	require.Len(t, receivedEvents, 1)
	require.Equal(t, generatedEvents[0], &receivedEvents[0].Event)

	helperMustCloseRelay(t, relay)
	wg.Wait()
}
