// SPDX-License-Identifier: ice License 1.0

package loadtest

import (
	"context"
	"math/rand"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"
	"github.com/panjf2000/ants/v2"
	"github.com/rs/zerolog/log"
)

// NewLoadTester creates a new LoadTester instance
func NewLoadTester(config *Config) *LoadTester {
	return &LoadTester{
		config:  config,
		clients: make([]*NostrClient, 0, config.Connections),
	}
}

// Start initializes and starts all client connections
func (lt *LoadTester) Start(ctx context.Context) error {
	log.Info().Str("context", "LOADTEST").
		Int("connections", lt.config.Connections).
		Str("relay_url", lt.config.RelayURL).
		Msg("starting load test")

	errChan := make(chan error, lt.config.Connections)
	pool, err := ants.NewPool(1024)
	if err != nil {
		return errors.Wrap(err, "can not create goroutine pool")
	}
	defer pool.Release()
	setupWg := sync.WaitGroup{}

	for i := 0; i < lt.config.Connections; i++ {
		id := i
		setupWg.Add(1)
		err = pool.Submit(func() {
			defer setupWg.Done()

			randomDelay := rand.Int63n(lt.config.setupDuration.Milliseconds())
			time.Sleep(time.Duration(randomDelay) * time.Millisecond)
			log.Info().Str("context", "LOADTEST").Int("client_id", id).Msg("client starting setup")

			client, err := lt.connect(ctx, id)
			if err != nil {
				errChan <- err
				return
			}

			lt.mu.Lock()
			lt.clients = append(lt.clients, client)
			lt.mu.Unlock()
			log.Info().Str("context", "LOADTEST").
				Int("current_clients", len(lt.clients)).
				Int("total_connections", lt.config.Connections).
				Int("client_id", id).
				Msg("client setup completed successfully")
		})
		if err != nil {
			return errors.Wrap(err, "error submitting setup goroutine")
		}
	}

	// Wait for setup completion
	setupDone := make(chan struct{})
	go func() {
		setupWg.Wait()
		close(setupDone)
		close(errChan)
	}()

	select {
	case <-setupDone:
		for err := range errChan {
			if err != nil {
				log.Error().Str("context", "LOADTEST").Err(err).Msg("connection error")
			}
		}
		log.Info().Msg("setup phase completed")
	case <-time.After(maxConnectTime):
		lt.mu.RLock()
		currentCount := len(lt.clients)
		lt.mu.RUnlock()
		return errors.Newf("timeout waiting for connections - got %d/%d", currentCount, lt.config.Connections)
	case <-ctx.Done():
		return errors.New("cancelled during setup")
	}

	lt.mu.RLock()
	connectedCount := len(lt.clients)
	lt.mu.RUnlock()

	if connectedCount == 0 {
		return errors.New("no clients connected successfully")
	}

	log.Info().Str("context", "LOADTEST").
		Int("connected_count", connectedCount).
		Int("total_connections", lt.config.Connections).
		Msg("successfully established connections")

	return nil
}

func (lt *LoadTester) connect(ctx context.Context, id int) (*NostrClient, error) {
	// Create client with unique or shared key
	client, err := NewNostrClient(id, lt.config)
	if err != nil {
		return nil, errors.Wrapf(err, "client creation error")
	}

	// Connect and subscribe
	if err := client.Connect(ctx); err != nil {
		return nil, errors.Wrapf(err, "client connection error")
	}

	kinds := []int{nostr.KindTextNote}
	offset := time.Duration(0)
	if lt.config.Mode == ModeNoDB {
		kinds = []int{nostr.KindBidConfirmation}
		offset = 100 * time.Hour
	}
	if err := client.Subscribe(ctx, offset, kinds...); err != nil {
		client.Close()
		return nil, errors.Wrapf(err, "client subscription error")
	}
	return client, nil
}

// PublishTestEvents publishes test events from all connected clients
func (lt *LoadTester) PublishTestEvents(ctx context.Context) {
	successCount := atomic.Int32{}
	wg := sync.WaitGroup{}
	kind := nostr.KindTextNote
	if lt.config.Mode == ModeNoDB {
		kind = 20000 // ephemeral, should not be stored in DB
	}
	for i, c := range lt.clients {
		wg.Go(func() {
			randomDelay := rand.Int63n(lt.config.sendDuration.Milliseconds())
			time.Sleep(time.Duration(randomDelay) * time.Millisecond)
			content := "Test message from client " + strconv.Itoa(i) + " at " + time.Now().Format(time.RFC3339)
			if err := c.PublishEvent(ctx, kind, content); err != nil {
				log.Error().
					Str("context", "LOADTEST").
					Err(err).
					Int("client_id", i).
					Msg("failed to publish from client")
			} else {
				successCount.Add(1)
			}
		})
	}
	wg.Wait()

	log.Info().Str("context", "LOADTEST").
		Int("successful_clients", int(successCount.Load())).
		Int("total_clients", len(lt.clients)).
		Msg("published test events")
}

// GetStats returns aggregated statistics for all clients
func (lt *LoadTester) GetStats() *LoadTestStats {
	lt.mu.RLock()
	defer lt.mu.RUnlock()

	stats := &LoadTestStats{
		ActiveClients: len(lt.clients),
		TotalClients:  lt.config.Connections,
		ClientStats:   make([]ClientStats, 0, len(lt.clients)),
	}

	for i, c := range lt.clients {
		clientStats := c.GetStats()
		isConnected := c.IsConnected()
		if isConnected {
			stats.ConnectedClients++
		}
		stats.TotalEvents += clientStats.EventsReceived

		stats.ClientStats = append(stats.ClientStats, ClientStats{
			ID:             i,
			Connected:      isConnected,
			EventsReceived: clientStats.EventsReceived,
			LastEventTime:  clientStats.LastEventTime,
		})
	}

	return stats
}

// PrintStats prints current statistics for all clients
func (lt *LoadTester) PrintStats() {
	stats := lt.GetStats()
	stats.Print()
}

// PrintLastStats prints last statistics for all clients before shutdown
func (lt *LoadTester) PrintLastStats() {
	if lt.lastStats != nil {
		lt.lastStats.Print()
	}
}

// Shutdown gracefully closes all client connections
func (lt *LoadTester) Shutdown() {
	lt.lastStats = lt.GetStats()
	log.Info().Msg("shutting down load tester")
	for _, client := range lt.clients {
		log.Info().
			Str("context", "LOADTEST").
			Int("client_id", client.id).
			Msg("client shutting down")
		client.Close()
	}
	log.Info().Msg("load tester shutdown complete")
}
