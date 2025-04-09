// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"log"
	"math"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip42"
	"github.com/puzpuzpuz/xsync/v4"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/validation"
)

var (
	protectedEventKindsE2E = map[int]struct{}{
		nostr.KindGiftWrap: {},
	}

	communityProtectedEventKinds = map[int]struct{}{
		nostr.KindTextNote:                  {},
		nostr.KindArticle:                   {},
		nostr.KindDraftArticle:              {},
		model.CustomIONKindEditableTextNote: {},
		nostr.KindRepost:                    {},
		nostr.KindGenericRepost:             {},
	}

	errAttestationRecordNotFound    = errors.New("attestation record not found")
	errAttestationRecordExpired     = errors.New("attestation record is expired")
	errAttestationRecordRevoked     = errors.New("attestation record is revoked")
	errAttestationRecordIsNotActive = errors.New("attestation record is not active yet")

	errEventInvalidID   = errors.New("event id is invalid")
	errEventInvalidSign = errors.New("event signature is invalid")
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
	master, pk, _, kinds := model.GetUserDataFromContext(ctx)

	return canForwardCommunityEvent(ctx, in, master) && canForwardEvent(in, kinds, master, pk)
}

func canForwardEvent(in *model.Event, currentkinds map[int]struct{}, masterPubkey, deviceKey string) bool {
	if len(currentkinds) > 0 {
		if _, ok := currentkinds[in.Kind]; !ok {
			return false
		}
	}

	if _, ok := protectedEventKindsE2E[in.Kind]; !ok {
		return true
	}

	// E2E encrypted events cannot be decrypted with master key, so do not forward them.
	dest := [][]string{
		{"p", deviceKey},
		{"p", masterPubkey, "", deviceKey},
	}
	for _, pattern := range dest {
		for range in.Tags.All(pattern) {
			return true
		}
	}

	return false
}

func canForwardCommunityEvent(ctx context.Context, in *model.Event, masterPubkey string) bool {
	hTag := in.GetTag(model.CustomIONTagCommunity).Value()
	if hTag == "" {
		return true
	}
	if _, ok := communityProtectedEventKinds[in.Kind]; !ok {
		return true
	}
	communityDefinitionEvent, err := validation.GetCommunityDefinition(ctx, hTag)
	if err != nil {
		log.Printf("ERROR: failed to get community event: %v", err)

		return false
	}
	if communityDefinitionEvent.GetTag("private") != nil {
		if err := validation.IsUserPartOfCommunity(ctx, communityDefinitionEvent, masterPubkey); err != nil {
			return false
		}
		if err := validation.IsUserBanned(ctx, masterPubkey, hTag); err != nil {
			return false
		}
	}

	return true
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
	conn, _ := h.connSubs.LoadOrCompute(respWriter, func() (connSubscriptions, bool) {
		return connSubscriptions{
			Subscriptions: xsync.NewMap[string, *model.Subscription](),
		}, false
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

func validateOnBehalfAccess(ctx context.Context, e *model.Event) (map[int]struct{}, error) {
	it := query.GetStoredEvents(ctx, &model.Subscription{
		Filters: []model.Filter{
			{
				Kinds:   []int{model.CustomIONKindAttestation},
				Authors: []string{e.GetMasterPublicKey()},
				Tags:    model.TagMap{}.Set("p", &e.PubKey),
				Limit:   1,
			},
		},
	})
	var attestationEvent *model.Event
	for ev, err := range it {
		if err != nil {
			return nil, errors.Wrap(err, "failed to fetch attestation event")
		}
		attestationEvent = ev
	}
	if attestationEvent == nil {
		return nil, errAttestationRecordNotFound
	}

	records, err := model.ParseAttestationTags(attestationEvent.Tags)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse attestation tags")
	}

	record, ok := records[e.PubKey]
	if !ok {
		return nil, errAttestationRecordNotFound
	}

	now := time.Now()
	if record.Revoked != nil && now.After(*record.Revoked) {
		return nil, errAttestationRecordRevoked
	} else if record.End != nil && now.After(*record.End) {
		return nil, errAttestationRecordExpired
	} else if record.Start != nil && now.Before(*record.Start) {
		return nil, errAttestationRecordIsNotActive
	}

	kinds := make(map[int]struct{}, len(record.Kinds))
	for _, kind := range record.Kinds {
		kinds[kind] = struct{}{}
	}

	return kinds, nil
}

func (h *handler) handleAuth(ctx context.Context, respWriter Writer, e *model.Event) *nostr.OKEnvelope {
	var resp = nostr.OKEnvelope{EventID: e.Event.ID}

	state, ok := h.connAuth.Load(respWriter)
	if !ok {
		resp.Reason = "received unexpected auth message: no challenge was sent"

		return &resp
	} else if state.Authenticated && state.PublicKey != e.PubKey {
		resp.Reason = "received unexpected auth message: already authenticated with a different public key"

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

	var userdata connAuthData
	if !strings.Contains(h.relayURL, ".testnet.") {
		if e.PubKey != e.GetMasterPublicKey() {
			var err error
			if userdata.Kinds, err = validateOnBehalfAccess(ctx, e); err != nil {
				resp.Reason = "failed to validate on-behalf access: " + err.Error()

				return &resp
			}
		}
	}
	userdata.Challenge = state.Challenge
	userdata.MasterPublicKey = e.GetMasterPublicKey()
	userdata.PublicKey = e.PubKey
	userdata.Authenticated = true

	h.connAuth.Store(respWriter, userdata)

	resp.OK = true

	return &resp
}

func (h *handler) prepareSubscription(ctx context.Context, sub *model.Subscription) *model.Subscription {
	for i := range sub.Filters {
		if !(strings.Contains(sub.Filters[i].Search, model.ExtensionTextMRF) && sub.Filters[i].Tags.HasValues("p")) {
			continue
		}
		m, pk, authenticated, kinds := model.GetUserDataFromContext(ctx)
		sub.OneShot = true
		if !authenticated {
			// Should not happen, but just in case. Also set it to OneShot mode.
			continue
		} else if _, ok := kinds[nostr.KindFollowList]; len(kinds) > 0 && !ok {
			// Not allowed to access the requested data.
			continue
		}
		sub.Filters[i] = model.Filter{
			Kinds:   []int{nostr.KindFollowList},
			Authors: []string{m, pk},
			Search:  "include:dependencies:kind3>kind0+p+|" + strings.Join(sub.Filters[i].Tags.All("p"), ",") + "|",
			Limit:   1,
		}
		sub.Reduce = func(e *model.Event) bool {
			return e.Kind != nostr.KindProfileMetadata
		}
	}
	return sub
}

func (h *handler) handleReq(ctx context.Context, respWriter Writer, sub *model.Subscription) error {
	if reqMustAuth != nil {
		if authRequired := reqMustAuth(ctx, sub); authRequired {
			status, _ := h.connAuth.LoadOrCompute(respWriter, func() (connAuthData, bool) {
				return connAuthData{
					Challenge: generateChallenge(sub.SubscriptionID),
				}, false
			})
			if !status.Authenticated {
				return h.authRequiredReq(respWriter, sub, status.Challenge)
			} else if !status.IsFilterAllowed(sub.Filters...) {
				return h.writeResponse(respWriter, &nostr.ClosedEnvelope{
					SubscriptionID: sub.SubscriptionID,
					Reason:         "error: not allowed to access the requested data",
				})
			}
		}
	}
	if wsSubscriptionListeners != nil {
		sub = h.prepareSubscription(ctx, sub)
		fetchCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		for _, listener := range wsSubscriptionListeners {
			for event, err := range listener(fetchCtx, sub) {
				if err != nil {
					return errors.Wrapf(err, "failed to fetch events for subscription %+v", sub)
				} else if !canForwardEventContext(fetchCtx, event) {
					continue
				} else if sub.Reduce != nil && sub.Reduce(event) {
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
			status, _ := h.connAuth.LoadOrCompute(respWriter, func() (connAuthData, bool) {
				return connAuthData{
					Challenge: generateChallenge(),
				}, false
			})
			if !status.Authenticated {
				err := h.writeResponse(respWriter, &nostr.AuthEnvelope{
					Challenge: &status.Challenge,
				})
				if err != nil {
					return errors.Wrap(err, "failed to write AUTH message")
				}
				return errAuthRequired
			} else if !status.IsEventAllowed(events...) {
				return errors.New("error: not allowed to publish given events")
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
	if !evt.CheckID() {
		return errEventInvalidID
	}

	var ok bool
	if ok, err = evt.CheckSignature(); err != nil {
		return errors.Wrap(err, "signature check failed")
	} else if !ok {
		return errEventInvalidSign
	}
	if vErr := validation.Validate(ctx, evt); vErr != nil {
		return errors.Wrap(vErr, "wrong event parameters")
	}
	if cErr := evt.CheckNIP13Difficulty(cfg.NIP13MinLeadingZeroBits); cErr != nil {
		return errors.Wrap(cErr, "wrong event difficulty")
	}

	return nil
}

func filtersMatchWithMasterKey(filters model.Filters, ev *model.Event, masterPubKey, deviceKey string) bool {
	if filters.Match(&ev.Event) {
		return true
	}

	for _, filter := range filters {
		if !slices.Contains(filter.Authors, deviceKey) {
			continue
		}

		n := filter.Clone()
		n.Authors = nil
		if n.Tags == nil {
			n.Tags = model.TagMap{}
		}
		n.Tags.Set(model.CustomIONTagOnBehalfOf, &masterPubKey)
		if n.Matches(&ev.Event) {
			return true
		}
	}

	return false
}

func (h *handler) notifyListenersAboutNewEvents(ctx context.Context, events ...*model.Event) error {
	var broadcast = map[Writer][]nostr.EventEnvelope{}

	// Collect events for each subscription.
	h.connSubs.Range(func(writer Writer, conn connSubscriptions) bool {
		authData, _ := h.connAuth.Load(writer)
		conn.Subscriptions.Range(func(_ string, sub *model.Subscription) bool {
			var envelope = nostr.EventEnvelope{SubscriptionID: &sub.SubscriptionID}
			for _, event := range events {
				if !filtersMatchWithMasterKey(sub.Filters, event, authData.MasterPublicKey, authData.PublicKey) {
					continue
				} else if !canForwardEvent(event, authData.Kinds, authData.MasterPublicKey, authData.PublicKey) ||
					!canForwardCommunityEvent(ctx, event, authData.MasterPublicKey) {
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
