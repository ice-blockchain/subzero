// SPDX-License-Identifier: ice License 1.0

package dvm

import (
	"context"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

func TestCountBasedOnGroups(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		evList   []*nostr.Event
		groups   []string
		expected map[string]uint64
	}{
		{
			name:     "tag marker reply empty",
			evList:   []*nostr.Event{},
			groups:   []string{model.TagMarkerReply},
			expected: map[string]uint64{},
		},
		{
			name: "tag marker reply",
			evList: []*nostr.Event{
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde1", "", model.TagMarkerReply},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde1", "", model.TagMarkerReply},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde2", "", model.TagMarkerReply},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde2", "", model.TagMarkerReply},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde2", "", model.TagMarkerReply},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde3", "", model.TagMarkerReply},
					},
				},
			},
			groups: []string{model.TagMarkerReply},
			expected: map[string]uint64{
				"1234567890abcde1": 2,
				"1234567890abcde2": 3,
				"1234567890abcde3": 1,
			},
		},
		{
			name:     "tag marker reply empty",
			evList:   []*nostr.Event{},
			groups:   []string{model.TagMarkerRoot},
			expected: map[string]uint64{},
		},
		{
			name: "tag marker root",
			evList: []*nostr.Event{
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde1", "", model.TagMarkerRoot},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde2", "", model.TagMarkerRoot},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde2", "", model.TagMarkerRoot},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde2", "", model.TagMarkerRoot},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde2", "", model.TagMarkerRoot},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde3", "", model.TagMarkerRoot},
					},
				},
			},
			groups: []string{model.TagMarkerRoot},
			expected: map[string]uint64{
				"1234567890abcde1": 1,
				"1234567890abcde2": 4,
				"1234567890abcde3": 1,
			},
		},
		{
			name:     "group by reactions, empty",
			evList:   []*nostr.Event{},
			groups:   []string{NostrEventCountGroupContent},
			expected: map[string]uint64{},
		},
		{
			name: "group by reactions",
			evList: []*nostr.Event{
				{
					Content: "+",
				},
				{
					Content: "-",
				},
				{
					Content: "🤙",
				},
				{
					Content: "🤣",
				},
				{
					Content: "🤣",
				},
				{
					Content: "❤️",
				},
				{
					Content: "🍺",
				},
				{
					Content: "🍺",
				},
				{
					Content: "🍺",
				},
			},
			groups: []string{NostrEventCountGroupContent},
			expected: map[string]uint64{
				"+":  1,
				"-":  1,
				"🤙":  1,
				"🤣":  2,
				"❤️": 1,
				"🍺":  3,
			},
		},
		{
			name:     "group by pubkey, empty list",
			evList:   []*nostr.Event{},
			groups:   []string{NostrEventCountGroupPubkey},
			expected: map[string]uint64{},
		},
		{
			name: "events with different pubkey",
			evList: []*nostr.Event{
				{
					PubKey: "1234567890abcde1",
				},
				{
					PubKey: "1234567890abcde1",
				},
				{
					PubKey: "1234567890abcde2",
				},
				{
					PubKey: "1234567890abcde3",
				},
				{
					PubKey: "1234567890abcde3",
				},
				{
					PubKey: "1234567890abcde3",
				},
				{
					PubKey: "1234567890abcde3",
				},
			},
			groups: []string{NostrEventCountGroupPubkey},
			expected: map[string]uint64{
				"1234567890abcde1": 2,
				"1234567890abcde2": 1,
				"1234567890abcde3": 4,
			},
		},
		{
			name: "All groups in 1 request",
			evList: []*nostr.Event{
				{
					PubKey: "1234567890abcde1",
				},
				{
					PubKey: "1234567890abcde1",
				},
				{
					PubKey: "1234567890abcde2",
				},
				{
					PubKey: "1234567890abcde3",
				},
				{
					PubKey: "1234567890abcde3",
				},
				{
					PubKey: "1234567890abcde3",
				},
				{
					PubKey: "1234567890abcde3",
				},
				{
					Content: "+",
				},
				{
					Content: "-",
				},
				{
					Content: "🤙",
				},
				{
					Content: "🤣",
				},
				{
					Content: "🤣",
				},
				{
					Content: "❤️",
				},
				{
					Content: "🍺",
				},
				{
					Content: "🍺",
				},
				{
					Content: "🍺",
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde1", "", model.TagMarkerRoot},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde2", "", model.TagMarkerRoot},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde2", "", model.TagMarkerRoot},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde2", "", model.TagMarkerRoot},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde2", "", model.TagMarkerRoot},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde3", "", model.TagMarkerRoot},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde1", "", model.TagMarkerReply},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde1", "", model.TagMarkerReply},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde2", "", model.TagMarkerReply},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde2", "", model.TagMarkerReply},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde2", "", model.TagMarkerReply},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde3", "", model.TagMarkerReply},
					},
				},
			},
			groups: []string{NostrEventCountGroupPubkey, model.TagMarkerRoot, NostrEventCountGroupContent, model.TagMarkerReply},
			expected: map[string]uint64{
				"":                 28,
				"+":                1,
				"-":                1,
				"1234567890abcde1": 5,
				"1234567890abcde2": 8,
				"1234567890abcde3": 6,
				"Hello world!":     12,
				"❤️":               1,
				"🍺":                3,
				"🤙":                1,
				"🤣":                2,
			},
		},
		{
			name: "Group: any other value will be assumed to be a tag name. The first matching tag's value will be used.",
			evList: []*nostr.Event{
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"t", "val1"},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"t", "val1"},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"r", "val2"},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"r", "val2"},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"d", "val3"},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"a", "val4", "", model.TagMarkerReply},
					},
				},
			},
			groups: []string{"t", "r", "d", "a"},
			expected: map[string]uint64{
				"val1": 2,
				"val2": 2,
				"val3": 1,
				"val4": 1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := countBasedOnGroups(tt.evList, tt.groups...)
			require.Equal(t, tt.expected, actual)
		})
	}
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
			actual := collectSourceRelayURLsFromEvent(tt.event, "")
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

func helperExecuteJob(t *testing.T, ctx context.Context, d *dvm, req *model.Event) (*model.Event, error) {
	t.Helper()

	req.Tags = append(req.Tags, model.Tag{model.CustomIONTagOnBehalfOf, d.PublicKey})

	req.CreatedAt = nostr.Now()
	require.NoError(t, req.SignWithAlg(d.Config.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	ch, err := d.AcceptJob(ctx, req)
	if err != nil {
		return nil, err
	}
	require.NotNil(t, ch)

	return <-ch, nil
}

func helperMustExecuteJob(t *testing.T, ctx context.Context, d *dvm, req *model.Event) *model.Event {
	t.Helper()

	result, err := helperExecuteJob(t, ctx, d, req)
	require.NoError(t, err)
	require.NotNil(t, result)

	return result
}

func helperReadFromDB(t *testing.T, req model.Filter) *model.Event {
	var result *model.Event

	it := query.GetStoredEvents(t.Context(), req)
	for ev, err := range it {
		require.NoError(t, err)
		require.NotNil(t, ev)
		if ev.Kind == model.KindDVMCountResponse {
			result = ev
			break
		}
	}
	require.NotNil(t, result, "No DVM event found in the database")

	return result
}

func helperCompareResults(t *testing.T, dbResult, dvmResult *model.Event) {
	t.Helper()

	require.Equal(t, dbResult.Kind, dvmResult.Kind)
	require.Equal(t, dbResult.PubKey, dvmResult.PubKey)
	require.Equal(t, dbResult.Content, dvmResult.Content)

	dbRequest, dvmRequest := dbResult.GetTag("request").Value(), dvmResult.GetTag("request").Value()
	require.NotEmpty(t, dbRequest)
	require.NotEmpty(t, dvmRequest)

	var dbRequestEvent, dvmRequestEvent model.Event
	require.NoError(t, dbRequestEvent.UnmarshalJSON([]byte(dbRequest)))
	require.NoError(t, dvmRequestEvent.UnmarshalJSON([]byte(dvmRequest)))

	require.Equal(t, dbRequestEvent.Kind, dvmRequestEvent.Kind)
	require.Equal(t, dbRequestEvent.PubKey, dvmRequestEvent.PubKey)
	require.JSONEq(t, dbRequestEvent.Content, dvmRequestEvent.Content)
}

func TestEventCountersConsistency(t *testing.T) {
	t.Parallel()

	d := mustNewDVM(t.Context())

	cases := []struct {
		Name       string
		RequestDVM model.Event
		RequestDB  model.Filter
		Count      string
		Events     func(t *testing.T, d *dvm) []*model.Event
		Before     func(t *testing.T, d *dvm, events []*model.Event, reqDB *model.Filter, reqDVM *model.Event)
	}{
		{
			Name: "For every kind 1 that the subscription finds also include the count of replies that it has",
			RequestDB: model.Filter{
				Tags:   model.TagMap{}.SetLiterals("x", "y"),
				Search: "include:dependencies:kind1>kind6400+kind1+group+reply",
			},
			RequestDVM: model.Event{
				Event: nostr.Event{
					Kind: model.KindJobNostrEventCount,
				},
			},
			Before: func(t *testing.T, d *dvm, events []*model.Event, reqDB *model.Filter, reqDVM *model.Event) {
				reqDB.Authors = []string{d.PublicKey}
				reqDVM.Content = model.Filters{
					{
						Kinds: []int{1},
						Tags:  model.TagMap{}.Set("e", &events[0].ID, nil, model.PointerOf(model.TagMarkerReply)),
					}}.String()
			},
			Events: func(t *testing.T, d *dvm) []*model.Event {
				var ev1 model.Event

				ev1.Kind = nostr.KindTextNote
				ev1.Content = "Hello world!"
				ev1.Tags = append(ev1.Tags, model.Tag{"x", "y"})
				require.NoError(t, ev1.SignWithAlg(d.Config.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

				var reply1 model.Event
				reply1.CreatedAt = 1
				reply1.Kind = nostr.KindTextNote
				reply1.Content = "Hello world! reply 1"
				reply1.Tags = model.Tags{
					{"e", ev1.ID, "", model.TagMarkerRoot},
					{"e", ev1.ID, "", model.TagMarkerReply},
				}
				require.NoError(t, reply1.SignWithAlg(d.Config.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

				var reply2 model.Event
				reply2.CreatedAt = 2
				reply2.Kind = nostr.KindTextNote
				reply2.Tags = reply1.Tags
				reply2.Content = "Hello world! reply 2"
				require.NoError(t, reply2.SignWithAlg(d.Config.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

				return []*model.Event{&ev1, &reply1, &reply2}
			},
			Count: "2",
		},
		{
			Name: "For every kind 1 that the subscription finds also include the count of reposts that it has",
			RequestDB: model.Filter{
				Tags:   model.TagMap{}.SetLiterals("x", "y"),
				Search: "include:dependencies:kind1>kind6400+kind6+group+e",
			},
			RequestDVM: model.Event{
				Event: nostr.Event{
					Kind: model.KindJobNostrEventCount,
				},
			},
			Before: func(t *testing.T, d *dvm, events []*model.Event, reqDB *model.Filter, reqDVM *model.Event) {
				reqDB.Authors = []string{d.PublicKey}
				reqDVM.Content = model.Filters{
					{
						Kinds: []int{6},
						Tags:  model.TagMap{}.Set("e", &events[0].ID),
					}}.String()
			},
			Events: func(t *testing.T, d *dvm) []*model.Event {
				var ev1 model.Event

				ev1.Kind = nostr.KindTextNote
				ev1.Content = "Hello world!!"
				ev1.Tags = append(ev1.Tags, model.Tag{"x", "y"})
				require.NoError(t, ev1.SignWithAlg(d.Config.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

				var repost1 model.Event
				repost1.CreatedAt = 1
				repost1.Kind = nostr.KindRepost
				repost1.Content = ev1.String()
				repost1.Tags = append(repost1.Tags, model.Tag{"e", ev1.ID})
				require.NoError(t, repost1.SignWithAlg(d.Config.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

				var repost2 model.Event
				repost2.CreatedAt = 2
				repost2.Kind = nostr.KindRepost
				repost2.Content = ev1.String()
				repost2.Tags = slices.Clone(repost1.Tags)
				repost2.Tags = append(repost2.Tags, model.Tag{"extra", "tag"})
				require.NoError(t, repost2.SignWithAlg(d.Config.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

				return []*model.Event{&ev1, &repost1, &repost2}
			},
			Count: "2",
		},
		{
			Name: "For every kind 1 that the subscription finds also include the count of quotes that it has",
			RequestDB: model.Filter{
				Tags:   model.TagMap{}.SetLiterals("x", "y"),
				Search: "include:dependencies:kind1>kind6400+kind1+group+q",
			},
			RequestDVM: model.Event{
				Event: nostr.Event{
					Kind: model.KindJobNostrEventCount,
				},
			},
			Before: func(t *testing.T, d *dvm, events []*model.Event, reqDB *model.Filter, reqDVM *model.Event) {
				reqDB.Authors = []string{d.PublicKey}
				reqDVM.Content = model.Filters{
					{
						Kinds: []int{1},
						Tags:  model.TagMap{}.Set("q", &events[0].ID),
					}}.String()
			},
			Events: func(t *testing.T, d *dvm) []*model.Event {
				var ev1 model.Event

				ev1.Kind = nostr.KindTextNote
				ev1.Content = "Hello world!!!"
				ev1.Tags = append(ev1.Tags, model.Tag{"x", "y"})
				require.NoError(t, ev1.SignWithAlg(d.Config.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

				var quote1 model.Event
				quote1.CreatedAt = 1
				quote1.Kind = nostr.KindTextNote
				quote1.Content = "quote 1"
				quote1.Tags = append(quote1.Tags, model.Tag{"q", ev1.ID})
				require.NoError(t, quote1.SignWithAlg(d.Config.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

				var quote2 model.Event
				quote2.CreatedAt = 2
				quote2.Kind = nostr.KindTextNote
				quote2.Content = "quote 2"
				quote2.Tags = quote1.Tags
				require.NoError(t, quote2.SignWithAlg(d.Config.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

				return []*model.Event{&ev1, &quote1, &quote2}
			},
			Count: "2",
		},
		{
			Name: "For every kind 1 that the subscription finds also include the count of reactions that it has",
			RequestDB: model.Filter{
				Tags:   model.TagMap{}.SetLiterals("x", "y"),
				Search: "include:dependencies:kind1>kind6400+kind7+group+content",
			},
			RequestDVM: model.Event{
				Event: nostr.Event{
					Kind: model.KindJobNostrEventCount,
					Tags: model.Tags{
						{"output", "JSON"},
						{"param", "group", "content"},
					},
				},
			},
			Before: func(t *testing.T, d *dvm, events []*model.Event, reqDB *model.Filter, reqDVM *model.Event) {
				reqDB.Authors = []string{d.PublicKey}
				reqDVM.Content = model.Filters{
					{
						Kinds: []int{7},
						Tags:  model.TagMap{}.Set("e", &events[0].ID),
					}}.String()
			},
			Events: func(t *testing.T, d *dvm) []*model.Event {
				var ev1 model.Event

				ev1.Kind = nostr.KindTextNote
				ev1.Content = "Hello world!"
				ev1.Tags = append(ev1.Tags, model.Tag{"x", "y"})
				require.NoError(t, ev1.SignWithAlg(d.Config.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

				var reaction1 model.Event
				reaction1.CreatedAt = 1
				reaction1.Kind = nostr.KindReaction
				reaction1.Content = "-"
				reaction1.Tags = append(reaction1.Tags, model.Tag{"e", ev1.ID})
				require.NoError(t, reaction1.SignWithAlg(d.Config.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

				var reaction2 model.Event
				reaction2.CreatedAt = 2
				reaction2.Content = "+"
				reaction2.Kind = nostr.KindReaction
				reaction2.Tags = reaction1.Tags
				require.NoError(t, reaction2.SignWithAlg(d.Config.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

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
					Kind: model.KindJobNostrEventCount,
				},
			},
			Before: func(t *testing.T, d *dvm, events []*model.Event, reqDB *model.Filter, reqDVM *model.Event) {
				reqDVM.Content = model.Filters{
					{
						Kinds: []int{3},
						Tags:  model.TagMap{}.SetLiterals("p", d.PublicKey),
					}}.String()
			},
			Events: func(t *testing.T, d *dvm) []*model.Event {
				var ev1 model.Event

				ev1.Kind = nostr.KindProfileMetadata
				ev1.Content = `{"name:":"Alice"}`
				ev1.Tags = append(ev1.Tags, model.Tag{"imeta", "url https://foo.barr"})
				require.NoError(t, ev1.SignWithAlg(d.Config.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

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
			events := c.Events(t, d)
			if c.Before != nil {
				c.Before(t, d, events, &c.RequestDB, &c.RequestDVM)
			}
			t.Run("Insert", func(t *testing.T) {
				require.NoError(t, query.AcceptEvents(t.Context(), events...))
			})
			t.Run("Do", func(t *testing.T) {
				resultDB := helperReadFromDB(t, c.RequestDB)
				resultDVM := helperMustExecuteJob(t, t.Context(), d, &c.RequestDVM)
				helperCompareResults(t, resultDB, resultDVM)
				require.JSONEq(t, c.Count, resultDB.Content)
			})
		})
	}
}

func TestCountMostRelevantFollowers(t *testing.T) {
	// NOT PARALLEL.
	t.Cleanup(func() {
		query.DeleteAllEvents(t.Context())
	})

	d := mustNewDVM(t.Context())

	t.Run("Populate", func(t *testing.T) {
		t.Run("Create metadata", func(t *testing.T) {
			var bobMeta, aliceMeta, alexMeta, annaMeta, johnMeta, martinMeta model.Event
			bobMeta.ID = "id1"
			bobMeta.Kind = nostr.KindProfileMetadata
			bobMeta.PubKey = "bob"
			bobMeta.Content = "{\"name\":\"Bob\"}"

			aliceMeta.ID = "id4"
			aliceMeta.Kind = nostr.KindProfileMetadata
			aliceMeta.PubKey = "alice"
			aliceMeta.Content = "{\"name\":\"Alice\"}"

			alexMeta.ID = "id2"
			alexMeta.Kind = nostr.KindProfileMetadata
			alexMeta.PubKey = "alex"
			alexMeta.Content = "{\"name\":\"Alex\"}"

			annaMeta.ID = "id3"
			annaMeta.Kind = nostr.KindProfileMetadata
			annaMeta.PubKey = "anna"
			annaMeta.Content = "{\"name\":\"Anna\"}"

			johnMeta.ID = "id5"
			johnMeta.Kind = nostr.KindProfileMetadata
			johnMeta.PubKey = "john"
			johnMeta.Content = "{\"name\":\"John\"}"

			martinMeta.ID = "id6"
			martinMeta.Kind = nostr.KindProfileMetadata
			martinMeta.PubKey = "martin"
			martinMeta.Content = "{\"name\":\"Martin\"}"

			require.NoError(t, query.AcceptEvents(t.Context(), &bobMeta, &aliceMeta, &alexMeta, &annaMeta, &johnMeta, &martinMeta))
		})
		t.Run("Create follow lists", func(t *testing.T) {
			var johnList, bobList, aliceList, alexList, annaList, martinList model.Event
			bobList.Kind = nostr.KindFollowList
			bobList.PubKey = "bob"
			bobList.ID = "bob_id"
			bobList.Tags = model.Tags{
				{"p", "john"},
				{"p", "alice"},
				{"p", "anna"},
			}

			aliceList.Kind = nostr.KindFollowList
			aliceList.PubKey = "alice"
			aliceList.ID = "alice_id"
			aliceList.Tags = model.Tags{
				{"p", "john"},
				{"p", "bob"},
				{"p", "alex"},
			}

			alexList.Kind = nostr.KindFollowList
			alexList.PubKey = "alex"
			alexList.ID = "alex_id"
			alexList.Tags = model.Tags{
				{"p", "anna"},
				{"p", "john"},
			}

			annaList.Kind = nostr.KindFollowList
			annaList.PubKey = "anna"
			annaList.ID = "anna_id"
			annaList.Tags = model.Tags{
				{"p", "alex"},
				{"p", "alice"},
			}

			martinList.Kind = nostr.KindFollowList
			martinList.PubKey = "martin"
			martinList.ID = "martin_id"
			martinList.Tags = model.Tags{
				{"p", "john"},
				{"p", "bob"},
			}

			johnList.Kind = nostr.KindFollowList
			johnList.PubKey = "john"
			johnList.ID = "john_id"
			johnList.Tags = model.Tags{
				{"p", "alex"},
				{"p", "anna"},
				{"p", "bob"},
				{"p", "alice"},
			}

			require.NoError(t, query.AcceptEvents(t.Context(), &johnList, &bobList, &aliceList, &alexList, &annaList, &martinList))
		})
	})

	t.Run("Find most relevant followers of john with alice", func(t *testing.T) {
		var ev model.Event
		ev.Kind = model.KindJobNostrEventCount
		ev.Content = model.Filters{
			{
				Search: model.ExtensionTextMRF,
				Tags:   model.TagMap{}.SetLiterals("p", "alice"),
			},
		}.String()

		ctx := model.SetUserDataInContext(t.Context(), model.UserDataContext{
			PublicKey:       "john",
			MasterPublicKey: "john",
			Authenticated:   true,
		})
		result := helperMustExecuteJob(t, ctx, d, &ev)
		require.Equal(t, "2", result.Content, "expected 2 followers") // Anna and Bob.
	})
	t.Run("Find most relevant followers of unknown with alice", func(t *testing.T) {
		var ev model.Event
		ev.Kind = model.KindJobNostrEventCount
		ev.Content = model.Filters{
			{
				Search: model.ExtensionTextMRF,
				Tags:   model.TagMap{}.SetLiterals("p", "alice"),
			},
		}.String()

		result := helperMustExecuteJob(t, t.Context(), d, &ev)
		require.Equal(t, nostr.KindJobFeedback, result.Kind)
		require.Equal(t, string(model.JobFeedbackStatusError), result.GetTag("status").Value())
		require.Contains(t, result.Content, model.ErrNotAuthorized.Error())
	})
}

func TestCountUserStories(t *testing.T) {
	t.Parallel()

	const storiesCount = 5
	pk := model.GeneratePrivateKey()

	d := mustNewDVM(t.Context())

	t.Run("Create user stories", func(t *testing.T) {
		for i := range storiesCount {
			var ev model.Event
			ev.Kind = model.CustomIONKindEditableTextNote
			ev.Content = "user story content " + strconv.Itoa(i)
			ev.CreatedAt = nostr.Now()
			ev.Tags = model.Tags{
				{"d", "story_" + strconv.Itoa(i)},
				{"expiration", ev.CreatedAt.Add(time.Hour).String()},
			}
			require.NoError(t, ev.SignWithAlg(pk, model.SignAlgEDDSA, model.KeyAlgCurve25519))

			var note model.Event
			note.Kind = model.CustomIONKindEditableTextNote
			note.Content = "user story note " + strconv.Itoa(i)
			note.CreatedAt = ev.CreatedAt.Add(time.Minute)
			note.Tags = model.Tags{
				{"d", "note_" + strconv.Itoa(i)},
			}
			require.NoError(t, note.SignWithAlg(pk, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.NoError(t, query.AcceptEvents(t.Context(), &note, &ev))
		}
	})
	t.Run("Count user stories", func(t *testing.T) {
		var ev model.Event

		pub, err := model.GetPublicKey(pk)
		require.NoError(t, err)

		ev.Kind = model.KindJobNostrEventCount
		ev.Content = model.Filters{
			{
				Kinds:   []int{model.CustomIONKindEditableTextNote},
				Authors: []string{pub},
				Search:  "expiration:true",
			},
		}.String()

		result, err := helperExecuteJob(t, t.Context(), d, &ev)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equalf(t, strconv.Itoa(storiesCount), result.Content, "expected %d user stories", storiesCount)
	})
}
