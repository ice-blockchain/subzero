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
		RelayURL      string `yaml:"relayURL" mapstructure:"relayURL"`
		Connections   int    `yaml:"connections" mapstructure:"connections"`
		PrivateKey    string `yaml:"privateKey" mapstructure:"privateKey"`
		Mode          string `yaml:"mode" mapstructure:"mode"`
		SetupDuration string `yaml:"setupDuration" mapstructure:"setupDuration"`
		SendDuration  string `yaml:"sendDuration" mapstructure:"sendDuration"`
		setupDuration time.Duration
		sendDuration  time.Duration
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

func (c *Config) Defaults() {
	if c.Mode == "" {
		c.Mode = ModeFull
	}
	if c.SetupDuration == "" {
		c.SetupDuration = (time.Second * time.Duration(c.Connections)).String()
	}
	if c.SendDuration == "" {
		c.SendDuration = c.SetupDuration
	}
}

func (c *Config) Validate() error {
	switch c.Mode {
	case ModeFull:
	case ModeNoDB:
	default:
		return fmt.Errorf("invalid mode: %s", c.Mode)
	}

	setupDuration, err := time.ParseDuration(c.SetupDuration)
	if err != nil {
		return fmt.Errorf("bad setup duration: %w", err)
	}
	c.setupDuration = setupDuration

	sendDuration, err := time.ParseDuration(c.SendDuration)
	if err != nil {
		return fmt.Errorf("bad send duration: %w", err)
	}
	c.sendDuration = sendDuration

	return nil
}

func (c *Config) GetSetupDuration() time.Duration {
	return c.setupDuration
}

func (c *Config) GetSendDuration() time.Duration {
	return c.sendDuration
}

const (
	ModeNoDB = "no-db"
	ModeFull = "full"
)
