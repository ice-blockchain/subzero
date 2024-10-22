// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log"

	"github.com/cockroachdb/errors"
	"github.com/hashicorp/go-multierror"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

func (h *handler) handleReq(ctx context.Context, respWriter Writer, sub *subscription) error {
	if wsSubscriptionListener != nil {
		fetchCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		for event, err := range wsSubscriptionListener(fetchCtx, sub.Subscription) {
			if err != nil {
				return errors.Wrapf(err, "failed to fetch events for subscription %+v", sub)
			}
			wErr := h.writeResponse(respWriter, &nostr.EventEnvelope{SubscriptionID: &sub.SubscriptionID, Event: event.Event})
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
		subsFromCurrConnection = make(map[string]*subscription)
		if h.subListeners == nil {
			h.subListeners = make(map[Writer]map[string]*subscription)
		}
		h.subListeners[respWriter] = subsFromCurrConnection
	}
	subsFromCurrConnection[sub.SubscriptionID] = sub

	return err
}

func (h *handler) handleEvents(ctx context.Context, events []*model.Event, cfg *Config) error {
	for i := range events {
		if err := h.validateIncomingEvent(events[i], cfg); err != nil {
			return errors.Wrapf(err, "event %v: invalid", events[i])
		}
	}

	if wsEventListener == nil {
		log.Panic("wsEventListener is not set")
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

	for writer, subs := range h.subListeners {
		for _, sub := range subs {
			for eventIdx := range events {
				if sub.Filters.Match(&events[eventIdx].Event) {
					err = multierror.Append(
						err,
						h.writeResponse(writer, &model.EventEnvelope{SubscriptionID: &sub.SubscriptionID, Events: []*model.Event{events[eventIdx]}}),
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
