// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
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
	privkey := nostr.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	customerPubKey := hex.EncodeToString([]byte("customer pubkey"))
	customerPrivKey := hex.EncodeToString([]byte("customer privkey"))
	dataVendingMachine := dvm.NewDvms(NIP13MinLeadingZeroBits, customerPubKey, customerPrivKey)
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
	relay, err := fixture.NewRelayClient(ctx, "wss://localhost:9998", fixture.LocalhostTLS(fixture.LocalhostCrt))
	if err != nil {
		log.Panic(err)
	}
	pubsubServers[0].Reset()

	var (
		pubkeyToSearch       = "searchedPubkey"
		dummyPubKey          = "dummyPubkey"
		repostID             = uuid.NewString()
		pubKeyOfRepostedNote = "pubkey1"
		repostedKind         = nostr.KindArticle
		expectedEvents       []*model.Event
		jobEvents            []*model.Event
	)

	relayToSearchResult, err := fixture.NewRelayClient(ctx, "wss://localhost:9997", fixture.LocalhostTLS(fixture.LocalhostCrt))
	require.NoError(t, err)
	t.Run("send reaction 1 with pubkey", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    pubkeyToSearch,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindReaction,
				Tags:      nostr.Tags{[]string{"e", repostID, "relay"}, []string{"p", pubKeyOfRepostedNote}, []string{"k", fmt.Sprint(repostedKind)}},
				Content:   "+",
				Sig:       uuid.NewString(),
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relayToSearchResult.Publish(ctx, ev.Event))
		expectedEvents = append(expectedEvents, ev)
	})
	t.Run("send reaction 2 with pubkey", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    pubkeyToSearch,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindReaction,
				Tags:      nostr.Tags{[]string{"e", repostID, "relay"}, []string{"p", pubKeyOfRepostedNote}, []string{"k", fmt.Sprint(repostedKind)}},
				Content:   "-",
				Sig:       uuid.NewString(),
			},
		}
		ev.SetExtra("extra", uuid.NewString())
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relayToSearchResult.Publish(ctx, ev.Event))
		expectedEvents = append(expectedEvents, ev)
	})
	t.Run("send reaction 3 with another dummy pubkey", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    dummyPubKey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindReaction,
				Tags:      nostr.Tags{[]string{"e", repostID, "relay"}, []string{"p", pubKeyOfRepostedNote}, []string{"k", fmt.Sprint(repostedKind)}},
				Content:   "-",
				Sig:       uuid.NewString(),
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relayToSearchResult.Publish(ctx, ev.Event))
		expectedEvents = append(expectedEvents, ev)
	})
	t.Run("send reaction 4 with another dummy pubkey", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    dummyPubKey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindReaction,
				Tags:      nostr.Tags{[]string{"e", repostID, "relay"}, []string{"p", pubKeyOfRepostedNote}, []string{"k", fmt.Sprint(repostedKind)}},
				Content:   "-",
				Sig:       uuid.NewString(),
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relayToSearchResult.Publish(ctx, ev.Event))
		expectedEvents = append(expectedEvents, ev)
	})
	time.Sleep(time.Second * 2)
	t.Run("send dvm search nostr count job for author filter", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    customerPubKey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindJobNostrEventCount,
				Tags:      nostr.Tags{[]string{"param", "relay", "wss://localhost:9997"}, []string{"p", customerPubKey}},
				Content:   fmt.Sprintf(`[{"authors":["%v"]}]`, expectedEvents[0].PubKey),
				Sig:       uuid.NewString(),
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relay.Publish(ctx, ev.Event))
		expectedEvents = append(expectedEvents, ev)
		jobEvents = append(jobEvents, ev)
	})

	t.Run("send dvm search nostr count job for kinds and #e filter groupped by pubkey", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    customerPubKey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindJobNostrEventCount,
				Tags:      nostr.Tags{[]string{"param", "relay", "wss://localhost:9997"}, []string{"p", customerPubKey}, []string{"param", "group", "pubkey"}},
				Content:   fmt.Sprintf(`[{"kinds":[%v],"#e":["%v"]}]`, nostr.KindReaction, repostID),
				Sig:       uuid.NewString(),
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relay.Publish(ctx, ev.Event))
		expectedEvents = append(expectedEvents, ev)
		jobEvents = append(jobEvents, ev)
	})
	t.Run("send dvm search nostr count job for kinds and #e filter groupped by content", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    customerPubKey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindJobNostrEventCount,
				Tags:      nostr.Tags{[]string{"param", "relay", "wss://localhost:9997"}, []string{"p", customerPubKey}, []string{"param", "group", "content"}},
				Content:   fmt.Sprintf(`[{"kinds":[%v],"#e":["%v"]}]`, nostr.KindReaction, repostID),
				Sig:       uuid.NewString(),
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relay.Publish(ctx, ev.Event))
		expectedEvents = append(expectedEvents, ev)
		jobEvents = append(jobEvents, ev)
	})
	t.Run("send dvm search nostr count job with 0 result for group", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    customerPubKey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindJobNostrEventCount,
				Tags:      nostr.Tags{[]string{"param", "relay", "wss://localhost:9997"}, []string{"p", customerPubKey}, []string{"param", "group", "pubkey"}},
				Content:   fmt.Sprintf(`[{"kinds":[%v],"#e":["%v"]}]`, nostr.KindReaction, expectedEvents[0].PubKey),
				Sig:       uuid.NewString(),
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relay.Publish(ctx, ev.Event))
		expectedEvents = append(expectedEvents, ev)
		jobEvents = append(jobEvents, ev)
	})
	require.NoError(t, relay.Close())

	time.Sleep(time.Second * 5)
	events, err := relayToSearchResult.QueryEvents(ctx, nostr.Filter{Kinds: []int{6400, 7000}, Limit: 1})
	require.NoError(t, err)
	evList := make([]*nostr.Event, 0)
	for ev := range events {
		evList = append(evList, ev)
	}
	require.Equal(t, 4, len(evList))
	require.NoError(t, relayToSearchResult.Close())

	shutdownCtx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
	for pubsubServers[0].ReaderExited.Load() != uint64(1) {
		if shutdownCtx.Err() != nil {
			log.Panic(errors.Errorf("shutdown timeout %v of %v", pubsubServers[0].ReaderExited.Load(), 1))
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.Equal(t, uint64(1), pubsubServers[0].ReaderExited.Load())
	for _, event := range storedEvents {
		if event.Kind == model.KindJobNostrEventCount+1000 {
			continue
		}
		require.Contains(t, expectedEvents, event)
	}
	require.Equal(t, 4, len(evList))
	expectedResponses := []string{`{"+":1,"-":1}`, `0`, `6`, fmt.Sprintf(`{"%v":2}`, expectedEvents[0].PubKey)}
	for _, event := range evList {
		for _, jEv := range jobEvents {
			stringifed, err := jEv.Strinfify()
			require.NoError(t, err)
			if event.Tags.GetFirst([]string{"request"}).Value() == stringifed {
				require.Equal(t, event.Tags.GetFirst([]string{"e"}).Value(), jEv.GetID())
				require.Equal(t, event.Tags.GetFirst([]string{"i"}).Value(), jEv.Content)
				require.Equal(t, event.Tags.GetFirst([]string{"p"}).Value(), jEv.PubKey)
			}
		}
		require.Contains(t, expectedResponses, event.Content)
	}
}

func TestJobDeletion(t *testing.T) {
	privkey := nostr.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	customerPubKey := hex.EncodeToString([]byte("customer pubkey"))
	customerPrivKey := hex.EncodeToString([]byte("customer privkey"))
	dataVendingMachine := dvm.NewDvms(NIP13MinLeadingZeroBits, customerPubKey, customerPrivKey)
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
	relay, err := fixture.NewRelayClient(ctx, "wss://localhost:9998", fixture.LocalhostTLS(fixture.LocalhostCrt))
	if err != nil {
		log.Panic(err)
	}
	pubsubServers[0].Reset()
	var (
		pubkeyToSearch       = "searchedPubkey"
		repostID             = uuid.NewString()
		pubKeyOfRepostedNote = "pubkey1"
		repostedKind         = nostr.KindArticle
		expectedEvents       []*model.Event
	)

	relayToSearchResult, err := fixture.NewRelayClient(ctx, "wss://localhost:9997", fixture.LocalhostTLS(fixture.LocalhostCrt))
	require.NoError(t, err)
	t.Run("send reaction 1 with pubkey", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    pubkeyToSearch,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindReaction,
				Tags:      nostr.Tags{[]string{"e", repostID, "relay"}, []string{"p", pubKeyOfRepostedNote}, []string{"k", fmt.Sprint(repostedKind)}},
				Content:   "+",
				Sig:       uuid.NewString(),
			},
		}
		ev.SetExtra("extra", uuid.NewString())
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relayToSearchResult.Publish(ctx, ev.Event))
		expectedEvents = append(expectedEvents, ev)
	})
	t.Run("send reaction 2 with pubkey", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    pubkeyToSearch,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindReaction,
				Tags:      nostr.Tags{[]string{"e", repostID, "relay"}, []string{"p", pubKeyOfRepostedNote}, []string{"k", fmt.Sprint(repostedKind)}},
				Content:   "-",
				Sig:       uuid.NewString(),
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relayToSearchResult.Publish(ctx, ev.Event))
		expectedEvents = append(expectedEvents, ev)
	})
	var jobEvent *model.Event
	t.Run("send dvm search nostr count job for author filter", func(t *testing.T) {
		jobEvent = &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    customerPubKey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindJobNostrEventCount,
				Tags:      nostr.Tags{[]string{"param", "relay", "wss://localhost:9997"}, []string{"param", "relay", "wss://localhost:9996"}, []string{"param", "relay", "wss://localhost:9995"}, []string{"param", "relay", "wss://localhost:9994"}, []string{"p", customerPubKey}},
				Content:   fmt.Sprintf(`[{"authors":["%v"]}]`, expectedEvents[0].PubKey),
				Sig:       uuid.NewString(),
			},
		}
		helperSignWithMinLeadingZeroBits(t, jobEvent, privkey)
		require.NoError(t, relay.Publish(ctx, jobEvent.Event))
		expectedEvents = append(expectedEvents, jobEvent)
	})

	t.Run("send delete job request", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    customerPubKey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindDeletion,
				Tags:      nostr.Tags{[]string{"e", jobEvent.GetID()}, []string{"k", fmt.Sprint(model.KindJobNostrEventCount)}},
				Sig:       uuid.NewString(),
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relay.Publish(ctx, ev.Event))
		expectedEvents = append(expectedEvents, ev)
	})
	time.Sleep(time.Second * 5)
	events, err := relayToSearchResult.QueryEvents(ctx, nostr.Filter{Kinds: []int{6400, 7000}, Limit: 1})
	require.NoError(t, err)
	evList := make([]*nostr.Event, 0)
	for ev := range events {
		evList = append(evList, ev)
	}

	require.NoError(t, relayToSearchResult.Close())
	require.NoError(t, relay.Close())
	for _, event := range storedEvents {
		if event.Kind == model.KindJobNostrEventCount {
			for _, expected := range expectedEvents {
				if expected.Kind == model.KindJobNostrEventCount {
					require.EqualValues(t, event, expected)
				}
			}
			continue
		}
		if event.Kind == model.KindJobNostrEventCount+1000 {
			continue
		}
		require.Contains(t, expectedEvents, event)
	}
	require.Equal(t, 1, len(evList))
	require.Equal(t, "4", evList[0].Content)
	stringified, err := jobEvent.Strinfify()
	require.NoError(t, err)
	require.Equal(t, stringified, evList[0].Tags.GetFirst([]string{"request"}).Value())
}

func TestErrorFeedback(t *testing.T) {
	privkey := nostr.GeneratePrivateKey()
	storedEvents := []*model.Event{}
	customerPubKey := hex.EncodeToString([]byte("customer pubkey"))
	customerPrivKey := hex.EncodeToString([]byte("customer privkey"))
	dataVendingMachine := dvm.NewDvms(NIP13MinLeadingZeroBits, customerPubKey, customerPrivKey)
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
	relay, err := fixture.NewRelayClient(ctx, "wss://localhost:9998", fixture.LocalhostTLS(fixture.LocalhostCrt))
	if err != nil {
		log.Panic(err)
	}
	pubsubServers[0].Reset()

	var (
		pubkeyToSearch       = "searchedPubkey"
		dummyPubKey          = "dummyPubkey"
		repostID             = uuid.NewString()
		pubKeyOfRepostedNote = "pubkey1"
		repostedKind         = nostr.KindArticle
		expectedEvents       []*model.Event
		jobEvents            []*model.Event
	)

	relayToSearchResult, err := fixture.NewRelayClient(ctx, "wss://localhost:9997", fixture.LocalhostTLS(fixture.LocalhostCrt))
	require.NoError(t, err)
	t.Run("send reaction 1 with pubkey", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    pubkeyToSearch,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindReaction,
				Tags:      nostr.Tags{[]string{"e", repostID, "relay"}, []string{"p", pubKeyOfRepostedNote}, []string{"k", fmt.Sprint(repostedKind)}},
				Content:   "+",
				Sig:       uuid.NewString(),
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relayToSearchResult.Publish(ctx, ev.Event))
		expectedEvents = append(expectedEvents, ev)
	})
	t.Run("send reaction 2 with pubkey", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    pubkeyToSearch,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindReaction,
				Tags:      nostr.Tags{[]string{"e", repostID, "relay"}, []string{"p", pubKeyOfRepostedNote}, []string{"k", fmt.Sprint(repostedKind)}},
				Content:   "-",
				Sig:       uuid.NewString(),
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relayToSearchResult.Publish(ctx, ev.Event))
		expectedEvents = append(expectedEvents, ev)
	})
	t.Run("send reaction 3 with another dummy pubkey", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    dummyPubKey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindReaction,
				Tags:      nostr.Tags{[]string{"e", repostID, "relay"}, []string{"p", pubKeyOfRepostedNote}, []string{"k", fmt.Sprint(repostedKind)}},
				Content:   "-",
				Sig:       uuid.NewString(),
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relayToSearchResult.Publish(ctx, ev.Event))
		expectedEvents = append(expectedEvents, ev)
	})
	t.Run("send reaction 4 with another dummy pubkey", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    dummyPubKey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindReaction,
				Tags:      nostr.Tags{[]string{"e", repostID, "relay"}, []string{"p", pubKeyOfRepostedNote}, []string{"k", fmt.Sprint(repostedKind)}},
				Content:   "-",
				Sig:       uuid.NewString(),
			},
		}
		ev.SetExtra("extra", uuid.NewString())
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relayToSearchResult.Publish(ctx, ev.Event))
		expectedEvents = append(expectedEvents, ev)
	})
	time.Sleep(time.Second * 2)
	t.Run("send dvm search nostr count job for author filter", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    customerPubKey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.KindJobNostrEventCount,
				Tags:      nostr.Tags{[]string{"param", "relay", "wss://localhost:9997"}, []string{"p", customerPubKey}},
				Content:   `aaaa`,
				Sig:       uuid.NewString(),
			},
		}
		ev.SetExtra("extra", uuid.NewString())
		helperSignWithMinLeadingZeroBits(t, ev, privkey)
		require.NoError(t, relay.Publish(ctx, ev.Event))
		expectedEvents = append(expectedEvents, ev)
		jobEvents = append(jobEvents, ev)
	})
	require.NoError(t, relay.Close())

	time.Sleep(time.Second * 5)
	events, err := relayToSearchResult.QueryEvents(ctx, nostr.Filter{Kinds: []int{7000}, Limit: 1})
	require.NoError(t, err)
	evList := make([]*nostr.Event, 0)
	for ev := range events {
		evList = append(evList, ev)
	}
	require.Equal(t, 1, len(evList))
	for _, event := range evList {
		fmt.Printf("event: %v\n", event.Kind)
	}
	require.NoError(t, relayToSearchResult.Close())

	shutdownCtx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
	for pubsubServers[0].ReaderExited.Load() != uint64(1) {
		if shutdownCtx.Err() != nil {
			log.Panic(errors.Errorf("shutdown timeout %v of %v", pubsubServers[0].ReaderExited.Load(), 1))
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.Equal(t, uint64(1), pubsubServers[0].ReaderExited.Load())
	for _, event := range storedEvents {
		if event.Kind != nostr.KindJobFeedback {
			continue
		}
		require.Contains(t, event.Content, "error: failed to parse filters")
	}
}
