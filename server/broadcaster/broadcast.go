// SPDX-License-Identifier: ice License 1.0

package broadcaster

import (
	"context"
	"crypto/tls"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/llxisdsh/pb"
	"github.com/nbd-wtf/go-nostr"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/singleflight"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

type (
	Broadcaster struct {
		relays *pb.MapOf[string, *nostr.Relay] // URL -> Relay.
		sf     singleflight.Group
		conf   Config
		mu     sync.RWMutex
	}
	Config struct {
		QueryFunc  func(ctx context.Context, filters ...model.Filter) query.EventIterator
		RelayURL   string
		PrivateKey string
	}
)

func New(conf Config) *Broadcaster {
	if conf.QueryFunc == nil {
		conf.QueryFunc = query.GetStoredEvents
	}
	return &Broadcaster{
		relays: pb.NewMapOf[string, *nostr.Relay](),
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
		log.Panic().Str("context", "BROADCASTER").Err(err).Msg("failed to sign authentication event")
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
		if strings.Contains(err.Error(), "auth-required:") {
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
			log.Error().Str("context", "BROADCASTER").Err(err).Str("relay_url", url).Msg("failed to connect to relay")
		}
		return r, r == nil
	})
	return relay
}

func (b *Broadcaster) collectTargets(ctx context.Context, events model.Events) (map[string][]string, error) {
	authoritative := true
	authors := make([]string, 0, len(events))
	targets := make(map[string][]string, len(authors)) // Master public key -> relay URLs.
	addresses := make([]string, 0, len(events))
	wrapReceivers := make([]string, 0, len(events))
	for _, event := range events {
		switch event.Kind {
		case nostr.KindGiftWrap:
			if v := event.GetTag("p").Value(); v != "" {
				wrapReceivers = append(wrapReceivers, v)
			}
		case model.CustomIONKindEphemeralEmbedding:
			authoritative = false
			var content model.Event
			err := content.UnmarshalJSON([]byte(event.Content))
			if err != nil {
				return nil, errors.Wrapf(err, "malformed %v event, incorrect content %v", model.CustomIONKindEphemeralEmbedding, event.Content)
			}
			switch content.Kind {
			case nostr.KindRelayListMetadata:
				relays := model.CollectRelaysFromRelayEvent(&content)
				if len(relays) > 0 {
					targets[event.GetMasterPublicKey()] = model.DeduplicateSlice(relays, strings.ToLower)
				}
			}
		default:
			authors = append(authors, event.GetMasterPublicKey())
			for _, tag := range event.Tags {
				if (tag.Key() == "a" || tag.Key() == "e") && tag.Value() != "" {
					addresses = append(addresses, tag.Value())
				}
			}
		}
	}

	var filters model.Filters
	if len(wrapReceivers) > 0 {
		filters = append(filters, model.Filter{
			Authors: wrapReceivers,
			Kinds:   []model.Kind{nostr.KindRelayListMetadata},
			Limit:   len(wrapReceivers),
		})
	}

	if authoritative {
		// In authoritative mode, use event authors as targets.
		if len(authors) > 0 {
			lookup := make([]string, 0, len(authors))
			for _, author := range authors {
				if _, exists := targets[author]; !exists {
					lookup = append(lookup, author)
				}
			}
			if len(lookup) > 0 {
				filters = append(filters, model.Filter{
					Authors: lookup,
					Kinds:   []model.Kind{nostr.KindRelayListMetadata},
					Limit:   len(lookup),
				})
			}
		}
	} else if len(addresses) > 0 {
		// In non-authoritative mode, use event references as targets, and try to find relays for the authors of the original events.
		filters = append(filters, model.Filter{
			Addresses: addresses,
			Search:    "include:dependencies:kind" + strconv.Itoa(model.KindAny) + ">kind10002",
		})
	}

	if len(filters) == 0 {
		return targets, nil // No filters to query.
	}

	it := b.conf.QueryFunc(ctx, filters...)
	for event, err := range it {
		if err != nil {
			return nil, errors.Wrap(err, "failed to query relay list metadata")
		} else if event.Kind != nostr.KindRelayListMetadata {
			continue // Skip non-relay list metadata events.
		}

		relays := model.CollectRelaysFromRelayEvent(event)
		if len(relays) > 0 {
			targets[event.GetMasterPublicKey()] = model.DeduplicateSlice(relays, strings.ToLower)
		}
	}
	return targets, nil
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
	if len(events) == 0 {
		return nil // Nothing to broadcast.
	}

	start := time.Now()
	targets, err := b.collectTargets(ctx, events)
	if err != nil {
		return err
	} else if len(targets) == 0 {
		log.Trace().
			Str("context", "Broadcaster").
			Int("event_count", len(events)).
			Str("events", model.Events(events).String()).
			Msg("no targets found for events")
		return nil
	}
	end := time.Since(start)

	log.Trace().
		Str("context", "Broadcaster").
		Int("event_count", len(events)).
		Int("target_count", len(targets)).
		Interface("event_ids", model.Events(events).IDs()).
		Interface("targets", targets).
		Dur("collect_duration", end).
		Msg("broadcasting events to users")

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
				start := time.Now()
				bxErr := b.broadcastTo(ctx, relay, events)
				end := time.Since(start)
				log.Trace().
					Str("context", "Broadcaster").
					Int("event_count", len(events)).
					Interface("event_ids", model.Events(events).IDs()).
					Str("relay", relay).
					Interface("result", bxErr).
					Dur("duration", end).
					Msg("publishing events to relay")
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
