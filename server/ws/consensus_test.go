// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"bytes"
	"context"
	"crypto/sha256"
	"github.com/cockroachdb/errors"
	"github.com/google/uuid"
	"math/rand/v2"
	"os"

	//"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jamiealquiza/tachymeter"
	"github.com/nbd-wtf/go-nostr"
	"github.com/puzpuzpuz/xsync/v3"
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
			if consensusPort != 0 && s.Consenus.DiscoveryPort() == consensusPort {
				return s
			}
		}
		return nil
	}

	defer func() {
		require.NoError(t, os.RemoveAll("../../.cometbft"))
		require.NoError(t, os.RemoveAll("../../.cometbft2"))
		require.NoError(t, os.RemoveAll("../../.cometbft3"))
	}()

	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		t.Logf("received events: %v on %v", events, ctx.Value("serverPort"))
		if qErr := mapPort(ctx).DB.AcceptEvents(ctx, events...); qErr != nil {
			return qErr
		}
		if cErr := mapPort(ctx).Consenus.AcceptEvents(ctx, events...); cErr != nil {
			return cErr
		}
		return nil
	})
	command.RegisterRollbackListener(query.RollbackEvents)
	consensusDone := map[string]chan bool{}
	for _, s := range pubsubServers {
		consensusDone[s.Endpoint()] = make(chan bool, 1000)
	}
	accepted := make(map[string]bool)
	command.RegisterAcceptListener(func(ctx context.Context, events ...*model.Event) error {
		if qErr := mapPort(ctx).DB.AcceptEvents(ctx, events...); qErr != nil {
			return qErr
		}
		if _, ok := accepted[mapPort(ctx).Endpoint()+helperHashEvents(t, events...)]; ok {
			return nil
		}
		consensusDone[mapPort(ctx).Endpoint()] <- true
		accepted[mapPort(ctx).Endpoint()+helperHashEvents(t, events...)] = true
		t.Log("ACCEPT", mapPort(ctx).Endpoint(), events[0].Kind, events[0].Content)
		return nil
	})
	RegisterWSSubscriptionListener(func(ctx context.Context, subscription *model.Subscription) EventIterator {
		var filters model.Filters
		if subscription != nil {
			filters = subscription.Filters
		}
		return mapPort(ctx).DB.SelectEvents(ctx, filters...)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	relay := helperMustNewRelay(t, pubsubServers[0])
	var ev, ev2 *model.Event
	var attestationEvent *model.Event
	masterPrivKey, masterPubkey := model.GenerateKeyPair()
	t.Run("SaveEvent", func(t *testing.T) {
		attestationEvent = &model.Event{Event: nostr.Event{
			Kind:      model.CustomIONKindAttestation,
			CreatedAt: 1,
			Tags: model.Tags{
				{model.TagAttestationName, pk, "", model.CustomIONAttestationKindActive + ":" + strconv.Itoa(int(time.Now().Unix()-10))},
			},
		}}
		require.NoError(t, attestationEvent.SignWithAlg(masterPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, attestationEvent.GenerateNIP13(context.Background(), NIP13MinLeadingZeroBits))
		require.NoError(t, attestationEvent.SignWithAlg(masterPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		relaysList := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindRelayListMetadata,
			Tags: nostr.Tags{
				[]string{model.CustomIONTagOnBehalfOf, masterPubkey},
				[]string{"r", pubsubServers[0].Endpoint()},
				[]string{"r", pubsubServers[1].Endpoint()},
				[]string{"r", pubsubServers[2].Endpoint()},
			},
		}}
		helperSignWithMinLeadingZeroBits(t, relaysList, privkey)
		require.NoError(t, relay.PublishMany(ctx, &attestationEvent.Event, &relaysList.Event))
		helperAwaitConsensus(t, relay, consensusDone)
		ev = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.CustomIONKindEditableTextNote,
			Tags: nostr.Tags{
				[]string{model.CustomIONTagOnBehalfOf, masterPubkey},
				[]string{"published_at", strconv.FormatInt(time.Now().Unix(), 10)},
				[]string{"d", uuid.NewString()},
			},
			Content: "validEvent from relay1",
		}}
		helperSignWithMinLeadingZeroBits(t, ev, privkey)

		require.NoError(t, relay.Publish(ctx, ev.Event))
		helperAwaitConsensus(t, relay, consensusDone)
	})
	secondRelay := helperMustNewRelay(t, pubsubServers[1])
	t.Run("query events", func(t *testing.T) {
		receivedEventsFromFirstRelay := helperQueryEvents(t, ctx, relay, nostr.Filter{Kinds: []int{model.CustomIONKindEditableTextNote}})
		require.Len(t, receivedEventsFromFirstRelay, 1)
		require.Contains(t, receivedEventsFromFirstRelay, ev)
		receivedEventsFromSecondRelay := helperQueryEvents(t, ctx, secondRelay, nostr.Filter{Kinds: []int{model.CustomIONKindEditableTextNote}})
		require.Len(t, receivedEventsFromSecondRelay, 1)
		require.Equal(t, receivedEventsFromFirstRelay, receivedEventsFromSecondRelay)
		ev2 = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.CustomIONKindEditableTextNote,
			Tags: nostr.Tags{
				[]string{model.CustomIONTagOnBehalfOf, masterPubkey},
				[]string{"published_at", strconv.FormatInt(time.Now().Unix(), 10)},
				[]string{"d", uuid.NewString()},
			},
			Content: "validEvent from relay2",
		}}
		helperSignWithMinLeadingZeroBits(t, ev2, privkey)
		require.NoError(t, secondRelay.Publish(ctx, ev2.Event))
		helperAwaitConsensus(t, secondRelay, consensusDone)
		receivedEventsFromFirstRelay = helperQueryEvents(t, ctx, relay, nostr.Filter{Kinds: []int{model.CustomIONKindEditableTextNote}})
		require.Len(t, receivedEventsFromFirstRelay, 2)
		require.Contains(t, receivedEventsFromFirstRelay, ev)
		require.Contains(t, receivedEventsFromFirstRelay, ev2)
	})

	command.RegisterAcceptListener(func(ctx context.Context, events ...*model.Event) error {
		if ctx.Value("consensusPort").(uint16) == 19977 {
			return errors.New("simulating remote relay did not accept tx - it should be rolled back")
		}
		if qErr := mapPort(ctx).DB.AcceptEvents(ctx, events...); qErr != nil {
			return qErr
		}
		if _, ok := accepted[mapPort(ctx).Endpoint()+helperHashEvents(t, events...)]; ok {
			return nil
		}
		consensusDone[mapPort(ctx).Endpoint()] <- true
		accepted[mapPort(ctx).Endpoint()+helperHashEvents(t, events...)] = true
		t.Log("ACCEPT", mapPort(ctx).Endpoint(), events[0].Kind, events[0].Content)

		return nil
	})

	var notAcceptedEvent *model.Event
	thirdRelay := helperMustNewRelay(t, pubsubServers[2])
	t.Run("failed consensus rolled back", func(t *testing.T) {
		rolledBack := map[string]chan bool{}
		for _, s := range pubsubServers {
			rolledBack[s.Endpoint()] = make(chan bool, 1000)
		}
		command.RegisterRollbackListener(func(ctx context.Context, event ...*model.Event) error {
			err := mapPort(ctx).DB.RollbackEvents(ctx, event...)
			rolledBack[mapPort(ctx).Endpoint()] <- true
			t.Log("ROLLED BACK", mapPort(ctx).Endpoint(), event[0].Kind, event[0].Content)

			return err
		})
		notAcceptedEvent = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.CustomIONKindEditableTextNote,
			Tags: nostr.Tags{
				[]string{model.CustomIONTagOnBehalfOf, masterPubkey},
				[]string{"published_at", strconv.FormatInt(time.Now().Unix(), 10)},
				[]string{"d", uuid.NewString()},
			},
			Content: "validEvent not gonna be accepted because of failed consensus",
		}}
		helperSignWithMinLeadingZeroBits(t, notAcceptedEvent, privkey)
		require.Error(t, relay.Publish(ctx, notAcceptedEvent.Event))
		helperAwaitConsensus(t, relay, rolledBack)
		receivedEventsFromFirstRelay := helperQueryEvents(t, ctx, relay, nostr.Filter{Kinds: []int{model.CustomIONKindEditableTextNote}})
		receivedEventsFromSecondRelay := helperQueryEvents(t, ctx, secondRelay, nostr.Filter{Kinds: []int{model.CustomIONKindEditableTextNote}})
		receivedEventsFromThirdRelay := helperQueryEvents(t, ctx, thirdRelay, nostr.Filter{Kinds: []int{model.CustomIONKindEditableTextNote}})

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

	helperMustCloseRelay(t, relay)

	//command.MustInit(t.Context(), command.WithConfig(&command.Config{
	//	AbsoluteRootPath:        "../../.cometbft3",
	//	NodePrivKey:             "./../database/command/.testdata/node_key3.json",
	//	DiscoveryPort:           19966,
	//	helperSignWithMinLeadingZeroBits(t, relaysList, privkey)
	//	require.NoError(t, relay.Publish(ctx, relaysList.Event))
	//})	//	NIP13MinLeadingZeroBits: 0,
	//}))
	//t.Run("add relay", func(t *testing.T) {
	//	relaysList := &model.Event{Event: nostr.Event{
	//		CreatedAt: nostr.Timestamp(time.Now().Unix()),
	//		Kind:      nostr.KindRelayListMetadata,
	//		Tags: nostr.Tags{
	//			[]string{model.CustomIONTagOnBehalfOf, masterPubkey},
	//			[]string{"r", pubsubServers[0].Endpoint()},
	//			[]string{"r", pubsubServers[1].Endpoint()},
	//			[]string{"r", pubsubServers[2].Endpoint()},
	//		},
	//	}}

	//var notAcceptedEvent *model.Event
	//t.Run("failed consensus rolled back", func(t *testing.T) {
	//	command.RegisterAcceptListener(func(ctx context.Context, event ...*model.Event) error {
	//		require.NoError(t, query.AcceptEvents(ctx, event...))
	//		return errors.New("rejected")
	//	})
	//	var rolledBack bool
	//	command.RegisterRollbackListener(func(ctx context.Context, event ...*model.Event) error {
	//		rolledBack = true
	//		return query.RollbackEvents(ctx, event...)
	//	})
	//	notAcceptedEvent = &model.Event{Event: nostr.Event{
	//		CreatedAt: nostr.Timestamp(time.Now().Unix()),
	//		Kind:      nostr.KindTextNote,
	//		Tags: nostr.Tags{
	//			[]string{model.CustomIONTagOnBehalfOf, masterPubkey},
	//		},
	//		Content: "validEvent not gonna be accepted because of failed consensus",
	//	}}
	//	helperSignWithMinLeadingZeroBits(t, ev, privkey)
	//	require.Error(t, relay.Publish(ctx, ev.Event))
	//	require.True(t, rolledBack)
	//})
	//t.Run("query events", func(t *testing.T) {
	//	events, err := relay.QuerySync(ctx, nostr.Filter{Kinds: []int{nostr.KindTextNote}})
	//	require.NoError(t, err)
	//	var receivedEvents []*model.Event
	//	for _, recEv := range events {
	//		receivedEvents = append(receivedEvents, &model.Event{Event: *recEv})
	//	}
	//	require.Len(t, receivedEvents, 1)
	//	require.Contains(t, receivedEvents, ev)
	//	require.Contains(t, receivedEvents, attestationEvent)
	//	require.NotContains(t, receivedEvents, notAcceptedEvent)
	//})

	helperMustCloseRelay(t, relay)
}

func BenchmarkConcurrentConsensusEvents(b *testing.B) {
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
			if consensusPort != 0 && s.Consenus.DiscoveryPort() == consensusPort {
				return s
			}
		}
		return nil
	}
	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		if qErr := mapPort(ctx).DB.AcceptEvents(ctx, events...); qErr != nil {
			return qErr
		}
		if cErr := mapPort(ctx).Consenus.AcceptEvents(ctx, events...); cErr != nil {
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
	RegisterWSSubscriptionListener(func(ctx context.Context, subscription *model.Subscription) EventIterator {
		var filters model.Filters
		if subscription != nil {
			filters = subscription.Filters
		}
		return mapPort(ctx).DB.SelectEvents(ctx, filters...)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var ev *model.Event
	users := xsync.NewMapOf[string, string]()
	b.Run("Save attesttations", func(b *testing.B) {
		var wg sync.WaitGroup
		wg.Add(usersCount)
		completed := uint64(0)
		for _ = range usersCount {
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
				require.NoError(b, attestationEvent.SignWithAlg(masterPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
				require.NoError(b, attestationEvent.GenerateNIP13(context.Background(), NIP13MinLeadingZeroBits))
				require.NoError(b, attestationEvent.SignWithAlg(masterPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

				relaysList := &model.Event{Event: nostr.Event{
					CreatedAt: nostr.Timestamp(time.Now().Unix()),
					Kind:      nostr.KindRelayListMetadata,
					Tags: nostr.Tags{
						[]string{model.CustomIONTagOnBehalfOf, masterPubkey},
						[]string{"r", pubsubServers[0].Endpoint()},
						[]string{"r", pubsubServers[1].Endpoint()},
						[]string{"r", pubsubServers[2].Endpoint()},
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
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.CustomIONKindEditableTextNote,
				Tags: nostr.Tags{
					[]string{model.CustomIONTagOnBehalfOf, keys[usrIdx]},
					[]string{"published_at", strconv.FormatInt(time.Now().Unix(), 10)},
					[]string{"d", uuid.NewString()},
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

func helperAwaitConsensus(t testing.TB, broadcastFrom *nostrRelay, consensusDone map[string]chan bool) {
	t.Helper()
	for endpoint, done := range consensusDone {
		select {
		case <-done:
			break
		case <-time.After(30 * time.Second):
			t.Fatalf("timeout awaiting consensus from %v", endpoint)
		}
	}
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
