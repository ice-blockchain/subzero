// SPDX-License-Identifier: ice License 1.0

package model

import (
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

func TestFilterMatches(t *testing.T) {
	t.Parallel()

	var cases = []struct {
		Filter Filter
		Event  Event
		Match  bool
	}{
		{
			Filter: Filter{
				Tags: TagMap{
					"e": nil,
				},
			},
			Event: Event{
				Event: nostr.Event{
					Tags: Tags{{"e", "1"}},
				},
			},
			Match: true,
		},
		{
			Filter: Filter{
				Tags: TagMap{
					"e": []TagValues{{nil, nil}, {PointerOf("1")}},
				},
			},
			Event: Event{
				Event: nostr.Event{
					Tags: Tags{{"e", "1"}},
				},
			},
			Match: true,
		},
		{
			Filter: Filter{
				Tags: TagMap{
					"e": []TagValues{{nil, PointerOf("2"), nil}, {PointerOf("1")}},
				},
			},
			Event: Event{
				Event: nostr.Event{
					Tags: Tags{{"e", "1", "2", "3"}},
				},
			},
			Match: true,
		},
		{
			Filter: Filter{
				Tags: TagMap{
					"e": []TagValues{{nil, PointerOf("2"), nil}, {PointerOf("1")}},
				},
			},
			Event: Event{
				Event: nostr.Event{
					Tags: Tags{{"e", "0", "2", "3"}},
				},
			},
		},
		{
			Filter: Filter{
				Tags: TagMap{
					"x": nil,
				},
			},
			Event: Event{},
		},
	}

	for _, c := range cases {
		r := c.Filter.Matches(&c.Event)
		require.Equalf(t, c.Match, r, "filter: %+v, event: %+v", c.Filter, c.Event)
	}
}

func TestFilterToNostr(t *testing.T) {
	t.Parallel()

	var cases = []struct {
		In  Filter
		Out []nostr.Filter
	}{
		{
			In: Filter{
				Tags: TagMap{}.SetLiterals("e", "1", "2", "3"),
			},
			Out: []nostr.Filter{
				{
					Tags: nostr.TagMap{
						"e": {"1", "2", "3"},
					},
				},
			},
		},
		{
			In: Filter{
				IDs: []string{"id"},
				Tags: TagMap{}.SetLiterals("e", "1", "2", "3").
					Append("e", PointerOf("4")),
			},
			Out: []nostr.Filter{
				{
					IDs: []string{"id"},
					Tags: nostr.TagMap{
						"e": {"1", "2", "3"},
					},
				},
				{
					IDs: []string{"id"},
					Tags: nostr.TagMap{
						"e": {"4"},
					},
				},
			},
		},
	}

	for _, c := range cases {
		r := c.In.ToNostr()
		require.Equalf(t, c.Out, r, "filter: %+v", c.In)
	}
}
