// SPDX-License-Identifier: ice License 1.0

package model

import "testing"

func TestCompareRelaysURLs(t *testing.T) {
	t.Parallel()

	var cases = []struct {
		relay1   string
		relay2   string
		expected bool
	}{
		{"wss://relay.example.com", "wss://relay.example.com", true},
		{"wss://RELAY.EXAMPLE.COM", "wss://relay.example.com", true},
		{"wss://relay.example.com:443", "wss://relay.example.com", true},
		{"wss://relay.example.com:443", "wss://relay.example.com:8080", true},
		{"wss://relay.example.com/path", "wss://relay.example.com/path", true},
		{"wss://relay.example.com/path", "wss://RELAY.EXAMPLE.COM/path", true},
		{"wss://relay.example.com/path", "wss://relay.example.com/otherpath", false},
		{"wss://relay1.example.com", "wss://relay2.example.com", false},
		{"ws://relay.example.com", "wss://relay.example.com", false},
	}

	for _, c := range cases {
		result := CompareRelaysURLs(c.relay1, c.relay2)
		if result != c.expected {
			t.Errorf("CompareRelaysURLs(%q, %q) = %v; want %v", c.relay1, c.relay2, result, c.expected)
		}
	}
}
