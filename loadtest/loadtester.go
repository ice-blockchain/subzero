// SPDX-License-Identifier: ice License 1.0

package loadtest

import (
	"context"
	"fmt"
	"github.com/nbd-wtf/go-nostr"
	"log"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/errors"
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

			log.Printf("Client %d: Setup completed successfully", id)
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
	case <-time.After(30 * time.Second):
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
		return nil, fmt.Errorf("client creation error: %w", err)
	}

	// Connect and subscribe
	if err := client.Connect(ctx); err != nil {
		return nil, fmt.Errorf("client connection error: %w", err)
	}

	kinds := []int{nostr.KindTextNote, nostr.KindReaction, nostr.KindChannelMessage}
	offset := time.Duration(0)
	if lt.config.Mode == ModeNoDB {
		kinds = []int{nostr.KindBidConfirmation}
		offset = 100 * time.Hour
	}
	if err := client.Subscribe(ctx, offset, kinds...); err != nil {
		client.Close()
		return nil, fmt.Errorf("client subscription error: %w", err)
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

// PrintStats prints current statistics for all clients
func (lt *LoadTester) PrintStats() {
	lt.mu.RLock()
	defer lt.mu.RUnlock()

	log.Println("\n=== Load Test Statistics ===")
	log.Printf("Active clients: %d/%d", len(lt.clients), lt.config.Connections)

	totalEvents := int64(0)
	connectedCount := 0

	for i, c := range lt.clients {
		stats := c.GetStats()
		if c.IsConnected() {
			connectedCount++
		}
		totalEvents += stats.EventsReceived

		lastEventStr := "Never"
		if !stats.LastEventTime.IsZero() {
			lastEventStr = stats.LastEventTime.Format("15:04:05")
		}

		log.Printf("  Client %d: Connected=%v, Events=%d, LastEvent=%s",
			i, c.IsConnected(), stats.EventsReceived, lastEventStr)
	}

	log.Printf("\nTotal events received: %d", totalEvents)
	log.Printf("Connected clients: %d/%d", connectedCount, len(lt.clients))
	log.Println("============================\n")
}

// Shutdown gracefully closes all client connections
func (lt *LoadTester) Shutdown() {
	log.Println("Shutting down load tester...")
	lt.wg.Wait()
	log.Println("Load tester shutdown complete")
}
