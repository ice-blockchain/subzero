// SPDX-License-Identifier: ice License 1.0

package loadtest

import (
	"fmt"
	"sync"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

type (
	// Config holds the configuration for the load tester
	Config struct {
		RelayURL    string `yaml:"relayURL" mapstructure:"relayURL"`
		Connections int    `yaml:"connections" mapstructure:"connections"`
		PrivateKey  string `yaml:"privateKey" mapstructure:"privateKey"`
		Mode        string `yaml:"mode" ma;structure:"mode"`
	}

	// LoadTester orchestrates multiple Nostr clients for load testing
	LoadTester struct {
		config  *Config
		clients []*NostrClient
		mu      sync.RWMutex
		wg      sync.WaitGroup
	}

	// NostrClient represents a single Nostr client connection
	NostrClient struct {
		id         int
		config     *Config
		relay      *nostr.Relay
		sub        *nostr.Subscription
		events     chan *nostr.Event
		privateKey string
		publicKey  string
		stats      *Stats
		mu         sync.RWMutex
	}

	// Stats tracks statistics for a single client
	Stats struct {
		EventsReceived int64
		Connected      bool
		LastEventTime  time.Time
		mu             sync.RWMutex
	}
)

func (c *Config) Validate() error {
	switch c.Mode {
	case ModeFull:
	case ModeNoDB:
		break
	default:
		return fmt.Errorf("invalid mode: %s", c.Mode)
	}
	return nil
}

const (
	ModeNoDB = "no-db"
	ModeFull = "full"
)
