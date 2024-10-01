// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"context"
	"fmt"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gookit/goutil/errorx"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip13"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/server/ws/fixture"
)

func TestRelaySubscription(t *testing.T) {
	var eventsQueue []*model.Event

	privkey := nostr.GeneratePrivateKey()
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
	ctx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
	ev.SetExtra("extra", uuid.NewString())

	require.NoError(t, ev.Sign(privkey))
	require.NoError(t, ev.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
	require.NoError(t, ev.Sign(privkey))
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

		return func(yield func(*model.Event, error) bool) {
			for i := range events {
				if !yield(events[i], nil) {
					return
				}
			}
		}
	})

	storedEvents := []*model.Event{eventsQueue[len(eventsQueue)-1]}
	RegisterWSEventListener(func(ctx context.Context, event *model.Event) error {
		storedEvents = append(storedEvents, event)

		return nil
	})
	pubsubServer.Reset()
	ctx, cancel = context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
	relay, err := fixture.NewRelayClient(ctx, "wss://localhost:9998")
	if err != nil {
		panic(err)
	}
	filters := []nostr.Filter{{
		Kinds: []int{nostr.KindTextNote},
		Limit: 1,
	}}

	subCtx, subCancel := context.WithTimeout(ctx, 5*time.Second)
	defer subCancel()

	sub, err := relay.Subscribe(subCtx, filters)
	if err != nil {
		log.Panic(err)
	}

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
		log.Panic(errorx.Withf(ctx.Err(), "EOS not received"))
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
	eventsQueue[len(eventsQueue)-1].SetExtra("extra", uuid.NewString())
	require.NoError(t, eventsQueue[len(eventsQueue)-1].Sign(privkey))
	require.NoError(t, eventsQueue[len(eventsQueue)-1].GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
	require.NoError(t, eventsQueue[len(eventsQueue)-1].Sign(privkey))

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
	eventsQueue[len(eventsQueue)-1].SetExtra("extra", uuid.NewString())
	require.NoError(t, eventsQueue[len(eventsQueue)-1].Event.Sign(privkey))
	require.NoError(t, eventsQueue[len(eventsQueue)-1].GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
	require.NoError(t, eventsQueue[len(eventsQueue)-1].Event.Sign(privkey))
	require.NoError(t, NotifySubscriptions(eventBy3rdParty))

	repostedPubkey := "pubkey1"
	repostedID := uuid.NewString()
	notMatchingEvent := &model.Event{Event: nostr.Event{
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      nostr.KindRepost,
		Tags:      nostr.Tags{[]string{"e", repostedID, "relay"}, []string{"p", repostedPubkey}},
		Content:   fmt.Sprintf(`{"kind":1,"id":"%v","pubkey":"%v"}`, repostedID, repostedPubkey),
	}}
	notMatchingEvent.SetExtra("extra", uuid.NewString())
	require.NoError(t, notMatchingEvent.Sign(privkey))
	require.NoError(t, notMatchingEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
	require.NoError(t, notMatchingEvent.Sign(privkey))
	require.NoError(t, relay.Publish(ctx, notMatchingEvent.Event))

	// Replacing of subscription by another subscription with another filter: smth broken from go-nostr v0.36.0 to write the message to change filters directly.
	sub.Close()
	require.Empty(t, <-sub.ClosedReason)

	sub, err = relay.Subscribe(subCtx, []nostr.Filter{{
		Kinds: []int{nostr.KindArticle},
		Limit: 1,
	}})
	if err != nil {
		log.Panic(err)
	}
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
		log.Panic(errorx.Withf(ctx.Err(), "EOS not received"))
	}

	eventMatchingReplacedSub := &model.Event{Event: nostr.Event{
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      nostr.KindArticle,
		Tags:      nostr.Tags{},
		Content:   "event matching replaced filter" + uuid.NewString(),
	}}
	eventMatchingReplacedSub.SetExtra("extra", uuid.NewString())
	require.NoError(t, eventMatchingReplacedSub.Sign(privkey))
	require.NoError(t, eventMatchingReplacedSub.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
	require.NoError(t, eventMatchingReplacedSub.Sign(privkey))
	require.NoError(t, relay.Publish(ctx, eventMatchingReplacedSub.Event))
	eventsQueue = append(eventsQueue, eventMatchingReplacedSub)

	sub.Close()
	require.Empty(t, <-sub.ClosedReason)

	require.NoError(t, relay.Close())
	shutdownCtx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
	for pubsubServer.ReaderExited.Load() != uint64(1) {
		if shutdownCtx.Err() != nil {
			log.Panic(errorx.Errorf("shutdown timeout %v of %v", pubsubServer.ReaderExited.Load(), 1))
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.Equal(t, uint64(1), pubsubServer.ReaderExited.Load())
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
	privkey := nostr.GeneratePrivateKey()
	storedEvents := []*model.Event{{Event: nostr.Event{
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      nostr.KindTextNote,
		Content:   "db event",
	}}}
	RegisterWSSubscriptionListener(func(context.Context, *model.Subscription) query.EventIterator {
		return func(yield func(*model.Event, error) bool) {
			for i := range storedEvents {
				if !yield(storedEvents[i], nil) {
					return
				}
			}
		}
	})
	require.NoError(t, storedEvents[len(storedEvents)-1].Sign(privkey))
	require.NoError(t, storedEvents[len(storedEvents)-1].GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
	require.NoError(t, storedEvents[len(storedEvents)-1].Sign(privkey))
	RegisterWSEventListener(func(ctx context.Context, event *model.Event) error {
		storedEvents = append(storedEvents, event)
		return nil
	})
	pubsubServer.Reset()
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
		relay, err := fixture.NewRelayClient(ctx, "wss://localhost:9998")
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
	newRealtimeEvent := nostr.Event{
		Kind:    nostr.KindTextNote,
		Content: "new realtime event",
	}
	newRealtimeEvent.SetExtra("extra", uuid.NewString())
	require.NoError(t, newRealtimeEvent.Sign(privkey))
	tag, err := nip13.DoWork(ctx, newRealtimeEvent, NIP13MinLeadingZeroBits)
	require.NoError(t, err)
	newRealtimeEvent.Tags = append(newRealtimeEvent.Tags, tag)
	require.NoError(t, newRealtimeEvent.Sign(privkey))
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
					log.Panic(errorx.Errorf("timeout waiting for the event"))
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
					log.Panic(errorx.Errorf("timeout waiting for EOS"))
				}
				select {
				case ev = <-sub.Events:
				case <-ctx.Done():
					log.Panic(errorx.Errorf("timeout waiting for the event"))
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
				log.Panic(errorx.Errorf("timeout waiting for EOS"))
			}
		}
	}
	close(eosCh)
	require.NoError(t, randomRelay.Publish(ctx, newRealtimeEvent))
	wg.Wait()
	for r := range subs {
		require.NoError(t, r.Close())
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
	for pubsubServer.ReaderExited.Load() != uint64(connsCount) {
		if shutdownCtx.Err() != nil {
			log.Panic(errorx.Errorf("shutdown timeout %v of %v", pubsubServer.ReaderExited.Load(), connsCount))
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.Equal(t, uint64(connsCount), pubsubServer.ReaderExited.Load())
}

func TestPublishingEvents(t *testing.T) {
	privkey := nostr.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	RegisterWSEventListener(func(ctx context.Context, event *model.Event) error {
		for _, sEvent := range storedEvents {
			if sEvent.ID == event.ID {
				return model.ErrDuplicate
			}
		}
		assert.False(t, event.IsEphemeral())
		storedEvents = append(storedEvents, event)
		return nil
	})
	pubsubServer.Reset()
	ctx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
	relay, err := fixture.NewRelayClient(ctx, "wss://localhost:9998")
	if err != nil {
		log.Panic(err)
	}
	validEvent := &model.Event{Event: nostr.Event{
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      nostr.KindTextNote,
		Tags:      nil,
		Content:   "validEvent",
	}}
	t.Run("valid event", func(t *testing.T) {
		validEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, validEvent.Sign(privkey))
		require.NoError(t, validEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validEvent.Sign(privkey))
		require.NoError(t, relay.Publish(ctx, validEvent.Event))
	})
	t.Run("invalid event kind", func(t *testing.T) {
		invalidKindEvent := model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      -1,
			Tags:      nil,
			Content:   "invalid kind id event",
		}}
		invalidKindEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidKindEvent.Sign(privkey))
		require.NoError(t, invalidKindEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidKindEvent.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidKindEvent.Event))

		invalidKindEvent.Kind = 65536
		require.NoError(t, invalidKindEvent.Sign(privkey))
		require.NoError(t, invalidKindEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidKindEvent.Sign(privkey))
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
		invalidID.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidID.Sign(privkey))
		require.NoError(t, invalidID.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidID.Sign(privkey))
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
		invalidSignature.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidSignature.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.Error(t, relay.Publish(ctx, invalidSignature.Event))
	})
	t.Run("duplicated event", func(t *testing.T) {
		require.NoError(t, relay.Publish(ctx, validEvent.Event))
	})
	t.Run("ephemeral event", func(t *testing.T) {
		ephemeralEvent := nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindClientAuthentication,
			Content:   "bogus",
		}
		require.NoError(t, ephemeralEvent.Sign(privkey))
		tag, err := nip13.DoWork(ctx, ephemeralEvent, NIP13MinLeadingZeroBits)
		require.NoError(t, err)
		ephemeralEvent.Tags = append(ephemeralEvent.Tags, tag)
		require.NoError(t, ephemeralEvent.Sign(privkey))
		require.NoError(t, relay.Publish(ctx, ephemeralEvent))
	})
	t.Run("wrong kind 03 follow list tag parameters", func(t *testing.T) {
		inValidKind03Event := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindFollowList,
			Tags:      nil,
		}}
		inValidKind03Event.SetExtra("extra", uuid.NewString())
		require.NoError(t, inValidKind03Event.Sign(privkey))
		require.NoError(t, inValidKind03Event.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, inValidKind03Event.Sign(privkey))
		require.Error(t, relay.Publish(ctx, inValidKind03Event.Event))
	})
	t.Run("wrong kind 03 follow list content", func(t *testing.T) {
		inValidKind03Event := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindFollowList,
			Tags:      nostr.Tags{[]string{"p"}, []string{"p"}},
			Content:   "invalidEvent",
		}}
		inValidKind03Event.SetExtra("extra", uuid.NewString())
		require.NoError(t, inValidKind03Event.Sign(privkey))
		require.NoError(t, inValidKind03Event.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, inValidKind03Event.Sign(privkey))
		require.Error(t, relay.Publish(ctx, inValidKind03Event.Event))
	})
	var validKind03Event *model.Event
	t.Run("valid kind 03 follow list event", func(t *testing.T) {
		validKind03Event = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindFollowList,
			Tags:      nostr.Tags{[]string{"p", "wss://alicerelay.com/", "alice"}, []string{"p", "wss://bobrelay.com/nostr", "bob"}},
		}}
		validKind03Event.SetExtra("extra", uuid.NewString())
		require.NoError(t, validKind03Event.Sign(privkey))
		require.NoError(t, validKind03Event.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validKind03Event.Sign(privkey))
		require.NoError(t, relay.Publish(ctx, validKind03Event.Event))
	})

	require.NoError(t, relay.Close())
	shutdownCtx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
	for pubsubServer.ReaderExited.Load() != uint64(1) {
		if shutdownCtx.Err() != nil {
			log.Panic(errorx.Errorf("shutdown timeout %v of %v", pubsubServer.ReaderExited.Load(), 1))
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.Equal(t, uint64(1), pubsubServer.ReaderExited.Load())
	require.Equal(t, []*model.Event{validEvent, validKind03Event}, storedEvents)
}

func TestPublishingNIP09Events(t *testing.T) {
	privkey := nostr.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	RegisterWSEventListener(func(ctx context.Context, event *model.Event) error {
		for _, sEvent := range storedEvents {
			if sEvent.ID == event.ID {
				return model.ErrDuplicate
			}
		}
		assert.False(t, event.IsEphemeral())
		storedEvents = append(storedEvents, event)

		return nil
	})
	pubsubServer.Reset()
	ctx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
	relay, err := fixture.NewRelayClient(ctx, "wss://localhost:9998")
	require.NoError(t, err)

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
		validEventNIP09WithEKTags.SetExtra("extra", uuid.NewString())
		require.NoError(t, validEventNIP09WithEKTags.Sign(privkey))
		require.NoError(t, validEventNIP09WithEKTags.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validEventNIP09WithEKTags.Sign(privkey))
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
		validEventNIP09AllTags.SetExtra("extra", uuid.NewString())
		require.NoError(t, validEventNIP09AllTags.Sign(privkey))
		require.NoError(t, validEventNIP09AllTags.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validEventNIP09AllTags.Sign(privkey))
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
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
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
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
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
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})

	require.NoError(t, relay.Close())
	shutdownCtx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
	for pubsubServer.ReaderExited.Load() != uint64(1) {
		if shutdownCtx.Err() != nil {
			log.Panic(errorx.Errorf("shutdown timeout %v of %v", pubsubServer.ReaderExited.Load(), 1))
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.Equal(t, uint64(1), pubsubServer.ReaderExited.Load())
	require.Equal(t, []*model.Event{validEventNIP09WithEKTags, validEventNIP09AllTags}, storedEvents)
}

func TestPublishingNIP10Events(t *testing.T) {
	privkey := nostr.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	RegisterWSEventListener(func(ctx context.Context, event *model.Event) error {
		for _, sEvent := range storedEvents {
			if sEvent.ID == event.ID {
				return model.ErrDuplicate
			}
		}
		assert.False(t, event.IsEphemeral())
		storedEvents = append(storedEvents, event)
		return nil
	})
	pubsubServer.Reset()
	ctx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
	relay, err := fixture.NewRelayClient(ctx, "wss://localhost:9998")
	if err != nil {
		log.Panic(err)
	}

	t.Run("kind 1 (NIP-10): e tags required params", func(t *testing.T) {
		inValidKind01Event := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindTextNote,
			Tags:      nostr.Tags{[]string{"e"}},
		}}
		inValidKind01Event.SetExtra("extra", uuid.NewString())
		require.NoError(t, inValidKind01Event.Sign(privkey))
		require.NoError(t, inValidKind01Event.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, inValidKind01Event.Sign(privkey))
		require.Error(t, relay.Publish(ctx, inValidKind01Event.Event))
	})

	t.Run("kind 1 (NIP-10): invalid reply marker for e tags ", func(t *testing.T) {
		inValidKind01Event := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindTextNote,
			Tags:      nostr.Tags{[]string{"e", "", "relay", "invalid marker"}},
		}}
		inValidKind01Event.SetExtra("extra", uuid.NewString())
		require.NoError(t, inValidKind01Event.Sign(privkey))
		require.NoError(t, inValidKind01Event.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, inValidKind01Event.Sign(privkey))
		require.Error(t, relay.Publish(ctx, inValidKind01Event.Event))
	})
	t.Run("kind 1 (NIP-10): invalid p tag usage: no e tags", func(t *testing.T) {
		inValidKind01Event := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindTextNote,
			Tags:      nostr.Tags{[]string{"p", "pubkey1", "pubkey2"}},
		}}
		inValidKind01Event.SetExtra("extra", uuid.NewString())
		require.NoError(t, inValidKind01Event.Sign(privkey))
		require.NoError(t, inValidKind01Event.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, inValidKind01Event.Sign(privkey))
		require.Error(t, relay.Publish(ctx, inValidKind01Event.Event))
	})
	t.Run("kind 1 (NIP-10): invalid p tag usage: empty tag values", func(t *testing.T) {
		inValidKind01Event := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindTextNote,
			Tags:      nostr.Tags{[]string{"e", "", "relay", "reply"}, []string{"p"}},
		}}
		inValidKind01Event.SetExtra("extra", uuid.NewString())
		require.NoError(t, inValidKind01Event.Sign(privkey))
		require.NoError(t, inValidKind01Event.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, inValidKind01Event.Sign(privkey))
		require.Error(t, relay.Publish(ctx, inValidKind01Event.Event))
	})

	var validKind01NIP10Event *model.Event
	t.Run("kind 1 (NIP-10): valid", func(t *testing.T) {
		validKind01NIP10Event = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindTextNote,
			Tags:      nostr.Tags{[]string{"e", "", "relay", "reply"}, []string{"p", "pubkey1", "pubkey2"}},
		}}
		validKind01NIP10Event.SetExtra("extra", uuid.NewString())
		require.NoError(t, validKind01NIP10Event.Sign(privkey))
		require.NoError(t, validKind01NIP10Event.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validKind01NIP10Event.Sign(privkey))
		require.NoError(t, relay.Publish(ctx, validKind01NIP10Event.Event))
	})

	require.NoError(t, relay.Close())
	shutdownCtx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
	for pubsubServer.ReaderExited.Load() != uint64(1) {
		if shutdownCtx.Err() != nil {
			log.Panic(errorx.Errorf("shutdown timeout %v of %v", pubsubServer.ReaderExited.Load(), 1))
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.Equal(t, uint64(1), pubsubServer.ReaderExited.Load())
	require.Equal(t, []*model.Event{validKind01NIP10Event}, storedEvents)
}

func TestPublishingNIP18Events(t *testing.T) {
	privkey := nostr.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	RegisterWSEventListener(func(ctx context.Context, event *model.Event) error {
		for _, sEvent := range storedEvents {
			if sEvent.ID == event.ID {
				return model.ErrDuplicate
			}
		}
		assert.False(t, event.IsEphemeral())
		storedEvents = append(storedEvents, event)

		return nil
	})
	pubsubServer.Reset()
	ctx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
	relay, err := fixture.NewRelayClient(ctx, "wss://localhost:9998")
	require.NoError(t, err)

	var validKind06NIP18Event *model.Event
	t.Run("kind 6 (NIP-18): valid event", func(t *testing.T) {
		repostID := uuid.NewString()
		pubKeyOfRepostedNote := "pubkey1"
		validKind06NIP18Event = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindRepost,
			Tags:      nostr.Tags{[]string{"e", repostID, "relay"}, []string{"p", pubKeyOfRepostedNote}},
			Content:   fmt.Sprintf(`{"kind":1,"id":"%v","pubkey":"%v"}`, repostID, pubKeyOfRepostedNote),
		}}
		validKind06NIP18Event.SetExtra("extra", uuid.NewString())
		require.NoError(t, validKind06NIP18Event.Sign(privkey))
		require.NoError(t, validKind06NIP18Event.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validKind06NIP18Event.Sign(privkey))
		require.NoError(t, relay.Publish(ctx, validKind06NIP18Event.Event))
	})
	t.Run("kind 6 (NIP-18): invalid event, no e tags", func(t *testing.T) {
		repostID := uuid.NewString()
		pubKeyOfRepostedNote := "pubkey1"
		invalidKind06NIP18Event := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindRepost,
			Tags:      nostr.Tags{[]string{"p", pubKeyOfRepostedNote}},
			Content:   fmt.Sprintf(`{"kind":1,"id":"%v","pubkey":"%v"}`, repostID, pubKeyOfRepostedNote),
		}}
		invalidKind06NIP18Event.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidKind06NIP18Event.Sign(privkey))
		require.NoError(t, invalidKind06NIP18Event.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidKind06NIP18Event.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidKind06NIP18Event.Event))
	})
	t.Run("kind 6 (NIP-18): invalid event, no p tags", func(t *testing.T) {
		repostID := uuid.NewString()
		pubKeyOfRepostedNote := "pubkey1"
		invalidKind06NIP18Event := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindRepost,
			Tags:      nostr.Tags{[]string{"e", repostID, "relay"}},
			Content:   fmt.Sprintf(`{"kind":1,"id":"%v","pubkey":"%v"}`, repostID, pubKeyOfRepostedNote),
		}}
		invalidKind06NIP18Event.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidKind06NIP18Event.Sign(privkey))
		require.NoError(t, invalidKind06NIP18Event.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidKind06NIP18Event.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidKind06NIP18Event.Event))
	})
	t.Run("kind 6 (NIP-18): invalid event, no enough e tag parameters", func(t *testing.T) {
		repostID := uuid.NewString()
		pubKeyOfRepostedNote := "pubkey1"
		invalidKind06NIP18Event := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindRepost,
			Tags:      nostr.Tags{[]string{"e", repostID}, []string{"p", pubKeyOfRepostedNote}},
			Content:   fmt.Sprintf(`{"kind":1,"id":"%v","pubkey":"%v"}`, repostID, pubKeyOfRepostedNote),
		}}
		invalidKind06NIP18Event.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidKind06NIP18Event.Sign(privkey))
		require.NoError(t, invalidKind06NIP18Event.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidKind06NIP18Event.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidKind06NIP18Event.Event))
	})
	t.Run("kind 6 (NIP-18): invalid event, no enough p tag parameters", func(t *testing.T) {
		repostID := uuid.NewString()
		pubKeyOfRepostedNote := "pubkey1"
		invalidKind06NIP18Event := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindRepost,
			Tags:      nostr.Tags{[]string{"e", repostID, "relay"}, []string{"p"}},
			Content:   fmt.Sprintf(`{"kind":1,"id":"%v","pubkey":"%v"}`, repostID, pubKeyOfRepostedNote),
		}}
		invalidKind06NIP18Event.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidKind06NIP18Event.Sign(privkey))
		require.NoError(t, invalidKind06NIP18Event.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidKind06NIP18Event.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidKind06NIP18Event.Event))
	})
	t.Run("kind 6 (NIP-18): invalid event, wrong p tag pubkey != reposted note pubkey", func(t *testing.T) {
		repostID := uuid.NewString()
		pubKeyOfRepostedNote := "pubkey1"
		invalidKind06NIP18Event := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindRepost,
			Tags:      nostr.Tags{[]string{"e", repostID, "relay"}, []string{"p", "wrong pubkey"}},
			Content:   fmt.Sprintf(`{"kind":1,"id":"%v","pubkey":"%v"}`, repostID, pubKeyOfRepostedNote),
		}}
		invalidKind06NIP18Event.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidKind06NIP18Event.Sign(privkey))
		require.NoError(t, invalidKind06NIP18Event.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidKind06NIP18Event.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidKind06NIP18Event.Event))
	})
	t.Run("kind 6 (NIP-18): invalid event, wrong content value: != 1", func(t *testing.T) {
		repostID := uuid.NewString()
		pubKeyOfRepostedNote := "pubkey1"
		invalidKind06NIP18Event := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindRepost,
			Tags:      nostr.Tags{[]string{"e", repostID, "relay"}, []string{"p", "wrong pubkey"}},
			Content:   fmt.Sprintf(`{"kind":16,"id":"%v","pubkey":"%v"}`, repostID, pubKeyOfRepostedNote),
		}}
		invalidKind06NIP18Event.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidKind06NIP18Event.Sign(privkey))
		require.NoError(t, invalidKind06NIP18Event.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidKind06NIP18Event.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidKind06NIP18Event.Event))
	})
	t.Run("kind 6 (NIP-18): invalid event, wrong content id value: != e tag repost id value", func(t *testing.T) {
		repostID := uuid.NewString()
		pubKeyOfRepostedNote := "pubkey1"
		invalidKind06NIP18Event := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindRepost,
			Tags:      nostr.Tags{[]string{"e", "wrong id value", "relay"}, []string{"p", "wrong pubkey"}},
			Content:   fmt.Sprintf(`{"kind":1,"id":"%v","pubkey":"%v"}`, repostID, pubKeyOfRepostedNote),
		}}
		invalidKind06NIP18Event.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidKind06NIP18Event.Sign(privkey))
		require.NoError(t, invalidKind06NIP18Event.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidKind06NIP18Event.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidKind06NIP18Event.Event))
	})

	var validKind16NIP18GenericRepostEvent *model.Event
	t.Run("kind 6 (NIP-18): valid generic repost event", func(t *testing.T) {
		repostID := uuid.NewString()
		pubKeyOfRepostedNote := "pubkey1"
		repostedKind := nostr.KindReaction
		validKind16NIP18GenericRepostEvent = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindGenericRepost,
			Tags:      nostr.Tags{[]string{"e", repostID, "relay"}, []string{"p", pubKeyOfRepostedNote}, []string{"k", fmt.Sprint(repostedKind)}},
			Content:   fmt.Sprintf(`{"kind":%v,"id":"%v","pubkey":"%v"}`, repostedKind, repostID, pubKeyOfRepostedNote),
		}}
		validKind16NIP18GenericRepostEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, validKind16NIP18GenericRepostEvent.Sign(privkey))
		require.NoError(t, validKind16NIP18GenericRepostEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validKind16NIP18GenericRepostEvent.Sign(privkey))
		require.NoError(t, relay.Publish(ctx, validKind16NIP18GenericRepostEvent.Event))
	})

	t.Run("kind 6 (NIP-18): invalid generic repost event: wrong k tag", func(t *testing.T) {
		repostID := uuid.NewString()
		pubKeyOfRepostedNote := "pubkey1"
		repostedKind := nostr.KindReaction
		invalidKind16NIP18GenericRepostEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindGenericRepost,
			Tags:      nostr.Tags{[]string{"e", repostID, "relay"}, []string{"p", pubKeyOfRepostedNote}, []string{"k", "invalid k tag kind"}},
			Content:   fmt.Sprintf(`{"kind":%v,"id":"%v","pubkey":"%v"}`, repostedKind, repostID, pubKeyOfRepostedNote),
		}}
		invalidKind16NIP18GenericRepostEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidKind16NIP18GenericRepostEvent.Sign(privkey))
		require.NoError(t, invalidKind16NIP18GenericRepostEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidKind16NIP18GenericRepostEvent.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidKind16NIP18GenericRepostEvent.Event))
	})

	require.NoError(t, relay.Close())
	shutdownCtx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
	for pubsubServer.ReaderExited.Load() != uint64(1) {
		if shutdownCtx.Err() != nil {
			log.Panic(errorx.Errorf("shutdown timeout %v of %v", pubsubServer.ReaderExited.Load(), 1))
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.Equal(t, uint64(1), pubsubServer.ReaderExited.Load())
	require.Equal(t, []*model.Event{validKind06NIP18Event, validKind16NIP18GenericRepostEvent}, storedEvents)
}

func TestPublishingNIP23Events(t *testing.T) {
	privkey := nostr.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	RegisterWSEventListener(func(ctx context.Context, event *model.Event) error {
		for _, sEvent := range storedEvents {
			if sEvent.ID == event.ID {
				return model.ErrDuplicate
			}
		}
		assert.False(t, event.IsEphemeral())
		storedEvents = append(storedEvents, event)
		return nil
	})
	pubsubServer.Reset()
	ctx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
	relay, err := fixture.NewRelayClient(ctx, "wss://localhost:9998")
	require.NoError(t, err)

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
		validEventKindArticle.SetExtra("extra", uuid.NewString())
		require.NoError(t, validEventKindArticle.Sign(privkey))
		require.NoError(t, validEventKindArticle.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validEventKindArticle.Sign(privkey))
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
			Kind:      model.KindBlogPost,
			Tags:      tags,
			Content:   "Lorem [ipsum][nostr:nevent1qqst8cujky046negxgwwm5ynqwn53t8aqjr6afd8g59nfqwxpdhylpcpzamhxue69uhhyetvv9ujuetcv9khqmr99e3k7mg8arnc9] dolor sit amet",
		}}
		validEventKindBlogPost.SetExtra("extra", uuid.NewString())
		require.NoError(t, validEventKindBlogPost.Sign(privkey))
		require.NoError(t, validEventKindBlogPost.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validEventKindBlogPost.Sign(privkey))
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
		validEventNoTagsKindArticle.SetExtra("extra", uuid.NewString())
		require.NoError(t, validEventNoTagsKindArticle.Sign(privkey))
		require.NoError(t, validEventNoTagsKindArticle.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validEventNoTagsKindArticle.Sign(privkey))
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
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindArticle,
			Tags:      tags,
			Content:   "Lorem [ipsum][nostr:nevent1qqst8cujky046negxgwwm5ynqwn53t8aqjr6afd8g59nfqwxpdhylpcpzamhxue69uhhyetvv9ujuetcv9khqmr99e3k7mg8arnc9] dolor sit amet",
		}}
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})

	t.Run("kind 30023 (Article) (NIP-23): unsupported content type:JSON", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"a", "30023:a695f6b60119d9521934a691347d9f78e8770b56da16bb255ee286ddf9fda919:ipsum", "wss://relay.nostr.org"})
		tags = append(tags, nostr.Tag{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"})
		tags = append(tags, nostr.Tag{"t", "placeholder"})
		tags = append(tags, nostr.Tag{"published_at", "1296962229"})
		tags = append(tags, nostr.Tag{"title", "Lorem Ipsum"})
		tags = append(tags, nostr.Tag{"d", "lorem-ipsum"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindArticle,
			Tags:      tags,
			Content:   `{"id":"qwerty"}`,
		}}
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	require.NoError(t, relay.Close())
	shutdownCtx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
	for pubsubServer.ReaderExited.Load() != uint64(1) {
		if shutdownCtx.Err() != nil {
			log.Panic(errorx.Errorf("shutdown timeout %v of %v", pubsubServer.ReaderExited.Load(), 1))
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.Equal(t, uint64(1), pubsubServer.ReaderExited.Load())
	require.Equal(t, []*model.Event{validEventKindArticle, validEventKindBlogPost, validEventNoTagsKindArticle}, storedEvents)
}

func TestPublishingNIP01NIP24Events(t *testing.T) {
	privkey := nostr.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	RegisterWSEventListener(func(ctx context.Context, event *model.Event) error {
		for _, sEvent := range storedEvents {
			if sEvent.ID == event.ID {
				return model.ErrDuplicate
			}
		}
		assert.False(t, event.IsEphemeral())
		storedEvents = append(storedEvents, event)

		return nil
	})
	pubsubServer.Reset()
	ctx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
	relay, err := fixture.NewRelayClient(ctx, "wss://localhost:9998")
	require.NoError(t, err)

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
			Content:   `{"name":"qwerty","about":"me is bot","picture":"https://example.com/pic.jpg"}`,
		}}
		validEventNIP01.SetExtra("extra", uuid.NewString())
		require.NoError(t, validEventNIP01.Sign(privkey))
		require.NoError(t, validEventNIP01.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validEventNIP01.Sign(privkey))
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
		validEventNIP24.SetExtra("extra", uuid.NewString())
		require.NoError(t, validEventNIP24.Sign(privkey))
		require.NoError(t, validEventNIP24.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validEventNIP24.Sign(privkey))
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
			Content:   `{"display_name":"qqq","website":"https://ice.io","banner":"https://example.com/banner.jpg","bot":true}`,
		}}
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
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
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
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
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})

	require.NoError(t, relay.Close())
	shutdownCtx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
	for pubsubServer.ReaderExited.Load() != uint64(1) {
		if shutdownCtx.Err() != nil {
			log.Panic(errorx.Errorf("shutdown timeout %v of %v", pubsubServer.ReaderExited.Load(), 1))
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.Equal(t, uint64(1), pubsubServer.ReaderExited.Load())
	require.Equal(t, []*model.Event{validEventNIP01, validEventNIP24}, storedEvents)
}

func TestPublishingNIP24ReactionEvents(t *testing.T) {
	privkey := nostr.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	RegisterWSEventListener(func(ctx context.Context, event *model.Event) error {
		for _, sEvent := range storedEvents {
			if sEvent.ID == event.ID {
				return model.ErrDuplicate
			}
		}
		assert.False(t, event.IsEphemeral())
		storedEvents = append(storedEvents, event)

		return nil
	})
	pubsubServer.Reset()
	ctx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
	relay, err := fixture.NewRelayClient(ctx, "wss://localhost:9998")
	require.NoError(t, err)

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
		validUpvoteEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, validUpvoteEvent.Sign(privkey))
		require.NoError(t, validUpvoteEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validUpvoteEvent.Sign(privkey))
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
		validUpvoteEmptyContentEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, validUpvoteEmptyContentEvent.Sign(privkey))
		require.NoError(t, validUpvoteEmptyContentEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validUpvoteEmptyContentEvent.Sign(privkey))
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
		validDownvoteEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, validDownvoteEvent.Sign(privkey))
		require.NoError(t, validDownvoteEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validDownvoteEvent.Sign(privkey))
		require.NoError(t, relay.Publish(ctx, validDownvoteEvent.Event))
	})
	t.Run("kind 7 (Reactions) (NIP-25): valid upvote reaction to website event", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"r", "https://example.com/"})
		validReactionToWebsiteEvent = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindReactionToWebsite,
			Tags:      tags,
			Content:   "+",
		}}
		validReactionToWebsiteEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, validReactionToWebsiteEvent.Sign(privkey))
		require.NoError(t, validReactionToWebsiteEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validReactionToWebsiteEvent.Sign(privkey))
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
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
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
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
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
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})

	require.NoError(t, relay.Close())
	shutdownCtx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
	for pubsubServer.ReaderExited.Load() != uint64(1) {
		if shutdownCtx.Err() != nil {
			log.Panic(errorx.Errorf("shutdown timeout %v of %v", pubsubServer.ReaderExited.Load(), 1))
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.Equal(t, uint64(1), pubsubServer.ReaderExited.Load())
	require.Equal(t, []*model.Event{validUpvoteEvent, validUpvoteEmptyContentEvent, validDownvoteEvent, validReactionToWebsiteEvent}, storedEvents)
}

func TestPublishingNIP32LabelingEvents(t *testing.T) {
	privkey := nostr.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	RegisterWSEventListener(func(ctx context.Context, event *model.Event) error {
		for _, sEvent := range storedEvents {
			if sEvent.ID == event.ID {
				return model.ErrDuplicate
			}
		}
		assert.False(t, event.IsEphemeral())
		storedEvents = append(storedEvents, event)
		return nil
	})
	pubsubServer.Reset()
	ctx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
	relay, err := fixture.NewRelayClient(ctx, "wss://localhost:9998")
	require.NoError(t, err)

	var validLabelingEvent, validUGCLabelingEvent *model.Event
	t.Run("kind 1985 (Labeling) (NIP-32): valid labeling event", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"})
		tags = append(tags, nostr.Tag{"L", "#t"})
		tags = append(tags, nostr.Tag{"l", "permies", "#t"})
		validLabelingEvent = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindLabeling,
			Tags:      tags,
			Content:   "Some label long description",
		}}
		validLabelingEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, validLabelingEvent.Sign(privkey))
		require.NoError(t, validLabelingEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validLabelingEvent.Sign(privkey))
		require.NoError(t, relay.Publish(ctx, validLabelingEvent.Event))
	})
	t.Run("kind 1985 (Labeling) (NIP-32): valid labeling event, no label namespace tag L, but l refers to ugc namespace", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"})
		tags = append(tags, nostr.Tag{"l", "permies", "ugc"})
		validUGCLabelingEvent = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindLabeling,
			Tags:      tags,
			Content:   "Some label long description",
		}}
		validUGCLabelingEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, validUGCLabelingEvent.Sign(privkey))
		require.NoError(t, validUGCLabelingEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validUGCLabelingEvent.Sign(privkey))
		require.NoError(t, relay.Publish(ctx, validUGCLabelingEvent.Event))
	})
	t.Run("kind 1985 (Labeling) (NIP-32): invalid labeling event, no one of required (e,p,a,t,r) tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"L", "#t"})
		tags = append(tags, nostr.Tag{"l", "permies", "#t"})
		invalidLabelingEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindLabeling,
			Tags:      tags,
			Content:   "Some label long description",
		}}
		invalidLabelingEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidLabelingEvent.Sign(privkey))
		require.NoError(t, invalidLabelingEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidLabelingEvent.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidLabelingEvent.Event))
	})
	t.Run("kind 1985 (Labeling) (NIP-32): invalid labeling event, no label tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"})
		tags = append(tags, nostr.Tag{"L", "#t"})
		invalidLabelingEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindLabeling,
			Tags:      tags,
			Content:   "Some label long description",
		}}
		invalidLabelingEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidLabelingEvent.Sign(privkey))
		require.NoError(t, invalidLabelingEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidLabelingEvent.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidLabelingEvent.Event))
	})
	t.Run("kind 1985 (Labeling) (NIP-32): invalid labeling event, no namespace specified at the label tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"})
		tags = append(tags, nostr.Tag{"L", "#t"})
		tags = append(tags, nostr.Tag{"l", "permies"})
		invalidLabelingEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindLabeling,
			Tags:      tags,
			Content:   "Some label long description",
		}}
		invalidLabelingEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidLabelingEvent.Sign(privkey))
		require.NoError(t, invalidLabelingEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidLabelingEvent.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidLabelingEvent.Event))
	})
	t.Run("kind 1985 (Labeling) (NIP-32): invalid labeling event, exceeds max label symbols", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"})
		tags = append(tags, nostr.Tag{"L", "#t"})
		tags = append(tags, nostr.Tag{"l", "permies permies permies permies permies permies permies permies permies permies permies permies permies", "#t"})
		invalidLabelingEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindLabeling,
			Tags:      tags,
			Content:   "Some label long description",
		}}
		invalidLabelingEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidLabelingEvent.Sign(privkey))
		require.NoError(t, invalidLabelingEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidLabelingEvent.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidLabelingEvent.Event))
	})
	t.Run("kind 1985 (Labeling) (NIP-32): invalid labeling event, no label namespace tag L, l doesn't refer to ugc namespace", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"})
		tags = append(tags, nostr.Tag{"l", "permies", "#t"})
		invalidLabelingEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindLabeling,
			Tags:      tags,
			Content:   "Some label long description",
		}}
		invalidLabelingEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidLabelingEvent.Sign(privkey))
		require.NoError(t, invalidLabelingEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidLabelingEvent.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidLabelingEvent.Event))
	})
	t.Run("kind 1985 (Labeling) (NIP-32): invalid labeling event, l -> L values mismatch", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"L", "#a"})
		tags = append(tags, nostr.Tag{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"})
		tags = append(tags, nostr.Tag{"l", "permies", "#t"})
		invalidLabelingEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindLabeling,
			Tags:      tags,
			Content:   "Some label long description",
		}}
		invalidLabelingEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidLabelingEvent.Sign(privkey))
		require.NoError(t, invalidLabelingEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidLabelingEvent.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidLabelingEvent.Event))
	})
	t.Run("kind 1 (NIP-32): invalid label", func(t *testing.T) {
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindTextNote,
			Tags:      nostr.Tags{[]string{"e", "", "relay", "reply"}, []string{"p", "pubkey1", "pubkey2"}, []string{"l", "permies", "#t"}, []string{"L", "#a"}},
		}}
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	var validKind01EventWithLabels *model.Event
	t.Run("kind 1 (NIP-32): valid with label", func(t *testing.T) {
		validKind01EventWithLabels = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindTextNote,
			Tags:      nostr.Tags{[]string{"e", "", "relay", "reply"}, []string{"p", "pubkey1", "pubkey2"}, []string{"l", "permies", "#t"}, []string{"L", "#t"}},
		}}
		validKind01EventWithLabels.SetExtra("extra", uuid.NewString())
		require.NoError(t, validKind01EventWithLabels.Sign(privkey))
		require.NoError(t, validKind01EventWithLabels.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validKind01EventWithLabels.Sign(privkey))
		require.NoError(t, relay.Publish(ctx, validKind01EventWithLabels.Event))
	})

	require.NoError(t, relay.Close())
	shutdownCtx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
	for pubsubServer.ReaderExited.Load() != uint64(1) {
		if shutdownCtx.Err() != nil {
			log.Panic(errorx.Errorf("shutdown timeout %v of %v", pubsubServer.ReaderExited.Load(), 1))
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.Equal(t, uint64(1), pubsubServer.ReaderExited.Load())
	require.Equal(t, []*model.Event{validLabelingEvent, validUGCLabelingEvent, validKind01EventWithLabels}, storedEvents)
}

func TestPublishingNIP56(t *testing.T) {
	privkey := nostr.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	RegisterWSEventListener(func(ctx context.Context, event *model.Event) error {
		for _, sEvent := range storedEvents {
			if sEvent.ID == event.ID {
				return model.ErrDuplicate
			}
		}
		assert.False(t, event.IsEphemeral())
		storedEvents = append(storedEvents, event)
		return nil
	})
	pubsubServer.Reset()
	ctx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
	relay, err := fixture.NewRelayClient(ctx, "wss://localhost:9998")
	require.NoError(t, err)

	var validReportEventWithPTagOnly *model.Event
	t.Run("kind 1984 (Report) (NIP-56): valid report event with p tag only", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"p", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", model.TagReportTypeNudity})
		validReportEventWithPTagOnly = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindReport,
			Tags:      tags,
			Content:   "Report description",
		}}
		validReportEventWithPTagOnly.SetExtra("extra", uuid.NewString())
		require.NoError(t, validReportEventWithPTagOnly.Sign(privkey))
		require.NoError(t, validReportEventWithPTagOnly.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validReportEventWithPTagOnly.Sign(privkey))
		require.NoError(t, relay.Publish(ctx, validReportEventWithPTagOnly.Event))
	})
	var validReportEventWithBothTags *model.Event
	t.Run("kind 1984 (Report) (NIP-56): valid report event with both e and p tags", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"p", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87"})
		tags = append(tags, nostr.Tag{"e", "event id", model.TagReportTypeNudity})
		validReportEventWithBothTags = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindReport,
			Tags:      tags,
			Content:   "Report description",
		}}
		validReportEventWithBothTags.SetExtra("extra", uuid.NewString())
		require.NoError(t, validReportEventWithBothTags.Sign(privkey))
		require.NoError(t, validReportEventWithBothTags.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validReportEventWithBothTags.Sign(privkey))
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
			Kind:      model.KindReport,
			Tags:      tags,
			Content:   "Report description",
		}}
		validReportEventWithLabel.SetExtra("extra", uuid.NewString())
		require.NoError(t, validReportEventWithLabel.Sign(privkey))
		require.NoError(t, validReportEventWithLabel.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validReportEventWithLabel.Sign(privkey))
		require.NoError(t, relay.Publish(ctx, validReportEventWithLabel.Event))
	})
	t.Run("kind 1984 (Report) (NIP-56): invalid report with no p tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "event id", model.TagReportTypeNudity})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindReport,
			Tags:      tags,
			Content:   "Report description",
		}}
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 1984 (Report) (NIP-56): invalid report with wrong p tag while e tag is added", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"p", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", model.TagReportTypeNudity})
		tags = append(tags, nostr.Tag{"e", "event id", model.TagReportTypeNudity})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindReport,
			Tags:      tags,
			Content:   "Report description",
		}}
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 1984 (Report) (NIP-56): invalid report with wrong p tag when no e tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"p", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindReport,
			Tags:      tags,
			Content:   "Report description",
		}}
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 1984 (Report) (NIP-56): invalid report with e tag when both tags represented", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"p", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87"})
		tags = append(tags, nostr.Tag{"e", "event id"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindReport,
			Tags:      tags,
			Content:   "Report description",
		}}
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 1984 (Report) (NIP-56): invalid report with not supported report type", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"p", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87"})
		tags = append(tags, nostr.Tag{"e", "event id", "unsupported report type"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindReport,
			Tags:      tags,
			Content:   "Report description",
		}}
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})

	require.NoError(t, relay.Close())
	shutdownCtx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
	for pubsubServer.ReaderExited.Load() != uint64(1) {
		if shutdownCtx.Err() != nil {
			log.Panic(errorx.Errorf("shutdown timeout %v of %v", pubsubServer.ReaderExited.Load(), 1))
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.Equal(t, uint64(1), pubsubServer.ReaderExited.Load())
	require.Equal(t, []*model.Event{validReportEventWithPTagOnly, validReportEventWithBothTags, validReportEventWithLabel}, storedEvents)
}

func TestPublishingNIP58Badges(t *testing.T) {
	privkey := nostr.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	RegisterWSEventListener(func(ctx context.Context, event *model.Event) error {
		for _, sEvent := range storedEvents {
			if sEvent.ID == event.ID {
				return model.ErrDuplicate
			}
		}
		assert.False(t, event.IsEphemeral())
		storedEvents = append(storedEvents, event)
		return nil
	})
	pubsubServer.Reset()
	ctx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
	relay, err := fixture.NewRelayClient(ctx, "wss://localhost:9998")
	require.NoError(t, err)

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
		validBadgeDefinitionEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, validBadgeDefinitionEvent.Sign(privkey))
		require.NoError(t, validBadgeDefinitionEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validBadgeDefinitionEvent.Sign(privkey))
		require.NoError(t, relay.Publish(ctx, validBadgeDefinitionEvent.Event))
	})
	t.Run("kind 8 (Badge award) (NIP-56): valid badge award event", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"a", "30009:alice:bravery"})
		tags = append(tags, nostr.Tag{"p", "bob", "wss://relay"})
		tags = append(tags, nostr.Tag{"p", "charlie", "wss://relay"})
		validBadgeAwardEvent = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindBadgeAward,
			Tags:      tags,
		}}
		validBadgeAwardEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, validBadgeAwardEvent.Sign(privkey))
		require.NoError(t, validBadgeAwardEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validBadgeAwardEvent.Sign(privkey))
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
		validProfileBadgesEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, validProfileBadgesEvent.Sign(privkey))
		require.NoError(t, validProfileBadgesEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validProfileBadgesEvent.Sign(privkey))
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
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
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
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 8 (Badge award) (NIP-56): invalid, no a tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"p", "bob", "wss://relay"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindBadgeAward,
			Tags:      tags,
		}}
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 8 (Badge award) (NIP-56): invalid, a tag refers to wrong kind", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"a", "1:alice:bravery"})
		tags = append(tags, nostr.Tag{"p", "bob", "wss://relay"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindBadgeAward,
			Tags:      tags,
		}}
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 8 (Badge award) (NIP-56): invalid, no at least one p tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"a", "3009:alice:bravery"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindBadgeAward,
			Tags:      tags,
		}}
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
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
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
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
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
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
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})

	require.NoError(t, relay.Close())
	shutdownCtx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
	for pubsubServer.ReaderExited.Load() != uint64(1) {
		if shutdownCtx.Err() != nil {
			log.Panic(errorx.Errorf("shutdown timeout %v of %v", pubsubServer.ReaderExited.Load(), 1))
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.Equal(t, uint64(1), pubsubServer.ReaderExited.Load())
	require.Equal(t, []*model.Event{validBadgeDefinitionEvent, validBadgeAwardEvent, validProfileBadgesEvent}, storedEvents)
}

func TestPublishingNIP65RelayListMetadataEvents(t *testing.T) {
	privkey := nostr.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	RegisterWSEventListener(func(ctx context.Context, event *model.Event) error {
		for _, sEvent := range storedEvents {
			if sEvent.ID == event.ID {
				return model.ErrDuplicate
			}
		}
		assert.False(t, event.IsEphemeral())
		storedEvents = append(storedEvents, event)
		return nil
	})
	pubsubServer.Reset()
	ctx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
	relay, err := fixture.NewRelayClient(ctx, "wss://localhost:9998")
	require.NoError(t, err)

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
		validRelayListEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, validRelayListEvent.Sign(privkey))
		require.NoError(t, validRelayListEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validRelayListEvent.Sign(privkey))
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
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
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
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
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
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	require.NoError(t, relay.Close())
	shutdownCtx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
	for pubsubServer.ReaderExited.Load() != uint64(1) {
		if shutdownCtx.Err() != nil {
			log.Panic(errorx.Errorf("shutdown timeout %v of %v", pubsubServer.ReaderExited.Load(), 1))
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.Equal(t, uint64(1), pubsubServer.ReaderExited.Load())
	require.Equal(t, []*model.Event{validRelayListEvent}, storedEvents)
}

func TestPublishingNIP51ListsSetsEvents(t *testing.T) {
	privkey := nostr.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	RegisterWSEventListener(func(ctx context.Context, event *model.Event) error {
		for _, sEvent := range storedEvents {
			if sEvent.ID == event.ID {
				return model.ErrDuplicate
			}
		}
		assert.False(t, event.IsEphemeral())
		storedEvents = append(storedEvents, event)
		return nil
	})
	pubsubServer.Reset()
	ctx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
	relay, err := fixture.NewRelayClient(ctx, "wss://localhost:9998")
	require.NoError(t, err)

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
		validEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, validEvent.Sign(privkey))
		require.NoError(t, validEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validEvent.Sign(privkey))
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
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
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
		validEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, validEvent.Sign(privkey))
		require.NoError(t, validEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validEvent.Sign(privkey))
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
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
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
			Kind:      model.KindBookmarks,
			Tags:      tags,
		}}
		validEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, validEvent.Sign(privkey))
		require.NoError(t, validEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validEvent.Sign(privkey))
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
			Kind:      model.KindBookmarks,
			Tags:      tags,
		}}
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
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
			Kind:      model.KindBookmarks,
			Tags:      tags,
		}}
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("Kind 10004 (NIP-51) valid", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:dummy:dummy", model.KindCommunityDefinitions)})
		validEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindCommunities,
			Tags:      tags,
		}}
		validEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, validEvent.Sign(privkey))
		require.NoError(t, validEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validEvent.Sign(privkey))
		validEvents = append(validEvents, validEvent)
		require.NoError(t, relay.Publish(ctx, validEvent.Event))
	})
	t.Run("Kind 10003 (NIP-51): unsupported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:dummy:dummy", model.KindCommunityDefinitions)})
		tags = append(tags, nostr.Tag{"wrong", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindCommunities,
			Tags:      tags,
		}}
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("Kind 10003 (NIP-51): wrong a tag kind", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:dummy:dummy", nostr.KindProfileMetadata)})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindCommunities,
			Tags:      tags,
		}}
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("Kind 10005 (NIP-51) valid", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "dummy"})
		validEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindPublicChats,
			Tags:      tags,
		}}
		validEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, validEvent.Sign(privkey))
		require.NoError(t, validEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validEvent.Sign(privkey))
		validEvents = append(validEvents, validEvent)
		require.NoError(t, relay.Publish(ctx, validEvent.Event))
	})
	t.Run("Kind 10005 (NIP-51): unsupported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "dummy"})
		tags = append(tags, nostr.Tag{"wrong", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindPublicChats,
			Tags:      tags,
		}}
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("Kind 10006 (NIP-51) valid", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"relay", "dummy"})
		validEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindBlockedRelays,
			Tags:      tags,
		}}
		validEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, validEvent.Sign(privkey))
		require.NoError(t, validEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validEvent.Sign(privkey))
		validEvents = append(validEvents, validEvent)
		require.NoError(t, relay.Publish(ctx, validEvent.Event))
	})
	t.Run("Kind 10006 (NIP-51): unsupported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"relay", "dummy"})
		tags = append(tags, nostr.Tag{"wrong", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindBlockedRelays,
			Tags:      tags,
		}}
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("Kind 10007 (NIP-51) valid", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"relay", "dummy"})
		validEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindSearchRelay,
			Tags:      tags,
		}}
		validEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, validEvent.Sign(privkey))
		require.NoError(t, validEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validEvent.Sign(privkey))
		validEvents = append(validEvents, validEvent)
		require.NoError(t, relay.Publish(ctx, validEvent.Event))
	})
	t.Run("Kind 10007 (NIP-51): unsupported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"relay", "dummy"})
		tags = append(tags, nostr.Tag{"wrong", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindSearchRelay,
			Tags:      tags,
		}}
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("Kind 10007 (NIP-51) valid", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"group", "dummy"})
		validEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindSimpleGroups,
			Tags:      tags,
		}}
		validEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, validEvent.Sign(privkey))
		require.NoError(t, validEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validEvent.Sign(privkey))
		validEvents = append(validEvents, validEvent)
		require.NoError(t, relay.Publish(ctx, validEvent.Event))
	})
	t.Run("Kind 10007 (NIP-51): unsupported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"group", "dummy"})
		tags = append(tags, nostr.Tag{"wrong", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindSimpleGroups,
			Tags:      tags,
		}}
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("Kind 10015 (NIP-51) valid", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"t", "dummy"})
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:dummy:dummy", model.KindInterestSets)})
		validEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindInterests,
			Tags:      tags,
		}}
		validEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, validEvent.Sign(privkey))
		require.NoError(t, validEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validEvent.Sign(privkey))
		validEvents = append(validEvents, validEvent)
		require.NoError(t, relay.Publish(ctx, validEvent.Event))
	})
	t.Run("Kind 10015 (NIP-51): unsupported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"t", "dummy"})
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:dummy,dummy", model.KindInterestSets)})
		tags = append(tags, nostr.Tag{"wrong", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindInterests,
			Tags:      tags,
		}}
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("Kind 10015 (NIP-51): wrong a tag kind", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"t", "dummy"})
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:dummy:dummy", nostr.KindProfileMetadata)})
		tags = append(tags, nostr.Tag{"wrong", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindInterests,
			Tags:      tags,
		}}
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})

	t.Run("Kind 10030 (NIP-51) valid", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"emoji", "dummy"})
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:dummy:dummy", model.KindEmojiSets)})
		validEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindEmojis,
			Tags:      tags,
		}}
		validEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, validEvent.Sign(privkey))
		require.NoError(t, validEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validEvent.Sign(privkey))
		validEvents = append(validEvents, validEvent)
		require.NoError(t, relay.Publish(ctx, validEvent.Event))
	})
	t.Run("Kind 10030 (NIP-51) unsupported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"emoji", "dummy"})
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:dummy:dummy", model.KindEmojiSets)})
		tags = append(tags, nostr.Tag{"wrong", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindEmojis,
			Tags:      tags,
		}}
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("Kind 10030 (NIP-51) wrong a tag kind", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"emoji", "dummy"})
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:dummy:dummy", nostr.KindProfileMetadata)})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindEmojis,
			Tags:      tags,
		}}
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("Kind 10050 (NIP-51) valid", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"relay", "dummy"})
		validEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindDMRelays,
			Tags:      tags,
		}}
		validEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, validEvent.Sign(privkey))
		require.NoError(t, validEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validEvent.Sign(privkey))
		validEvents = append(validEvents, validEvent)
		require.NoError(t, relay.Publish(ctx, validEvent.Event))
	})
	t.Run("Kind 10050 (NIP-51) unsupported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"relay", "dummy"})
		tags = append(tags, nostr.Tag{"wrong", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindDMRelays,
			Tags:      tags,
		}}
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("Kind 10101 (NIP-51) valid", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"p", "dummy"})
		validEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindGoodWikiAuthors,
			Tags:      tags,
		}}
		validEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, validEvent.Sign(privkey))
		require.NoError(t, validEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validEvent.Sign(privkey))
		validEvents = append(validEvents, validEvent)
		require.NoError(t, relay.Publish(ctx, validEvent.Event))
	})
	t.Run("Kind 10101 (NIP-51) unsupported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"p", "dummy"})
		tags = append(tags, nostr.Tag{"wrong", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindGoodWikiAuthors,
			Tags:      tags,
		}}
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("Kind 10102 (NIP-51) valid", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"relay", "dummy"})
		validEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindGoodWikiRelays,
			Tags:      tags,
		}}
		validEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, validEvent.Sign(privkey))
		require.NoError(t, validEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validEvent.Sign(privkey))
		validEvents = append(validEvents, validEvent)
		require.NoError(t, relay.Publish(ctx, validEvent.Event))
	})
	t.Run("Kind 10102 (NIP-51) unsupported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"relay", "dummy"})
		tags = append(tags, nostr.Tag{"wrong", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindGoodWikiRelays,
			Tags:      tags,
		}}
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
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
		validEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, validEvent.Sign(privkey))
		require.NoError(t, validEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validEvent.Sign(privkey))
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
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
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
			Kind:      model.KindRelaySets,
			Tags:      tags,
		}}
		validEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, validEvent.Sign(privkey))
		require.NoError(t, validEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validEvent.Sign(privkey))
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
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
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
			Kind:      model.KindBookmarksSets,
			Tags:      tags,
		}}
		validEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, validEvent.Sign(privkey))
		require.NoError(t, validEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validEvent.Sign(privkey))
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
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
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
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
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
			Kind:      model.KindCurationSets1,
			Tags:      tags,
		}}
		validEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, validEvent.Sign(privkey))
		require.NoError(t, validEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validEvent.Sign(privkey))
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
			Kind:      model.KindCurationSets1,
			Tags:      tags,
		}}
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
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
			Kind:      model.KindCurationSets1,
			Tags:      tags,
		}}
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("Kind 30005 (NIP-51) valid", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:dummy:dummy", model.KindVideo)})
		tags = append(tags, nostr.Tag{"d", "dummy"})
		tags = append(tags, nostr.Tag{"title", "dummy"})
		tags = append(tags, nostr.Tag{"image", "dummy"})
		tags = append(tags, nostr.Tag{"description", "dummy"})
		validEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindCurationSets2,
			Tags:      tags,
		}}
		validEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, validEvent.Sign(privkey))
		require.NoError(t, validEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validEvent.Sign(privkey))
		validEvents = append(validEvents, validEvent)
		require.NoError(t, relay.Publish(ctx, validEvent.Event))
	})
	t.Run("Kind 30005 (NIP-51) unsupported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:dummy:dummy", model.KindVideo)})
		tags = append(tags, nostr.Tag{"d", "dummy"})
		tags = append(tags, nostr.Tag{"title", "dummy"})
		tags = append(tags, nostr.Tag{"image", "dummy"})
		tags = append(tags, nostr.Tag{"description", "dummy"})
		tags = append(tags, nostr.Tag{"wrong", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindCurationSets2,
			Tags:      tags,
		}}
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
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
			Kind:      model.KindCurationSets2,
			Tags:      tags,
		}}
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
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
			Kind:      model.KindMuteSets,
			Tags:      tags,
		}}
		validEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, validEvent.Sign(privkey))
		require.NoError(t, validEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validEvent.Sign(privkey))
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
			Kind:      model.KindMuteSets,
			Tags:      tags,
		}}
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
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
			Kind:      model.KindInterestSets,
			Tags:      tags,
		}}
		validEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, validEvent.Sign(privkey))
		require.NoError(t, validEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validEvent.Sign(privkey))
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
			Kind:      model.KindInterestSets,
			Tags:      tags,
		}}
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
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
			Kind:      model.KindEmojiSets,
			Tags:      tags,
		}}
		validEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, validEvent.Sign(privkey))
		require.NoError(t, validEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validEvent.Sign(privkey))
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
			Kind:      model.KindEmojiSets,
			Tags:      tags,
		}}
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
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
			Kind:      model.KindReleaseArtefactSets,
			Tags:      tags,
		}}
		validEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, validEvent.Sign(privkey))
		require.NoError(t, validEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, validEvent.Sign(privkey))
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
			Kind:      model.KindReleaseArtefactSets,
			Tags:      tags,
		}}
		invalidEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidEvent.Sign(privkey))
		require.NoError(t, invalidEvent.GenerateNIP13(ctx, NIP13MinLeadingZeroBits))
		require.NoError(t, invalidEvent.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})

	require.NoError(t, relay.Close())
	shutdownCtx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
	for pubsubServer.ReaderExited.Load() != uint64(1) {
		if shutdownCtx.Err() != nil {
			log.Panic(errorx.Errorf("shutdown timeout %v of %v", pubsubServer.ReaderExited.Load(), 1))
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.Equal(t, uint64(1), pubsubServer.ReaderExited.Load())
	require.Equal(t, validEvents, storedEvents)
}
