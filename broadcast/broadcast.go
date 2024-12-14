// SPDX-License-Identifier: ice License 1.0

package broadcast

import (
	"context"
	"crypto/tls"
	"log"
	"math/rand/v2"
	"sync/atomic"
	"time"

	"github.com/alitto/pond/v2"
	"github.com/cespare/xxhash/v2"
	"github.com/cockroachdb/errors"
	"github.com/elastic/go-freelru"
	"github.com/nbd-wtf/go-nostr"
	"golang.org/x/sync/singleflight"

	"github.com/ice-blockchain/subzero/model"
)

type (
	DatabaseAdapter interface {
		ReadRelays(ctx context.Context, userPubkey string) ([]string, error)
	}
	Option func(*broadcaster)

	config struct {
		Concurrency   int           `yaml:"concurrency"`
		QueueSize     int           `yaml:"queue-size"`
		MaxRelaysOpen uint32        `yaml:"max-relays-open"`
		RelayOpenTime time.Duration `yaml:"relay-open-time"`
		RelayURL      string        `yaml:"relay-url" validate:"required,url"`
	}
	broadcasterTODO struct {
		AuthorPubKey string
		Event        *model.Event
	}
	broadcaster struct {
		WorkerPool pond.Pool
		RelayPool  *freelru.ShardedLRU[string, *nostr.Relay]
		RelayTLS   *tls.Config
		RelayTTL   time.Duration
		Queue      chan broadcasterTODO
		DB         DatabaseAdapter
		SfGroup    singleflight.Group
		SelfURL    string
		closed     atomic.Bool
	}
)

var (
	ErrNoRelays  = errors.New("no relays available")
	ErrQueueFull = errors.New("broadcast queue is full")
	ErrClosed    = errors.New("broadcaster is closed")
)

func WithCustomDatabaseAdapter(db DatabaseAdapter) Option {
	return func(b *broadcaster) {
		b.DB = db
	}
}

func WithRelayTLSConfig(tlsConfig *tls.Config) Option {
	return func(b *broadcaster) {
		b.RelayTLS = tlsConfig
	}
}

func newBroadcaster(ctx context.Context, conf *config, options ...Option) (*broadcaster, error) {
	r, err := freelru.NewSharded[string, *nostr.Relay](conf.MaxRelaysOpen, func(k string) uint32 {
		return uint32(xxhash.Sum64String(k))
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create relay pool")
	}

	b := &broadcaster{
		WorkerPool: pond.NewPool(conf.Concurrency, pond.WithContext(ctx)),
		RelayPool:  r,
		RelayTTL:   conf.RelayOpenTime,
		DB:         new(nativeDatabaseAdapter),
		Queue:      make(chan broadcasterTODO, conf.QueueSize),
		SelfURL:    conf.RelayURL,
	}
	for i := range options {
		options[i](b)
	}

	r.SetLifetime(conf.RelayOpenTime)
	r.SetOnEvict(func(k string, v *nostr.Relay) {
		log.Printf("BROADCAST: closing relay %q due to eviction", k)
		v.Close()
	})

	go b.worker(ctx)

	return b, nil
}

func (b *broadcaster) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return

		case todo, ok := <-b.Queue:
			if !ok {
				// The queue is closed.
				return
			}
			b.WorkerPool.Submit(func() {
				err := b.executeTODO(ctx, todo)
				if err != nil && !errors.Is(err, context.Canceled) {
					log.Printf("BROADCAST: failed to execute TODO: %v: %v", todo, err)
				}
			})
		}
	}
}

func (b *broadcaster) getRelayPair(ctx context.Context, todo broadcasterTODO) (cc *nostr.Relay, dest *nostr.Relay, err error) {
	var destRelays []string
	for _, tag := range todo.Event.Tags {
		if tag.Key() == "p" {
			relays, err := b.DB.ReadRelays(ctx, tag.Value())
			if err != nil {
				return nil, nil, errors.Wrapf(err, "failed to collect destination relays for event %v", todo.Event.ID)
			}
			destRelays = append(destRelays, relays...)
		}
	}

	sourceRelays, err := b.DB.ReadRelays(ctx, todo.AuthorPubKey)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to read source relays for event %v", todo.Event.ID)
	}

	cc, err = b.selectRelay(ctx, sourceRelays)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to select source relay for event %v", todo.Event.ID)
	}

	dest, err = b.selectRelay(ctx, destRelays)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to select destination relay for event %v", todo.Event.ID)
	}

	return cc, dest, nil
}

func (b *broadcaster) executeTODO(ctx context.Context, todo broadcasterTODO) error {
	cc, dest, err := b.getRelayPair(ctx, todo)
	if err != nil {
		return err
	} else if cc == nil && dest == nil {
		// No relays available.
		return nil
	}

	var errDest, errCC error
	if dest != nil {
		errDest = errors.Wrap(dest.Publish(ctx, todo.Event.Event), "failed to publish event to destination relay")
	}
	if cc != nil && (dest == nil || cc.URL != dest.URL) {
		errCC = errors.Wrap(cc.Publish(ctx, todo.Event.Event), "failed to publish event to source relay")
	}

	return errors.Join(errDest, errCC)
}

func (b *broadcaster) ensureRelay(ctx context.Context, relayURL string) (*nostr.Relay, error) {
	relayURL = nostr.NormalizeURL(relayURL)

	relay, ok := b.RelayPool.GetAndRefresh(relayURL, b.RelayTTL)
	if ok && relay.IsConnected() {
		return relay, nil
	} else if ok {
		// Close the relay as it's not connected anymore.
		b.RelayPool.Remove(relayURL)
	}

	result, err, _ := b.SfGroup.Do(relayURL, func() (any, error) {
		relay = nostr.NewRelay(ctx, relayURL)
		err := relay.ConnectWithTLS(ctx, b.RelayTLS)
		if err != nil {
			return nil, err
		}
		b.RelayPool.Add(relayURL, relay)

		return relay, nil
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to connect to relay %q", relayURL)
	}

	return result.(*nostr.Relay), nil
}

func (b *broadcaster) selectRelay(ctx context.Context, relays []string) (*nostr.Relay, error) {
	if len(relays) == 0 {
		// Nothing to select from.
		return nil, nil
	}

	// Select a random relay that is not the broadcaster itself and available.
	rand.Shuffle(len(relays), func(i, j int) {
		relays[i], relays[j] = relays[j], relays[i]
	})
	for i := range relays {
		if relays[i] == b.SelfURL {
			continue
		}

		relay, err := b.ensureRelay(ctx, relays[i])
		if err == nil {
			return relay, nil
		}
	}

	if len(relays) == 1 && relays[0] == b.SelfURL {
		return nil, nil
	}
	return nil, ErrNoRelays
}

func (b *broadcaster) Broadcast(ctx context.Context, authorPubKey string, event *model.Event) error {
	if b.closed.Load() {
		return ErrClosed
	}

	select {
	case <-ctx.Done():
		return ctx.Err()

	case b.Queue <- broadcasterTODO{AuthorPubKey: authorPubKey, Event: event}:
		return nil

	default:
		return errors.Wrapf(ErrQueueFull, "failed to broadcast event %v", event)
	}
}

func (b *broadcaster) Close() {
	b.closed.Store(true)

	// Close the input queue.
	close(b.Queue)

	// Wait for all workers to finish.
	b.WorkerPool.StopAndWait()

	// Remove all expired relays.
	b.RelayPool.Purge()

	// Close all alive relays.
	for _, relay := range b.RelayPool.Keys() {
		b.RelayPool.Remove(relay)
	}
}
