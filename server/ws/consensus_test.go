// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"bytes"
	"context"
	"crypto/sha256"
	"math/rand/v2"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/jamiealquiza/tachymeter"
	"github.com/nbd-wtf/go-nostr"
	"github.com/puzpuzpuz/xsync/v4"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/database/command"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/server/ws/fixture"
)

func TestConsensusEvents(t *testing.T) {
	privkey, pk := model.GenerateKeyPair()
	mapPort := func(ctx context.Context) *fixture.MockService {
		var port, consensusPort uint16
		if portVal := ctx.Value("serverPort"); portVal != nil {
			port = portVal.(uint16)
		}
		if portVal := ctx.Value("consensusPort"); portVal != nil {
			consensusPort = portVal.(uint16)
		}

		for _, s := range pubsubServers {
			if port != 0 && strings.HasSuffix(s.Endpoint(), strconv.FormatInt(int64(port), 10)) {
				return s
			}
			if consensusPort != 0 && s.Consensus.DiscoveryPort() == consensusPort {
				return s
			}
		}
		for _, s := range pubsubServersExtra {
			if port != 0 && strings.HasSuffix(s.Endpoint(), strconv.FormatInt(int64(port), 10)) {
				return s
			}
			if consensusPort != 0 && s.Consensus.DiscoveryPort() == consensusPort {
				return s
			}
		}
		return nil
	}

	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		t.Logf("received events: %v on %v", events, ctx.Value("serverPort"))
		if qErr := mapPort(ctx).DB.AcceptEvents(ctx, events...); qErr != nil {
			return qErr
		}
		if cErr := mapPort(ctx).Consensus.AcceptEvents(ctx, events...); cErr != nil {
			return cErr
		}
		return nil
	})
	command.RegisterRollbackListener(query.RollbackEvents)
	consensusDone := map[string]chan bool{} // relays accepted event
	finalizedDone := map[string]chan bool{} // block commit includes event
	for _, s := range pubsubServers {
		consensusDone[s.Endpoint()] = make(chan bool, 1000)
		finalizedDone[s.Endpoint()] = make(chan bool, 1000)
	}
	accepted := xsync.NewMap[string, bool]()
	normalAccept := func(ctx context.Context, events ...*model.Event) error {
		if _, ok := accepted.Load(mapPort(ctx).Endpoint() + helperHashEvents(t, events...)); ok {
			return nil
		}
		if qErr := mapPort(ctx).DB.AcceptEvents(ctx, events...); qErr != nil {
			return qErr
		}
		consensusDone[mapPort(ctx).Endpoint()] <- true
		accepted.Store(mapPort(ctx).Endpoint()+helperHashEvents(t, events...), true)
		t.Log("ACCEPT", mapPort(ctx).Endpoint(), events[0].Kind, events[0].Content)
		return nil
	}
	command.RegisterAcceptListener(normalAccept)

	committed := xsync.NewMap[string, bool]()
	normalCommit := func(ctx context.Context, events ...*model.Event) error {
		if _, ok := committed.Load(mapPort(ctx).Endpoint() + helperHashEvents(t, events...)); ok {
			return nil
		}
		finalizedDone[mapPort(ctx).Endpoint()] <- true
		committed.Store(mapPort(ctx).Endpoint()+helperHashEvents(t, events...), true)
		t.Log("COMMIT", mapPort(ctx).Endpoint(), events[0].Kind, events[0].Content)
		return nil
	}
	command.RegisterCommitListener(normalCommit)

	RegisterWSSubscriptionListener(func(ctx context.Context, filters ...model.Filter) EventIterator {
		return mapPort(ctx).DB.SelectEvents(ctx, filters...)
	})

	relay := helperMustNewRelay(t, pubsubServers[0])       // :9988
	secondRelay := helperMustNewRelay(t, pubsubServers[1]) // :9977
	thirdRelay := helperMustNewRelay(t, pubsubServers[2])  // :9966
	fourthRelay := helperMustNewRelay(t, pubsubServers[3]) // :9955

	var ev, ev2 *model.Event
	var attestationEvent *model.Event
	masterPrivKey, masterPubkey := model.GenerateKeyPair()
	t.Run("SaveEvent", func(t *testing.T) {
		attestationEvent = &model.Event{Event: nostr.Event{
			Kind:      model.CustomIONKindAttestation,
			CreatedAt: 1,
			Tags: model.Tags{
				{model.TagAttestationName, pk, "", model.CustomIONAttestationKindActive + ":1"},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, attestationEvent, masterPrivKey)

		relaysList := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindRelayListMetadata,
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, masterPubkey},
				{"r", pubsubServers[0].Endpoint()},
				{"r", pubsubServers[1].Endpoint()},
				{"r", pubsubServers[2].Endpoint()},
				{"r", pubsubServers[3].Endpoint()},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, relaysList, privkey)
		require.NoError(t, relay.PublishMany(t.Context(), &attestationEvent.Event, &relaysList.Event))
		require.NoError(t, helperAwaitConsensus(t, consensusDone, relay))
		require.NoError(t, helperAwaitFinalized(t, finalizedDone))

		ev = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, masterPubkey},
			},
			Content: "validEvent from relay1",
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkey)

		require.NoError(t, relay.Publish(t.Context(), ev.Event))
		require.NoError(t, helperAwaitConsensus(t, consensusDone, relay))
		require.NoError(t, helperAwaitFinalized(t, finalizedDone))
	})

	t.Run("query events", func(t *testing.T) {
		receivedEventsFromFirstRelay := helperQueryEvents(t, t.Context(), relay, nostr.Filter{Kinds: []int{nostr.KindTextNote}})
		require.Len(t, receivedEventsFromFirstRelay, 1)
		require.Contains(t, receivedEventsFromFirstRelay, ev)
		time.Sleep(1 * time.Second)
		receivedEventsFromSecondRelay := helperQueryEvents(t, t.Context(), secondRelay, nostr.Filter{Kinds: []int{nostr.KindTextNote}})
		require.Len(t, receivedEventsFromSecondRelay, 1)
		require.Equal(t, receivedEventsFromFirstRelay, receivedEventsFromSecondRelay)
		ev2 = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, masterPubkey},
			},
			Content: "validEvent from relay2",
		}}
		helperSignWithMinLeadingZeroBits(t, ev2, privkey)
		require.NoError(t, secondRelay.Publish(t.Context(), ev2.Event))
		require.NoError(t, helperAwaitConsensus(t, consensusDone, secondRelay))
		require.NoError(t, helperAwaitFinalized(t, finalizedDone))
		receivedEventsFromFirstRelay = helperQueryEvents(t, t.Context(), relay, nostr.Filter{Kinds: []int{nostr.KindTextNote}})
		require.Len(t, receivedEventsFromFirstRelay, 2)
		require.Contains(t, receivedEventsFromFirstRelay, ev)
		require.Contains(t, receivedEventsFromFirstRelay, ev2)
	})

	command.RegisterAcceptListener(func(ctx context.Context, events ...*model.Event) error {
		hasFailedTx := false
		for _, e := range events {
			if e.Content == "validEvent not gonna be accepted because of failed consensus" {
				hasFailedTx = true
				break
			}
		}
		if hasFailedTx && (ctx.Value("consensusPort").(uint16) == 19977 || ctx.Value("consensusPort").(uint16) == 19966 || ctx.Value("consensusPort").(uint16) == 19955) {
			return errors.New("simulating remote relay did not accept tx - it should be rolled back")
		}
		if _, ok := accepted.Load(mapPort(ctx).Endpoint() + helperHashEvents(t, events...)); ok {
			return nil
		}
		if qErr := mapPort(ctx).DB.AcceptEvents(ctx, events...); qErr != nil {
			return qErr
		}
		consensusDone[mapPort(ctx).Endpoint()] <- true
		accepted.Store(mapPort(ctx).Endpoint()+helperHashEvents(t, events...), true)
		t.Log("ACCEPT", mapPort(ctx).Endpoint(), events[0].Kind, events[0].Content)

		return nil
	})

	var notAcceptedEvent *model.Event
	t.Run("failed consensus rolled back", func(t *testing.T) {
		rolledBack := map[string]chan bool{}
		for _, s := range pubsubServers {
			rolledBack[s.Endpoint()] = make(chan bool, 1000)
		}
		command.RegisterRollbackListener(func(ctx context.Context, event ...*model.Event) error {
			err := mapPort(ctx).DB.RollbackEvents(ctx, event...)
			t.Log("ROLLED BACK0", mapPort(ctx).Endpoint(), event[0].Kind, event[0].Content)
			rolledBack[mapPort(ctx).Endpoint()] <- true
			t.Log("ROLLED BACK", mapPort(ctx).Endpoint(), event[0].Kind, event[0].Content)

			return err
		})
		notAcceptedEvent = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, masterPubkey},
			},
			Content: "validEvent not gonna be accepted because of failed consensus",
		}}
		helperSignWithMinLeadingZeroBits(t, notAcceptedEvent, privkey)
		require.Error(t, secondRelay.Publish(t.Context(), notAcceptedEvent.Event))
		require.NoError(t, helperAwaitConsensus(t, rolledBack, secondRelay))
		receivedEventsFromFirstRelay := helperQueryEvents(t, t.Context(), relay, nostr.Filter{Kinds: []int{nostr.KindTextNote}})
		receivedEventsFromSecondRelay := helperQueryEvents(t, t.Context(), secondRelay, nostr.Filter{Kinds: []int{nostr.KindTextNote}})
		receivedEventsFromThirdRelay := helperQueryEvents(t, t.Context(), thirdRelay, nostr.Filter{Kinds: []int{nostr.KindTextNote}})

		require.Len(t, receivedEventsFromFirstRelay, 2)
		require.Contains(t, receivedEventsFromFirstRelay, ev)
		require.Contains(t, receivedEventsFromFirstRelay, ev2)
		require.NotContains(t, receivedEventsFromFirstRelay, notAcceptedEvent)

		require.Len(t, receivedEventsFromSecondRelay, 2)
		require.Contains(t, receivedEventsFromSecondRelay, ev)
		require.Contains(t, receivedEventsFromSecondRelay, ev2)
		require.NotContains(t, receivedEventsFromSecondRelay, notAcceptedEvent)

		require.Len(t, receivedEventsFromThirdRelay, 2)
		require.Contains(t, receivedEventsFromThirdRelay, ev)
		require.Contains(t, receivedEventsFromThirdRelay, ev2)
		require.NotContains(t, receivedEventsFromThirdRelay, notAcceptedEvent)
	})

	command.RegisterAcceptListener(normalAccept)
	command.RegisterRollbackListener(query.RollbackEvents)
	var eventMissedByRelay3DuringBroadcastTime,
		eventMissedByRelay3DuringBroadcastTime2,
		eventMissedByRelay3DuringBroadcastTime3,
		eventAfterNodeComesUp *model.Event
	t.Run("relay fetches missed data after downtime, broadcast still works as 2/3 reached", func(t *testing.T) {
		stopCtx, stopFn := context.WithCancel(context.Background())
		defer stopFn()

		t.Logf("stopping consensus on relay %v %v", pubsubServers[2].Endpoint(), pubsubServers[2].Consensus.NodeID())
		err := pubsubServers[2].Consensus.Stop(stopCtx, 5*time.Second)
		time.Sleep(10 * time.Second) // wait for shutdown..
		t.Logf("stopped consensus on relay %v %v: %v", pubsubServers[2].Endpoint(), pubsubServers[2].Consensus.NodeID(), err)

		// 3 events each broadcast in its own block, we make sure of this with
		// the helperAwaitFinalized call before sending next events.
		eventsMissedByRelay3DuringBroadcastTime := []*model.Event{
			&model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Tags: model.Tags{
					{model.CustomIONTagOnBehalfOf, masterPubkey},
				},
				Content: "eventMissedByRelay3DuringBroadcastTime",
			}},
			&model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Tags: model.Tags{
					{model.CustomIONTagOnBehalfOf, masterPubkey},
				},
				Content: "eventMissedByRelay3DuringBroadcastTime2",
			}},
			&model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Tags: model.Tags{
					{model.CustomIONTagOnBehalfOf, masterPubkey},
				},
				Content: "eventMissedByRelay3DuringBroadcastTime3",
			}},
		}

		// broadcast+commit missedEvent #1
		eventMissedByRelay3DuringBroadcastTime = eventsMissedByRelay3DuringBroadcastTime[0]
		helperSignWithMinLeadingZeroBits(t, eventMissedByRelay3DuringBroadcastTime, privkey)
		require.NoError(t, relay.Publish(t.Context(), eventMissedByRelay3DuringBroadcastTime.Event))
		waitAcceptErr := helperAwaitConsensus(t, consensusDone, relay, thirdRelay)
		require.NoError(t, waitAcceptErr)
		require.NoError(t, helperAwaitFinalized(t, finalizedDone, thirdRelay))

		// missedEvent #1 sanity checks.
		receivedEventsFromFirstRelay := helperQueryEvents(t, t.Context(), relay, nostr.Filter{Kinds: []int{nostr.KindTextNote}})
		require.Contains(t, receivedEventsFromFirstRelay, eventMissedByRelay3DuringBroadcastTime)
		require.NotContains(t, receivedEventsFromFirstRelay, notAcceptedEvent)
		receivedEventsFromSecondRelay := helperQueryEvents(t, t.Context(), secondRelay, nostr.Filter{Kinds: []int{nostr.KindTextNote}})
		require.Contains(t, receivedEventsFromSecondRelay, eventMissedByRelay3DuringBroadcastTime)
		require.NotContains(t, receivedEventsFromSecondRelay, notAcceptedEvent)
		receivedEventsFromThirdRelay := helperQueryEvents(t, t.Context(), thirdRelay, nostr.Filter{Kinds: []int{nostr.KindTextNote}})
		require.NotContains(t, receivedEventsFromThirdRelay, eventMissedByRelay3DuringBroadcastTime)

		// broadcast+commit missedEvent #2
		eventMissedByRelay3DuringBroadcastTime2 = eventsMissedByRelay3DuringBroadcastTime[1]
		helperSignWithMinLeadingZeroBits(t, eventMissedByRelay3DuringBroadcastTime2, privkey)
		require.NoError(t, relay.Publish(t.Context(), eventMissedByRelay3DuringBroadcastTime2.Event))
		require.NoError(t, helperAwaitConsensus(t, consensusDone, relay, thirdRelay))
		require.NoError(t, helperAwaitFinalized(t, finalizedDone, thirdRelay))

		// broadcast+commit missedEvent #3
		eventMissedByRelay3DuringBroadcastTime3 = eventsMissedByRelay3DuringBroadcastTime[2]
		helperSignWithMinLeadingZeroBits(t, eventMissedByRelay3DuringBroadcastTime3, privkey)
		require.NoError(t, relay.Publish(t.Context(), eventMissedByRelay3DuringBroadcastTime3.Event))
		require.NoError(t, helperAwaitConsensus(t, consensusDone, relay, thirdRelay))
		require.NoError(t, helperAwaitFinalized(t, finalizedDone, thirdRelay))

		backendCtx, backendCancelFn := context.WithCancel(context.Background())
		defer backendCancelFn()

		t.Logf("starting consensus on relay %v %v", pubsubServers[2].Endpoint(), pubsubServers[2].Consensus.NodeID())
		pubsubServers[2].Consensus.Start(backendCtx)
		time.Sleep(10 * time.Second) // wait for "Start"
		t.Logf("started consensus on relay %v %v", pubsubServers[2].Endpoint(), pubsubServers[2].Consensus.NodeID())

		broadcastCtx, broadcastCancelFn := context.WithTimeout(backendCtx, 30*time.Second)
		defer broadcastCancelFn()

		// thirdRelay is back and will blocksync #1,#2,#3 before working on afterEvent #4.
		eventAfterNodeComesUp = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, masterPubkey},
			},
			Content: "eventAfterNodeComesUp",
		}}
		helperSignWithMinLeadingZeroBits(t, eventAfterNodeComesUp, privkey)
		require.NoError(t, relay.Publish(broadcastCtx, eventAfterNodeComesUp.Event))
		require.NoError(t, helperAwaitConsensus(t, consensusDone, relay))

		// require +3 blocks from thirdRelay only (others have already finalized those).
		require.NoError(t, helperAwaitFinalized(t, finalizedDone, relay, secondRelay, fourthRelay))
		require.NoError(t, helperAwaitFinalized(t, finalizedDone, relay, secondRelay, fourthRelay))
		require.NoError(t, helperAwaitFinalized(t, finalizedDone, relay, secondRelay, fourthRelay))
		// require block with eventAfterNodeComesUp from all
		require.NoError(t, helperAwaitFinalized(t, finalizedDone))

		// all events verification, sanity checks for queries.
		receivedEventsFromFirstRelay = helperQueryEvents(t, t.Context(), relay, nostr.Filter{Kinds: []int{nostr.KindTextNote}})
		require.Contains(t, receivedEventsFromFirstRelay, eventMissedByRelay3DuringBroadcastTime)
		require.Contains(t, receivedEventsFromFirstRelay, eventMissedByRelay3DuringBroadcastTime2)
		require.Contains(t, receivedEventsFromFirstRelay, eventMissedByRelay3DuringBroadcastTime3)
		require.Contains(t, receivedEventsFromFirstRelay, eventAfterNodeComesUp)
		require.NotContains(t, receivedEventsFromFirstRelay, notAcceptedEvent)
		receivedEventsFromSecondRelay = helperQueryEvents(t, t.Context(), secondRelay, nostr.Filter{Kinds: []int{nostr.KindTextNote}})
		require.Contains(t, receivedEventsFromSecondRelay, eventMissedByRelay3DuringBroadcastTime)
		require.Contains(t, receivedEventsFromSecondRelay, eventMissedByRelay3DuringBroadcastTime2)
		require.Contains(t, receivedEventsFromSecondRelay, eventMissedByRelay3DuringBroadcastTime3)
		require.Contains(t, receivedEventsFromSecondRelay, eventAfterNodeComesUp)
		require.NotContains(t, receivedEventsFromSecondRelay, notAcceptedEvent)
		receivedEventsFromThirdRelay = helperQueryEvents(t, broadcastCtx, thirdRelay, nostr.Filter{Kinds: []int{nostr.KindTextNote}})
		require.Contains(t, receivedEventsFromThirdRelay, eventAfterNodeComesUp)
		require.Contains(t, receivedEventsFromThirdRelay, eventMissedByRelay3DuringBroadcastTime)
		require.Contains(t, receivedEventsFromThirdRelay, eventMissedByRelay3DuringBroadcastTime2)
		require.Contains(t, receivedEventsFromThirdRelay, eventMissedByRelay3DuringBroadcastTime3)
		require.NotContains(t, receivedEventsFromThirdRelay, notAcceptedEvent)
	})

	t.Run("relay list is updated for the user including new relays to bootstrap", func(t *testing.T) {
		t.Skip("TODO: make it more predictable and reliable")
		pubsubServersExtra[0].Consensus.Start(t.Context())
		pubsubServersExtra[1].Consensus.Start(t.Context())
		relaysList := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindRelayListMetadata,
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, masterPubkey},
				{"r", pubsubServers[0].Endpoint()},
				{"r", pubsubServers[1].Endpoint()},
				{"r", pubsubServers[2].Endpoint()},
				{"r", pubsubServersExtra[0].Endpoint()},
				{"r", pubsubServersExtra[1].Endpoint()},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, relaysList, privkey)
		require.NoError(t, relay.Publish(t.Context(), relaysList.Event))
		time.Sleep(2 * time.Second)
		eventAfterBringingUpNewNode := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, masterPubkey},
			},
			Content: "eventAfterBringingUpNewNode",
		}}
		helperSignWithMinLeadingZeroBits(t, eventAfterBringingUpNewNode, privkey)
		require.NoError(t, relay.Publish(t.Context(), eventAfterBringingUpNewNode.Event))
		t.Logf("waiting for bootstrap data")
		time.Sleep(30 * time.Second) // Wait for bootstrap data.
		relay := helperMustNewRelay(t, pubsubServersExtra[0])
		receivedEventsFromFourthRelay := helperQueryEvents(t, t.Context(), relay, nostr.Filter{Kinds: []int{nostr.KindTextNote}})
		require.Contains(t, receivedEventsFromFourthRelay, ev)
		require.Contains(t, receivedEventsFromFourthRelay, ev2)
		require.NotContains(t, receivedEventsFromFourthRelay, notAcceptedEvent)
		helperMustCloseRelay(t, relay)
		for i := range pubsubServersExtra {
			t.Logf("shutting down extra server on port %v / %v", pubsubServersExtra[i].Endpoint(),
				pubsubServersExtra[i].Consensus.DiscoveryPort())
			require.NoError(t, pubsubServersExtra[i].Consensus.Stop(t.Context(), time.Minute*3))
		}
	})
	helperMustCloseRelay(t, relay)
	helperMustCloseRelay(t, secondRelay)
	helperMustCloseRelay(t, thirdRelay)
	helperMustCloseRelay(t, fourthRelay)
}

func BenchmarkConcurrentConsensusEvents(b *testing.B) {
	if os.Getenv("CI") != "" {
		b.Skip("skipping test on CI")
	}
	meter := tachymeter.New(&tachymeter.Config{Size: b.N})
	b.ResetTimer()
	b.ReportAllocs()
	b.Log(b.N)
	b.SetParallelism(50)
	usersCount := b.N
	mapPort := func(ctx context.Context) *fixture.MockService {
		var port, consensusPort uint16
		if portVal := ctx.Value("serverPort"); portVal != nil {
			port = portVal.(uint16)
		}
		if portVal := ctx.Value("consensusPort"); portVal != nil {
			consensusPort = portVal.(uint16)
		}

		for _, s := range pubsubServers {
			if port != 0 && strings.HasSuffix(s.Endpoint(), strconv.FormatInt(int64(port), 10)) {
				return s
			}
			if consensusPort != 0 && s.Consensus.DiscoveryPort() == consensusPort {
				return s
			}
		}
		return nil
	}
	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		if qErr := mapPort(ctx).DB.AcceptEvents(ctx, events...); qErr != nil {
			return qErr
		}
		if cErr := mapPort(ctx).Consensus.AcceptEvents(ctx, events...); cErr != nil {
			return cErr
		}
		return nil
	})
	command.RegisterRollbackListener(query.RollbackEvents)
	consensusDone := map[string]chan bool{}
	for _, s := range pubsubServers {
		consensusDone[s.Endpoint()] = make(chan bool, 10000)
	}
	command.RegisterAcceptListener(func(ctx context.Context, events ...*model.Event) error {
		if qErr := mapPort(ctx).DB.AcceptEvents(ctx, events...); qErr != nil {
			return qErr
		}
		consensusDone[mapPort(ctx).Endpoint()] <- true
		return nil
	})
	RegisterWSSubscriptionListener(func(ctx context.Context, filters ...model.Filter) EventIterator {
		return mapPort(ctx).DB.SelectEvents(ctx, filters...)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var ev *model.Event
	users := xsync.NewMap[string, string]()
	b.Run("Save attesttations", func(b *testing.B) {
		var wg sync.WaitGroup
		wg.Add(usersCount)
		completed := uint64(0)
		for range usersCount {
			go func() {
				defer func() {
					b.Log("COMPLETED: ", atomic.AddUint64(&completed, 1))
					wg.Done()
				}()
				var attestationEvent *model.Event
				masterPrivKey, masterPubkey := model.GenerateKeyPair()
				privkey, pk := model.GenerateKeyPair()
				users.Store(masterPubkey, masterPrivKey)
				attestationEvent = &model.Event{Event: nostr.Event{
					Kind:      model.CustomIONKindAttestation,
					CreatedAt: 1,
					Tags: model.Tags{
						{model.TagAttestationName, pk, "", model.CustomIONAttestationKindActive + ":" + strconv.Itoa(int(time.Now().Unix()-10))},
					},
				}}
				helperSignWithMinLeadingZeroBits(b, attestationEvent, masterPrivKey)
				relaysList := &model.Event{Event: nostr.Event{
					CreatedAt: nostr.Now(),
					Kind:      nostr.KindRelayListMetadata,
					Tags: model.Tags{
						{model.CustomIONTagOnBehalfOf, masterPubkey},
						{"r", pubsubServers[0].Endpoint()},
						{"r", pubsubServers[1].Endpoint()},
						{"r", pubsubServers[2].Endpoint()},
					},
				}}
				helperSignWithMinLeadingZeroBits(b, relaysList, privkey)
				require.NoError(b, helperPickRandomRelay(b).PublishMany(ctx, &attestationEvent.Event, &relaysList.Event))
				//helperAwaitConsensus(b, consensusDone)
			}()
		}
		wg.Wait()
	})
	keys := []string{}
	users.Range(func(k, v string) bool {
		keys = append(keys, k)
		return true
	})
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			r := helperPickRandomRelay(b)
			usrIdx := rand.IntN(users.Size())
			privkey, ok := users.Load(keys[usrIdx])
			require.True(b, ok)
			ev = &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Tags: model.Tags{
					{model.CustomIONTagOnBehalfOf, keys[usrIdx]},
				},
				Content: "validEvent from relay1",
			}}
			helperSignWithMinLeadingZeroBits(b, ev, privkey)
			start := time.Now()
			require.NoError(b, r.Publish(ctx, ev.Event))
			meter.AddTime(time.Since(start))
		}
	})
	helperBenchReportMetrics(b, meter)
}

func helperPickRandomRelay(tb testing.TB) *nostrRelay {
	tb.Helper()
	idx := rand.IntN(len(pubsubServers))
	relay := helperMustNewRelay(tb, pubsubServers[idx])

	return relay
}

// Wait (concurrently) for all relays except broadcastFrom to accept.
// consensusDone is written on by the consensusEventListener (AcceptBroadcastTx).
func helperAwaitConsensus(t testing.TB, consensusDone map[string]chan bool, broadcastFrom ...*nostrRelay) error {
	t.Helper()
	skipUrls := make([]string, 0, len(broadcastFrom))
	for _, skipRelay := range broadcastFrom {
		skipUrls = append(skipUrls, skipRelay.URL)
	}

	var wg sync.WaitGroup
	errChan := make(chan error, 1)

	for endpoint, done := range consensusDone {
		if slices.Contains(skipUrls, endpoint) {
			continue
		}

		wg.Add(1)
		go func(ep string, ch chan bool) {
			defer wg.Done()
			select {
			case <-ch:
				return
			case <-time.After(30 * time.Second):
				select {
				case errChan <- errors.Errorf("timeout awaiting consensus from %v", ep):
				default:
				}
			}
		}(endpoint, done)
	}

	wg.Wait()
	close(errChan)

	return <-errChan
}

// Wait (concurrently) for all relays except skipRelays to commit.
// consensusDone is written on by the commitEventListener (CommitBroadcastTx).
func helperAwaitFinalized(t testing.TB, finalizedDone map[string]chan bool, skipRelays ...*nostrRelay) error {
	t.Helper()
	skipUrls := make([]string, 0, len(skipRelays))
	for _, skipRelay := range skipRelays {
		skipUrls = append(skipUrls, skipRelay.URL)
	}

	var wg sync.WaitGroup
	errChan := make(chan error, 1)

	for endpoint, done := range finalizedDone {
		if slices.Contains(skipUrls, endpoint) {
			continue
		}

		wg.Add(1)
		go func(ep string, ch chan bool) {
			defer wg.Done()
			select {
			case <-ch:
				return
			case <-time.After(60 * time.Second):
				select {
				case errChan <- errors.Errorf("timeout awaiting block commit from %v", ep):
				default:
				}
			}
		}(endpoint, done)
	}

	wg.Wait()
	close(errChan)

	return <-errChan
}

func helperBenchReportMetrics(
	t interface {
		Helper()
		ReportMetric(float64, string)
	},
	meter *tachymeter.Tachymeter,
) {
	t.Helper()

	metric := meter.Calc()
	t.ReportMetric(float64(metric.Time.Avg.Milliseconds()), "avg-ms/op")
	t.ReportMetric(float64(metric.Time.StdDev.Milliseconds()), "stddev-ms/op")
	t.ReportMetric(float64(metric.Time.P50.Milliseconds()), "p50-ms/op")
	t.ReportMetric(float64(metric.Time.P95.Milliseconds()), "p95-ms/op")
}

func helperHashEvents(t testing.TB, events ...*model.Event) (hash string) {
	t.Helper()

	var buf bytes.Buffer

	for _, e := range events {
		buf.WriteString(e.Address())
	}
	sum := sha256.Sum256(buf.Bytes())

	return string(sum[:])
}
