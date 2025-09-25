// SPDX-License-Identifier: ice License 1.0

package loadtest

import (
	"context"
	"log"
	"math/rand"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"
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
	log.Printf("Starting load test with %d connections to %s", lt.config.Connections, lt.config.RelayURL)

	errChan := make(chan error, lt.config.Connections)
	setupWg := sync.WaitGroup{}

	for i := 0; i < lt.config.Connections; i++ {
		id := i
		setupWg.Go(func() {
			randomDelay := rand.Int63n(lt.config.setupDuration.Milliseconds())
			time.Sleep(time.Duration(randomDelay) * time.Millisecond)
			log.Printf("Client %d: Starting setup...", id)

			client, err := lt.connect(ctx, id)
			if err != nil {
				errChan <- err
				return
			}

			// Start client runtime goroutine
			lt.wg.Go(func() {
				defer client.Close()
				<-ctx.Done()
				log.Printf("Client %d: Shutting down", id)
			})

			lt.mu.Lock()
			lt.clients = append(lt.clients, client)
			lt.mu.Unlock()
			log.Printf("[%d/%d] Client %d: Setup completed successfully", len(lt.clients), lt.config.Connections, id)
		})
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
				log.Printf("Connection error: %v", err)
			}
		}
		log.Printf("Setup phase completed")
	case <-time.After(2 * lt.config.setupDuration):
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

	log.Printf("Successfully established %d/%d connections", connectedCount, lt.config.Connections)

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
				log.Printf("Failed to publish from client %d: %v", i, err)
			} else {
				successCount.Add(1)
			}
		})
	}
	wg.Wait()

	log.Printf("Published test events from %d/%d clients", successCount.Load(), len(lt.clients))
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
	log.Println("Shutting down load tester...")
	lt.wg.Wait()
	log.Println("Load tester shutdown complete")
}
