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
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip42"
	"github.com/puzpuzpuz/xsync/v3"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/validation"
)

const (
	filterTextMRF = `most relevant followers`
)

var (
	protectedEventKinds = map[int]struct{}{
		nostr.KindGiftWrap: {},
	}
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

func canForwardEventContext(ctx context.Context, in *model.Event) bool {
	master, pk, _ := model.GetUserDataFromContext(ctx)

	return canForwardEvent(in, master, pk)
}

func canForwardEvent(in *model.Event, currentKeys ...string) bool {
	if _, ok := protectedEventKinds[in.Kind]; !ok {
		return true
	}

	for _, key := range currentKeys {
		for range in.Tags.All([]string{"p", key}) {
			return true
		}
	}
	return false
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

func (h *handler) handleAuth(_ context.Context, respWriter Writer, e *model.Event) *nostr.OKEnvelope {
	var resp = nostr.OKEnvelope{EventID: e.Event.ID}

	state, ok := h.connAuth.Load(respWriter)
	if !ok {
		resp.Reason = "received unexpected auth message: no challenge was sent"

		return &resp
	} else if state.Authenticated {
		resp.Reason = "received unexpected auth message: already authenticated"

		return &resp
	}

	_, err := nip42.ValidateAuthEvent(
		&e.Event,
		state.Challenge,
		h.relayURL,
		nip42.WithCustomVerificator(func(nostrEvent *nostr.Event) (bool, error) {
			return (&model.Event{Event: *nostrEvent}).CheckSignature()
		}))
	if err != nil {
		resp.Reason = "failed to validate auth event: " + err.Error()

		return &resp
	}

	h.connAuth.Store(respWriter, connAuthData{
		Challenge:       state.Challenge,
		MasterPublicKey: e.GetMasterPublicKey(),
		PublicKey:       e.PubKey,
		Authenticated:   true,
	})

	resp.OK = true

	return &resp
}

func (h *handler) prepareSubscription(ctx context.Context, sub *model.Subscription) *model.Subscription {
	for i := range sub.Filters {
		if !(strings.Contains(sub.Filters[i].Search, filterTextMRF) && sub.Filters[i].Tags.HasValues("p")) {
			continue
		}
		m, pk, authenticated := model.GetUserDataFromContext(ctx)
		sub.OneShot = true
		if !authenticated {
			// Should not happen, but just in case. Also set it to OneShot mode.
			continue
		}
		sub.Filters[i] = model.Filter{
			Kinds:   []int{nostr.KindFollowList},
			Authors: []string{m, pk},
			Search:  "include:dependencies:kind3>kind0+p+|" + strings.Join(sub.Filters[i].Tags.All("p"), ",") + "|",
			Limit:   1,
		}
	}
	return sub
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
	if wsSubscriptionListeners != nil {
		for _, listener := range wsSubscriptionListeners {
			fetchCtx, cancel := context.WithCancel(ctx)
			defer cancel()

			for event, err := range listener(fetchCtx, sub) {
				if err != nil {
					return errors.Wrapf(err, "failed to fetch events for subscription %+v", sub)
				} else if !canForwardEventContext(fetchCtx, event) {
					continue
				}
				wErr := h.writeResponse(respWriter, &nostr.EventEnvelope{SubscriptionID: &sub.SubscriptionID, Events: []*nostr.Event{&event.Event}})
				if wErr != nil {
					return errors.Wrapf(wErr, "failed to write event[%+v]", event)
				}
			}
		}
	} else {
		log.Printf("WARN: RegisterWSSubscriptionListener not registered, ignoring query part")
	}

	err := h.writeResponse(respWriter, model.PointerOf(nostr.EOSEEnvelope(sub.SubscriptionID)))
	if err == nil {
		if sub.OneShot {
			err = h.writeResponse(respWriter, &nostr.ClosedEnvelope{
				SubscriptionID: sub.SubscriptionID,
				Reason:         "processed: single request only subscription",
			})
		} else {
			h.linkSubscription(respWriter, sub)
		}
	}

	return err
}

func (h *handler) handleEvents(ctx context.Context, respWriter Writer, events []*model.Event, cfg *Config) error {
	for i := range events {
		if err := h.validateIncomingEvent(ctx, events[i], cfg); err != nil {
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

func (h *handler) validateIncomingEvent(ctx context.Context, evt *model.Event, cfg *Config) (err error) {
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
	if vErr := validation.Validate(ctx, evt); vErr != nil {
		return errors.Wrap(vErr, "wrong event parameters")
	}
	if cErr := evt.CheckNIP13Difficulty(cfg.NIP13MinLeadingZeroBits); cErr != nil {
		return errors.Wrap(cErr, "wrong event difficulty")
	}

	return nil
}

func CtxMatchEventsWithSubscription(ctx context.Context, sub *model.Subscription, events ...*model.Event) []*model.Event {
	master, pk, _ := model.GetUserDataFromContext(ctx)
	return matchEventsWithSubscription(master, pk, sub, events...)
}

func matchEventsWithSubscription(masterPublicKey, publicKey string, sub *model.Subscription, events ...*model.Event) []*model.Event {
	filtered := make([]*model.Event, 0, len(events))
	for _, event := range events {
		if !sub.Filters.Match(&event.Event) {
			continue
		} else if !canForwardEvent(event, masterPublicKey, publicKey) {
			continue
		}
		filtered = append(filtered, event)
	}
	return filtered
}

func (h *handler) notifyListenersAboutNewEvents(ctx context.Context, events ...*model.Event) error {
	var broadcast = map[Writer][]nostr.EventEnvelope{}

	// Collect events for each subscription.
	h.connSubs.Range(func(writer Writer, conn connSubscriptions) bool {
		authData, _ := h.connAuth.Load(writer)
		conn.Subscriptions.Range(func(_ string, sub *model.Subscription) bool {
			envelope := nostr.EventEnvelope{SubscriptionID: &sub.SubscriptionID}
			matchedEvents := matchEventsWithSubscription(authData.MasterPublicKey, authData.PublicKey, sub, events...)
			for _, ev := range matchedEvents {
				envelope.Events = append(envelope.Events, &ev.Event)
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

func (h *handler) handleCount(ctx context.Context, envelope *nostr.CountEnvelope) error {
	count, err := query.CountEvents(ctx, &model.Subscription{Filters: envelope.Filters})
	if err != nil {
		return errors.Wrap(err, "failed to count events")
	}

	envelope.Count = &count

	return nil
}
