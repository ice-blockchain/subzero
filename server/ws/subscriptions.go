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
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hashicorp/go-multierror"
	"github.com/nbd-wtf/go-nostr"

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

	eos := nostr.EOSEEnvelope(sub.SubscriptionID)
	err := h.writeResponse(respWriter, &eos)

	h.subListenersMx.Lock()
	defer h.subListenersMx.Unlock()
	subsFromCurrConnection, ok := h.subListeners[respWriter]
	if !ok {
		subsFromCurrConnection = make(map[string]*model.Subscription)
		if h.subListeners == nil {
			h.subListeners = make(map[Writer]map[string]*model.Subscription)
		}
		h.subListeners[respWriter] = subsFromCurrConnection
	}
	subsFromCurrConnection[sub.SubscriptionID] = sub

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

	if err := h.notifyListenersAboutNewEvents(events...); err != nil {
		return errors.Wrap(err, "failed to notify subscribers about new events")
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

func (h *handler) notifyListenersAboutNewEvents(events ...*model.Event) error {
	var err *multierror.Error

	// TODO: FIX race condition here (concurrent map read and map write).
	for writer, subs := range h.subListeners {
		for _, sub := range subs {
			for eventIdx := range events {
				if sub.Filters.Match(&events[eventIdx].Event) {
					err = multierror.Append(
						err,
						h.writeResponse(writer, &nostr.EventEnvelope{SubscriptionID: &sub.SubscriptionID, Events: []*nostr.Event{&events[eventIdx].Event}}),
					)
				}
			}
		}
	}

	return err.ErrorOrNil()
}

func (h *handler) CancelSubscription(_ context.Context, respWriter Writer, subID *string) error {
	h.subListenersMx.Lock()
	defer h.subListenersMx.Unlock()
	if subs, found := h.subListeners[respWriter]; found {
		if subID == nil {
			delete(h.subListeners, respWriter)
			h.connAuth.Delete(respWriter)

			return nil
		}
		delete(h.subListeners[respWriter], *subID)
		if len(subs) == 0 {
			delete(h.subListeners, respWriter)
		}
		if err := h.writeResponse(respWriter, &nostr.ClosedEnvelope{SubscriptionID: *subID, Reason: ""}); err != nil {
			return errors.Wrap(err, "failed to write CLOSED message")
		}
	}

	return nil
}

func (h *handler) handleCount(ctx context.Context, envelope *nostr.CountEnvelope) error {
	count, err := query.CountEvents(ctx, &model.Subscription{Filters: envelope.Filters})
	if err != nil {
		return errors.Wrap(err, "failed to count events")
	}

	envelope.Count = &count

	return nil
}
