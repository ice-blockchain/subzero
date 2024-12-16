// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"log"
	"math"
	"math/rand/v2"
	"strconv"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"
	"github.com/puzpuzpuz/xsync/v3"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

func generateChallenge(hints ...string) string {
	const valueMin = 1_000_000_000

	h := sha512.New()
	h.Write([]byte(time.Now().UTC().Truncate(time.Minute).Format(time.Stamp)))
	h.Write([]byte(strconv.FormatUint(rand.Uint64N(math.MaxUint64)+valueMin, 16)))
	for i := range hints {
		h.Write([]byte(hints[i]))
	}

	return base64.URLEncoding.EncodeToString(h.Sum(nil))
}

func (h *handler) authRequiredReq(respWriter Writer, sub *model.Subscription, challenge string) error {
	err := h.writeResponse(respWriter, &nostr.AuthEnvelope{
		Challenge: &challenge,
	})
	if err != nil {
		return errors.Wrap(err, "failed to write AUTH message")
	}

	err = h.writeResponse(respWriter, &nostr.ClosedEnvelope{
		SubscriptionID: sub.SubscriptionID,
		Reason:         errAuthRequired.Error(),
	})

	return errors.Wrap(err, "failed to write CLOSED message")
}

func (h *handler) linkSubscription(respWriter Writer, sub *model.Subscription) {
	conn, _ := h.connSubs.LoadOrCompute(respWriter, func() connSubscriptions {
		return connSubscriptions{
			Subscriptions: xsync.NewMapOf[string, *model.Subscription](),
		}
	})
	conn.Subscriptions.Store(sub.SubscriptionID, sub)
}

func (h *handler) unlinkSubscription(respWriter Writer, ID *string) bool {
	if ID == nil {
		// Connection is closing, remove all subscriptions.
		h.connSubs.Delete(respWriter)

		return false
	}

	conn, ok := h.connSubs.Load(respWriter)
	if !ok {
		return false
	}

	_, ok = conn.Subscriptions.LoadAndDelete(*ID)

	return ok
}

func (h *handler) handleReq(ctx context.Context, respWriter Writer, sub *model.Subscription) error {
	if reqMustAuth != nil {
		if authRequired := reqMustAuth(ctx, sub); authRequired {
			status, _ := h.connAuth.LoadOrCompute(respWriter, func() connAuthData {
				return connAuthData{
					Challenge: generateChallenge(sub.SubscriptionID),
				}
			})
			if authRequired && !status.Authenticated {
				return h.authRequiredReq(respWriter, sub, status.Challenge)
			}
		}
	}
	if wsSubscriptionListener != nil {
		fetchCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		for event, err := range wsSubscriptionListener(fetchCtx, sub) {
			if err != nil {
				return errors.Wrapf(err, "failed to fetch events for subscription %+v", sub)
			}
			wErr := h.writeResponse(respWriter, &nostr.EventEnvelope{SubscriptionID: &sub.SubscriptionID, Events: []*nostr.Event{&event.Event}})
			if wErr != nil {
				return errors.Wrapf(wErr, "failed to write event[%+v]", event)
			}
		}
	} else {
		log.Printf("WARN: RegisterWSSubscriptionListener not registered, ignoring query part")
	}

	err := h.writeResponse(respWriter, model.PointerOf(nostr.EOSEEnvelope(sub.SubscriptionID)))
	if err == nil {
		h.linkSubscription(respWriter, sub)
	}

	return err
}

func (h *handler) handleEvents(ctx context.Context, respWriter Writer, events []*model.Event, cfg *Config) error {
	for i := range events {
		if err := h.validateIncomingEvent(events[i], cfg); err != nil {
			return errors.Wrapf(err, "event %v: invalid", events[i])
		}
	}

	if wsEventListener == nil {
		log.Panic("wsEventListener is not set")
	}

	if eventMustAuth != nil {
		if authRequired := eventMustAuth(ctx, events...); authRequired {
			status, _ := h.connAuth.LoadOrCompute(respWriter, func() connAuthData {
				return connAuthData{
					Challenge: generateChallenge(),
				}
			})
			if authRequired && !status.Authenticated {
				err := h.writeResponse(respWriter, &nostr.AuthEnvelope{
					Challenge: &status.Challenge,
				})
				if err != nil {
					return errors.Wrap(err, "failed to write AUTH message")
				}
				return errAuthRequired
			}
		}
	}

	if err := wsEventListener(ctx, events...); err != nil {
		return errors.Wrap(err, "failed to store events")
	}

	if err := h.notifyListenersAboutNewEvents(ctx, events...); err != nil {
		return errors.Wrap(ErrNotifyFailed, err.Error())
	}

	return nil
}

func (h *handler) validateIncomingEvent(evt *model.Event, cfg *Config) (err error) {
	hash := sha256.Sum256(evt.Serialize())
	if id := hex.EncodeToString(hash[:]); id != evt.ID {
		return errors.New("event id is invalid")
	}
	var ok bool
	if ok, err = evt.CheckSignature(); err != nil {
		return errors.Wrap(err, "invalid event signature")
	} else if !ok {
		return errors.New("invalid event signature")
	}
	if vErr := evt.Validate(); vErr != nil {
		return errors.Wrap(vErr, "wrong event parameters")
	}
	if cErr := evt.CheckNIP13Difficulty(cfg.NIP13MinLeadingZeroBits); cErr != nil {
		return errors.Wrap(cErr, "wrong event difficulty")
	}

	return nil
}

func (h *handler) notifyListenersAboutNewEvents(ctx context.Context, events ...*model.Event) error {
	var broadcast = map[Writer][]nostr.EventEnvelope{}

	// Collect events for each subscription.
	h.connSubs.Range(func(writer Writer, conn connSubscriptions) bool {
		conn.Subscriptions.Range(func(_ string, sub *model.Subscription) bool {
			var envelope = nostr.EventEnvelope{SubscriptionID: &sub.SubscriptionID}
			for _, event := range events {
				if !sub.Filters.Match(&event.Event) {
					continue
				}
				envelope.Events = append(envelope.Events, &event.Event)
			}
			if len(envelope.Events) > 0 {
				broadcast[writer] = append(broadcast[writer], envelope)
			}
			return true
		})
		return true
	})

	var wg sync.WaitGroup
	wg.Add(len(broadcast))
	ch := make(chan error, len(broadcast))
	for writer, envelopes := range broadcast {
		go func() {
			defer wg.Done()

			for i := range envelopes {
				if ctx.Err() != nil {
					break
				}

				err := h.writeResponse(writer, &envelopes[i])
				if err != nil {
					ch <- errors.Wrapf(err, "failed to write events for subscription %v", envelopes[i].SubscriptionID)
					break // Stop writing events for this writer.
				}
			}
		}()
	}
	go func() {
		wg.Wait()
		close(ch)
	}()

	var err error
	for writeErr := range ch {
		err = errors.Join(err, writeErr)
	}
	return err
}

func (h *handler) CancelSubscription(_ context.Context, respWriter Writer, subID *string) (err error) {
	if !h.unlinkSubscription(respWriter, subID) {
		// Subscription not found.
		return
	}

	if subID != nil {
		err = errors.Wrap(h.writeResponse(respWriter, &nostr.ClosedEnvelope{SubscriptionID: *subID, Reason: ""}), "failed to write CLOSED message")
	}

	return err
}

func (h *handler) handleCount(ctx context.Context, envelope *nostr.CountEnvelope) error {
	count, err := query.CountEvents(ctx, &model.Subscription{Filters: envelope.Filters})
	if err != nil {
		return errors.Wrap(err, "failed to count events")
	}

	envelope.Count = &count

	return nil
}
