// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"math"
	"math/rand/v2"
	"strconv"
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

var (
	protectedEventKinds = map[int]struct{}{
		nostr.KindGiftWrap: {},
	}

	ErrCommunityActionForbidden = errors.New("only admin, owner or moderator can remove user/post/comment/repost from the community")
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
			} else if !canForwardEventContext(fetchCtx, event) {
				continue
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
	var allEvents []*model.Event
	for i := range events {
		if err := h.validateIncomingEvent(ctx, events[i], cfg); err != nil {
			return errors.Wrapf(err, "event %v: invalid", events[i])
		}
		if events[i].Kind == nostr.KindDeletion {
			evs, err := prepareCommunityEventsForDeletion(ctx, events[i])
			if err != nil {
				return err
			}
			if len(evs) > 0 {
				allEvents = append(allEvents, evs...)

				continue
			}
		}
		allEvents = append(allEvents, events[i])
	}

	if wsEventListener == nil {
		log.Panic("wsEventListener is not set")
	}

	if eventMustAuth != nil {
		if authRequired := eventMustAuth(ctx, allEvents...); authRequired {
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

	if err := wsEventListener(ctx, allEvents...); err != nil {
		return errors.Wrap(err, "failed to store events")
	}

	if err := h.notifyListenersAboutNewEvents(ctx, allEvents...); err != nil {
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

func (h *handler) notifyListenersAboutNewEvents(ctx context.Context, events ...*model.Event) error {
	var broadcast = map[Writer][]nostr.EventEnvelope{}

	// Collect events for each subscription.
	h.connSubs.Range(func(writer Writer, conn connSubscriptions) bool {
		authData, _ := h.connAuth.Load(writer)
		conn.Subscriptions.Range(func(_ string, sub *model.Subscription) bool {
			var envelope = nostr.EventEnvelope{SubscriptionID: &sub.SubscriptionID}
			for _, event := range events {
				if !sub.Filters.Match(&event.Event) {
					continue
				} else if !canForwardEvent(event, authData.MasterPublicKey, authData.PublicKey) {
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

func (h *handler) handleCount(ctx context.Context, envelope *nostr.CountEnvelope) error {
	count, err := query.CountEvents(ctx, &model.Subscription{Filters: envelope.Filters})
	if err != nil {
		return errors.Wrap(err, "failed to count events")
	}

	envelope.Count = &count

	return nil
}

func prepareCommunityEventsForDeletion(ctx context.Context, incomingEvent *model.Event) (evs []*model.Event, err error) {
	var ids []string
	for _, eTag := range incomingEvent.Tags.GetAll([]string{"e"}) {
		if eTag.Key() == "e" {
			ids = append(ids, eTag.Value())
		}
	}
	res := make([]*model.Event, 0)
	var communityEventsToCheck []*model.Event
	for ev := range query.GetStoredEvents(ctx, &model.Subscription{Filters: model.Filters{nostr.Filter{IDs: ids}}}) {
		hTag := ev.GetTag("h")
		if hTag == nil {
			continue
		}
		communityEventsToCheck = append(communityEventsToCheck, ev)
		res = append(res, &model.Event{
			Event: nostr.Event{
				Kind: nostr.KindDeletion,
				ID:   ev.ID,
				Tags: model.Tags{
					{"k", fmt.Sprint(ev.Kind)},
					{"e", fmt.Sprint(ev.ID)},
					{"a", fmt.Sprintf("%v:%v:%v", ev.Kind, ev.PubKey, ev.Tags.GetD())},
				},
				PubKey: ev.PubKey,
			},
		})
	}
	for _, ev := range communityEventsToCheck {
		if err := validation.ValidateDeleteEvent(ctx, ev, incomingEvent); err != nil {
			return nil, errors.Wrap(err, "failed to validate delete event")
		}
	}

	return res, nil
}
