// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip42"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/validation"
)

func validateUserAttestation(ctx context.Context, e, attestationEvent *model.Event) (map[int]struct{}, error) {
	if attestationEvent == nil {
		return nil, errors.Wrap(errAttestationRecordNotFound, e.PubKey)
	}

	if err := validation.Validate(ctx, attestationEvent); err != nil {
		return nil, errors.Wrap(err, "failed to validate attestation event")
	}

	if attestationEvent.Kind != model.CustomIONKindAttestation {
		return nil, errors.Wrapf(errAttestationRecordNotFound, "attestation event has unexpected kind %d", attestationEvent.Kind)
	} else if owner := e.GetMasterPublicKey(); attestationEvent.PubKey != owner {
		return nil, errors.Wrapf(errAttestationRecordNotFound, "attestation event has unexpected author %q, expected %q", attestationEvent.PubKey, owner)
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

func validateUserAccessAuthoritative(ctx context.Context, currentRelayURL string, e *model.Event) (map[int]struct{}, error) {
	owner := e.GetMasterPublicKey()
	it := query.GetStoredEvents(ctx,
		model.Filter{
			Kinds:   []int{model.CustomIONKindAttestation},
			Authors: []string{owner},
			Tags:    model.TagMap{}.Set("p", &e.PubKey),
			Limit:   1,
		},
		model.Filter{
			Kinds:   []int{nostr.KindRelayListMetadata},
			Authors: []string{owner},
			Tags:    model.TagMap{}.Set("r", &currentRelayURL),
			Limit:   1,
		},
	)

	var attestationEvent, relayListEvent *model.Event
	for ev, err := range it {
		if err != nil {
			return nil, errors.Wrap(err, "failed to fetch user events")
		}
		switch ev.Kind {
		case model.CustomIONKindAttestation:
			attestationEvent = ev
		case nostr.KindRelayListMetadata:
			relayListEvent = ev
		}
	}

	if relayListEvent == nil {
		return nil, errors.Wrapf(errRelayNotAuthoritative, "current relay %q not found in user's relay list", currentRelayURL)
	}

	return validateUserAttestation(ctx, e, attestationEvent)
}

func validateUserAccessNotAuthoritative(ctx context.Context, e, attestation *model.Event) (map[int]struct{}, error) {
	return validateUserAttestation(ctx, e, attestation)
}

func (h *handler) validateUserAccess(ctx context.Context, e *model.Event) (allowedKinds map[int]struct{}, err error) {
	attestation := e.GetTag("attestation").Value()
	if attestation == "" {
		// User request for authoritative relay.
		return validateUserAccessAuthoritative(ctx, h.RelayURL, e)
	}

	var attestationEvent model.Event
	if err := attestationEvent.UnmarshalJSON([]byte(attestation)); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal attestation event from tag")
	}

	return validateUserAccessNotAuthoritative(ctx, e, &attestationEvent)
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
		if userdata.Kinds, err = h.validateUserAccess(ctx, e); err != nil {
			if errors.IsAny(err, errAttestationRecordNotFound, errRelayNotAuthoritative) {
				resp.Reason = errRelayNotAuthoritative.Error()
			} else {
				resp.Reason = "failed to validate on-behalf access: " + err.Error()
			}

			return &resp
		}
	}
	userdata.Challenge = state.Challenge
	userdata.MasterPublicKey = e.GetMasterPublicKey()
	userdata.PublicKey = e.PubKey
	userdata.UserAgent = e.GetTag("user-agent").Value()
	userdata.Authenticated = true

	h.ConnAuth.Store(respWriter, userdata)

	resp.OK = true

	return &resp
}
