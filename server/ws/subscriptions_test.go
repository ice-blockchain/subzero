// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/google/uuid"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip13"
	"github.com/nbd-wtf/go-nostr/nip19"
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

func helperRegisterWSSubscriptionListenerWithStorage(t *testing.T, storedEvents *[]*model.Event) {
	t.Helper()

	RegisterWSSubscriptionListener(func(ctx context.Context, subscription *model.Subscription) EventIterator {
		if subscription == nil || len(subscription.Filters) == 0 {
			return helperNewIterator(t, *storedEvents)
		}

		var filteredEvents []*model.Event
		for i := range *storedEvents {
			ev := (*storedEvents)[i]
			if subscription.Filters.Match(&ev.Event) {
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

func TestRelayEventsBroadcastMultipleSubs(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), testDeadline)
	defer cancel()
	privkey := model.GeneratePrivateKey()
	storedEvents := []*model.Event{{Event: nostr.Event{
		CreatedAt: nostr.Now(),
		Kind:      nostr.KindTextNote,
		Content:   "db event",
	}}}
	RegisterWSSubscriptionListener(func(context.Context, *model.Subscription) EventIterator {
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

func TestPublishingNIP09Events(t *testing.T) {
	privkey := model.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	helperRegisterWSEventListenerProxyWithStorage(t, &storedEvents)
	ctx := t.Context()
	relay := helperMustNewRelay(t, pubsubServers[0])

	var validEventNIP09WithEKTags, validEventNIP09AllTags, validEventAccountDelete *model.Event
	t.Run("kind 5 (Deletion) (NIP-05): valid event with e/k tag", func(t *testing.T) {
		validEventNIP09WithEKTags = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindDeletion,
			Tags: model.Tags{
				{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"},
				{"k", "1"},
			},
			Content: "Deletion reason",
		}}
		helperSignWithMinLeadingZeroBits(t, validEventNIP09WithEKTags, privkey)
		require.NoError(t, relay.Publish(ctx, validEventNIP09WithEKTags.Event))
	})
	t.Run("kind 5 (Deletion) (NIP-05): valid event with all tag", func(t *testing.T) {
		validEventNIP09AllTags = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindDeletion,
			Tags: model.Tags{
				{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"},
				{"k", "1"},
				{"a", "1:foo:"},
			},
			Content: "Deletion reason",
		}}
		helperSignWithMinLeadingZeroBits(t, validEventNIP09AllTags, privkey)
		require.NoError(t, relay.Publish(ctx, validEventNIP09AllTags.Event))
	})
	t.Run("kind 5 (Deletion) (NIP-05): account deletion", func(t *testing.T) {
		validEventAccountDelete = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindDeletion,
			Content:   "account deletion reason",
		}}
		helperSignWithMinLeadingZeroBits(t, validEventAccountDelete, privkey)
		require.NoError(t, relay.Publish(ctx, validEventAccountDelete.Event))
	})
	t.Run("kind 5 (Deletion) (NIP-05): invalid event, mismatch e -> k tags", func(t *testing.T) {
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindDeletion,
			Tags: model.Tags{
				{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"},
			},
			Content: "Deletion reason",
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("kind 5 (Deletion) (NIP-05): unsupported tag", func(t *testing.T) {
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindDeletion,
			Tags: model.Tags{
				{"r", "wss://relay.example.com"},
			},
			Content: "Deletion reason",
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})

	helperMustCloseRelay(t, relay)
	require.ElementsMatch(t, []*model.Event{validEventNIP09WithEKTags, validEventAccountDelete, validEventNIP09AllTags}, storedEvents)
}

func TestPublishingNIP09Events_NoEvent(t *testing.T) {
	privkey, pubkey := model.GenerateKeyPair()

	RegisterWSSubscriptionListener(func(ctx context.Context, s *model.Subscription) EventIterator {
		return query.GetStoredEvents(ctx, s)
	})
	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		require.True(t, len(events) > 0)
		require.NoError(t, query.AcceptEvents(ctx, events...))

		return nil
	})
	ctx := t.Context()
	relay := helperMustNewRelay(t, pubsubServers[0])

	var deletionEvent *model.Event
	t.Run("kind 5 (Deletion) (NIP-05): no such event", func(t *testing.T) {
		deletionEvent = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindDeletion,
			Tags: model.Tags{
				{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"},
				{"k", "1"},
				{model.CustomIONTagOnBehalfOf, pubkey},
			},
			Content: "Deletion reason",
		}}
		helperSignWithMinLeadingZeroBits(t, deletionEvent, privkey)
		require.NoError(t, relay.Publish(ctx, deletionEvent.Event))
	})
	var validKind01NIP10Event *model.Event
	t.Run("kind 1 (NIP-10): valid", func(t *testing.T) {
		validKind01NIP10Event = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: model.Tags{
				{"e", "", "relay", "reply"},
				{"p", "pubkey1", "pubkey2"},
				{model.CustomIONTagOnBehalfOf, pubkey},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, validKind01NIP10Event, privkey)
		require.NoError(t, relay.Publish(ctx, validKind01NIP10Event.Event))
	})
	t.Run("kind 5 (Deletion) (NIP-05): delete event that exists", func(t *testing.T) {
		deletionEvent = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindDeletion,
			Tags: model.Tags{
				{"e", validKind01NIP10Event.ID, "wss://relay.example.com"},
				{"k", strconv.Itoa(validKind01NIP10Event.Kind)},
				{model.CustomIONTagOnBehalfOf, pubkey},
			},
			Content: "Deletion reason",
		}}
		helperSignWithMinLeadingZeroBits(t, deletionEvent, privkey)
		require.NoError(t, relay.Publish(ctx, deletionEvent.Event))
	})
	t.Run("kind 5 (Deletion) (NIP-05): delete event one more time again", func(t *testing.T) {
		deletionEvent = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindDeletion,
			Tags: model.Tags{
				{"e", validKind01NIP10Event.ID, "wss://relay.example.com"},
				{"k", strconv.Itoa(validKind01NIP10Event.Kind)},
				{model.CustomIONTagOnBehalfOf, pubkey},
			},
			Content: "Deletion reason",
		}}
		helperSignWithMinLeadingZeroBits(t, deletionEvent, privkey)
		require.NoError(t, relay.Publish(ctx, deletionEvent.Event))
	})
	t.Run("kind 5 (Deletion) (NIP-05): delete user account", func(t *testing.T) {
		deletionEvent = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindDeletion,
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, pubkey},
			},
			Content: "Remebmer me",
		}}
		helperSignWithMinLeadingZeroBits(t, deletionEvent, privkey)
		require.NoError(t, relay.Publish(ctx, deletionEvent.Event))
	})

	helperMustCloseRelay(t, relay)
}

func TestPublishingNIP10Events(t *testing.T) {
	privkey := model.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	helperRegisterWSEventListenerProxyWithStorage(t, &storedEvents)
	ctx := t.Context()
	relay := helperMustNewRelay(t, pubsubServers[0])

	t.Run("kind 1 (NIP-10): e tags required params", func(t *testing.T) {
		inValidKind01Event := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags:      nostr.Tags{[]string{"e"}},
		}}
		helperSignWithMinLeadingZeroBits(t, inValidKind01Event, privkey)
		require.Error(t, relay.Publish(ctx, inValidKind01Event.Event))
	})

	t.Run("kind 1 (NIP-10): invalid reply marker for e tags ", func(t *testing.T) {
		inValidKind01Event := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags:      nostr.Tags{[]string{"e", "", "relay", "invalid marker"}},
		}}
		helperSignWithMinLeadingZeroBits(t, inValidKind01Event, privkey)
		require.Error(t, relay.Publish(ctx, inValidKind01Event.Event))
	})
	t.Run("kind 1 (NIP-10): invalid p tag usage: no e tags", func(t *testing.T) {
		inValidKind01Event := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags:      nostr.Tags{[]string{"p", "pubkey1", "pubkey2"}},
		}}
		helperSignWithMinLeadingZeroBits(t, inValidKind01Event, privkey)
		require.Error(t, relay.Publish(ctx, inValidKind01Event.Event))
	})
	t.Run("kind 1 (NIP-10): invalid p tag usage: empty tag values", func(t *testing.T) {
		inValidKind01Event := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags:      nostr.Tags{[]string{"e", "", "relay", "reply"}, []string{"p"}},
		}}
		helperSignWithMinLeadingZeroBits(t, inValidKind01Event, privkey)
		require.Error(t, relay.Publish(ctx, inValidKind01Event.Event))
	})

	var validKind01NIP10Event *model.Event
	t.Run("kind 1 (NIP-10): valid", func(t *testing.T) {
		validKind01NIP10Event = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags:      nostr.Tags{[]string{"e", "", "relay", "reply"}, []string{"p", "pubkey1", "pubkey2"}},
		}}
		helperSignWithMinLeadingZeroBits(t, validKind01NIP10Event, privkey)
		require.NoError(t, relay.Publish(ctx, validKind01NIP10Event.Event))
	})

	helperMustCloseRelay(t, relay)
	require.Equal(t, []*model.Event{validKind01NIP10Event}, storedEvents)
}

func TestPublishingNIP23Events(t *testing.T) {
	privkey := model.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	helperRegisterWSEventListenerProxyWithStorage(t, &storedEvents)
	ctx := t.Context()
	relay := helperMustNewRelay(t, pubsubServers[0])

	var validEventKindArticle, validEventKindBlogPost *model.Event
	t.Run("kind 30023 (Article) (NIP-23): valid event", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"a", "30023:a695f6b60119d9521934a691347d9f78e8770b56da16bb255ee286ddf9fda919:ipsum", "wss://relay.nostr.org"})
		tags = append(tags, nostr.Tag{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"})
		tags = append(tags, nostr.Tag{"t", "placeholder"})
		tags = append(tags, nostr.Tag{"published_at", "1296962229"})
		tags = append(tags, nostr.Tag{"title", "Lorem Ipsum"})
		tags = append(tags, nostr.Tag{"d", "lorem-ipsum"})
		validEventKindArticle = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindDraftArticle,
			Tags:      tags,
			Content:   "Lorem [ipsum][nostr:nevent1qqst8cujky046negxgwwm5ynqwn53t8aqjr6afd8g59nfqwxpdhylpcpzamhxue69uhhyetvv9ujuetcv9khqmr99e3k7mg8arnc9] dolor sit amet",
		}}
		helperSignWithMinLeadingZeroBits(t, validEventKindBlogPost, privkey)
		require.NoError(t, relay.Publish(ctx, validEventKindBlogPost.Event))
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
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindArticle,
			Tags:      tags,
			Content:   "Lorem [ipsum][nostr:nevent1qqst8cujky046negxgwwm5ynqwn53t8aqjr6afd8g59nfqwxpdhylpcpzamhxue69uhhyetvv9ujuetcv9khqmr99e3k7mg8arnc9] dolor sit amet",
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	helperMustCloseRelay(t, relay)
	require.Equal(t, []*model.Event{validEventKindArticle, validEventKindBlogPost}, storedEvents)
}

func TestPublishingNIP01NIP24Events(t *testing.T) {
	privkey := model.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	helperRegisterWSEventListenerProxyWithStorage(t, &storedEvents)
	ctx := t.Context()
	relay := helperMustNewRelay(t, pubsubServers[0])

	var validEventNIP01, validEventNIP24 *model.Event
	t.Run("kind 0 (ProfileMetadata) (NIP-01): valid event", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"a", "30023:a695f6b60119d9521934a691347d9f78e8770b56da16bb255ee286ddf9fda919:ipsum", "wss://relay.nostr.org"})
		tags = append(tags, nostr.Tag{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"})
		tags = append(tags, nostr.Tag{"p", "pubkey1"})
		tags = append(tags, nostr.Tag{"alt", "reply"})
		validEventNIP01 = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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

func TestPublishingNIP32LabelingEvents(t *testing.T) {
	privkey := model.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	helperRegisterWSEventListenerProxyWithStorage(t, &storedEvents)
	ctx := t.Context()
	relay := helperMustNewRelay(t, pubsubServers[0])

	var validLabelingEvent, validUGCLabelingEvent *model.Event
	t.Run("kind 1985 (Labeling) (NIP-32): valid labeling event", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"})
		tags = append(tags, nostr.Tag{"L", "#t"})
		tags = append(tags, nostr.Tag{"l", "permies", "#t"})
		validLabelingEvent = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindLabel,
			Tags:      tags,
			Content:   "Some label long description",
		}}
		helperSignWithMinLeadingZeroBits(t, invalidLabelingEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidLabelingEvent.Event))
	})
	t.Run("kind 1 (NIP-32): invalid label", func(t *testing.T) {
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags:      nostr.Tags{[]string{"e", "", "relay", "reply"}, []string{"p", "pubkey1", "pubkey2"}, []string{"l", "permies", "#t"}, []string{"L", "#a"}},
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	var validKind01EventWithLabels *model.Event
	t.Run("kind 1 (NIP-32): valid with label", func(t *testing.T) {
		validKind01EventWithLabels = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
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
	ctx := t.Context()
	relay := helperMustNewRelay(t, pubsubServers[0])

	var validReportEventWithPTagOnly *model.Event
	t.Run("kind 1984 (Report) (NIP-56): valid report event with p tag only", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"p", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", model.TagReportTypeNudity})
		validReportEventWithPTagOnly = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
	ctx := t.Context()
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
	ctx := t.Context()
	relay := helperMustNewRelay(t, pubsubServers[0])

	var validRelayListEvent *model.Event
	t.Run("kind 10002 (Relay list) (NIP-65): valid relay list", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"r", "wss://alicerelay.example.com"})
		tags = append(tags, nostr.Tag{"r", "wss://brando-relay.com"})
		tags = append(tags, nostr.Tag{"r", "wss://expensive-relay.example2.com", "write"})
		tags = append(tags, nostr.Tag{"r", "wss://nostr-relay.example.com", "read"})
		validRelayListEvent = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
	ctx := t.Context()
	relay := helperMustNewRelay(t, pubsubServers[0])

	var validEvents []*model.Event
	t.Run("Kind 10000 (NIP-51) valid", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"p", "pubkey"})
		tags = append(tags, nostr.Tag{"t", "hash"})
		tags = append(tags, nostr.Tag{"e", "event"})
		tags = append(tags, nostr.Tag{"word", "dummy"})
		validEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindBookmarkList,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, invalidEvent, privkey)
		require.Error(t, relay.Publish(ctx, invalidEvent.Event))
	})
	t.Run("Kind 10004 (NIP-51) valid", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:dummy:dummy", model.CustomIONKindCommunityDefinition)})
		validEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindCommunityList,
			Tags:      tags,
		}}
		helperSignWithMinLeadingZeroBits(t, validEvent, privkey)
		validEvents = append(validEvents, validEvent)
		require.NoError(t, relay.Publish(ctx, validEvent.Event))
	})
	t.Run("Kind 10003 (NIP-51): unsupported tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:dummy:dummy", model.CustomIONKindCommunityDefinition)})
		tags = append(tags, nostr.Tag{"wrong", "dummy"})
		invalidEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
	privkey, pubkey := model.GenerateKeyPair()
	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		t.Logf("received events: %v", events)
		return query.AcceptEvents(ctx, events...)
	})

	ctx := t.Context()
	relay := helperMustNewRelay(t, pubsubServers[0])

	t.Run("SaveEvent", func(t *testing.T) {
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			PubKey:    pubkey,
			Tags:      nil,
			Content:   "validEvent",
		}}

		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("CountEvents", func(t *testing.T) {
		c, err := relay.Count(ctx, nostr.Filters{{Kinds: []int{nostr.KindTextNote}, Search: "test", Authors: []string{pubkey}}})
		require.NoError(t, err)
		require.Equal(t, int64(1), c)
	})
	helperMustCloseRelay(t, relay)
}

func helperSignWithMinLeadingZeroBits(t testing.TB, event *model.Event, privkey string) {
	t.Helper()
	require.NoError(t, event.SignWithAlg(privkey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, event.GenerateNIP13(t.Context(), NIP13MinLeadingZeroBits))
	require.NoError(t, event.SignWithAlg(privkey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
}

func TestPublishingNIP92IMetaTag(t *testing.T) {
	privkey := model.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	helperRegisterWSEventListenerProxyWithStorage(t, &storedEvents)
	ctx := t.Context()
	relay := helperMustNewRelay(t, pubsubServers[0])

	var validEvents []*model.Event
	t.Run("kind 1 (text note), imeta (NIP-92): valid imeta tag", func(t *testing.T) {
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{
			"imeta",
			"url https://alicerelay.example.com",
			"m image/jpg",
			"dim 3024x4032",
			"alt A scenic photo overlooking the coast of Costa Rica",
			fmt.Sprintf("ox %x", []byte("https://alicerelay.example.com")),
		})
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
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
			"dim 3024x4032",
			"alt A scenic photo overlooking the coast of Costa Rica",
			fmt.Sprintf("ox %x", []byte("https://alicerelay.example.com")),
			"dummy dummy",
		})
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			CreatedAt: nostr.Now(),
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
			"dim 3024",
			"alt A scenic photo overlooking the coast of Costa Rica",
			fmt.Sprintf("ox %x", []byte("https://alicerelay.example.com")),
		})
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
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
			"alt A scenic photo overlooking the coast of Costa Rica",
			"ox a",
		})
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
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

func TestWhoCanReplySettings_FollowingSettings(t *testing.T) {
	privkeyPostOwner, _ := model.GenerateKeyPair()
	privkeyUser1, pubkeyUser1 := model.GenerateKeyPair()
	privkeyUser2, _ := model.GenerateKeyPair()
	RegisterWSSubscriptionListener(func(ctx context.Context, s *model.Subscription) EventIterator {
		return query.GetStoredEvents(ctx, s)
	})
	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		require.True(t, len(events) > 0)
		require.NoError(t, query.AcceptEvents(ctx, events...))

		return nil
	})
	ctx := t.Context()
	relay := helperMustNewRelay(t, pubsubServers[0])

	var post *model.Event
	t.Run("create post with following settings", func(t *testing.T) {
		post = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: nostr.Tags{
				{"settings", model.WhoCanReplySettings, model.FollowingWhoCanReplySettings, strconv.FormatInt(time.Now().Unix(), 10)},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, post, privkeyPostOwner)
		require.NoError(t, relay.Publish(ctx, post.Event))
	})
	t.Run("create followers list for post owner", func(t *testing.T) {
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindFollowList,
			Tags: nostr.Tags{
				{"p", pubkeyUser1, "", "alice"},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyPostOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("create reply for the initial post by user1", func(t *testing.T) {
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: nostr.Tags{
				{"e", post.GetID(), "", model.TagMarkerRoot},
				{"p", post.GetMasterPublicKey(), pubkeyUser1},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser1)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("create reply for the initial post by user2 that is not in the followers list, forbidden", func(t *testing.T) {
		post = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: nostr.Tags{
				{"e", post.ID, "", model.TagMarkerRoot},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, post, privkeyUser2)
		require.Error(t, relay.Publish(ctx, post.Event))
	})
	helperMustCloseRelay(t, relay)
}

func TestWhoCanReplySettings_MentionedSettings(t *testing.T) {
	privkeyPostOwner, _ := model.GenerateKeyPair()
	privkeyUser1, pubkeyUser1 := model.GenerateKeyPair()
	privkeyUser2, _ := model.GenerateKeyPair()
	RegisterWSSubscriptionListener(func(ctx context.Context, s *model.Subscription) EventIterator {
		return query.GetStoredEvents(ctx, s)
	})
	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		require.True(t, len(events) > 0)
		require.NoError(t, query.AcceptEvents(ctx, events...))

		return nil
	})
	ctx := t.Context()
	relay := helperMustNewRelay(t, pubsubServers[0])

	var post *model.Event
	t.Run("create post with mentioned settings", func(t *testing.T) {
		pkey, err := nip19.EncodePublicKey(pubkeyUser1)
		require.NoError(t, err)
		post = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Content:   fmt.Sprintf("hello world: %v", pkey),
			Tags: nostr.Tags{
				{"settings", model.WhoCanReplySettings, model.MentionWhoCanReplySettings, strconv.FormatInt(time.Now().Unix(), 10)},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, post, privkeyPostOwner)
		require.NoError(t, relay.Publish(ctx, post.Event))
	})
	t.Run("create reply for the initial post by user1", func(t *testing.T) {
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: nostr.Tags{
				{"e", post.GetID(), "", model.TagMarkerRoot},
				{"p", post.GetMasterPublicKey(), pubkeyUser1},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser1)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("create reply for the initial post by user2 that was not mentioned, forbidden", func(t *testing.T) {
		post = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: nostr.Tags{
				{"e", post.ID, "", model.TagMarkerRoot},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, post, privkeyUser2)
		require.Error(t, relay.Publish(ctx, post.Event))
	})
	helperMustCloseRelay(t, relay)
}

func TestWhoCanReplySettings_BadgeSettings(t *testing.T) {
	privkeyPostOwner, pubkeyPostOwner := model.GenerateKeyPair()
	privkeyUser1, pubkeyUser1 := model.GenerateKeyPair()
	privkeyUser2, _ := model.GenerateKeyPair()
	RegisterWSSubscriptionListener(func(ctx context.Context, s *model.Subscription) EventIterator {
		return query.GetStoredEvents(ctx, s)
	})
	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		require.True(t, len(events) > 0)
		require.NoError(t, query.AcceptEvents(ctx, events...))

		return nil
	})
	ctx := t.Context()
	relay := helperMustNewRelay(t, pubsubServers[0])

	var post *model.Event
	dBadgeTagVal := "bravery"
	t.Run("create post with badge settings", func(t *testing.T) {
		pkey, err := nip19.EncodePublicKey(pubkeyUser1)
		require.NoError(t, err)
		post = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Content:   fmt.Sprintf("hello world: %v", pkey),
			Tags: nostr.Tags{
				{"settings", model.WhoCanReplySettings, fmt.Sprintf("%v|%v:%v:%v", model.BadgeWhoCanReplySettingsPrefix, nostr.KindBadgeDefinition, pubkeyPostOwner, dBadgeTagVal), strconv.FormatInt(time.Now().Unix(), 10)},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, post, privkeyPostOwner)
		require.NoError(t, relay.Publish(ctx, post.Event))
	})
	var defineBraveryBadgeEv *model.Event
	t.Run("define bravery badge", func(t *testing.T) {
		defineBraveryBadgeEv = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindBadgeDefinition,
			Tags: nostr.Tags{
				{"d", dBadgeTagVal},
				{"name", "Medal of Bravery"},
				{"description", "Awarded to users demonstrating bravery"},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, defineBraveryBadgeEv, privkeyPostOwner)
		require.NoError(t, relay.Publish(ctx, defineBraveryBadgeEv.Event))
	})
	var awardEvent *model.Event
	t.Run("award user1 by bravery badge", func(t *testing.T) {
		awardEvent = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindBadgeAward,
			Tags: nostr.Tags{
				{"a", fmt.Sprintf("%v:%v:%v", nostr.KindBadgeDefinition, pubkeyPostOwner, dBadgeTagVal)},
				{"p", pubkeyUser1, ""},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, awardEvent, privkeyPostOwner)
		require.NoError(t, relay.Publish(ctx, awardEvent.Event))
	})
	t.Run("profile badges event", func(t *testing.T) {
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindProfileBadges,
			Tags: nostr.Tags{
				{"a", fmt.Sprintf("%v:%v:%v", nostr.KindBadgeDefinition, pubkeyPostOwner, dBadgeTagVal)},
				{"e", awardEvent.GetMasterPublicKey(), ""},
				{"d", "profile_badges"},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser1)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})

	t.Run("create reply for the initial post by user1", func(t *testing.T) {
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: nostr.Tags{
				{"e", post.GetID(), "", model.TagMarkerRoot},
				{"p", post.GetMasterPublicKey(), pubkeyUser1},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser1)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("create reply for the initial post by user2 that doesn't have badge, forbidden", func(t *testing.T) {
		post = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: nostr.Tags{
				{"e", post.ID, "", model.TagMarkerRoot},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, post, privkeyUser2)
		require.Error(t, relay.Publish(ctx, post.Event))
	})
	helperMustCloseRelay(t, relay)
}

func TestWhoCanReplySettings_ComplexSettings(t *testing.T) {
	privkeyPostOwner, pubkeyPostOwner := model.GenerateKeyPair()
	privkeyUser1, pubkeyUser1 := model.GenerateKeyPair()
	privkeyUser2, pubkeyUser2 := model.GenerateKeyPair()
	_, pubkeyUser3 := model.GenerateKeyPair()
	privkeyUser4, _ := model.GenerateKeyPair()
	RegisterWSSubscriptionListener(func(ctx context.Context, s *model.Subscription) EventIterator {
		return query.GetStoredEvents(ctx, s)
	})
	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		require.True(t, len(events) > 0)
		require.NoError(t, query.AcceptEvents(ctx, events...))

		return nil
	})
	ctx := t.Context()
	relay := helperMustNewRelay(t, pubsubServers[0])

	var post *model.Event
	dBadgeTagVal := "bravery"
	t.Run("create post with complex settings", func(t *testing.T) {
		settingsConfiguration := fmt.Sprintf("%v,%v,%v|%v:%v:%v", model.FollowingWhoCanReplySettings, model.MentionWhoCanReplySettings, model.BadgeWhoCanReplySettingsPrefix, nostr.KindBadgeDefinition, pubkeyPostOwner, dBadgeTagVal)
		pkey, err := nip19.EncodePublicKey(pubkeyUser3)
		require.NoError(t, err)
		post = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Content:   fmt.Sprintf("hello world: %v", pkey),
			Tags: nostr.Tags{
				{"settings", model.WhoCanReplySettings, settingsConfiguration, strconv.FormatInt(time.Now().Unix(), 10)},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, post, privkeyPostOwner)
		require.NoError(t, relay.Publish(ctx, post.Event))
	})
	t.Run("create followers list for post owner", func(t *testing.T) {
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindFollowList,
			Tags: nostr.Tags{
				{"p", pubkeyUser1, "", "alice"},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyPostOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	var defineBraveryBadgeEv *model.Event
	t.Run("define bravery badge", func(t *testing.T) {
		defineBraveryBadgeEv = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindBadgeDefinition,
			Tags: nostr.Tags{
				{"d", dBadgeTagVal},
				{"name", "Medal of Bravery"},
				{"description", "Awarded to users demonstrating bravery"},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, defineBraveryBadgeEv, privkeyPostOwner)
		require.NoError(t, relay.Publish(ctx, defineBraveryBadgeEv.Event))
	})
	var awardEvent *model.Event
	t.Run("award user2 by bravery badge", func(t *testing.T) {
		awardEvent = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindBadgeAward,
			Tags: nostr.Tags{
				{"a", fmt.Sprintf("%v:%v:%v", nostr.KindBadgeDefinition, pubkeyPostOwner, dBadgeTagVal)},
				{"p", pubkeyUser2, ""},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, awardEvent, privkeyPostOwner)
		require.NoError(t, relay.Publish(ctx, awardEvent.Event))
	})
	t.Run("profile badges event", func(t *testing.T) {
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindProfileBadges,
			Tags: nostr.Tags{
				{"a", fmt.Sprintf("%v:%v:%v", nostr.KindBadgeDefinition, pubkeyPostOwner, dBadgeTagVal)},
				{"e", awardEvent.GetMasterPublicKey(), ""},
				{"d", "profile_badges"},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser2)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("create reply for the initial post by user1 followed, ok", func(t *testing.T) {
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: nostr.Tags{
				{"e", post.GetID(), "", model.TagMarkerRoot},
				{"p", post.GetMasterPublicKey(), pubkeyUser1},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser1)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("create reply for the initial post by user2 badge awarded, ok", func(t *testing.T) {
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: nostr.Tags{
				{"e", post.GetID(), "", model.TagMarkerRoot},
				{"p", post.GetMasterPublicKey(), pubkeyUser2},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser2)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("create reply for the initial post by user3 mentioned, ok", func(t *testing.T) {
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: nostr.Tags{
				{"e", post.GetID(), "", model.TagMarkerRoot},
				{"p", post.GetMasterPublicKey(), pubkeyUser2},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser2)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("create reply for the initial post by user4, forbidden", func(t *testing.T) {
		post = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: nostr.Tags{
				{"e", post.GetID(), "", model.TagMarkerRoot},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, post, privkeyUser4)
		require.Error(t, relay.Publish(ctx, post.Event))
	})
	helperMustCloseRelay(t, relay)
}

func TestWhoCanReplySettings_ModifiableEvent(t *testing.T) {
	privkeyPostOwner, _ := model.GenerateKeyPair()
	privkeyUser1, pubkeyUser1 := model.GenerateKeyPair()
	privkeyUser2, _ := model.GenerateKeyPair()
	RegisterWSSubscriptionListener(func(ctx context.Context, s *model.Subscription) EventIterator {
		return query.GetStoredEvents(ctx, s)
	})
	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		require.True(t, len(events) > 0)
		require.NoError(t, query.AcceptEvents(ctx, events...))

		return nil
	})
	ctx := t.Context()
	relay := helperMustNewRelay(t, pubsubServers[0])

	var post *model.Event
	t.Run("create post with following settings", func(t *testing.T) {
		post = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      model.CustomIONKindEditableTextNote,
			Tags: nostr.Tags{
				{"settings", model.WhoCanReplySettings, model.FollowingWhoCanReplySettings, strconv.FormatInt(time.Now().Unix(), 10)},
				{"published_at", "1296962229"},
				{"d", "dummy"},
			},
			Content: "dummy",
		}}
		helperSignWithMinLeadingZeroBits(t, post, privkeyPostOwner)
		require.NoError(t, relay.Publish(ctx, post.Event))
	})
	t.Run("create followers list for post owner", func(t *testing.T) {
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindFollowList,
			Tags: nostr.Tags{
				{"p", pubkeyUser1, "", "alice"},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyPostOwner)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	var reply *model.Event
	t.Run("create reply for the initial post by user1", func(t *testing.T) {
		reply = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      model.CustomIONKindEditableTextNote,
			Tags: nostr.Tags{
				{"a", post.Address(), "", model.TagMarkerRoot},
				{"p", post.GetMasterPublicKey(), pubkeyUser1},
				{"published_at", "1296962229"},
				{"d", "dummy"},
			},
			Content: "dummy reply",
		}}
		helperSignWithMinLeadingZeroBits(t, reply, privkeyUser1)
		require.NoError(t, relay.Publish(ctx, reply.Event))
	})
	t.Run("create reply of reply for the initial post by user1", func(t *testing.T) {
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      model.CustomIONKindEditableTextNote,
			Tags: nostr.Tags{
				{"a", reply.Address(), "", model.TagMarkerReply},
				{"a", post.Address(), "", model.TagMarkerRoot},
				{"p", post.GetMasterPublicKey(), pubkeyUser1},
				{"published_at", "1296962229"},
				{"d", "dummy"},
			},
			Content: "dummy reply of reply",
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser1)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("create reply of reply by user1 with missed `a` root tag: no root tag, we don't know settings, then ok", func(t *testing.T) {
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      model.CustomIONKindEditableTextNote,
			Tags: nostr.Tags{
				{"a", reply.Address(), "", model.TagMarkerReply},
				{"p", post.GetMasterPublicKey(), pubkeyUser1},
				{"published_at", "1296962229"},
				{"d", "dummy"},
			},
			Content: "dummy reply of reply",
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser1)
		require.NoError(t, relay.Publish(ctx, ev.Event))
	})
	t.Run("create reply for the initial post by user2 that is not in the followers list, forbidden", func(t *testing.T) {
		post = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      model.CustomIONKindEditableTextNote,
			Tags: nostr.Tags{
				{"a", post.Address(), "", model.TagMarkerRoot},
				{"published_at", "1296962229"},
				{"d", "dummy"},
			},
			Content: "dummy reply",
		}}
		helperSignWithMinLeadingZeroBits(t, post, privkeyUser2)
		require.Error(t, relay.Publish(ctx, post.Event))
	})
	helperMustCloseRelay(t, relay)
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

func TestSubscriptionMostRelevantFollowers(t *testing.T) {
	t.Cleanup(func() {
		RegisterReqMustAuthenticate(nil)
		RegisterEventMustAuthenticate(nil)
	})

	privKey, pubKey := model.GenerateKeyPair()
	RegisterWSEventListener(func(context.Context, ...*model.Event) error {
		return nil
	})
	RegisterWSSubscriptionListener(func(ctx context.Context, subscription *model.Subscription) EventIterator {
		t.Logf("subscription: %v: %s", subscription.SubscriptionID, subscription.Filters.String())
		require.Len(t, subscription.Filters, 1)
		require.Len(t, subscription.Filters[0].Kinds, 1)
		require.Equal(t, nostr.KindFollowList, subscription.Filters[0].Kinds[0])
		require.Contains(t, subscription.Filters[0].Authors, pubKey)
		require.Equal(t, `include:dependencies:kind3>kind0+p+|foo,bar|`, subscription.Filters[0].Search)

		return query.GetStoredEvents(ctx, subscription)
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

func TestFiltersMatchWithMasterKey(t *testing.T) {
	t.Parallel()

	privkey, pubkey := model.GenerateKeyPair()
	masterPriv, masterPubkey := model.GenerateKeyPair()

	createEvent := func(kind int, tags model.Tags, pk string) *model.Event {
		ev := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      kind,
			Tags:      tags,
			Content:   "test content",
		}}
		require.NoError(t, ev.SignWithAlg(pk, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		return ev
	}

	t.Run("direct match - event kind", func(t *testing.T) {
		ev := createEvent(nostr.KindTextNote, nil, privkey)
		filters := model.Filters{{Kinds: []int{nostr.KindTextNote}}}

		result := filtersMatchWithMasterKey(filters, ev, masterPubkey, pubkey)
		require.True(t, result)
	})

	t.Run("direct match - event tag", func(t *testing.T) {
		ev := createEvent(nostr.KindTextNote, model.Tags{{"t", "test"}}, privkey)
		filters := model.Filters{{Tags: model.TagMap{}.Set("t", model.PointerOf("test"))}}

		result := filtersMatchWithMasterKey(filters, ev, masterPubkey, pubkey)
		require.True(t, result)
	})

	t.Run("master key match", func(t *testing.T) {
		ev := createEvent(nostr.KindTextNote, model.Tags{{model.CustomIONTagOnBehalfOf, masterPubkey}}, masterPriv)

		filters := model.Filters{{
			Authors: []string{pubkey},
			Kinds:   []int{nostr.KindTextNote},
		}}

		result := filtersMatchWithMasterKey(filters, ev, masterPubkey, pubkey)
		require.True(t, result)
	})

	t.Run("no match", func(t *testing.T) {
		ev := createEvent(nostr.KindTextNote, nil, privkey)
		filters := model.Filters{{Kinds: []int{nostr.KindArticle}}}

		result := filtersMatchWithMasterKey(filters, ev, masterPubkey, pubkey)
		require.False(t, result)
	})

	t.Run("device key not in authors", func(t *testing.T) {
		ev := createEvent(nostr.KindTextNote, model.Tags{{model.CustomIONTagOnBehalfOf, masterPubkey}}, privkey)
		filters := model.Filters{{
			Authors: []string{"different_key"},
			Kinds:   []int{nostr.KindTextNote},
		}}

		result := filtersMatchWithMasterKey(filters, ev, masterPubkey, pubkey)
		require.False(t, result)
	})

	t.Run("master key substitution match", func(t *testing.T) {
		// Create an event with the master key tag
		ev := createEvent(nostr.KindTextNote, model.Tags{{model.CustomIONTagOnBehalfOf, masterPubkey}}, masterPriv)

		// Create a filter with device key that should match after substitution
		filters := model.Filters{{
			Authors: []string{pubkey},
		}}

		result := filtersMatchWithMasterKey(filters, ev, masterPubkey, pubkey)
		require.True(t, result)
	})

	t.Run("complex filters with both matches", func(t *testing.T) {
		ev := createEvent(nostr.KindTextNote, model.Tags{{"t", "test"}, {model.CustomIONTagOnBehalfOf, masterPubkey}}, masterPriv)

		filters := model.Filters{
			{Kinds: []int{nostr.KindArticle}},                        // No match
			{Tags: model.TagMap{}.Set("t", model.PointerOf("test"))}, // Direct match
			{Authors: []string{pubkey}},                              // Master key match
		}

		result := filtersMatchWithMasterKey(filters, ev, masterPubkey, pubkey)
		require.True(t, result)
	})
}
