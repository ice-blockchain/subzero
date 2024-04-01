package ws

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/wintr/log"

	"github.com/hashicorp/go-multierror"
	"github.com/nbd-wtf/go-nostr"
	"github.com/pkg/errors"
)

func (h *handler) handleReq(ctx context.Context, respWriter Writer, sub *model.Subscription) error {
	if wsSubscriptionListener != nil {
		var mErr *multierror.Error
		events, err := wsSubscriptionListener(ctx, sub)
		if err != nil {
			return errors.Wrapf(err, "failed to query events for subscription req %+v", sub)
		}
		for _, event := range events {
			mErr = multierror.Append(mErr, h.writeResponse(respWriter, &nostr.EventEnvelope{SubscriptionID: &sub.SubscriptionID, Event: event.Event}))
		}
		if mErr.ErrorOrNil() != nil {
			return errors.Wrapf(mErr, "failed to write events for subscription %+v", sub)
		}
	} else {
		log.Warn("RegisterWSSubscriptionListener not registered, ignoring query part")
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
func (h *handler) handleEvent(ctx context.Context, event *model.Event) (err error) {
	if err = h.validateIncomingEvent(event); err != nil {
		return errors.Wrapf(err, "invalid event")
	}
	if event.Kind == nostr.KindDeletion {
		return errors.Errorf("Not implemented yet")
	}
	isEphemeralEvent := (20000 <= event.Kind && event.Kind < 30000)
	if !isEphemeralEvent {
		if wsEventListener == nil {
			log.Panic(errors.Errorf("wsEventListener to store events not set"))
		}
		if saveErr := wsEventListener(ctx, event); saveErr != nil {
			switch {
			default:
				return errors.Wrapf(err, "failed to store event")
			}
		}
	}
	if err = h.notifyListenersAboutNewEvent(event); err != nil {
		return errors.Wrapf(err, "failed to notify subscribers about new event: %+v", event)
	}
	return nil
}

func (h *handler) validateIncomingEvent(evt *model.Event) (err error) {
	hash := sha256.Sum256(evt.Serialize())
	if id := hex.EncodeToString(hash[:]); id != evt.ID {
		return errors.New("event id is invalid")
	}
	var ok bool
	if ok, err = evt.CheckSignature(); err != nil {
		return errors.Wrapf(err, "invalid event signature")
	} else if !ok {
		return errors.New("invalid event signature")
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
		} else {
			delete(h.subListeners[respWriter], *subID)
			if len(subs) == 0 {
				delete(h.subListeners, respWriter)
			}
			return h.writeResponse(respWriter, &nostr.ClosedEnvelope{SubscriptionID: *subID, Reason: ""})
		}
	}
	return nil
}
