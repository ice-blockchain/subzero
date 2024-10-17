// SPDX-License-Identifier: ice License 1.0

package dvm

import (
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

func TestDeduplicateEvents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		events   []*nostr.Event
		expected []*nostr.Event
	}{
		{
			name:     "empty slice",
			events:   []*nostr.Event{},
			expected: []*nostr.Event{},
		},
		{
			name:     "single event",
			events:   []*nostr.Event{{ID: "1", PubKey: "1"}},
			expected: []*nostr.Event{{ID: "1", PubKey: "1"}},
		},
		{
			name:     "events with 5 same",
			events:   []*nostr.Event{{ID: "1", PubKey: "1"}, {ID: "1", PubKey: "1"}, {ID: "1", PubKey: "1"}, {ID: "1", PubKey: "1"}, {ID: "1", PubKey: "1"}},
			expected: []*nostr.Event{{ID: "1", PubKey: "1"}},
		},
		{
			name:     "events with 2 same and others",
			events:   []*nostr.Event{{ID: "1", PubKey: "1"}, {ID: "1", PubKey: "1"}, {ID: "2", PubKey: "2"}},
			expected: []*nostr.Event{{ID: "1", PubKey: "1"}, {ID: "2", PubKey: "2"}},
		},
		{
			name:     "nil events",
			events:   nil,
			expected: []*nostr.Event{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := deduplicateEvents(tt.events)
			require.EqualValues(t, tt.expected, actual)
		})
	}
}
