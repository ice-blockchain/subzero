// SPDX-License-Identifier: ice License 1.0

package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/ice-blockchain/subzero/cfg"
	"github.com/ice-blockchain/subzero/loadtest"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Parse command line flags
	sendDuration := flag.String("send-duration", "", "Loadtest event (text or ephemeral) sending duration (like 5m or 5s), events will be randomly spread over given duration, if not provided, will be defaulted to number of connection seconds")
	setupDuration := flag.String("setup-duration", "", "Loadtest setup duration (like 5m or 5s), connections will be randomly spread over given duration, if not provided, will be defaulted to number of connection seconds")
	mode := flag.String("mode", "", "Loadtest mode: full, no-db")
	relayURL := flag.String("relay", "", "Nostr relay URL (e.g., wss://relay.example.com)")
	connections := flag.Int("connections", 1, "Number of connections to open")
	privateKey := flag.String("key", "", "Private key (hex format, optional - will generate unique keys if not provided)")
	configFile := flag.String("config", "", "Configuration file path")
	flag.Parse()

	// Initialize configuration system
	if *configFile != "" {
		cfg.MustInit(*configFile)
	} else {
		cfg.MustInit()
	}

	// Load configuration
	config, err := cfg.Get[loadtest.Config]()
	if err != nil {
		log.Printf("Failed to load config from file, using defaults: %v", err)
		config = &loadtest.Config{} // Use empty config
	}

	// Override with command line flags
	if *relayURL != "" {
		config.RelayURL = *relayURL
	}
	if *connections > 0 {
		config.Connections = *connections
	}
	if *privateKey != "" {
		config.PrivateKey = *privateKey
	}
	if *mode != "" {
		config.Mode = *mode
	}
	if *setupDuration != "" {
		config.SetupDuration = *setupDuration
	}
	if *sendDuration != "" {
		config.SendDuration = *sendDuration
	}

	// Override with environment variables
	if envRelay := os.Getenv("NOSTR_RELAY"); envRelay != "" {
		config.RelayURL = envRelay
	}
	if envConnections := os.Getenv("NOSTR_CONNECTIONS"); envConnections != "" {
		if c, err := strconv.Atoi(envConnections); err == nil && c > 0 {
			config.Connections = c
		}
	}
	if envKey := os.Getenv("NOSTR_PRIVATE_KEY"); envKey != "" {
		config.PrivateKey = envKey
	}
	if envMode := os.Getenv("LT_MODE"); envMode != "" {
		config.Mode = envMode
	}
	if envSetupDuration := os.Getenv("LT_SETUP_DURATION"); envSetupDuration != "" {
		config.SetupDuration = envSetupDuration
	}
	if envSendDuration := os.Getenv("LT_SEND_DURATION"); envSendDuration != "" {
		config.SendDuration = envSendDuration
	}

	// Validate configuration
	if config.RelayURL == "" {
		log.Fatal("Relay URL must be provided via -relay flag, NOSTR_RELAY environment variable, or config file")
	}
	if config.Connections <= 0 {
		config.Connections = 1
	}

	config.Defaults()

	err = config.Validate()
	if err != nil {
		log.Fatalf("config error: %s", err.Error())
	}

	log.Printf("Configuration loaded - Relay: %s, Connections: %d", config.RelayURL, config.Connections)

	// Create and start load tester
	tester := loadtest.NewLoadTester(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start load test
	if err := tester.Start(ctx); err != nil {
		log.Fatal("Failed to start load test:", err)
	}
	defer tester.Shutdown()

	// Setup periodic stats and publishing
	statsTicker := time.NewTicker(30 * time.Second)
	defer statsTicker.Stop()

	publishTicker := time.NewTicker(config.GetSendDuration() + 10*time.Second)
	defer publishTicker.Stop()

	tester.PublishTestEvents(ctx)
	log.Println("Connections are open. Press Ctrl+C to close connections and shutdown...")

	for {
		select {
		case <-sigChan:
			log.Println("Received shutdown signal...")
			tester.PrintStats()
			cancel()
			return
		case <-statsTicker.C:
			tester.PrintStats()
		case <-publishTicker.C:
			go tester.PublishTestEvents(ctx)
		}
	}
}
