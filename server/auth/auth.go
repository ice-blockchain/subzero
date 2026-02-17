// SPDX-License-Identifier: ice License 1.0

package auth

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/validation"
)

var (
	ErrRelayNotAuthoritative = errors.New("relay-is-not-authoritative: relay is not authoritative for the user")
	ErrRelayAuthoritative    = errors.New("relay-is-authoritative: relay is authoritative for the user")
)

const (
	tagNameAttestation       = "attestation"
	tagNameRelayListMetadata = "relay-list-metadata"
)

func kindsToMap(kinds []int) map[int]struct{} {
	kindsMap := make(map[int]struct{}, len(kinds))
	for _, kind := range kinds {
		kindsMap[kind] = struct{}{}
	}
	return kindsMap
}

// ValidateUserAttestation validates the *provided* attestation event for the given user event and returns the allowed kinds if the attestation is valid.
func ValidateUserAttestation(ctx context.Context, e, attestationEvent *model.Event) (map[int]struct{}, error) {
	if attestationEvent.Kind != model.CustomIONKindAttestation {
		return nil, errors.Errorf("attestation event has unexpected kind %d", attestationEvent.Kind)
	} else if owner := e.GetMasterPublicKey(); attestationEvent.PubKey != owner {
		return nil, errors.Errorf("attestation event has unexpected author %q, expected %q", attestationEvent.PubKey, owner)
	}

	if err := validation.Validate(ctx, model.Events{attestationEvent}); err != nil {
		return nil, errors.Wrap(err, "failed to validate attestation event")
	}

	records, err := model.ParseAttestationTags(attestationEvent.Tags)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse attestation tags")
	}

	allowed, err := records.IsAccessAllowed(e.PubKey, -1, nostr.Now())
	if err != nil {
		return nil, err
	} else if !allowed {
		return nil, errors.Errorf("access not allowed for pubkey %q and kind %d", e.PubKey, e.Kind)
	}

	return kindsToMap(records.AllowedKinds(e.PubKey)), nil
}

func ValidateUserAccessAuthoritative(ctx context.Context, currentRelayURL string, e *model.Event) (map[int]struct{}, error) {
	authoritative, kinds, err := validation.IsRelayAuthoritativeForUser(ctx, currentRelayURL, e.GetMasterPublicKey(), e.PubKey)
	if err != nil {
		return nil, err
	}

	if !authoritative {
		return nil, errors.Wrapf(ErrRelayNotAuthoritative, "current relay %q not found in user's relay list", currentRelayURL)
	}

	return kindsToMap(kinds), nil
}

func validateUserAccessFromTags(ctx context.Context, relayUrl string, e, attestation, relayMetadata *model.Event) (allowedKinds map[int]struct{}, authoritative bool, err error) {
	if e.GetMasterPublicKey() != e.PubKey {
		allowedKinds, err = ValidateUserAttestation(ctx, e, attestation)
		if err != nil {
			return nil, false, err
		}
	}

	if relayMetadata == nil {
		// No relay metadata provided, we assume that the relay is not authoritative.
		return allowedKinds, false, nil
	}

	if relayMetadata.Kind != nostr.KindRelayListMetadata {
		return nil, false, errors.Errorf("relay metadata event has unexpected kind %d", relayMetadata.Kind)
	} else if relayMetadata.GetMasterPublicKey() != e.GetMasterPublicKey() {
		return nil, false, errors.Errorf("relay metadata event has unexpected author %q, expected %q", relayMetadata.GetMasterPublicKey(), attestation.PubKey)
	}

	err = validation.Validate(ctx, model.Events{relayMetadata})
	if err != nil {
		return nil, false, errors.Wrap(err, "failed to validate relay metadata event")
	}

	for _, tag := range relayMetadata.Tags {
		var userRelayURL string

		if tag.Key() == "r" {
			userRelayURL = tag.Value()
		}

		if userRelayURL == "" {
			continue
		}

		if model.CompareRelaysURLs(userRelayURL, relayUrl) {
			authoritative = true
			break
		}
	}

	return allowedKinds, authoritative, nil
}

func ValidateUserAccess(ctx context.Context, relayUrl string, e *model.Event) (allowedKinds map[int]struct{}, authoritative bool, err error) {
	attestation := e.GetTag(tagNameAttestation).Value()
	if attestation == "" {
		// User request for authoritative relay.
		allowedKinds, err = ValidateUserAccessAuthoritative(ctx, relayUrl, e)
		return allowedKinds, err == nil, err
	}

	var attestationEvent model.Event
	if err := attestationEvent.UnmarshalJSON([]byte(attestation)); err != nil {
		return nil, false, errors.Wrap(err, "failed to unmarshal attestation event from tag")
	}

	var relayMetadataEvent *model.Event
	if relayMetadataValue := e.GetTag(tagNameRelayListMetadata).Value(); relayMetadataValue != "" {
		var ev model.Event
		if err := ev.UnmarshalJSON([]byte(relayMetadataValue)); err != nil {
			return nil, false, errors.Wrap(err, "failed to unmarshal relay list metadata event from tag")
		}
		relayMetadataEvent = &ev
	}

	return validateUserAccessFromTags(ctx, relayUrl, e, &attestationEvent, relayMetadataEvent)
}
