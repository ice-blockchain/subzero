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

const minTestLeadingZeroBits = 20

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
	require.NoError(t, ev.GenerateNIP13(ctx, minTestLeadingZeroBits))
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
	require.NoError(t, eventsQueue[len(eventsQueue)-1].GenerateNIP13(ctx, minTestLeadingZeroBits))
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
	require.NoError(t, eventsQueue[len(eventsQueue)-1].GenerateNIP13(ctx, minTestLeadingZeroBits))
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
	require.NoError(t, notMatchingEvent.GenerateNIP13(ctx, minTestLeadingZeroBits))
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
	require.NoError(t, eventMatchingReplacedSub.GenerateNIP13(ctx, minTestLeadingZeroBits))
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
	require.NoError(t, storedEvents[len(storedEvents)-1].GenerateNIP13(ctx, minTestLeadingZeroBits))
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
	tag, err := nip13.DoWork(ctx, newRealtimeEvent, minTestLeadingZeroBits)
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
		isEphemeralEvent := (20000 <= event.Kind && event.Kind < 30000)
		assert.False(t, isEphemeralEvent)
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
		require.NoError(t, validEvent.GenerateNIP13(ctx, minTestLeadingZeroBits))
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
		require.NoError(t, invalidKindEvent.GenerateNIP13(ctx, minTestLeadingZeroBits))
		require.NoError(t, invalidKindEvent.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidKindEvent.Event))

		invalidKindEvent.Kind = 65536
		require.NoError(t, invalidKindEvent.Sign(privkey))
		require.NoError(t, invalidKindEvent.GenerateNIP13(ctx, minTestLeadingZeroBits))
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
		require.NoError(t, invalidID.GenerateNIP13(ctx, minTestLeadingZeroBits))
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
		require.NoError(t, invalidSignature.GenerateNIP13(ctx, minTestLeadingZeroBits))
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
		tag, err := nip13.DoWork(ctx, ephemeralEvent, minTestLeadingZeroBits)
		require.NoError(t, err)
		ephemeralEvent.Tags = append(ephemeralEvent.Tags, tag)
		require.NoError(t, ephemeralEvent.Sign(privkey))
		require.NoError(t, relay.Publish(ctx, ephemeralEvent))
	})
	t.Run("wrong kind 03 follow list tag parameters", func(t *testing.T) {
		inValidKind03Event := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindContactList,
			Tags:      nil,
		}}
		inValidKind03Event.SetExtra("extra", uuid.NewString())
		require.NoError(t, inValidKind03Event.Sign(privkey))
		require.NoError(t, inValidKind03Event.GenerateNIP13(ctx, minTestLeadingZeroBits))
		require.NoError(t, inValidKind03Event.Sign(privkey))
		require.Error(t, relay.Publish(ctx, inValidKind03Event.Event))
	})
	t.Run("wrong kind 03 follow list content", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"p"})
		tags = append(tags, nostr.Tag{"p"})
		inValidKind03Event := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindContactList,
			Tags:      tags,
			Content:   "invalidEvent",
		}}
		inValidKind03Event.SetExtra("extra", uuid.NewString())
		require.NoError(t, inValidKind03Event.Sign(privkey))
		require.NoError(t, inValidKind03Event.GenerateNIP13(ctx, minTestLeadingZeroBits))
		require.NoError(t, inValidKind03Event.Sign(privkey))
		require.Error(t, relay.Publish(ctx, inValidKind03Event.Event))
	})
	var validKind03Event *model.Event
	t.Run("valid kind 03 follow list event", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"p", "wss://alicerelay.com/", "alice"})
		tags = append(tags, nostr.Tag{"p", "wss://bobrelay.com/nostr", "bob"})
		validKind03Event = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindContactList,
			Tags:      tags,
		}}
		validKind03Event.SetExtra("extra", uuid.NewString())
		require.NoError(t, validKind03Event.Sign(privkey))
		require.NoError(t, validKind03Event.GenerateNIP13(ctx, minTestLeadingZeroBits))
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

func TestPublishingNIP10Events(t *testing.T) {
	privkey := nostr.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	RegisterWSEventListener(func(ctx context.Context, event *model.Event) error {
		for _, sEvent := range storedEvents {
			if sEvent.ID == event.ID {
				return model.ErrDuplicate
			}
		}
		isEphemeralEvent := (20000 <= event.Kind && event.Kind < 30000)
		assert.False(t, isEphemeralEvent)
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
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e"})
		inValidKind01Event := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindTextNote,
			Tags:      tags,
		}}
		inValidKind01Event.SetExtra("extra", uuid.NewString())
		require.NoError(t, inValidKind01Event.Sign(privkey))
		require.NoError(t, inValidKind01Event.GenerateNIP13(ctx, minTestLeadingZeroBits))
		require.NoError(t, inValidKind01Event.Sign(privkey))
		require.Error(t, relay.Publish(ctx, inValidKind01Event.Event))
	})

	t.Run("kind 1 (NIP-10): invalid reply marker for e tags ", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "", "relay", "invalid marker"})
		inValidKind01Event := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindTextNote,
			Tags:      tags,
		}}
		inValidKind01Event.SetExtra("extra", uuid.NewString())
		require.NoError(t, inValidKind01Event.Sign(privkey))
		require.NoError(t, inValidKind01Event.GenerateNIP13(ctx, minTestLeadingZeroBits))
		require.NoError(t, inValidKind01Event.Sign(privkey))
		require.Error(t, relay.Publish(ctx, inValidKind01Event.Event))
	})
	t.Run("kind 1 (NIP-10): invalid p tag usage: no e tags", func(t *testing.T) {
		var tags nostr.Tags
		// tags = append(tags, nostr.Tag{"e", "", "relay", "reply"})
		tags = append(tags, nostr.Tag{"p", "pubkey1", "pubkey2"})
		inValidKind01Event := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindTextNote,
			Tags:      tags,
		}}
		inValidKind01Event.SetExtra("extra", uuid.NewString())
		require.NoError(t, inValidKind01Event.Sign(privkey))
		require.NoError(t, inValidKind01Event.GenerateNIP13(ctx, minTestLeadingZeroBits))
		require.NoError(t, inValidKind01Event.Sign(privkey))
		require.Error(t, relay.Publish(ctx, inValidKind01Event.Event))
	})
	t.Run("kind 1 (NIP-10): invalid p tag usage: empty tag values", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "", "relay", "reply"})
		tags = append(tags, nostr.Tag{"p"})
		inValidKind01Event := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindTextNote,
			Tags:      tags,
		}}
		inValidKind01Event.SetExtra("extra", uuid.NewString())
		require.NoError(t, inValidKind01Event.Sign(privkey))
		require.NoError(t, inValidKind01Event.GenerateNIP13(ctx, minTestLeadingZeroBits))
		require.NoError(t, inValidKind01Event.Sign(privkey))
		require.Error(t, relay.Publish(ctx, inValidKind01Event.Event))
	})

	var validKind01NIP10Event *model.Event
	t.Run("kind 1 (NIP-10): valid", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "", "relay", "reply"})
		tags = append(tags, nostr.Tag{"p", "pubkey1", "pubkey2"})
		validKind01NIP10Event = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindTextNote,
			Tags:      tags,
		}}
		validKind01NIP10Event.SetExtra("extra", uuid.NewString())
		require.NoError(t, validKind01NIP10Event.Sign(privkey))
		require.NoError(t, validKind01NIP10Event.GenerateNIP13(ctx, minTestLeadingZeroBits))
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
		isEphemeralEvent := (20000 <= event.Kind && event.Kind < 30000)
		assert.False(t, isEphemeralEvent)
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
	var validKind06NIP18Event *model.Event
	t.Run("kind 6 (NIP-18): valid event", func(t *testing.T) {
		repostID := uuid.NewString()
		pubKeyOfRepostedNote := "pubkey1"
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", repostID, "relay"})
		tags = append(tags, nostr.Tag{"p", pubKeyOfRepostedNote})
		validKind06NIP18Event = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindRepost,
			Tags:      tags,
			Content:   fmt.Sprintf(`{"kind":1,"id":"%v","pubkey":"%v"}`, repostID, pubKeyOfRepostedNote),
		}}
		validKind06NIP18Event.SetExtra("extra", uuid.NewString())
		require.NoError(t, validKind06NIP18Event.Sign(privkey))
		require.NoError(t, validKind06NIP18Event.GenerateNIP13(ctx, minTestLeadingZeroBits))
		require.NoError(t, validKind06NIP18Event.Sign(privkey))
		require.NoError(t, relay.Publish(ctx, validKind06NIP18Event.Event))
	})
	t.Run("kind 6 (NIP-18): invalid event, no e tags", func(t *testing.T) {
		repostID := uuid.NewString()
		pubKeyOfRepostedNote := "pubkey1"
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"p", pubKeyOfRepostedNote})
		invalidKind06NIP18Event := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindRepost,
			Tags:      tags,
			Content:   fmt.Sprintf(`{"kind":1,"id":"%v","pubkey":"%v"}`, repostID, pubKeyOfRepostedNote),
		}}
		invalidKind06NIP18Event.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidKind06NIP18Event.Sign(privkey))
		require.NoError(t, invalidKind06NIP18Event.GenerateNIP13(ctx, minTestLeadingZeroBits))
		require.NoError(t, invalidKind06NIP18Event.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidKind06NIP18Event.Event))
	})
	t.Run("kind 6 (NIP-18): invalid event, no p tags", func(t *testing.T) {
		repostID := uuid.NewString()
		pubKeyOfRepostedNote := "pubkey1"
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", repostID, "relay"})
		invalidKind06NIP18Event := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindRepost,
			Tags:      tags,
			Content:   fmt.Sprintf(`{"kind":1,"id":"%v","pubkey":"%v"}`, repostID, pubKeyOfRepostedNote),
		}}
		invalidKind06NIP18Event.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidKind06NIP18Event.Sign(privkey))
		require.NoError(t, invalidKind06NIP18Event.GenerateNIP13(ctx, minTestLeadingZeroBits))
		require.NoError(t, invalidKind06NIP18Event.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidKind06NIP18Event.Event))
	})
	t.Run("kind 6 (NIP-18): invalid event, no enough e tag parameters", func(t *testing.T) {
		repostID := uuid.NewString()
		pubKeyOfRepostedNote := "pubkey1"
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", repostID})
		tags = append(tags, nostr.Tag{"p", pubKeyOfRepostedNote})
		invalidKind06NIP18Event := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindRepost,
			Tags:      tags,
			Content:   fmt.Sprintf(`{"kind":1,"id":"%v","pubkey":"%v"}`, repostID, pubKeyOfRepostedNote),
		}}
		invalidKind06NIP18Event.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidKind06NIP18Event.Sign(privkey))
		require.NoError(t, invalidKind06NIP18Event.GenerateNIP13(ctx, minTestLeadingZeroBits))
		require.NoError(t, invalidKind06NIP18Event.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidKind06NIP18Event.Event))
	})
	t.Run("kind 6 (NIP-18): invalid event, no enough p tag parameters", func(t *testing.T) {
		repostID := uuid.NewString()
		pubKeyOfRepostedNote := "pubkey1"
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", repostID, "relay"})
		tags = append(tags, nostr.Tag{"p"})
		invalidKind06NIP18Event := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindRepost,
			Tags:      tags,
			Content:   fmt.Sprintf(`{"kind":1,"id":"%v","pubkey":"%v"}`, repostID, pubKeyOfRepostedNote),
		}}
		invalidKind06NIP18Event.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidKind06NIP18Event.Sign(privkey))
		require.NoError(t, invalidKind06NIP18Event.GenerateNIP13(ctx, minTestLeadingZeroBits))
		require.NoError(t, invalidKind06NIP18Event.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidKind06NIP18Event.Event))
	})
	t.Run("kind 6 (NIP-18): invalid event, wrong p tag pubkey != reposted note pubkey", func(t *testing.T) {
		repostID := uuid.NewString()
		pubKeyOfRepostedNote := "pubkey1"
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", repostID, "relay"})
		tags = append(tags, nostr.Tag{"p", "wrong pubkey"})
		invalidKind06NIP18Event := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindRepost,
			Tags:      tags,
			Content:   fmt.Sprintf(`{"kind":1,"id":"%v","pubkey":"%v"}`, repostID, pubKeyOfRepostedNote),
		}}
		invalidKind06NIP18Event.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidKind06NIP18Event.Sign(privkey))
		require.NoError(t, invalidKind06NIP18Event.GenerateNIP13(ctx, minTestLeadingZeroBits))
		require.NoError(t, invalidKind06NIP18Event.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidKind06NIP18Event.Event))
	})
	t.Run("kind 6 (NIP-18): invalid event, wrong content value: != 1", func(t *testing.T) {
		repostID := uuid.NewString()
		pubKeyOfRepostedNote := "pubkey1"
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", repostID, "relay"})
		tags = append(tags, nostr.Tag{"p", "wrong pubkey"})
		invalidKind06NIP18Event := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindRepost,
			Tags:      tags,
			Content:   fmt.Sprintf(`{"kind":16,"id":"%v","pubkey":"%v"}`, repostID, pubKeyOfRepostedNote),
		}}
		invalidKind06NIP18Event.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidKind06NIP18Event.Sign(privkey))
		require.NoError(t, invalidKind06NIP18Event.GenerateNIP13(ctx, minTestLeadingZeroBits))
		require.NoError(t, invalidKind06NIP18Event.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidKind06NIP18Event.Event))
	})
	t.Run("kind 6 (NIP-18): invalid event, wrong content id value: != e tag repost id value", func(t *testing.T) {
		repostID := uuid.NewString()
		pubKeyOfRepostedNote := "pubkey1"
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "wrong id value", "relay"})
		tags = append(tags, nostr.Tag{"p", "wrong pubkey"})
		invalidKind06NIP18Event := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindRepost,
			Tags:      tags,
			Content:   fmt.Sprintf(`{"kind":1,"id":"%v","pubkey":"%v"}`, repostID, pubKeyOfRepostedNote),
		}}
		invalidKind06NIP18Event.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidKind06NIP18Event.Sign(privkey))
		require.NoError(t, invalidKind06NIP18Event.GenerateNIP13(ctx, minTestLeadingZeroBits))
		require.NoError(t, invalidKind06NIP18Event.Sign(privkey))
		require.Error(t, relay.Publish(ctx, invalidKind06NIP18Event.Event))
	})

	var validKind16NIP18GenericRepostEvent *model.Event
	t.Run("kind 6 (NIP-18): valid generic repost event", func(t *testing.T) {
		repostID := uuid.NewString()
		pubKeyOfRepostedNote := "pubkey1"
		repostedKind := nostr.KindReaction
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", repostID, "relay"})
		tags = append(tags, nostr.Tag{"p", pubKeyOfRepostedNote})
		tags = append(tags, nostr.Tag{"k", fmt.Sprint(repostedKind)})
		validKind16NIP18GenericRepostEvent = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindGenericRepost,
			Tags:      tags,
			Content:   fmt.Sprintf(`{"kind":%v,"id":"%v","pubkey":"%v"}`, repostedKind, repostID, pubKeyOfRepostedNote),
		}}
		validKind16NIP18GenericRepostEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, validKind16NIP18GenericRepostEvent.Sign(privkey))
		require.NoError(t, validKind16NIP18GenericRepostEvent.GenerateNIP13(ctx, minTestLeadingZeroBits))
		require.NoError(t, validKind16NIP18GenericRepostEvent.Sign(privkey))
		require.NoError(t, relay.Publish(ctx, validKind16NIP18GenericRepostEvent.Event))
	})

	t.Run("kind 6 (NIP-18): invalid generic repost event: wrong k tag", func(t *testing.T) {
		repostID := uuid.NewString()
		pubKeyOfRepostedNote := "pubkey1"
		repostedKind := nostr.KindReaction
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", repostID, "relay"})
		tags = append(tags, nostr.Tag{"p", pubKeyOfRepostedNote})
		tags = append(tags, nostr.Tag{"k", "invalid k tag kind"})
		invalidKind16NIP18GenericRepostEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.KindGenericRepost,
			Tags:      tags,
			Content:   fmt.Sprintf(`{"kind":%v,"id":"%v","pubkey":"%v"}`, repostedKind, repostID, pubKeyOfRepostedNote),
		}}
		invalidKind16NIP18GenericRepostEvent.SetExtra("extra", uuid.NewString())
		require.NoError(t, invalidKind16NIP18GenericRepostEvent.Sign(privkey))
		require.NoError(t, invalidKind16NIP18GenericRepostEvent.GenerateNIP13(ctx, minTestLeadingZeroBits))
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
