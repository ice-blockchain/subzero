// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"math"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"
	"github.com/rs/zerolog/log"

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
		log.Error().Str("context", "WEBSOCKET").Err(err).Msg("failed to get community event")

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
		log.Warn().Str("context", "WEBSOCKET").Str("subscription_id", sub.ID).Msg("subscription already exists, overwriting it")
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

func (h *handler) prepareSubscription(ctx context.Context, sub *model.Subscription) *model.Subscription {
	for i := range sub.Filters {
		if !(strings.Contains(sub.Filters[i].Search, model.ExtensionTextMRF) && sub.Filters[i].Tags.HasValues("p")) {
			continue
		}
		data := model.GetUserDataFromContext(ctx)
		sub.OneShot = true
		if !data.Authenticated {
			// Should not happen, but just in case. Also set it to OneShot mode.
			continue
		} else if !data.IsKindAllowed(nostr.KindFollowList) {
			// Not allowed to access the requested data.
			continue
		}
		sub.Filters[i] = model.Filter{
			Kinds:   []int{nostr.KindFollowList},
			Authors: []string{data.MasterPublicKey, data.PublicKey},
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
		data := model.GetUserDataFromContext(ctx)
		if isValidGiftWrapFilter(filters[idx], data.MasterPublicKey, data.PublicKey) {
			now := time.Now()
			err := h.streamGiftWrapEvents(ctx, respWriter, sub, filters[idx])
			if err != nil {
				return errors.Wrap(err, "failed to stream gift wrap events")
			} else {
				h.logOperation(respWriter, time.Since(now), "gift wrap: streamed events for subscription %s", sub.ID)
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
		now := time.Now()
		var n int
		itNow := now
		var totalGet, totalSend time.Duration
		for event, err := range getter(getterCtx, filters...) {
			if err != nil {
				return errors.Wrapf(err, "getter %d: failed to fetch events for subscription %+v", i, sub)
			}
			n++
			totalGet += time.Since(itNow)
			if sub.Reduce(event) {
				continue
			}
			itNow = time.Now()
			err := h.writeResponse(ctx, respWriter,
				&nostr.EventEnvelope{
					SubscriptionID: &sub.ID,
					Events:         []*nostr.Event{&event.Event},
				})
			if err != nil {
				return errors.Wrapf(err, "failed to write event[%s]", event.String())
			}
			totalSend += time.Since(itNow)
		}
		h.logOperation(respWriter, time.Since(now), "req: getter %d: streamed [%d] events for subscription %s, get %v, send %v", i, n, sub.ID, totalGet, totalSend)
		itNow = time.Now()
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

	log.Info().Str("context", "WEBSOCKET").
		Str("subscription_id", sub.ID).
		Int("buffered_events", len(bufferedEvents)).
		Msg("subscription has buffered events")
	now := time.Now()
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
	h.logOperation(respWriter, time.Since(now), "events: send [%d] buffered events", len(bufferedEvents))

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
		log.Warn().Msg("registerWSSubscriptionListener not registered, ignoring query part")
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
	if err := validation.Validate(ctx, model.Events(events)); err != nil {
		if errors.Is(err, validation.ErrEphemeralForbidden) {
			return errRelayAuthoritative
		}
		return errors.Wrapf(err, "event validation failed: %s", model.Events(events).String())
	}

	if wsEventListener == nil {
		log.Fatal().Msg("wsEventListener is not set")
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
		if errors.Is(err, query.ErrReadOnly) {
			return errRelayReadOnly
		} else if errors.Is(err, query.ErrRaceCondition) {
			return errDuplicate
		}
		return errors.Wrapf(err, "failed to handle events: %s", model.Events(events).String())
	}

	return nil
}

func canForwardLiveEvent(ctx context.Context, filters model.Filters, in *model.Event, data *model.UserDataContext) bool {
	return model.FiltersMatch(filters, in, data.MasterPublicKey, data.PublicKey) &&
		canForwardEvent(in, data.Kinds, data.MasterPublicKey, data.PublicKey) &&
		canForwardCommunityEvent(ctx, in, data.MasterPublicKey)
}

func (h *handler) BroadcastNewEvents(ctx context.Context, events ...*model.Event) (numberOfSubscriptions int) {
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
					log.Warn().Str("context", "WEBSOCKET").
						Err(err).
						Str("event_id", event.ID).
						Str("subscription_id", sub.Source.ID).
						Msg("failed to write event to subscription")
				}
			} else {
				sub.Source.Push(event)
			}
			numberOfSubscriptions++
		}
		return true
	})
	return numberOfSubscriptions
}

func (h *handler) handleCount(ctx context.Context, envelope *nostr.CountEnvelope) error {
	count, err := query.CountEvents(ctx, envelope.Filters...)
	if err != nil {
		return errors.Wrap(err, "failed to count events")
	}

	envelope.Count = &count

	return nil
}
