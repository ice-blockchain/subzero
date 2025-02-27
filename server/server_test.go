// SPDX-License-Identifier: ice License 1.0

package server

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractServerNameFromRelayURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		relayURL string
		want     string
	}{
		{"http URL", "http://example.com", "example.com"},
		{"https URL", "https://example.com", "example.com"},
		{"URL with port", "http://example.com:8080", "example.com"},
		{"URL with path", "http://example.com/path/to/resource", "example.com"},
		{"URL with query", "http://example.com?param=value", "example.com"},
		{"URL with subdomain", "https://api.example.com", "api.example.com"},
		{"URL with username and password", "http://user:pass@example.com", "example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractServerNameFromRelayURL(tt.relayURL)
			require.Equal(t, tt.want, got)
		})
	}
}
