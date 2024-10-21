// SPDX-License-Identifier: ice License 1.0

package dvm

import (
	"fmt"
	"strings"
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func TestParseListOfFilters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		content     string
		expectedErr error
	}{
		{
			name:        "empty",
			content:     `[]`,
			expectedErr: nil,
		},
		{
			name:        "one filter",
			content:     `[{"#e":["7b0d90f1973da1cea186c85fbd09b3e4e455ce4d438b60a3d1f9aabc1681418f"],"#kinds":["7"]}]`,
			expectedErr: nil,
		},
		{
			name:        "two filters",
			content:     `[{"#e":["7b0d90f1973da1cea186c85fbd09b3e4e455ce4d438b60a3d1f9aabc1681418f"],"#kinds":["7"]},{"#e":["7b0d90f1973da1cea186c85fbd09b3e4e455ce4d438b60a3d1f9aabc1681418f"],"#kinds":["10"]}]`,
			expectedErr: nil,
		},
		{
			name:        "another tag filter",
			content:     `[{"#title":["dummy"],"#kinds":["30021"]}]`,
			expectedErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseListOfFilters(tt.content)
			require.Equal(t, err, tt.expectedErr)

			if tt.expectedErr == nil {
				var filters []string
				for _, f := range result {
					filters = append(filters, f.String())
				}
				actual := fmt.Sprintf("[%v]", strings.Join(filters, ","))
				require.Equal(t, actual, tt.content)
			}
		})
	}
}

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
			groups:   []string{NostrEventCountGroupReply},
			expected: map[string]uint64{},
		},
		{
			name: "tag marker reply",
			evList: []*nostr.Event{
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde1", model.TagMarkerReply},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde1", model.TagMarkerReply},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde2", model.TagMarkerReply},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde2", model.TagMarkerReply},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde2", model.TagMarkerReply},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde3", model.TagMarkerReply},
					},
				},
			},
			groups: []string{NostrEventCountGroupReply},
			expected: map[string]uint64{
				"1234567890abcde1": 2,
				"1234567890abcde2": 3,
				"1234567890abcde3": 1,
			},
		},
		{
			name:     "tag marker reply empty",
			evList:   []*nostr.Event{},
			groups:   []string{NostrEventCountGroupRoot},
			expected: map[string]uint64{},
		},
		{
			name: "tag marker root",
			evList: []*nostr.Event{
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde1", model.TagMarkerRoot},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde2", model.TagMarkerRoot},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde2", model.TagMarkerRoot},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde2", model.TagMarkerRoot},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde2", model.TagMarkerRoot},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde3", model.TagMarkerRoot},
					},
				},
			},
			groups: []string{NostrEventCountGroupRoot},
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
						{"e", "1234567890abcde1", model.TagMarkerRoot},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde2", model.TagMarkerRoot},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde2", model.TagMarkerRoot},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde2", model.TagMarkerRoot},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde2", model.TagMarkerRoot},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde3", model.TagMarkerRoot},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde1", model.TagMarkerReply},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde1", model.TagMarkerReply},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde2", model.TagMarkerReply},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde2", model.TagMarkerReply},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde2", model.TagMarkerReply},
					},
				},
				{
					Content: "Hello world!",
					Tags: nostr.Tags{
						{"e", "1234567890abcde3", model.TagMarkerReply},
					},
				},
			},
			groups: []string{NostrEventCountGroupPubkey, NostrEventCountGroupRoot, NostrEventCountGroupContent, NostrEventCountGroupReply},
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
						{"a", "val4", model.TagMarkerReply},
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
			actual := countBasedOnGroups(tt.evList, tt.groups)
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
			actual := collectRelayURLs(tt.event)
			require.Equal(t, tt.expected, actual)
		})
	}
}

func TestIsBidAmountEnough(t *testing.T) {
	t.Parallel()

	t.Run("required amount is 0", func(t *testing.T) {
		n := newNostrEventCountJob(nil, "", true)
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
