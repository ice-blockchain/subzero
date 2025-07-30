// SPDX-License-Identifier: ice License 1.0

package broadcaster

import (
	"context"
	"crypto/tls"
	"log"
	"runtime"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"
	"github.com/puzpuzpuz/xsync/v4"
	"golang.org/x/sync/singleflight"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

type (
	Broadcaster struct {
		relays *xsync.Map[string, *nostr.Relay] // URL -> Relay.
		sf     singleflight.Group
		conf   Config
		mu     sync.RWMutex
	}
	Config struct {
		RelayURL   string
		PrivateKey string
		QueryFunc  func(ctx context.Context, filters ...model.Filter) query.EventIterator
	}
)

func New(conf Config) *Broadcaster {
	if conf.QueryFunc == nil {
		conf.QueryFunc = query.GetStoredEvents
	}
	return &Broadcaster{
		relays: xsync.NewMap[string, *nostr.Relay](),
		conf:   conf,
	}
}

func (b *Broadcaster) newInitAuthEvent(url string) *model.Event {
	var ev model.Event

	ev.Kind = nostr.KindClientAuthentication
	ev.CreatedAt = nostr.Now()
	ev.Tags = model.Tags{
		{"challenge", "init"},
	}
	if err := ev.SignWithAlg(b.conf.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519); err != nil {
		log.Panicf("[Broadcaster] Failed to sign authentication event: %v", err)
	}

	return &ev
}

func (b *Broadcaster) newRelay(ctx context.Context, url string) (*nostr.Relay, error) {
	relay := nostr.NewRelay(ctx, url, nostr.WithSignatureChecker(func(e *nostr.Event) bool {
		ev := model.Event{Event: *e}
		ok, err := ev.CheckSignature()
		return ok && err == nil
	}))

	err := relay.ConnectWithTLS(ctx, &tls.Config{
		InsecureSkipVerify: true,
	})
	if err != nil {
		return nil, errors.Wrap(err, "relay connection failed")
	}

	err = relay.Publish(ctx, b.newInitAuthEvent(url).Event)
	if err != nil {
		if strings.HasPrefix(err.Error(), "auth-required:") {
			err = errors.Wrap(relay.Auth(ctx, func(event *nostr.Event) error {
				subZeroEvent := model.Event{Event: *event}
				subZeroEvent.Tags = append(subZeroEvent.Tags,
					model.Tag{"user-agent", runtime.GOOS + "/" + runtime.Version() + " subzero/1.0 (" + b.conf.RelayURL + ")"},
				)
				if err := subZeroEvent.SignWithAlg(b.conf.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519); err != nil {
					return err
				}
				*event = subZeroEvent.Event

				return nil
			}), "failed to authenticate to relay")

			if err == nil {
				return relay, nil // Authentication successful.
			}
		}

		relay.Close()
		return nil, errors.Wrapf(err, "failed to publish authentication event to %s", url)
	}

	return relay, nil
}

func (b *Broadcaster) relayConnect(ctx context.Context, url string) (*nostr.Relay, error) {
	val, err, _ := b.sf.Do(url, func() (any, error) {
		return b.newRelay(ctx, url)
	})
	if err != nil {
		return nil, err
	}
	return val.(*nostr.Relay), nil
}

func (b *Broadcaster) ensureRelay(ctx context.Context, url string) *nostr.Relay {
	relay, _ := b.relays.LoadOrCompute(url, func() (*nostr.Relay, bool) {
		r, err := b.relayConnect(ctx, url)
		if err != nil {
			log.Printf("[Broadcaster] Failed to connect to relay: %v", err)
		}
		return r, r == nil
	})
	return relay
}

func (b *Broadcaster) broadcastTo(ctx context.Context, target string, events model.Events) (err error) {
	relay := b.ensureRelay(ctx, target)
	if relay == nil {
		return errors.Errorf("%v is not available", target)
	}

	envelope := model.BroadcastEnvelope{
		Relay:  b.conf.RelayURL,
		Events: events,
	}
	publishErr := relay.PublishEnvelope(ctx, &envelope)
	if publishErr != nil {
		if stored, _ := b.relays.LoadAndDelete(target); stored != nil {
			stored.Close() // Close the relay if it failed to publish.
		}
		return errors.Wrapf(publishErr, "failed to publish broadcast envelope to %s", target)
	}
	return nil
}

func (b *Broadcaster) Broadcast(ctx context.Context, events ...*model.Event) (err error) {
	var authors []string
	for _, event := range events {
		authors = append(authors, event.GetMasterPublicKey())
	}
	if len(authors) == 0 {
		return nil // Nothing to broadcast.
	}

	it := b.conf.QueryFunc(ctx, model.Filter{
		Authors: authors,
		Kinds:   []model.Kind{nostr.KindRelayListMetadata},
		Limit:   len(authors),
	})

	targets := make(map[string][]string, len(authors)) // Map of author public keys to their relay URLs.
	for ev, err := range it {
		if err != nil {
			return errors.Wrap(err, "failed to query relay list metadata")
		}
		relays := model.CollectRelaysFromRelayEvent(ev)
		if len(relays) == 0 {
			continue
		}
		targets[ev.GetMasterPublicKey()] = model.DeduplicateSlice(relays, strings.ToLower)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(targets))
	for pubkey, relays := range targets {
		for _, relay := range relays {
			if strings.EqualFold(relay, b.conf.RelayURL) {
				continue // Skip broadcasting to self.
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				bxErr := b.broadcastTo(ctx, relay, events)
				errCh <- errors.Wrapf(bxErr, "failed to broadcast %d event(s) of %s", len(events), pubkey)
			}()
		}
	}

	go func() {
		wg.Wait()
		close(errCh)
	}()

	for bxErr := range errCh {
		err = errors.Join(err, bxErr)
	}

	return err
}

func (b *Broadcaster) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.relays.Range(func(_ string, relay *nostr.Relay) bool {
		if relay != nil {
			relay.Close()
		}
		return true
	})
	b.relays.Clear()
}
