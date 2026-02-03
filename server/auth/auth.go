// SPDX-License-Identifier: ice License 1.0

package auth

import (
	"context"
	"net/url"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/validation"
)

var (
	ErrRelayNotAuthoritative        = errors.New("relay-is-not-authoritative: relay is not authoritative for the user")
	ErrRelayAuthoritative           = errors.New("relay-is-authoritative: relay is authoritative for the user")
	ErrAttestationRecordNotFound    = errors.New("attestation record not found")
	ErrAttestationRecordExpired     = errors.New("attestation record is expired")
	ErrAttestationRecordRevoked     = errors.New("attestation record is revoked")
	ErrAttestationRecordIsNotActive = errors.New("attestation record is not active yet")
)

func ValidateUserAttestation(ctx context.Context, e, attestationEvent *model.Event) (map[int]struct{}, error) {
	if attestationEvent == nil {
		return nil, errors.Wrap(ErrAttestationRecordNotFound, e.PubKey)
	}

	if err := validation.Validate(ctx, model.Events{attestationEvent}); err != nil {
		return nil, errors.Wrap(err, "failed to validate attestation event")
	}

	if attestationEvent.Kind != model.CustomIONKindAttestation {
		return nil, errors.Wrapf(ErrAttestationRecordNotFound, "attestation event has unexpected kind %d", attestationEvent.Kind)
	} else if owner := e.GetMasterPublicKey(); attestationEvent.PubKey != owner {
		return nil, errors.Wrapf(ErrAttestationRecordNotFound, "attestation event has unexpected author %q, expected %q", attestationEvent.PubKey, owner)
	}

	records, err := model.ParseAttestationTags(attestationEvent.Tags)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse attestation tags")
	}

	record, ok := records[e.PubKey]
	if !ok {
		return nil, errors.Wrap(ErrAttestationRecordNotFound, e.PubKey)
	}

	now := nostr.Now()
	if record.Revoked != nil && now.After(*record.Revoked) {
		return nil, errors.Wrap(ErrAttestationRecordRevoked, e.PubKey)
	} else if record.End != nil && now.After(*record.End) {
		return nil, errors.Wrap(ErrAttestationRecordExpired, e.PubKey)
	} else if record.Start != nil && now.Before(*record.Start) {
		return nil, errors.Wrap(ErrAttestationRecordIsNotActive, e.PubKey)
	}

	kinds := make(map[int]struct{}, len(record.Kinds))
	for _, kind := range record.Kinds {
		kinds[kind] = struct{}{}
	}

	return kinds, nil
}

func ValidateUserAccessAuthoritative(ctx context.Context, currentRelayURL string, e *model.Event) (map[int]struct{}, error) {
	owner := e.GetMasterPublicKey()

	relayTag := model.TagMap{}.Set("r", &currentRelayURL)
	if u, err := url.Parse(currentRelayURL); err == nil && u.Port() != "" {
		u.Host = u.Hostname()
		relayTag = relayTag.Append("r", model.PointerOf(u.String()))
	}

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
			Tags:    relayTag,
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
		return nil, errors.Wrapf(ErrRelayNotAuthoritative, "current relay %q not found in user's relay list", currentRelayURL)
	}

	return ValidateUserAttestation(ctx, e, attestationEvent)
}

func validateUserAccessNotAuthoritative(ctx context.Context, e, attestation *model.Event) (map[int]struct{}, error) {
	return ValidateUserAttestation(ctx, e, attestation)
}

func ValidateUserAccess(ctx context.Context, relayUrl string, e *model.Event) (allowedKinds map[int]struct{}, authoritative bool, err error) {
	attestation := e.GetTag("attestation").Value()
	if attestation == "" {
		// User request for authoritative relay.
		allowedKinds, err = ValidateUserAccessAuthoritative(ctx, relayUrl, e)
		return allowedKinds, err == nil, err
	}

	var attestationEvent model.Event
	if err := attestationEvent.UnmarshalJSON([]byte(attestation)); err != nil {
		return nil, false, errors.Wrap(err, "failed to unmarshal attestation event from tag")
	}

	allowedKinds, err = validateUserAccessNotAuthoritative(ctx, e, &attestationEvent)
	return allowedKinds, false, err
}
