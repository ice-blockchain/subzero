// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"github.com/cockroachdb/errors"

	"github.com/ice-blockchain/subzero/model"
)

var (
	attestationStateMap = map[string]map[string]struct{}{
		model.CustomIONAttestationKindActive: { // Active -> [Revoked, Inactive].
			model.CustomIONAttestationKindRevoked:  {},
			model.CustomIONAttestationKindInactive: {},
		},
		model.CustomIONAttestationKindInactive: {}, // Inactive -> [].
		model.CustomIONAttestationKindRevoked:  {}, // Revoked -> [].
	}

	ErrAttestationUnknownAction        = errors.New("unknown attestation action")
	ErrAttestationInvalidFormat        = errors.New("invalid attestation tag format")
	ErrAttestationInvalidTransition    = errors.New("invalid attestation state transition")
	ErrAttestationInvalidTemporalOrder = errors.New("invalid temporal ordering of attestations")
	ErrAttestationInvalidPubkey        = errors.New("invalid pubkey in attestation tag")
)

func validateAttestationEvent(v *eventValidator, e *model.Event) error {
	type attestationState struct {
		Name      string
		Timestamp model.Timestamp
	}
	keys := make(map[string]attestationState, len(e.Tags))
	for i, tag := range e.Tags {
		if tag.Key() != model.TagAttestationName {
			continue
		} else if len(tag) < 4 {
			return errors.Wrapf(ErrAttestationInvalidFormat, "attestation tag %v at index %d is too short", tag[1:], i+1)
		}

		pubkey := tag[model.TagAttestationValueIndexPubkey]
		if pubkey == "" {
			return errors.Wrapf(ErrAttestationInvalidPubkey, "empty pubkey in attestation tag %v", tag[1:])
		} else if e.PubKey == pubkey {
			return errors.Wrapf(ErrAttestationInvalidPubkey, "pubkey in attestation tag at index %d matches event pubkey %q", i+1, e.PubKey)
		}

		action, ts, _, err := model.ParseAttestationString(tag[model.TagAttestationValueIndexAction])
		if err != nil {
			return err
		} else if _, ok := attestationStateMap[action]; !ok {
			return errors.Wrap(ErrAttestationUnknownAction, action)
		}

		if prevState, ok := keys[pubkey]; ok {
			if _, ok := attestationStateMap[prevState.Name][action]; !ok {
				return errors.Wrapf(ErrAttestationInvalidTransition, "from %q to %q for pubkey %q", prevState.Name, action, pubkey)
			} else if prevState.Timestamp.After(ts) {
				// If the previous state is newer than the current one, we have a problem.
				return errors.Wrapf(ErrAttestationInvalidTemporalOrder, "from %q to %q for pubkey %q, previous timestamp %s is after current timestamp %s",
					prevState.Name, action, pubkey, prevState.Timestamp.Time(), ts.Time())
			}
		}
		keys[pubkey] = attestationState{action, ts} // First occurrence OR state transition is valid.
	}
	return nil
}
