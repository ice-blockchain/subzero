// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompactRelays(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		relays   []string
		expected []string
	}{
		{
			name:     "empty",
			relays:   []string{},
			expected: nil,
		},
		{
			name:     "no duplicates",
			relays:   []string{"wss://relay1.com", "wss://relay2.com"},
			expected: []string{"wss://relay1.com", "wss://relay2.com"},
		},
		{
			name:     "exact duplicates",
			relays:   []string{"wss://relay1.com", "wss://relay1.com"},
			expected: []string{"wss://relay1.com"},
		},
		{
			name:     "duplicates with different ports",
			relays:   []string{"wss://relay1.com:4443", "wss://relay1.com", "wss://relay1.com:888"},
			expected: []string{"wss://relay1.com:4443"},
		},
		{
			name:     "duplicates with different cases",
			relays:   []string{"wss://RELAY1.com", "wss://relay1.com"},
			expected: []string{"wss://RELAY1.com"},
		},
		{
			name:     "mixed duplicates",
			relays:   []string{"wss://relay1.com:4443", "wss://relay2.com", "wss://relay1.com", "wss://RELAY2.com:888"},
			expected: []string{"wss://relay1.com:4443", "wss://relay2.com"},
		},
		{
			name:     "invalid urls",
			relays:   []string{"not-a-url", "not-a-url", "NOT-A-URL"},
			expected: []string{"not-a-url"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := compactRelays(tt.relays)
			require.Equal(t, tt.expected, actual)
		})
	}
}
