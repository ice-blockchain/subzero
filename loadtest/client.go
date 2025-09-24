// SPDX-License-Identifier: ice License 1.0

package loadtest

import (
	"context"
	"github.com/ice-blockchain/subzero/model"
	"log"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"
)

// NewNostrClient creates a new Nostr client for load testing
func NewNostrClient(id int, config *Config) (*NostrClient, error) {
	client := &NostrClient{
		id:     id,
		config: config,
		stats:  &Stats{},
	}

	if config.PrivateKey != "" {
		// Use shared key
		client.privateKey = config.PrivateKey
		var err error
		client.publicKey, err = nostr.GetPublicKey(client.privateKey)
		if err != nil {
			return nil, errors.Wrap(err, "failed to derive public key from provided private key")
		}
	} else {
		// Generate unique key pair using the project's model package
		client.privateKey, client.publicKey = model.GenerateKeyPair()
		log.Printf("Client %d: Generated new key pair (pubkey: %s)", id, client.publicKey)
	}

	return client, nil
}

// Connect establishes a connection to the Nostr relay with authentication
func (nc *NostrClient) Connect(ctx context.Context) error {
	log.Printf("Client %d: Connecting to %s...", nc.id, nc.config.RelayURL)

	relay, err := nostr.RelayConnect(ctx, nc.config.RelayURL, nostr.WithSignatureChecker(func(e *nostr.Event) bool {
		ev := model.Event{Event: *e}
		ok, err := ev.CheckSignature()
		return ok && err == nil
	}))
	if err != nil {
		return errors.Wrap(err, "relay connection failed")
	}

	// Try to publish an initial auth event to trigger auth if needed
	authEvent := &model.Event{
		Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindClientAuthentication,
			Tags: nostr.Tags{
				{"relay", nc.config.RelayURL},
				{"challenge", "init-auth"},
			},
			Content: "Authentication",
		},
	}

	if err := authEvent.SignWithAlg(nc.privateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519); err != nil {
		_ = relay.Close()
		return errors.Wrap(err, "failed to sign init auth event")
	}

	err = relay.Publish(ctx, authEvent.Event)
	if nc.authRequired(err) {
		err = nc.doAuth(ctx, relay)
		if err != nil {
			return err
		}
		log.Printf("Client %d: Successfully authenticated to relay", nc.id)
	}
	if err != nil {
		return errors.Wrap(err, "failed to authenticate")
	}

	nc.relay = relay
	nc.stats.Connected = true
	log.Printf("Client %d: Successfully connected", nc.id)
	return nil
}

func (nc *NostrClient) doAuth(ctx context.Context, relay *nostr.Relay) error {
	// Relay requires authentication
	err := relay.Auth(ctx, func(event *nostr.Event) error {
		subZeroEvent := model.Event{Event: *event}
		// Use standard Nostr signing
		if err := subZeroEvent.SignWithAlg(nc.privateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519); err != nil {
			return err
		}
		*event = subZeroEvent.Event
		return nil
	})

	if err != nil {
		_ = relay.Close()
		return errors.Wrap(err, "failed to authenticate to relay")
	}
	return nil
}

func (nc *NostrClient) authRequired(err error) bool {
	return err != nil && strings.Contains(err.Error(), "auth-required:")
}

// Subscribe creates a subscription to receive events from the relay
func (nc *NostrClient) Subscribe(ctx context.Context) error {
	nc.mu.Lock()
	defer nc.mu.Unlock()

	if nc.relay == nil {
		return errors.New("not connected")
	}

	since := nostr.Now() - 3600 // 1 hour ago
	filters := []nostr.Filter{{
		Kinds: []int{nostr.KindTextNote, nostr.KindReaction, nostr.KindChannelMessage},
		Limit: 100,
		Since: &since,
	}}

	sub, err := nc.relay.Subscribe(ctx, filters)
	if err != nil {
		return errors.Wrap(err, "failed to subscribe")
	}

	nc.sub = sub
	log.Printf("Client %d: Subscribed with ID: %s", nc.id, sub.GetID())

	// Start event handler
	go nc.handleEvents(ctx)

	return nil
}

// handleEvents processes incoming events from the subscription
func (nc *NostrClient) handleEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-nc.sub.Events:
			if event == nil {
				continue
			}

			nc.stats.mu.Lock()
			nc.stats.EventsReceived++
			nc.stats.LastEventTime = time.Now()
			nc.stats.mu.Unlock()

			contentPreview := event.Content
			if len(contentPreview) > 100 {
				contentPreview = contentPreview[:100] + "..."
			}

			log.Printf("Client %d: Event received - Kind: %d, Author: %s, Content: %s",
				nc.id, event.Kind, event.PubKey[:8], contentPreview)
		}
	}
}

// PublishEvent publishes a test event to the relay
func (nc *NostrClient) PublishEvent(ctx context.Context, content string) error {
	nc.mu.RLock()
	defer nc.mu.RUnlock()

	if nc.relay == nil {
		return errors.New("not connected")
	}

	event := model.Event{
		Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags:      nil,
			Content:   content,
		},
	}
	if err := event.SignWithAlg(nc.privateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519); err != nil {
		return errors.Wrap(err, "failed to sign event")
	}

	if err := nc.relay.Publish(ctx, event.Event); err != nil {
		return errors.Wrap(err, "failed to publish")
	}

	log.Printf("Client %d: Published event %s", nc.id, event.ID)
	return nil
}

// GetStats returns the current statistics for the client
func (nc *NostrClient) GetStats() Stats {
	nc.stats.mu.RLock()
	defer nc.stats.mu.RUnlock()

	return Stats{
		EventsReceived: nc.stats.EventsReceived,
		Connected:      nc.stats.Connected,
		LastEventTime:  nc.stats.LastEventTime,
	}
}

// IsConnected returns whether the client is currently connected to the relay
func (nc *NostrClient) IsConnected() bool {
	nc.mu.RLock()
	defer nc.mu.RUnlock()
	return nc.relay != nil && nc.relay.IsConnected()
}

// Close closes the client connection and cleans up resources
func (nc *NostrClient) Close() {
	nc.mu.Lock()
	defer nc.mu.Unlock()

	if nc.sub != nil {
		nc.sub.Unsub()
		nc.sub = nil
	}

	if nc.relay != nil {
		_ = nc.relay.Close()
		nc.relay = nil
	}

	nc.stats.Connected = false
	log.Printf("Client %d: Closed", nc.id)
}
