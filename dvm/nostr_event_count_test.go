// SPDX-License-Identifier: ice License 1.0

package dvm

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

func TestMain(m *testing.M) {
	ctx, cancel := context.WithCancel(context.Background())
	query.MustInit(ctx)
	MustInit()

	code := m.Run()
	cancel()

	if code == 0 {
		if err := goleak.Find(); err != nil {
			log.Printf("goleak: %v", err)
			code = 1
		}
	}

	os.Exit(code)
}

func TestCollectRelayURLs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		event    *model.Event
		expected []string
	}{
		{
			name: "no relay URLs",
			event: &model.Event{
				Event: nostr.Event{
					Tags: nostr.Tags{
						{"a", "1234567890abcde1", "wss://relay.example.com"},
						{"e", "1234567890abcde2", "wss://relay.example.com"},
						{"p", "1234567890abcde3", "wss://relay.example.com"},
					},
				},
			},
			expected: nil,
		},
		{
			name: "one relay URL",
			event: &model.Event{
				Event: nostr.Event{
					Tags: nostr.Tags{
						{"param", "relay", "wss://relay.example.com"},
					},
				},
			},
			expected: []string{"wss://relay.example.com"},
		},
		{
			name: "multiple relay URLs",
			event: &model.Event{
				Event: nostr.Event{
					Tags: nostr.Tags{
						{"param", "relay", "wss://relay1.example.com"},
						{"param", "relay", "wss://relay2.example.com"},
						{"param", "relay", "wss://relay3.example.com"},
					},
				},
			},
			expected: []string{"wss://relay1.example.com", "wss://relay2.example.com", "wss://relay3.example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := collectRelayURLsFromEvent(tt.event)
			require.Equal(t, tt.expected, actual)
		})
	}
}

func TestIsBidAmountEnough(t *testing.T) {
	t.Parallel()

	t.Run("required amount is 0", func(t *testing.T) {
		n := newNostrEventCountJob(nil)
		tests := []struct {
			amount   string
			expected bool
		}{
			{"0.00000001", true},
			{"0.00000000", true},
			{"0.0000000", true},
			{"0.000", true},
			{"0", true},
			{"", true},
			{"   ", true},
			{"0.00000001 ", true},
			{" 0.00000001", true},
			{"0.00000001\n", true},
			{"\n0.00000001", true},
			{"0.0000001", true},
			{"0.000001", true},
			{"0.00001", true},
			{"0.0001", true},
			{"0.001", true},
			{"0.01", true},
			{"0.1", true},
			{"1", true},
		}
		for _, tt := range tests {
			t.Run(tt.amount, func(t *testing.T) {
				actual := n.IsBidAmountEnough(tt.amount)
				require.Equal(t, tt.expected, actual)
			})
		}
	})
}

func helperExecuteJob(t *testing.T, req *model.Event) *model.Event {
	t.Helper()

	pk, err := PublicKey()
	require.NoError(t, err)
	req.Tags = append(req.Tags, model.Tag{model.CustomIONTagOnBehalfOf, pk})

	req.CreatedAt = model.Timestamp(time.Now().Unix())
	require.NoError(t, req.SignWithAlg(globalDVM.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, req.Validate())

	job := newNostrEventCountJob(nil)
	require.NotNil(t, job)

	payload, err := job.Process(context.Background(), req)
	require.NoError(t, err)

	result, err := globalDVM.finalizeJob(req, payload, job.RequiredPaymentAmount())
	require.NoError(t, err)

	return result
}

func helperReadFromDB(t *testing.T, req model.Filter) *model.Event {
	var result *model.Event

	it := query.GetStoredEvents(context.Background(), &model.Subscription{Filters: model.Filters{req}})
	for ev, err := range it {
		require.NoError(t, err)
		require.NotNil(t, ev)
		if ev.Kind == model.KindDVMCountResponse {
			result = ev
			break
		}
	}
	require.NotNil(t, result)

	return result
}

func helperCompareResults(t *testing.T, a, b *model.Event) {
	t.Helper()

	if a.CreatedAt != b.CreatedAt {
		a.CreatedAt, b.CreatedAt = 0, 0
		a.Sig, b.Sig = "", ""
	}

	require.JSONEq(t, a.String(), b.String())
}

func TestEventCountersConsistency(t *testing.T) {
	t.Parallel()

	pub, err := model.GetPublicKey(globalDVM.PrivateKey)
	require.NoError(t, err)

	cases := []struct {
		Name       string
		RequestDVM model.Event
		RequestDB  model.Filter
		Count      string
		Events     func(t *testing.T) []*model.Event
		Before     func(t *testing.T, events []*model.Event, reqDB *model.Filter, reqDVM *model.Event)
	}{
		{
			Name: "For every kind 1 that the subscription finds also include the count of replies that it has",
			RequestDB: model.Filter{
				Authors: []string{pub},
				Tags:    model.TagMap{}.SetLiterals("x", "y"),
				Search:  "include:dependencies:kind1>kind6400+kind1+group+root",
			},
			RequestDVM: model.Event{
				Event: nostr.Event{
					Kind: model.KindJobNostrEventCount,
					Tags: model.Tags{
						model.Tag{"param", "group", "root"},
						model.Tag{"param", "relay", globalConfig.RelayURL},
					},
				},
			},
			Before: func(t *testing.T, events []*model.Event, reqDB *model.Filter, reqDVM *model.Event) {
				reqDVM.Content = `[{"kinds":[1],"#e":["` + events[0].ID + `"]}]`
			},
			Events: func(t *testing.T) []*model.Event {
				var ev1 model.Event

				ev1.Kind = nostr.KindTextNote
				ev1.Content = "Hello world!"
				ev1.Tags = append(ev1.Tags, model.Tag{"x", "y"})
				require.NoError(t, ev1.SignWithAlg(globalDVM.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

				var reply1 model.Event
				reply1.CreatedAt = 1
				reply1.Kind = nostr.KindTextNote
				reply1.Tags = append(reply1.Tags, model.Tag{"e", ev1.ID, "", model.TagMarkerRoot})
				require.NoError(t, reply1.SignWithAlg(globalDVM.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

				var reply2 model.Event
				reply2.CreatedAt = 2
				reply2.Kind = nostr.KindTextNote
				reply2.Tags = reply1.Tags
				require.NoError(t, reply2.SignWithAlg(globalDVM.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

				return []*model.Event{&ev1, &reply1, &reply2}
			},
			Count: "2",
		},
		{
			Name: "For every kind 1 that the subscription finds also include the count of reposts that it has",
			RequestDB: model.Filter{
				Authors: []string{pub},
				Tags:    model.TagMap{}.SetLiterals("x", "y"),
				Search:  "include:dependencies:kind1>kind6400+kind6+group+e",
			},
			RequestDVM: model.Event{
				Event: nostr.Event{
					Kind: model.KindJobNostrEventCount,
					Tags: model.Tags{
						model.Tag{"param", "group", "e"},
						model.Tag{"param", "relay", globalConfig.RelayURL},
					},
				},
			},
			Before: func(t *testing.T, events []*model.Event, reqDB *model.Filter, reqDVM *model.Event) {
				reqDVM.Content = `[{"kinds":[6],"#e":["` + events[0].ID + `"]}]`
			},
			Events: func(t *testing.T) []*model.Event {
				var ev1 model.Event

				ev1.Kind = nostr.KindTextNote
				ev1.Content = "Hello world!"
				ev1.Tags = append(ev1.Tags, model.Tag{"x", "y"})
				require.NoError(t, ev1.SignWithAlg(globalDVM.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

				var repost1 model.Event
				repost1.CreatedAt = 1
				repost1.Kind = nostr.KindRepost
				repost1.Content = ev1.String()
				repost1.Tags = append(repost1.Tags, model.Tag{"e", ev1.ID})
				require.NoError(t, repost1.SignWithAlg(globalDVM.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

				var repost2 model.Event
				repost2.CreatedAt = 2
				repost2.Kind = nostr.KindRepost
				repost2.Content = ev1.String()
				repost2.Tags = repost1.Tags
				require.NoError(t, repost2.SignWithAlg(globalDVM.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

				return []*model.Event{&ev1, &repost1, &repost2}
			},
			Count: "2",
		},
		{
			Name: "For every kind 1 that the subscription finds also include the count of quotes that it has",
			RequestDB: model.Filter{
				Authors: []string{pub},
				Tags:    model.TagMap{}.SetLiterals("x", "y"),
				Search:  "include:dependencies:kind1>kind6400+kind1+group+q",
			},
			RequestDVM: model.Event{
				Event: nostr.Event{
					Kind: model.KindJobNostrEventCount,
					Tags: model.Tags{
						model.Tag{"param", "group", "q"},
						model.Tag{"param", "relay", globalConfig.RelayURL},
					},
				},
			},
			Before: func(t *testing.T, events []*model.Event, reqDB *model.Filter, reqDVM *model.Event) {
				reqDVM.Content = `[{"kinds":[1],"#q":["` + events[0].ID + `"]}]`
			},
			Events: func(t *testing.T) []*model.Event {
				var ev1 model.Event

				ev1.Kind = nostr.KindTextNote
				ev1.Content = "Hello world!"
				ev1.Tags = append(ev1.Tags, model.Tag{"x", "y"})
				require.NoError(t, ev1.SignWithAlg(globalDVM.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

				var quote1 model.Event
				quote1.CreatedAt = 1
				quote1.Kind = nostr.KindTextNote
				quote1.Tags = append(quote1.Tags, model.Tag{"q", ev1.ID})
				require.NoError(t, quote1.SignWithAlg(globalDVM.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

				var quote2 model.Event
				quote2.CreatedAt = 2
				quote2.Kind = nostr.KindTextNote
				quote2.Tags = quote1.Tags
				require.NoError(t, quote2.SignWithAlg(globalDVM.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

				return []*model.Event{&ev1, &quote1, &quote2}
			},
			Count: "2",
		},
		{
			Name: "For every kind 1 that the subscription finds also include the count of reactions that it has",
			RequestDB: model.Filter{
				Authors: []string{pub},
				Tags:    model.TagMap{}.SetLiterals("x", "y"),
				Search:  "include:dependencies:kind1>kind6400+kind7+group+content",
			},
			RequestDVM: model.Event{
				Event: nostr.Event{
					Kind: model.KindJobNostrEventCount,
					Tags: model.Tags{
						model.Tag{"output", "JSON"},
						model.Tag{"param", "group", "content"},
						model.Tag{"param", "relay", globalConfig.RelayURL},
					},
				},
			},
			Before: func(t *testing.T, events []*model.Event, reqDB *model.Filter, reqDVM *model.Event) {
				reqDVM.Content = `[{"kinds":[7],"#e":["` + events[0].ID + `"]}]`
			},
			Events: func(t *testing.T) []*model.Event {
				var ev1 model.Event

				ev1.Kind = nostr.KindTextNote
				ev1.Content = "Hello world!"
				ev1.Tags = append(ev1.Tags, model.Tag{"x", "y"})
				require.NoError(t, ev1.SignWithAlg(globalDVM.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

				var reaction1 model.Event
				reaction1.CreatedAt = 1
				reaction1.Kind = nostr.KindReaction
				reaction1.Content = "-"
				reaction1.Tags = append(reaction1.Tags, model.Tag{"e", ev1.ID})
				require.NoError(t, reaction1.SignWithAlg(globalDVM.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

				var reaction2 model.Event
				reaction2.CreatedAt = 2
				reaction2.Content = "+"
				reaction2.Kind = nostr.KindReaction
				reaction2.Tags = reaction1.Tags
				require.NoError(t, reaction2.SignWithAlg(globalDVM.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

				return []*model.Event{&ev1, &reaction1, &reaction2}
			},
			Count: `{"+":1,"-":1}`,
		},
		{
			Name: "For every kind 0 that the subscription finds also include the count of followers that it has",
			RequestDB: model.Filter{
				Kinds:  []int{nostr.KindProfileMetadata},
				Search: "include:dependencies:kind0>kind6400+kind3+group+p",
			},
			RequestDVM: model.Event{
				Event: nostr.Event{
					Kind:    model.KindJobNostrEventCount,
					Content: `[{"kinds":[3],"#p":["` + pub + `"]}]`,
					Tags: model.Tags{
						model.Tag{"param", "group", "p"},
						model.Tag{"param", "relay", globalConfig.RelayURL},
					},
				},
			},
			Events: func(t *testing.T) []*model.Event {
				var ev1 model.Event

				ev1.Kind = nostr.KindProfileMetadata
				ev1.Content = `{"name:":"Alice"}`
				ev1.Tags = append(ev1.Tags, model.Tag{"imeta", "url https://foo.barr"})
				require.NoError(t, ev1.SignWithAlg(globalDVM.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

				events := []*model.Event{&ev1}
				for range 42 {
					var ev model.Event

					ev.Kind = nostr.KindFollowList
					ev.Tags = append(ev.Tags, model.Tag{"p", ev1.PubKey, "wss://alicerelay.com/", "alice"})
					require.NoError(t, ev.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

					events = append(events, &ev)
				}

				return events
			},
			Count: "42",
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			events := c.Events(t)
			if c.Before != nil {
				c.Before(t, events, &c.RequestDB, &c.RequestDVM)
			}
			t.Run("Insert", func(t *testing.T) {
				require.NoError(t, query.AcceptEvents(context.Background(), events...))
			})
			t.Run("Do", func(t *testing.T) {
				resultDB := helperReadFromDB(t, c.RequestDB)
				resultDVM := helperExecuteJob(t, &c.RequestDVM)
				helperCompareResults(t, resultDB, resultDVM)
				require.Equal(t, c.Count, resultDB.Content)
			})
		})
	}
}
