// SPDX-License-Identifier: ice License 1.0

package query

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/cfg"
)

func TestValidateConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  *Config
		err  bool
	}{
		{
			name: "valid config",
			cfg: &Config{
				URL:         "http://localhost:5432",
				PrivateKey:  "private-key",
				RelayURL:    "http://localhost:8080",
				ReplicaURLs: []string{"http://localhost:5433"},
			},
		},
		{
			name: "missing relay URL",
			cfg: &Config{
				URL:         "http://localhost:5432",
				PrivateKey:  "private-key",
				ReplicaURLs: []string{"http://localhost:5433"},
			},
			err: true,
		},
		{
			name: "no replica URLs",
			cfg: &Config{
				URL:        "http://localhost:5432",
				PrivateKey: "private-key",
				RelayURL:   "http://localhost:8080",
			},
		},
		{
			name: "invalid replica URL",
			cfg: &Config{
				URL:         "http://localhost:5432",
				PrivateKey:  "private-key",
				RelayURL:    "http://localhost:8080",
				ReplicaURLs: []string{"invalid-url"},
			},
			err: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := cfg.Validate(tt.cfg)
			if tt.err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
