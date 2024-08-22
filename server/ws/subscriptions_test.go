package ws

import (
	"context"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gookit/goutil/errorx"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/server/ws/fixture"
)

func TestRelaySubscription(t *testing.T) {
	var eventsQueue []*model.Event

	privkey := nostr.GeneratePrivateKey()
	ev := model.Event{
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
	ev.SetExtra("extra", uuid.NewString())
	require.NoError(t, ev.Sign(privkey))
	eventsQueue = append(eventsQueue, &ev)

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
	ctx, cancel := context.WithTimeout(context.Background(), testDeadline)
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
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindTextNote,
			Tags:      nostr.Tags{},
			Content:   "realtime event matching filter" + uuid.NewString(),
		},
	})
	eventsQueue[len(eventsQueue)-1].SetExtra("extra", uuid.NewString())
	require.NoError(t, eventsQueue[len(eventsQueue)-1].Sign(privkey))
	require.NoError(t, relay.Publish(ctx, eventsQueue[len(eventsQueue)-1].Event))

	eventBy3rdParty := &model.Event{Event: nostr.Event{
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      nostr.KindTextNote,
		Tags:      nostr.Tags{},
		Content:   "eventBy3rdParty" + uuid.NewString(),
	}}
	eventsQueue = append(eventsQueue, eventBy3rdParty)
	storedEvents = append(storedEvents, eventBy3rdParty)
	eventsQueue[len(eventsQueue)-1].SetExtra("extra", uuid.NewString())
	require.NoError(t, eventsQueue[len(eventsQueue)-1].Event.Sign(privkey))
	require.NoError(t, NotifySubscriptions(eventBy3rdParty))

	notMatchingEvent := &model.Event{Event: nostr.Event{
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      nostr.KindRepost,
		Tags:      nostr.Tags{[]string{"e", eventsQueue[len(eventsQueue)-1].ID}},
		Content:   "realtime event NOT matching filter" + uuid.NewString(),
	}}
	notMatchingEvent.SetExtra("extra", uuid.NewString())
	require.NoError(t, notMatchingEvent.Sign(privkey))
	require.NoError(t, relay.Publish(ctx, notMatchingEvent.Event))
	assert.Equal(t, append(eventsQueue, notMatchingEvent), storedEvents)
	sub.Filters = []nostr.Filter{{
		Kinds: []int{nostr.KindArticle},
		Limit: 1,
	}}
	replacedSubEnvelope, _ := (nostr.ReqEnvelope{SubscriptionID: sub.GetID(), Filters: sub.Filters}).MarshalJSON()
	relay.Write(replacedSubEnvelope)
	eventMatchingReplacedSub := &model.Event{Event: nostr.Event{
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      nostr.KindArticle,
		Tags:      nostr.Tags{},
		Content:   "event matching replaced filter" + uuid.NewString(),
	}}
	eventsQueue = append(eventsQueue, eventMatchingReplacedSub)
	eventsQueue[len(eventsQueue)-1].SetExtra("extra", uuid.NewString())
	require.NoError(t, eventsQueue[len(eventsQueue)-1].Sign(privkey))

	require.NoError(t, relay.Publish(ctx, eventsQueue[len(eventsQueue)-1].Event))

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
	require.Equal(t, eventsQueue, receivedEvents)
}
func TestRelayEventsBroadcastMultipleSubs(t *testing.T) {
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
	RegisterWSEventListener(func(ctx context.Context, event *model.Event) error {
		storedEvents = append(storedEvents, event)
		return nil
	})
	pubsubServer.Reset()
	connsCount := 10
	subsPerConnectionCount := 10
	ctx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
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
		require.NoError(t, relay.Publish(ctx, validEvent.Event))
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
		}}
		invalidSignature.SetExtra("extra", uuid.NewString())
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
		require.NoError(t, relay.Publish(ctx, ephemeralEvent))

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
	require.Equal(t, []*model.Event{validEvent}, storedEvents)
}
