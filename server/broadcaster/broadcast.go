// SPDX-License-Identifier: ice License 1.0

package broadcaster

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"log"
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
		relays *xsync.Map[string, *nostr.Relay]
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

var (
	eventKindsNoBroadcast = map[model.Kind]struct{}{
		model.CustomIONKindEphemeralEmbeddding: {},
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

func (b *Broadcaster) relayConnect(ctx context.Context, url string) (*nostr.Relay, error) {
	val, err, _ := b.sf.Do(url, func() (any, error) {
		relay := nostr.NewRelay(ctx, url, nostr.WithSignatureChecker(func(e *nostr.Event) bool {
			ev := model.Event{Event: *e}
			ok, err := ev.CheckSignature()
			return ok && err == nil
		}))
		err := relay.ConnectWithTLS(ctx, &tls.Config{
			InsecureSkipVerify: true,
		})
		if err != nil {
			return nil, err
		}
		return relay, nil
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to ensure relay %s", url)
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

func (b *Broadcaster) broadcastTo(ctx context.Context, targets []string, data []byte) (err error) {
	var e model.BroadcastEnvelope

	e.Relay = b.conf.RelayURL
	e.Event.CreatedAt = nostr.Now()
	e.Event.Kind = model.CustomIONKindEphemeralBatch
	e.Event.Content = string(data)
	if err = e.Event.SignWithAlg(b.conf.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519); err != nil {
		return errors.Wrap(err, "failed to sign broadcast event")
	}

	for i := range targets {
		relay := b.ensureRelay(ctx, targets[i])
		if relay == nil {
			err = errors.Join(err, errors.Errorf("%v is not available", targets[i]))
			continue
		}
		publishErr := relay.PublishEnvelope(ctx, &e)
		if publishErr != nil {
			if stored, _ := b.relays.LoadAndDelete(targets[i]); stored != nil {
				stored.Close() // Close the relay if it failed to publish.
			}
			err = errors.Join(err, errors.Wrapf(publishErr, "failed to publish broadcast envelope to %s", targets[i]))
			continue
		}
	}

	return err
}

func (b *Broadcaster) Broadcast(ctx context.Context, events ...*model.Event) (err error) {
	var authors []string
	for _, event := range events {
		if _, ok := eventKindsNoBroadcast[event.Kind]; ok {
			continue
		}

		authors = append(authors, event.GetMasterPublicKey())
	}
	if len(authors) == 0 {
		return nil // Nothing to broadcast.
	}

	data, err := json.Marshal(events)
	if err != nil {
		return errors.Wrap(err, "failed to marshal events")
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
		relays := model.CollectRelaysFromRelayEvent(ev, func(t model.Tag) bool {
			if strings.EqualFold(t.Value(), b.conf.RelayURL) {
				return false // Skip the broadcaster's own relay.
			}
			return len(t) < 2 || t[2] == "read"
		})
		if len(relays) == 0 {
			continue
		}
		targets[ev.GetMasterPublicKey()] = model.DeduplicateSlice(relays, strings.ToLower)
	}

	for pubkey, relays := range targets {
		bxErr := b.broadcastTo(ctx, relays, data)
		err = errors.Join(err, errors.Wrapf(bxErr, "failed to broadcast %d event(s) to %s", len(events), pubkey))
	}
	return err
}

func (b *Broadcaster) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.relays.Range(func(_ string, relay *nostr.Relay) bool {
		if relay != nil {
			_ = relay.Close()
		}
		return true
	})
	b.relays.Clear()
}
