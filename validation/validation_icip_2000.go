// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/model"
)

var (
	attestationStateMap = map[string]map[string]struct{}{
		model.CustomIONAttestationKindActive: { // Active -> [Revoked, Inactive].
			model.CustomIONAttestationKindRevoked:  {},
			model.CustomIONAttestationKindInactive: {},
		},
		model.CustomIONAttestationKindInactive: { // Inactive -> [Active, Revoked].
			model.CustomIONAttestationKindActive:  {},
			model.CustomIONAttestationKindRevoked: {},
		},
		model.CustomIONAttestationKindRevoked: {}, // Revoked -> [].
	}

	ErrAttestationUnknownAction        = errors.New("unknown attestation action")
	ErrAttestationInvalidFormat        = errors.New("invalid attestation tag format")
	ErrAttestationInvalidTransition    = errors.New("invalid attestation state transition")
	ErrAttestationInvalidTemporalOrder = errors.New("invalid temporal ordering of attestations")
	ErrAttestationInvalidPubkey        = errors.New("invalid pubkey in attestation tag")
	ErrDeviceIdentificationProofFailed = errors.New("device identification proof failed")
)

const (
	deviceIdentificationProof = "device_identification_proof"
)

func validateAttestationEvent(_ *eventValidator, e *model.Event) error {
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

func (ev *eventValidator) validateKindAttestationEvent(ctx context.Context, rules *ruleSet, batch model.Events, e *model.Event) (err error) {
	if err = validateAttestationEvent(ev, e); err != nil {
		return errors.Wrap(err, "failed to validate attestation event")
	}
	return ev.validateDevices(ctx, rules, batch, e)
}

func (ev *eventValidator) validateDevices(ctx context.Context, rules *ruleSet, batch model.Events, newAttestationEvent *model.Event) (err error) {
	if rules.SkipKindAttestationProofDevicesVerify {
		return nil
	}
	oldAttestation, oldErr := ev.getEvent(ctx, fmt.Sprintf("%v:%v:", model.CustomIONKindAttestation, newAttestationEvent.GetMasterPublicKey()))
	if oldErr != nil {
		return errors.Wrapf(err, "[device-identification-proof] failed to get old attestation event")
	}
	for i, tag := range newAttestationEvent.Tags {
		if tag.Key() != model.TagAttestationName {
			continue
		}
		pubkey := tag[model.TagAttestationValueIndexPubkey]
		newDevice := true
		if oldAttestation != nil {
			for _, oldTag := range oldAttestation.Tags {
				if oldTag.Key() != model.TagAttestationName {
					continue
				}
				if oldTag[model.TagAttestationValueIndexPubkey] == pubkey {
					newDevice = false
					break
				}
			}
		}
		if newDevice {
			if deviceErr := ev.checkDeviceIdentificationProofs(ctx, batch, newAttestationEvent.GetMasterPublicKey(), pubkey); deviceErr != nil {
				err = errors.Join(err, errors.Wrapf(deviceErr, "failed to validate device identification proofs for pubkey %q at index %d", pubkey, i))
			}
		}
	}
	return err
}

func (ev *eventValidator) checkDeviceIdentificationProofs(_ context.Context, batch model.Events, userMasterKey, devicePubkey string) error {
	badgeDefinitionIndex := slices.IndexFunc(batch, func(e *model.Event) bool {
		return e.Kind == nostr.KindBadgeDefinition
	})
	badgeAwardIndex := slices.IndexFunc(batch, func(e *model.Event) bool {
		return e.Kind == nostr.KindBadgeAward
	})
	if badgeDefinitionIndex == -1 || badgeAwardIndex == -1 {
		return errors.Wrapf(ErrDeviceIdentificationProofFailed, "[device-identification-proof] missing badge definition or award events for device identification %s", devicePubkey)
	}
	badgeDefinition := batch[badgeDefinitionIndex]
	issuer, badgeDefinitionTargetPubkey := extractPubkeyFromDeviceIdentificationProof(badgeDefinition)
	if badgeDefinitionTargetPubkey == "" {
		return errors.Wrapf(ErrDeviceIdentificationProofFailed, "[device-identification-proof] badge definition does not have device pubkey")
	}
	if badgeDefinitionTargetPubkey != devicePubkey {
		return errors.Wrapf(ErrDeviceIdentificationProofFailed, "[device-identification-proof] device pubkey in badge definition (%s) doesn't match actual pubkey (%s)", badgeDefinitionTargetPubkey, devicePubkey)
	}
	if issuer != badgeDefinition.PubKey {
		return errors.Wrapf(ErrDeviceIdentificationProofFailed, "[device-identification-proof] issuer pubkey in badge definition (%s) doesn't match event sign key", issuer, badgeDefinition.GetMasterPublicKey())
	}
	if !slices.Contains(ev.IONIdentityPublicKeys(), issuer) {
		return errors.Wrapf(ErrDeviceIdentificationProofFailed, "[device-identification-proof] issuer of badge definition (%s) is not a ion identity key", issuer)
	}
	badgeAward := batch[badgeAwardIndex]
	awardIssuer, badgeAwardTargetPubkey := extractPubkeyFromDeviceIdentificationProof(badgeAward)
	if badgeAwardTargetPubkey == "" {
		return errors.Wrapf(ErrDeviceIdentificationProofFailed, "[device-identification-proof] badge award does not have device pubkey")
	}
	if badgeAwardTargetPubkey != devicePubkey {
		return errors.Wrapf(ErrDeviceIdentificationProofFailed, "[device-identification-proof] device publey in badge award (%s) doesn't match actual pubkey (%s)", badgeDefinitionTargetPubkey, devicePubkey)
	}
	pTag := badgeAward.GetTag("p")
	if pTag == nil || pTag.Value() == "" {
		return errors.Wrapf(ErrDeviceIdentificationProofFailed, "[device-identification-proof] no p tag in badge award for device identification")
	}
	if pTag.Value() != devicePubkey {
		return errors.Wrapf(ErrDeviceIdentificationProofFailed, "[device-identification-proof] p tag in badge award do not point to device pubkey")
	}
	if pTag.Value() == userMasterKey {
		return errors.Wrapf(ErrDeviceIdentificationProofFailed, "[device-identification-proof] p tag in badge award points to user master key, not the device")
	}
	if awardIssuer != badgeAward.PubKey {
		return errors.Wrapf(ErrDeviceIdentificationProofFailed, "[device-identification-proof] issuer pubkey in badge award (%s) doesn't match event sign key", issuer, badgeAward.GetMasterPublicKey())
	}
	if awardIssuer != issuer {
		return errors.Wrapf(ErrDeviceIdentificationProofFailed, "[device-identification-proof] issuer pubkey in badge award (%s) doesn't match issuer pubkey in badge definition (%s)", issuer, awardIssuer)
	}
	aTag := badgeAward.GetTag("a")
	if aTag == nil || aTag.Value() == "" {
		return errors.Wrapf(ErrDeviceIdentificationProofFailed, "[device-identification-proof] no a tag in badge award for device identification")
	}
	if aTag.Value() != badgeDefinition.Address() {
		return errors.Wrapf(ErrDeviceIdentificationProofFailed, "[device-identification-proof] a tag (%s) in badge award does not point to badge definition address", aTag.Value())
	}
	if !slices.Contains(ev.IONIdentityPublicKeys(), awardIssuer) {
		return errors.Wrapf(ErrDeviceIdentificationProofFailed, "[device-identification-proof] issuer of badge award (%s) is not a ion identity key", issuer)
	}
	return nil
}

func extractPubkeyFromDeviceIdentificationProof(ev *model.Event) (issuer string, devicePubkey string) {
	if ev.Kind == nostr.KindBadgeAward {
		if aTag := ev.GetTag("a"); len(aTag) >= 2 {
			parts := strings.Split(aTag.Value(), ":")
			// For badge award: 30009:issuerPubKey:device_identification_proof~devicePubKey
			if len(parts) >= 3 && strings.HasPrefix(parts[2], deviceIdentificationProof+"~") {
				devicePubkey = strings.TrimPrefix(parts[2], deviceIdentificationProof+"~")
				if devicePubkey != "" && parts[1] == ev.PubKey {
					return ev.PubKey, devicePubkey
				}
			}
		}
	} else if ev.Kind == nostr.KindBadgeDefinition {
		if dTag := ev.GetTag("d"); len(dTag) >= 2 {
			// For badge definition d-tag: device_identification_proof~devicePubKey
			if strings.HasPrefix(dTag.Value(), deviceIdentificationProof+"~") {
				devicePubkey = strings.TrimPrefix(dTag.Value(), deviceIdentificationProof+"~")
				if devicePubkey != "" {
					return ev.PubKey, devicePubkey
				}
			}
		}
	}

	return "", ""
}

// IsRelayAuthoritativeForUser checks if the relay is authoritative for the user by looking for an attestation event and a relay list event from the user's master public key.
// If both events are found, it checks if the attestation allows access for the device public key and returns the allowed kinds.
func (ev *eventValidator) IsRelayAuthoritativeForUser(ctx context.Context, relayURL string, masterKey, deviceKey string) (authoritative bool, kinds []int, err error) {
	relayTag := model.TagMap{}.Set("r", &relayURL)
	if u, err := url.Parse(relayURL); err == nil && u.Port() != "" {
		u.Host = u.Hostname()
		relayTag = relayTag.Append("r", new(u.String()))
	}

	deviceTag := model.TagMap{}
	if deviceKey != "" {
		deviceTag = deviceTag.Set("p", &deviceKey)
	}

	it := ev.QueryFunc(ctx,
		model.Filter{
			Kinds:   []int{model.CustomIONKindAttestation},
			Authors: []string{masterKey},
			Tags:    deviceTag,
			Limit:   1,
		},
		model.Filter{
			Kinds:   []int{nostr.KindRelayListMetadata},
			Authors: []string{masterKey},
			Tags:    relayTag,
			Limit:   1,
		},
	)

	var attestationEvent, relayListEvent *model.Event
	for ev, err := range it {
		if err != nil {
			return false, nil, errors.Wrap(err, "failed to fetch user events")
		}
		switch ev.Kind {
		case model.CustomIONKindAttestation:
			attestationEvent = ev
		case nostr.KindRelayListMetadata:
			relayListEvent = ev
		}
	}

	// If there's no relay list event or attestation event, we can't consider the relay authoritative.
	if relayListEvent == nil || attestationEvent == nil {
		return false, nil, nil
	}

	records, err := model.ParseAttestationTags(attestationEvent.Tags)
	if err != nil {
		return false, nil, errors.Wrap(err, "failed to parse attestation tags")
	}

	if deviceKey == "" {
		// If no device key is provided, we just check that given relay URL is in the user's relay list and it's authoritative.
		return true, nil, nil
	}

	allowed, err := records.IsAccessAllowed(deviceKey, -1, nostr.Now())
	if err != nil {
		return false, nil, err
	}
	if !allowed {
		return false, nil, errors.Errorf("access not allowed for pubkey %q", deviceKey)
	}

	return true, records.AllowedKinds(deviceKey), nil
}
