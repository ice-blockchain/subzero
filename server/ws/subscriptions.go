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
	"time"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip42"

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

func (h *handler) authRequiredReq(ctx context.Context, respWriter Writer, sub *model.Subscription, challenge string) error {
	err := h.writeResponse(ctx, respWriter, &nostr.AuthEnvelope{
		Challenge: &challenge,
	})
	if err != nil {
		return errors.Wrap(err, "failed to write AUTH message")
	}

	err = h.closeSubscriptionWithReason(ctx, respWriter, sub, errAuthRequired.Error())

	return errors.Wrap(err, "failed to write CLOSED message")
}

func (h *handler) linkSubscription(respWriter Writer, sub *model.Subscription) {
	if sub.OneShot {
		// OneShot subscriptions are not stored, they are processed immediately.
		return
	}

	_, loaded := h.Subscriptions.LoadAndStore(sub.ID, subscription{
		Source: sub,
		Writer: respWriter,
	})
	if loaded {
		log.Printf("WARN: subscription %s already exists, overwriting it", sub.ID)
	}
}

func (h *handler) unlinkSubscription(respWriter Writer, ID *string) bool {
	if ID == nil {
		// Connection is closing, remove all subscriptions.
		h.Subscriptions.Range(func(_ string, sub subscription) bool {
			if sub.Writer == respWriter {
				h.Subscriptions.Delete(sub.Source.ID)
			}
			return true
		})
		return false
	}

	_, ok := h.Subscriptions.LoadAndDelete(*ID)

	return ok
}

func validateOnBehalfAccess(ctx context.Context, e *model.Event) (map[int]struct{}, error) {
	var attestationEvent *model.Event

	owner := e.GetMasterPublicKey()
	if val := e.GetTag("attestation").Value(); val != "" {
		var ev model.Event

		if err := ev.UnmarshalJSON([]byte(val)); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal attestation event from tag")
		} else if err := validation.Validate(ctx, &ev); err != nil {
			return nil, errors.Wrap(err, "failed to validate attestation event")
		} else if ev.Kind != model.CustomIONKindAttestation {
			return nil, errors.Wrapf(errAttestationRecordNotFound, "attestation event has unexpected kind %d", ev.Kind)
		} else if ev.PubKey != owner {
			return nil, errors.Wrapf(errAttestationRecordNotFound, "attestation event has unexpected author %q, expected %q", ev.PubKey, owner)
		}
		attestationEvent = &ev
	} else {
		it := query.GetStoredEvents(ctx, model.Filter{
			Kinds:   []int{model.CustomIONKindAttestation},
			Authors: []string{owner},
			Tags:    model.TagMap{}.Set("p", &e.PubKey),
			Limit:   1,
		})
		for ev, err := range it {
			if err != nil {
				return nil, errors.Wrap(err, "failed to fetch attestation event")
			}
			attestationEvent = ev
		}
	}

	if attestationEvent == nil {
		return nil, errors.Wrap(errAttestationRecordNotFound, e.PubKey)
	}

	records, err := model.ParseAttestationTags(attestationEvent.Tags)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse attestation tags")
	}

	record, ok := records[e.PubKey]
	if !ok {
		return nil, errors.Wrap(errAttestationRecordNotFound, e.PubKey)
	}

	now := nostr.Now()
	if record.Revoked != nil && now.After(*record.Revoked) {
		return nil, errors.Wrap(errAttestationRecordRevoked, e.PubKey)
	} else if record.End != nil && now.After(*record.End) {
		return nil, errors.Wrap(errAttestationRecordExpired, e.PubKey)
	} else if record.Start != nil && now.Before(*record.Start) {
		return nil, errors.Wrap(errAttestationRecordIsNotActive, e.PubKey)
	}

	kinds := make(map[int]struct{}, len(record.Kinds))
	for _, kind := range record.Kinds {
		kinds[kind] = struct{}{}
	}

	return kinds, nil
}

func (h *handler) handleAuth(ctx context.Context, respWriter Writer, e *model.Event) *nostr.OKEnvelope {
	var resp = nostr.OKEnvelope{EventID: e.Event.ID}

	state, ok := h.ConnAuth.Load(respWriter)
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
		h.RelayURL,
		nip42.WithCustomVerificator(func(nostrEvent *nostr.Event) (bool, error) {
			return (&model.Event{Event: *nostrEvent}).CheckSignature()
		}))
	if err != nil {
		resp.Reason = "failed to validate auth event: " + err.Error()

		return &resp
	}

	var userdata connAuthData
	if e.PubKey != e.GetMasterPublicKey() {
		var err error
		if userdata.Kinds, err = validateOnBehalfAccess(ctx, e); err != nil {
			resp.Reason = "failed to validate on-behalf access: " + err.Error()

			return &resp
		}
	}
	userdata.Challenge = state.Challenge
	userdata.MasterPublicKey = e.GetMasterPublicKey()
	userdata.PublicKey = e.PubKey
	userdata.Authenticated = true

	h.ConnAuth.Store(respWriter, userdata)

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
		sub.WithReduce(func(e *model.Event) bool {
			return e.Kind != nostr.KindProfileMetadata
		})
	}
	return sub
}

func (h *handler) streamGiftWrapEvents(ctx context.Context, respWriter Writer, sub *model.Subscription, giftWrapFilter model.Filter) error {
	giftWrapFilter.Limit = 1000

	for ctx.Err() == nil {
		var oldestTimestamp model.Timestamp
		var eventCount int

		for event, err := range query.GetStoredEvents(ctx, giftWrapFilter) {
			if err != nil {
				return errors.Wrap(err, "failed to fetch events")
			}

			eventCount++
			oldestTimestamp = event.CreatedAt

			if sub.Reduce(event) {
				continue
			}

			err := h.writeResponse(ctx, respWriter,
				&nostr.EventEnvelope{
					SubscriptionID: &sub.ID,
					Events:         []*nostr.Event{&event.Event},
				})
			if err != nil {
				return errors.Wrapf(err, "failed to write event[%s]", event.String())
			}
		}

		if eventCount < giftWrapFilter.Limit {
			// No more events to process.
			break
		}

		giftWrapFilter.Until = &oldestTimestamp

		if giftWrapFilter.Since != nil && giftWrapFilter.Since.After(*giftWrapFilter.Until) {
			// Reached the end of the subscription time range.
			break
		}
	}
	return ctx.Err()
}

func getGiftWrapFilterIndex(filters model.Filters) int {
	return slices.IndexFunc(filters,
		func(filter model.Filter) bool {
			return len(filter.Kinds) == 1 &&
				filter.Kinds[0] == nostr.KindGiftWrap &&
				len(filter.Tags) == 1 &&
				len(filter.Tags["p"]) == 1 &&
				len(filter.Tags["p"][0]) == 3 // [master, '', device].
		})
}

func isValidGiftWrapFilter(filter model.Filter, master, device string) bool {
	expectedTags := model.TagValues{&master, model.PointerOf(""), &device}
	return slices.CompareFunc(filter.Tags["p"][0], expectedTags, compareStringPointers) == 0
}

func compareStringPointers(a, b *string) int {
	if a == nil && b == nil {
		return 0
	} else if a == nil {
		return -1
	} else if b == nil {
		return 1
	}
	return strings.Compare(*a, *b)
}

func (h *handler) streamEvents(ctx context.Context, respWriter Writer, sub *model.Subscription) error {
	applySubscriptionLimit(sub)

	filters := sub.Filters

	// Special case for global gift wrap subscription.
	// { "kinds":[1059], "#p": [[loggedinMasterKey, '', loggedinDevicekey]] }.
	if idx := getGiftWrapFilterIndex(filters); idx >= 0 {
		master, device, _, _ := model.GetUserDataFromContext(ctx)
		if isValidGiftWrapFilter(filters[idx], master, device) {
			err := h.streamGiftWrapEvents(ctx, respWriter, sub, filters[idx])
			if err != nil {
				return errors.Wrap(err, "failed to stream gift wrap events")
			} else {
				if len(filters) == 1 {
					return nil // No other filters to process.
				}

				// Remove the gift wrap filter from the initial request, but keep it for later use.
				filtersCopy := make(model.Filters, len(filters)-1)
				copy(filtersCopy, filters[:idx])
				copy(filtersCopy[idx:], filters[idx+1:])
				filters = filtersCopy
			}
		}
	}

	getterCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	for i, getter := range wsSubscriptionListeners {
		for event, err := range getter(getterCtx, filters...) {
			if err != nil {
				return errors.Wrapf(err, "getter %d: failed to fetch events for subscription %+v", i, sub)
			}

			if sub.Reduce(event) {
				continue
			}

			err := h.writeResponse(ctx, respWriter,
				&nostr.EventEnvelope{
					SubscriptionID: &sub.ID,
					Events:         []*nostr.Event{&event.Event},
				})
			if err != nil {
				return errors.Wrapf(err, "failed to write event[%s]", event.String())
			}
		}
	}

	return nil
}

func (h *handler) closeSubscriptionWithReason(ctx context.Context, respWriter Writer, sub *model.Subscription, reason string) error {
	h.unlinkSubscription(respWriter, &sub.ID)

	return h.writeResponse(ctx, respWriter, &nostr.ClosedEnvelope{
		SubscriptionID: sub.ID,
		Reason:         reason,
	})
}

func (h *handler) streamEventsBuffered(ctx context.Context, respWriter Writer, sub *model.Subscription) error {
	bufferedEvents := sub.GetPending()

	if len(bufferedEvents) == 0 {
		// No buffered events to send.
		return nil
	}

	log.Printf("INFO: subscription %s has %d buffered events", sub.ID, len(bufferedEvents))
	for i := range bufferedEvents {
		err := h.writeResponse(
			ctx,
			respWriter,
			&nostr.EventEnvelope{
				SubscriptionID: &sub.ID,
				Events:         []*nostr.Event{&bufferedEvents[i].Event},
			},
		)
		if err != nil {
			return err
		}
	}

	return nil
}

func (h *handler) handleReq(ctx context.Context, respWriter Writer, sub *model.Subscription) (err error) {
	if reqMustAuth != nil {
		if authRequired := reqMustAuth(ctx, sub); authRequired {
			status, _ := h.ConnAuth.LoadOrCompute(respWriter, func() (connAuthData, bool) {
				return connAuthData{
					Challenge: generateChallenge(sub.ID),
				}, false
			})
			if !status.Authenticated {
				return h.authRequiredReq(ctx, respWriter, sub, status.Challenge)
			} else if !status.IsFilterAllowed(sub.Filters...) {
				return h.closeSubscriptionWithReason(ctx, respWriter, sub,
					"error: not allowed to access the requested data")
			}
		}
	}

	sub = h.prepareSubscription(ctx, sub)
	h.linkSubscription(respWriter, sub)

	defer func() {
		if err != nil && !sub.OneShot {
			h.unlinkSubscription(respWriter, &sub.ID)
		}
	}()

	if wsSubscriptionListeners != nil {
		err = h.streamEvents(ctx, respWriter, sub)
	} else {
		log.Printf("WARN: RegisterWSSubscriptionListener not registered, ignoring query part")
	}

	if err != nil {
		return errors.Join(err, h.closeSubscriptionWithReason(ctx, respWriter, sub, err.Error()))
	}

	err = h.writeResponse(ctx, respWriter, model.PointerOf(nostr.EOSEEnvelope(sub.ID)))
	if err != nil {
		return errors.Wrap(err, "failed to write EOS message")
	}

	if sub.OneShot {
		return h.closeSubscriptionWithReason(ctx, respWriter, sub, "processed: single request only subscription")
	}

	sub.SetLive()

	return errors.Wrap(h.streamEventsBuffered(ctx, respWriter, sub), "failed to stream buffered events")
}

func (h *handler) handleEvents(ctx context.Context, respWriter Writer, events []*model.Event) error {
	if err := validation.Validate(ctx, events...); err != nil {
		return errors.Wrapf(err, "events %v: invalid", events)
	}

	if wsEventListener == nil {
		log.Panic("wsEventListener is not set")
	}

	if eventMustAuth != nil {
		if authRequired := eventMustAuth(ctx, events...); authRequired {
			status, _ := h.ConnAuth.LoadOrCompute(respWriter, func() (connAuthData, bool) {
				return connAuthData{
					Challenge: generateChallenge(),
				}, false
			})
			if !status.Authenticated {
				err := h.writeResponse(ctx, respWriter, &nostr.AuthEnvelope{
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
		return errors.Wrapf(err, "failed to handle events: %s", model.Events(events).String())
	}

	return nil
}

func canForwardLiveEvent(ctx context.Context, filters model.Filters, in *model.Event, data *model.UserDataContext) bool {
	return model.FiltersMatch(filters, in, data.MasterPublicKey, data.PublicKey) &&
		canForwardEvent(in, data.Kinds, data.MasterPublicKey, data.PublicKey) &&
		canForwardCommunityEvent(ctx, in, data.MasterPublicKey)
}

func (h *handler) BroadcastNewEvents(ctx context.Context, events ...*model.Event) {
	h.Subscriptions.Range(func(_ string, sub subscription) bool {
		authData, _ := h.ConnAuth.Load(sub.Writer)
		for _, event := range events {
			if !canForwardLiveEvent(ctx, sub.Source.Filters, event, &authData.UserDataContext) {
				continue
			}

			if sub.Source.IsLive() {
				err := h.writeResponse(ctx, sub.Writer, &nostr.EventEnvelope{
					Events:         []*nostr.Event{&event.Event},
					SubscriptionID: &sub.Source.ID,
				})
				if err != nil {
					log.Printf("WARN: failed to write event %s to subscription %s: %v", event.ID, sub.Source.ID, err)
				}
			} else {
				sub.Source.Push(event)
			}
		}
		return true
	})
}

func (h *handler) handleCount(ctx context.Context, envelope *nostr.CountEnvelope) error {
	count, err := query.CountEvents(ctx, envelope.Filters...)
	if err != nil {
		return errors.Wrap(err, "failed to count events")
	}

	envelope.Count = &count

	return nil
}
