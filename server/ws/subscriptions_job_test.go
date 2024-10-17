// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"context"
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/dvm"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/server/ws/fixture"
)

func TestJob(t *testing.T) {
	query.MustInit()
	privkey := nostr.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	serviceProviderPrivKey := nostr.GeneratePrivateKey()
	serivceProviderPubKey, err := nostr.GetPublicKey(serviceProviderPrivKey)
	require.NoError(t, err)
	dataVendingMachine := dvm.NewDvms(NIP13MinLeadingZeroBits, serviceProviderPrivKey, true)
	RegisterWSSubscriptionListener(func(context.Context, *model.Subscription) query.EventIterator {
		return func(yield func(*model.Event, error) bool) {
			for i := range storedEvents {
				if !yield(storedEvents[i], nil) {
					return
				}
			}
		}
	})
	RegisterWSEventListener(func(ctx context.Context, event *model.Event) error {
		require.NoError(t, dataVendingMachine.AcceptJob(ctx, event))
		for _, sEvent := range storedEvents {
			if sEvent.ID == event.ID {
				return model.ErrDuplicate
			}
		}
		assert.False(t, event.IsEphemeral())
		storedEvents = append(storedEvents, event)

		return nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
	relay, err := fixture.NewRelayClient(ctx, "wss://localhost:9998")
	if err != nil {
		log.Panic(err)
	}
	var (
		postID               = uuid.NewString()
		pubKeyOfRepostedNote = "pubkey1"
		repostedKind         = nostr.KindArticle
		expectedEvents       []*model.Event
		jobEvents            []*model.Event
	)

	relayToSearchResult, err := fixture.NewRelayClient(ctx, "wss://localhost:9997")
	require.NoError(t, err)
	t.Run("send reaction 1", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindReaction,
				Tags:      nostr.Tags{[]string{"e", postID, "relay"}, []string{"p", pubKeyOfRepostedNote}, []string{"k", fmt.Sprint(repostedKind)}},
				Content:   "+",
				Sig:       uuid.NewString(),
			},
		}
		ev.SetExtra("extra", uuid.NewString())
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relayToSearchResult.Publish(ctx, ev.Event))
		expectedEvents = append(expectedEvents, ev)
	})
	t.Run("send reaction 2", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindReaction,
				Tags:      nostr.Tags{[]string{"e", postID, "relay"}, []string{"p", pubKeyOfRepostedNote}, []string{"k", fmt.Sprint(repostedKind)}},
				Content:   "-",
				Sig:       uuid.NewString(),
			},
		}
		ev.SetExtra("extra", uuid.NewString())
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relayToSearchResult.Publish(ctx, ev.Event))
		expectedEvents = append(expectedEvents, ev)
	})
	t.Run("send article", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindArticle,
				Tags:      nostr.Tags{[]string{"title", "dummy"}},
				Content:   "dummy content",
				Sig:       uuid.NewString(),
			},
		}
		ev.SetExtra("extra", uuid.NewString())
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relayToSearchResult.Publish(ctx, ev.Event))
		expectedEvents = append(expectedEvents, ev)
	})
	t.Run("send article", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindArticle,
				Tags:      nostr.Tags{[]string{"title", "dummy"}},
				Content:   "dummy content",
				Sig:       uuid.NewString(),
			},
		}
		ev.SetExtra("extra", uuid.NewString())
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relayToSearchResult.Publish(ctx, ev.Event))
		expectedEvents = append(expectedEvents, ev)
	})
	time.Sleep(time.Second * 1)
	t.Run("send dvm search nostr count job for author filter", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindJobNostrEventCount,
				Tags:      nostr.Tags{[]string{"param", "relay", "wss://localhost:9997"}, []string{"p", serivceProviderPubKey}, []string{"relays", "wss://localhost:9998"}},
				Content:   fmt.Sprintf(`[{"authors":["%v"]}]`, expectedEvents[0].PubKey),
				Sig:       uuid.NewString(),
			},
		}
		ev.SetExtra("extra", uuid.NewString())
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relay.Publish(ctx, ev.Event))
		expectedEvents = append(expectedEvents, ev)
		jobEvents = append(jobEvents, ev)
	})
	t.Run("send dvm search nostr count job for kinds and #e filter groupped by pubkey", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindJobNostrEventCount,
				Tags:      nostr.Tags{[]string{"param", "relay", "wss://localhost:9997"}, []string{"p", serivceProviderPubKey}, []string{"param", "group", "pubkey"}, []string{"relays", "wss://localhost:9998"}},
				Content:   fmt.Sprintf(`[{"kinds":[%v],"#e":["%v"]}]`, nostr.KindReaction, postID),
				Sig:       uuid.NewString(),
			},
		}
		ev.SetExtra("extra", uuid.NewString())
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relay.Publish(ctx, ev.Event))
		expectedEvents = append(expectedEvents, ev)
		jobEvents = append(jobEvents, ev)
	})
	t.Run("send dvm search nostr count job for kinds and #e filter groupped by content", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindJobNostrEventCount,
				Tags:      nostr.Tags{[]string{"param", "relay", "wss://localhost:9997"}, []string{"p", serivceProviderPubKey}, []string{"param", "group", "content"}, []string{"relays", "wss://localhost:9998"}},
				Content:   fmt.Sprintf(`[{"kinds":[%v],"#e":["%v"]}]`, nostr.KindReaction, postID),
				Sig:       uuid.NewString(),
			},
		}
		ev.SetExtra("extra", uuid.NewString())
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relay.Publish(ctx, ev.Event))
		expectedEvents = append(expectedEvents, ev)
		jobEvents = append(jobEvents, ev)
	})
	t.Run("send dvm search nostr count job with 0 result for group", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindJobNostrEventCount,
				Tags:      nostr.Tags{[]string{"param", "relay", "wss://localhost:9997"}, []string{"p", serivceProviderPubKey}, []string{"param", "group", "pubkey"}, []string{"relays", "wss://localhost:9998"}},
				Content:   fmt.Sprintf(`[{"kinds":[%v],"#title":["dummy"]}]`, nostr.KindArticle),
				Sig:       uuid.NewString(),
			},
		}
		ev.SetExtra("extra", uuid.NewString())
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relay.Publish(ctx, ev.Event))
		expectedEvents = append(expectedEvents, ev)
		jobEvents = append(jobEvents, ev)
	})

	time.Sleep(time.Second * 2)
	events, err := relay.QueryEvents(ctx, nostr.Filter{Kinds: []int{6400}, Limit: 1})
	require.NoError(t, err)
	evList := make([]*nostr.Event, 0)
	for ev := range events {
		evList = append(evList, ev)
	}
	require.Equal(t, 4, len(evList))
	require.NoError(t, relay.Close())
	require.NoError(t, relayToSearchResult.Close())

	expectedResponses := []string{`{"+":1,"-":1}`, `0`, fmt.Sprintf(`{"%v":1}`, expectedEvents[0].PubKey), fmt.Sprintf(`{"%v":2}`, expectedEvents[0].PubKey)}
	for _, event := range storedEvents {
		if event.Kind == model.KindJobNostrEventCount+1000 {
			for _, jEv := range jobEvents {
				if event.Tags.GetFirst([]string{"request"}).Value() == fmt.Sprintf("%+v", jEv) {
					require.Equal(t, event.Tags.GetFirst([]string{"e"}).Value(), jEv.GetID())
					require.Equal(t, event.Tags.GetFirst([]string{"i"}).Value(), jEv.Content)
					require.Equal(t, event.Tags.GetFirst([]string{"p"}).Value(), jEv.PubKey)
				}
				require.Equal(t, event.PubKey, serivceProviderPubKey)
			}
			require.Contains(t, expectedResponses, event.Content)

			continue
		}
		require.Contains(t, expectedEvents, event)
	}
}

func TestJobDeletion(t *testing.T) {
	query.MustInit()
	privkey := nostr.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	serviceProviderPrivKey := nostr.GeneratePrivateKey()
	serivceProviderPubKey, err := nostr.GetPublicKey(serviceProviderPrivKey)
	require.NoError(t, err)
	dataVendingMachine := dvm.NewDvms(NIP13MinLeadingZeroBits, serviceProviderPrivKey, true)
	RegisterWSSubscriptionListener(func(context.Context, *model.Subscription) query.EventIterator {
		return func(yield func(*model.Event, error) bool) {
			for i := range storedEvents {
				if !yield(storedEvents[i], nil) {
					return
				}
			}
		}
	})
	RegisterWSEventListener(func(ctx context.Context, event *model.Event) error {
		require.NoError(t, dataVendingMachine.AcceptJob(ctx, event))
		for _, sEvent := range storedEvents {
			if sEvent.ID == event.ID {
				return model.ErrDuplicate
			}
		}
		assert.False(t, event.IsEphemeral())
		storedEvents = append(storedEvents, event)

		return nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
	relay, err := fixture.NewRelayClient(ctx, "wss://localhost:9998")
	if err != nil {
		log.Panic(err)
	}
	var (
		postID               = uuid.NewString()
		pubKeyOfRepostedNote = "pubkey1"
		repostedKind         = nostr.KindArticle
		expectedEvents       []*model.Event
		jobEvents            []*model.Event
	)

	relayToSearchResult, err := fixture.NewRelayClient(ctx, "wss://localhost:9997")
	require.NoError(t, err)
	t.Run("send reaction 1", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindReaction,
				Tags:      nostr.Tags{[]string{"e", postID, "relay"}, []string{"p", pubKeyOfRepostedNote}, []string{"k", fmt.Sprint(repostedKind)}},
				Content:   "+",
				Sig:       uuid.NewString(),
			},
		}
		ev.SetExtra("extra", uuid.NewString())
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relayToSearchResult.Publish(ctx, ev.Event))
		expectedEvents = append(expectedEvents, ev)
	})
	t.Run("send reaction 2", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindReaction,
				Tags:      nostr.Tags{[]string{"e", postID, "relay"}, []string{"p", pubKeyOfRepostedNote}, []string{"k", fmt.Sprint(repostedKind)}},
				Content:   "-",
				Sig:       uuid.NewString(),
			},
		}
		ev.SetExtra("extra", uuid.NewString())
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relayToSearchResult.Publish(ctx, ev.Event))
		expectedEvents = append(expectedEvents, ev)
	})
	t.Run("send article", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindArticle,
				Tags:      nostr.Tags{[]string{"title", "dummy"}},
				Content:   "dummy content",
				Sig:       uuid.NewString(),
			},
		}
		ev.SetExtra("extra", uuid.NewString())
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relayToSearchResult.Publish(ctx, ev.Event))
		expectedEvents = append(expectedEvents, ev)
	})
	t.Run("send article", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindArticle,
				Tags:      nostr.Tags{[]string{"title", "dummy"}},
				Content:   "dummy content",
				Sig:       uuid.NewString(),
			},
		}
		ev.SetExtra("extra", uuid.NewString())
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relayToSearchResult.Publish(ctx, ev.Event))
		expectedEvents = append(expectedEvents, ev)
	})
	time.Sleep(time.Second * 1)
	t.Run("send dvm search nostr count job for author filter", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindJobNostrEventCount,
				Tags:      nostr.Tags{[]string{"param", "relay", "wss://localhost:9997"}, []string{"p", serivceProviderPubKey}, []string{"relays", "wss://localhost:9998"}},
				Content:   fmt.Sprintf(`[{"authors":["%v"]}]`, expectedEvents[0].PubKey),
				Sig:       uuid.NewString(),
			},
		}
		ev.SetExtra("extra", uuid.NewString())
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relay.Publish(ctx, ev.Event))
		expectedEvents = append(expectedEvents, ev)
		jobEvents = append(jobEvents, ev)
	})
	t.Run("send dvm search nostr count job for kinds and #e filter groupped by pubkey", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindJobNostrEventCount,
				Tags:      nostr.Tags{[]string{"param", "relay", "wss://localhost:9997"}, []string{"p", serivceProviderPubKey}, []string{"param", "group", "pubkey"}, []string{"relays", "wss://localhost:9998"}},
				Content:   fmt.Sprintf(`[{"kinds":[%v],"#e":["%v"]}]`, nostr.KindReaction, postID),
				Sig:       uuid.NewString(),
			},
		}
		ev.SetExtra("extra", uuid.NewString())
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relay.Publish(ctx, ev.Event))
		expectedEvents = append(expectedEvents, ev)
		jobEvents = append(jobEvents, ev)
	})
	var jobEvent *model.Event
	t.Run("send dvm search nostr count job for author filter", func(t *testing.T) {
		jobEvent = &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindJobNostrEventCount,
				Tags:      nostr.Tags{[]string{"relays", "wss://localhost:9998"}, []string{"param", "relay", "wss://localhost:9997"}, []string{"param", "relay", "wss://localhost:9996"}, []string{"param", "relay", "wss://localhost:9995"}, []string{"param", "relay", "wss://localhost:9994"}},
				Content:   fmt.Sprintf(`[{"authors":["%v"]}]`, expectedEvents[0].PubKey),
				Sig:       uuid.NewString(),
			},
		}
		jobEvent.SetExtra("extra", uuid.NewString())
		helperSignWithMinLeadingZeroBits(t, jobEvent, privkey)
		require.NoError(t, relay.Publish(ctx, jobEvent.Event))
		expectedEvents = append(expectedEvents, jobEvent)
	})

	t.Run("send delete job request", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindDeletion,
				Tags:      nostr.Tags{[]string{"e", jobEvent.GetID()}, []string{"k", fmt.Sprint(model.KindJobNostrEventCount)}},
				Sig:       uuid.NewString(),
			},
		}
		ev.SetExtra("extra", uuid.NewString())
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relay.Publish(ctx, ev.Event))
		expectedEvents = append(expectedEvents, ev)
	})

	time.Sleep(time.Second * 2)
	events, err := relay.QueryEvents(ctx, nostr.Filter{Kinds: []int{6400}, Limit: 1})
	require.NoError(t, err)
	evList := make([]*nostr.Event, 0)
	for ev := range events {
		evList = append(evList, ev)
	}
	require.Equal(t, 3, len(evList))
	require.NoError(t, relay.Close())
	require.NoError(t, relayToSearchResult.Close())

	expectedResponses := []string{`{"+":1,"-":1}`, `0`, fmt.Sprintf(`{"%v":1}`, expectedEvents[0].PubKey), fmt.Sprintf(`{"%v":2}`, expectedEvents[0].PubKey)}
	for _, event := range storedEvents {
		if event.Kind == model.KindJobNostrEventCount+1000 {
			for _, jEv := range jobEvents {
				if event.Tags.GetFirst([]string{"request"}).Value() == fmt.Sprintf("%+v", jEv) {
					require.Equal(t, event.Tags.GetFirst([]string{"e"}).Value(), jEv.GetID())
					require.Equal(t, event.Tags.GetFirst([]string{"i"}).Value(), jEv.Content)
					require.Equal(t, event.Tags.GetFirst([]string{"p"}).Value(), jEv.PubKey)
				}
				require.Equal(t, event.PubKey, serivceProviderPubKey)
			}
			require.Contains(t, expectedResponses, event.Content)

			continue
		}
		require.Contains(t, expectedEvents, event)
	}
}

func TestErrorFeedback(t *testing.T) {
	query.MustInit()
	privkey := nostr.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	serviceProviderPrivKey := nostr.GeneratePrivateKey()
	serivceProviderPubKey, err := nostr.GetPublicKey(serviceProviderPrivKey)
	require.NoError(t, err)
	dataVendingMachine := dvm.NewDvms(NIP13MinLeadingZeroBits, serviceProviderPrivKey, true)
	RegisterWSSubscriptionListener(func(context.Context, *model.Subscription) query.EventIterator {
		return func(yield func(*model.Event, error) bool) {
			for i := range storedEvents {
				if !yield(storedEvents[i], nil) {
					return
				}
			}
		}
	})
	RegisterWSEventListener(func(ctx context.Context, event *model.Event) error {
		require.NoError(t, dataVendingMachine.AcceptJob(ctx, event))
		for _, sEvent := range storedEvents {
			if sEvent.ID == event.ID {
				return model.ErrDuplicate
			}
		}
		assert.False(t, event.IsEphemeral())
		storedEvents = append(storedEvents, event)

		return nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
	relay, err := fixture.NewRelayClient(ctx, "wss://localhost:9998")
	if err != nil {
		log.Panic(err)
	}
	var (
		postID               = uuid.NewString()
		pubKeyOfRepostedNote = "pubkey1"
		repostedKind         = nostr.KindArticle
		expectedEvents       []*model.Event
	)

	relayToSearchResult, err := fixture.NewRelayClient(ctx, "wss://localhost:9997")
	require.NoError(t, err)
	t.Run("send reaction 1", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindReaction,
				Tags:      nostr.Tags{[]string{"e", postID, "relay"}, []string{"p", pubKeyOfRepostedNote}, []string{"k", fmt.Sprint(repostedKind)}},
				Content:   "+",
				Sig:       uuid.NewString(),
			},
		}
		ev.SetExtra("extra", uuid.NewString())
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relayToSearchResult.Publish(ctx, ev.Event))
		expectedEvents = append(expectedEvents, ev)
	})
	t.Run("send reaction 2", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindReaction,
				Tags:      nostr.Tags{[]string{"e", postID, "relay"}, []string{"p", pubKeyOfRepostedNote}, []string{"k", fmt.Sprint(repostedKind)}},
				Content:   "-",
				Sig:       uuid.NewString(),
			},
		}
		ev.SetExtra("extra", uuid.NewString())
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relayToSearchResult.Publish(ctx, ev.Event))
		expectedEvents = append(expectedEvents, ev)
	})
	t.Run("send article", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindArticle,
				Tags:      nostr.Tags{[]string{"title", "dummy"}},
				Content:   "dummy content",
				Sig:       uuid.NewString(),
			},
		}
		ev.SetExtra("extra", uuid.NewString())
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relayToSearchResult.Publish(ctx, ev.Event))
		expectedEvents = append(expectedEvents, ev)
	})
	t.Run("send article", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindArticle,
				Tags:      nostr.Tags{[]string{"title", "dummy"}},
				Content:   "dummy content",
				Sig:       uuid.NewString(),
			},
		}
		ev.SetExtra("extra", uuid.NewString())
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relayToSearchResult.Publish(ctx, ev.Event))
		expectedEvents = append(expectedEvents, ev)
	})
	time.Sleep(time.Second * 1)
	var jobEvent *model.Event
	t.Run("send wrong filter dvm search nostr count job for author filter", func(t *testing.T) {
		jobEvent = &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindJobNostrEventCount,
				Tags:      nostr.Tags{[]string{"param", "relay", "wss://localhost:9997"}, []string{"p", serivceProviderPubKey}, []string{"relays", "wss://localhost:9998"}},
				Content:   `aaaa`,
				Sig:       uuid.NewString(),
			},
		}
		jobEvent.SetExtra("extra", uuid.NewString())
		helperSignWithMinLeadingZeroBits(t, jobEvent, privkey)
		require.NoError(t, relay.Publish(ctx, jobEvent.Event))
		expectedEvents = append(expectedEvents, jobEvent)
	})

	time.Sleep(time.Second * 2)
	events, err := relay.QueryEvents(ctx, nostr.Filter{Kinds: []int{6400, 7000}, Limit: 1})
	require.NoError(t, err)
	evList := make([]*nostr.Event, 0)
	for ev := range events {
		evList = append(evList, ev)
	}
	require.Equal(t, 1, len(evList))
	require.NoError(t, relay.Close())
	require.NoError(t, relayToSearchResult.Close())

	for _, event := range storedEvents {
		if event.Kind == nostr.KindJobFeedback {
			require.Equal(t, model.JobFeedbackStatusError, event.GetTag("status").Value())
			require.Equal(t, jobEvent.GetID(), event.GetTag("e").Value())
			require.Equal(t, jobEvent.PubKey, event.GetTag("p").Value())

			continue
		}
		require.Contains(t, expectedEvents, event)
	}
}

func TestOfflineJob(t *testing.T) {
	query.MustInit()
	privkey := nostr.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	serviceProviderPrivKey := nostr.GeneratePrivateKey()
	serivceProviderPubKey, err := nostr.GetPublicKey(serviceProviderPrivKey)
	require.NoError(t, err)
	dataVendingMachine := dvm.NewDvms(NIP13MinLeadingZeroBits, serviceProviderPrivKey, true)
	RegisterWSSubscriptionListener(func(context.Context, *model.Subscription) query.EventIterator {
		return func(yield func(*model.Event, error) bool) {
			for i := range storedEvents {
				if !yield(storedEvents[i], nil) {
					return
				}
			}
		}
	})
	RegisterWSEventListener(func(ctx context.Context, event *model.Event) error {
		require.NoError(t, dataVendingMachine.AcceptJob(ctx, event))
		for _, sEvent := range storedEvents {
			if sEvent.ID == event.ID {
				return model.ErrDuplicate
			}
		}
		assert.False(t, event.IsEphemeral())
		storedEvents = append(storedEvents, event)

		return nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
	relay, err := fixture.NewRelayClient(ctx, "wss://localhost:9998")
	if err != nil {
		log.Panic(err)
	}
	var (
		postID               = uuid.NewString()
		pubKeyOfRepostedNote = "pubkey1"
		repostedKind         = nostr.KindArticle
		expectedEvents       []*model.Event
		jobEvents            []*model.Event
	)
	require.NoError(t, err)
	t.Run("send reaction 1", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindReaction,
				Tags:      nostr.Tags{[]string{"e", postID, "relay"}, []string{"p", pubKeyOfRepostedNote}, []string{"k", fmt.Sprint(repostedKind)}},
				Content:   "+",
				Sig:       uuid.NewString(),
			},
		}
		ev.SetExtra("extra", uuid.NewString())
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relay.Publish(ctx, ev.Event))
		expectedEvents = append(expectedEvents, ev)
	})
	t.Run("send reaction 2", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindReaction,
				Tags:      nostr.Tags{[]string{"e", postID, "relay"}, []string{"p", pubKeyOfRepostedNote}, []string{"k", fmt.Sprint(repostedKind)}},
				Content:   "-",
				Sig:       uuid.NewString(),
			},
		}
		ev.SetExtra("extra", uuid.NewString())
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relay.Publish(ctx, ev.Event))
		expectedEvents = append(expectedEvents, ev)
	})
	t.Run("send article", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindArticle,
				Tags:      nostr.Tags{[]string{"title", "dummy"}},
				Content:   "dummy content",
				Sig:       uuid.NewString(),
			},
		}
		ev.SetExtra("extra", uuid.NewString())
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relay.Publish(ctx, ev.Event))
		expectedEvents = append(expectedEvents, ev)
	})
	t.Run("send article", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindArticle,
				Tags:      nostr.Tags{[]string{"title", "dummy"}},
				Content:   "dummy content",
				Sig:       uuid.NewString(),
			},
		}
		ev.SetExtra("extra", uuid.NewString())
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relay.Publish(ctx, ev.Event))
		expectedEvents = append(expectedEvents, ev)
	})
	time.Sleep(time.Second * 1)
	t.Run("send dvm search nostr count job for author filter", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindJobNostrEventCount,
				Tags:      nostr.Tags{[]string{"param", "relay", "wss://localhost:9996"}, []string{"p", serivceProviderPubKey}, []string{"relays", "wss://localhost:9998"}},
				Content:   fmt.Sprintf(`[{"authors":["%v"]}]`, expectedEvents[0].PubKey),
				Sig:       uuid.NewString(),
			},
		}
		ev.SetExtra("extra", uuid.NewString())
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relay.Publish(ctx, ev.Event))
		expectedEvents = append(expectedEvents, ev)
		jobEvents = append(jobEvents, ev)
	})
	t.Run("send dvm search nostr count job for kinds and #e filter groupped by pubkey", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindJobNostrEventCount,
				Tags:      nostr.Tags{[]string{"param", "relay", "wss://localhost:9997"}, []string{"p", serivceProviderPubKey}, []string{"param", "group", "pubkey"}, []string{"relays", "wss://localhost:9998"}},
				Content:   fmt.Sprintf(`[{"kinds":[%v],"#e":["%v"]}]`, nostr.KindReaction, postID),
				Sig:       uuid.NewString(),
			},
		}
		ev.SetExtra("extra", uuid.NewString())
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relay.Publish(ctx, ev.Event))
		expectedEvents = append(expectedEvents, ev)
		jobEvents = append(jobEvents, ev)
	})
	t.Run("send dvm search nostr count job for kinds and #e filter groupped by content", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindJobNostrEventCount,
				Tags:      nostr.Tags{[]string{"param", "relay", "wss://localhost:9997"}, []string{"p", serivceProviderPubKey}, []string{"param", "group", "content"}, []string{"relays", "wss://localhost:9998"}},
				Content:   fmt.Sprintf(`[{"kinds":[%v],"#e":["%v"]}]`, nostr.KindReaction, postID),
				Sig:       uuid.NewString(),
			},
		}
		ev.SetExtra("extra", uuid.NewString())
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relay.Publish(ctx, ev.Event))
		expectedEvents = append(expectedEvents, ev)
		jobEvents = append(jobEvents, ev)
	})
	t.Run("send dvm search nostr count job with 0 result for group", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindJobNostrEventCount,
				Tags:      nostr.Tags{[]string{"param", "relay", "wss://localhost:9997"}, []string{"p", serivceProviderPubKey}, []string{"param", "group", "pubkey"}, []string{"relays", "wss://localhost:9998"}},
				Content:   fmt.Sprintf(`[{"kinds":[%v],"#title":["dummy"]}]`, nostr.KindArticle),
				Sig:       uuid.NewString(),
			},
		}
		ev.SetExtra("extra", uuid.NewString())
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relay.Publish(ctx, ev.Event))
		expectedEvents = append(expectedEvents, ev)
		jobEvents = append(jobEvents, ev)
	})

	time.Sleep(time.Second * 2)
	events, err := relay.QueryEvents(ctx, nostr.Filter{Kinds: []int{6400}, Limit: 1})
	require.NoError(t, err)
	evList := make([]*nostr.Event, 0)
	for ev := range events {
		evList = append(evList, ev)
	}
	require.Equal(t, 4, len(evList))
	require.NoError(t, relay.Close())
	expectedResponses := []string{`{"+":1,"-":1}`, `0`, fmt.Sprintf(`{"%v":1}`, expectedEvents[0].PubKey), fmt.Sprintf(`{"%v":2}`, expectedEvents[0].PubKey)}
	for _, event := range storedEvents {
		if event.Kind == model.KindJobNostrEventCount+1000 {
			for _, jEv := range jobEvents {
				if event.Tags.GetFirst([]string{"request"}).Value() == fmt.Sprintf("%+v", jEv) {
					require.Equal(t, event.Tags.GetFirst([]string{"e"}).Value(), jEv.GetID())
					require.Equal(t, event.Tags.GetFirst([]string{"i"}).Value(), jEv.Content)
					require.Equal(t, event.Tags.GetFirst([]string{"p"}).Value(), jEv.PubKey)
				}
				require.Equal(t, event.PubKey, serivceProviderPubKey)
			}
			require.Contains(t, expectedResponses, event.Content)

			continue
		}
		require.Contains(t, expectedEvents, event)
	}
}
