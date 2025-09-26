// SPDX-License-Identifier: ice License 1.0

package loadtest

import (
	"fmt"
	"sync"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/rs/zerolog/log"
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
		config    *Config
		clients   []*NostrClient
		mu        sync.RWMutex
		lastStats *LoadTestStats
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

	// ClientStats represents statistics for a single client
	ClientStats struct {
		ID             int
		Connected      bool
		EventsReceived int64
		LastEventTime  time.Time
	}

	// LoadTestStats represents aggregated statistics for the entire load test
	LoadTestStats struct {
		ActiveClients    int
		TotalClients     int
		ConnectedClients int
		TotalEvents      int64
		ClientStats      []ClientStats
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

// Print outputs the load test statistics in a formatted way
func (s *LoadTestStats) Print() {
	log.Info().
		Int("connected-clients", s.ConnectedClients).
		Int("active-clients", s.ActiveClients).
		Int("total-clients", s.TotalClients).
		Int("events-received", s.TotalClients)

	for _, client := range s.ClientStats {
		lastEventStr := "Never"
		if !client.LastEventTime.IsZero() {
			lastEventStr = client.LastEventTime.Format("15:04:05")
		}

		log.Info().
			Int("client_id", client.ID).
			Bool("connected", client.Connected).
			Int64("events_received", client.EventsReceived).
			Str("last_event", lastEventStr).
			Msg("client status")
	}
}
