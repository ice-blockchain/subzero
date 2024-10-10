// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"

	"github.com/gookit/goutil/errorx"
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
				return errorx.Wrapf(err, "failed to fetch events for subscription %+v", sub)
			}
			wErr := h.writeResponse(respWriter, &nostr.EventEnvelope{SubscriptionID: &sub.SubscriptionID, Event: event.Event})
			if wErr != nil {
				return errorx.Wrapf(wErr, "failed to write event[%+v]", event)
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

func (h *handler) handleEvent(ctx context.Context, event *model.Event, cfg *Config) (err error) {
	if err = h.validateIncomingEvent(event, cfg); err != nil {
		return errorx.Withf(err, "invalid: event is invalid")
	}
	if !event.IsEphemeral() {
		if wsEventListener == nil {
			log.Panic(errorx.Errorf("wsEventListener to store events not set"))
		}
		if saveErr := wsEventListener(ctx, event); saveErr != nil {
			switch {
			case errors.Is(saveErr, model.ErrDuplicate):
				return nil
			default:
				return errorx.Withf(saveErr, "failed to store event")
			}
		}
	}
	if err = h.notifyListenersAboutNewEvent(event); err != nil {
		return errorx.Withf(err, "failed to notify subscribers about new event: %+v", event)
	}

	return nil
}

func (h *handler) validateIncomingEvent(evt *model.Event, cfg *Config) (err error) {
	hash := sha256.Sum256(evt.Serialize())
	if id := hex.EncodeToString(hash[:]); id != evt.ID {
		return errorx.New("event id is invalid")
	}
	var ok bool
	if ok, err = evt.CheckSignature(); err != nil {
		return errorx.Withf(err, "invalid event signature")
	} else if !ok {
		return errorx.New("invalid event signature")
	}
	if vErr := evt.Validate(); vErr != nil {
		return errorx.Withf(err, "wrong event parameters")
	}
	if cErr := evt.CheckNIP13Difficulty(cfg.NIP13MinLeadingZeroBits); cErr != nil {
		return errorx.Withf(cErr, "wrong event difficulty")
	}

	return nil
}

func (h *handler) notifyListenersAboutNewEvent(ev *model.Event) error {
	var err *multierror.Error
	for writer, subs := range h.subListeners {
		for _, sub := range subs {
			if sub.Filters.Match(&ev.Event) {
				err = multierror.Append(
					err,
					h.writeResponse(writer, &nostr.EventEnvelope{SubscriptionID: &sub.SubscriptionID, Event: ev.Event}),
				)
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
			return errorx.Withf(err, "failed to write CLOSED message")
		}
	}

	return nil
}

func (h *handler) handleCount(ctx context.Context, envelope *nostr.CountEnvelope) error {
	count, err := query.CountEvents(ctx, &model.Subscription{Filters: envelope.Filters})
	if err != nil {
		return errorx.With(err, "failed to count events")
	}

	envelope.Count = &count

	return nil
}
