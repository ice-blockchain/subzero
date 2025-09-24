// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/model"
)

var (
	ErrUserIsNotPresentedOnRelay = errors.New("user is not presented on relay")
)

func validateKindBadgeDefinitionEvent(e *model.Event) error {
	if dTag := e.Tags.GetD(); dTag == "" {
		return errors.Wrap(ErrWrongEventParams, "nip-58: d tag is required")
	}

	return nil
}

func (ev *eventValidator) validateKindBadgeAwardEvent(ctx context.Context, rules *ruleSet, batch model.Events, e *model.Event) error {
	if len(e.Tags.GetAll([]string{"a"})) == 0 {
		return errors.Wrapf(ErrWrongEventParams, "nip-58: a tag is required")
	}
	aTag := e.GetTag("a")
	if aTag != nil && aTag.Value() != "" {
		parts := strings.Split(aTag.Value(), ":")
		isUsernameProofBadge := len(parts) == 3 && strings.Contains(parts[2], "username_proof_of_ownership~")

		if !isUsernameProofBadge {
			if err := validateATags(e, nostr.KindBadgeDefinition); err != nil {
				return errors.Wrap(err, "nip-58")
			}
		} else {
			if len(parts) != 3 {
				return errors.Wrapf(ErrWrongEventParams, "nip-58: a tag should have 3 parts, but got %d: %v", len(parts), aTag.Value())
			}

			kind, err := strconv.ParseInt(parts[0], 10, 64)
			if err != nil {
				return errors.Wrapf(ErrWrongEventParams, "nip-58: a tag should have kind as first part, but got %q: %v", parts[0], err)
			}

			if int(kind) != nostr.KindBadgeDefinition {
				return errors.Wrapf(ErrWrongEventParams, "nip-58: a tag should reference badge definition (kind %d), but got %d", nostr.KindBadgeDefinition, kind)
			}
		}
	} else {
		return errors.Wrapf(ErrWrongEventParams, "nip-58: a tag is required")
	}

	if len(e.Tags.GetAll([]string{"p"})) == 0 {
		return errors.Wrapf(ErrWrongEventParams, "nip-58: p tag is required")
	}
	if err := ev.validateKindBadgeAwardOwnership(ctx, rules, batch, e); err != nil {
		return errors.Wrap(err, "can't validate badge award ownership")
	}

	return nil
}

func (ev *eventValidator) validateKindProfileBadgesEvent(ctx context.Context, rules *ruleSet, batch model.Events, e *model.Event) error {
	if dTag := e.Tags.GetD(); dTag != model.ProfileBadgesIdentifier {
		return errors.Wrapf(ErrWrongEventParams, "nip-58: no required d tag/wrong value: expected %q, got %q", model.ProfileBadgesIdentifier, dTag)
	}
	if err := validateATags(e, nostr.KindBadgeDefinition); err != nil {
		return errors.Wrap(err, "nip-58")
	}
	if alen, elen := len(e.Tags.GetAll([]string{"a"})), len(e.Tags.GetAll([]string{"e"})); alen != elen {
		return errors.Wrapf(ErrWrongEventParams, "nip-58: e/a tag mismatch: a len %d, e len %d", alen, elen)
	}

	aTags := e.Tags.GetAll([]string{"a"})
	eTags := e.Tags.GetAll([]string{"e"})
	userPubkeys := []string{e.GetMasterPublicKey()}

	for i, aTag := range aTags {
		if len(aTag) < 2 {
			continue
		}
		badgeRef := aTag[1]
		parts := strings.Split(badgeRef, ":")
		if len(parts) < 3 {
			return errors.Wrapf(ErrWrongEventParams, "invalid badge reference format: %s", badgeRef)
		}
		switch {
		case strings.HasPrefix(parts[2], "device_identification_proof~"):
			userPubkeys = []string{e.PubKey}
			// Previous device identification badge's p points to old devices, but e.Pubkey is new, take old ones from attestation for validation.
			for _, eventInBatch := range batch {
				if eventInBatch.Kind != model.CustomIONKindAttestation {
					continue
				}
				pTags := eventInBatch.Tags.GetAll([]string{"p"})
				for _, pTag := range pTags {
					userPubkeys = append(userPubkeys, pTag.Value())
				}
			}
		default:
			userPubkeys = []string{e.GetMasterPublicKey()}
		}
		if i < len(eTags) && len(eTags[i]) >= 2 {
			badgeAwardID := eTags[i][1]
			if badgeAwardID != "" {
				if err := ev.validateProfileBadgeAward(ctx, rules, batch, badgeRef, badgeAwardID, userPubkeys); err != nil {
					return errors.Wrapf(err, "invalid badge award reference %s", badgeAwardID)
				}
			}
		}
	}

	return nil
}

func (ev *eventValidator) validateKindBadgeAwardOwnership(ctx context.Context, _ *ruleSet, batch model.Events, e *model.Event) error {
	aTag := e.GetTag("a")
	if aTag == nil || aTag.Value() == "" {
		return errors.Wrapf(ErrUserIsNotPresentedOnRelay, "badge award missing a tag %v", e.ID)
	}
	parts := strings.Split(aTag.Value(), ":")
	if len(parts) < 3 {
		return errors.Wrapf(ErrUserIsNotPresentedOnRelay, "invalid a tag format %v", aTag.Value())
	}
	expectedBadgeAuthor := parts[1]
	if e.GetMasterPublicKey() != expectedBadgeAuthor {
		return errors.Wrapf(ErrUserIsNotPresentedOnRelay, "badge award not signed by badge definition author %v", e.ID)
	}
	badgeDefinitionIndex := slices.IndexFunc(batch, func(event *model.Event) bool {
		expectedDTag := parts[2]
		switch event.Kind {
		case nostr.KindBadgeDefinition:
			return event.GetMasterPublicKey() == expectedBadgeAuthor && event.Tags.GetD() == expectedDTag

		case model.CustomIONKindEphemeralEmbedding:
			var nestedEvent model.Event
			err := nestedEvent.UnmarshalJSON([]byte(event.Content))
			return err == nil &&
				nestedEvent.Kind == nostr.KindBadgeDefinition &&
				nestedEvent.GetMasterPublicKey() == expectedBadgeAuthor &&
				nestedEvent.Tags.GetD() == expectedDTag
		}
		return false
	})

	if badgeDefinitionIndex == -1 {
		badgePubkey := parts[1]
		badgeDTag := parts[2]

		it := ev.QueryFunc(ctx, model.Filter{
			Authors: []string{badgePubkey},
			Kinds:   []int{nostr.KindBadgeDefinition},
			Tags:    model.TagMap{}.SetLiterals("d", badgeDTag),
			Limit:   1,
		})
		badgeDefinitionFound := false
		for event, err := range it {
			if err != nil {
				return errors.Wrapf(err, "failed to query badge definition from database")
			}
			if event.Kind == nostr.KindBadgeDefinition {
				badgeDefinitionFound = true
				break
			}
		}

		if !badgeDefinitionFound {
			return errors.Wrapf(ErrUserIsNotPresentedOnRelay, "no badge definition found in events or database for badge %v", aTag.Value())
		}
	}
	if pTag := e.GetTag("p"); pTag == nil || pTag.Value() == "" {
		return errors.Wrapf(ErrUserIsNotPresentedOnRelay, "badge award missing p tag %v", e.ID)
	}

	return nil
}

func extractUsernameProofFromAddressTag(tag model.Tag) (username string, found bool) {
	parts := strings.Split(tag.Value(), ":")
	if len(parts) < 3 { // kind : pubkey : username_proof_of_ownership~username.
		return "", false
	}
	return strings.CutPrefix(parts[2], model.TagSuffixUsernameProof+"~")
}

func extractUsernameFromProofBadge(ev *model.Event) (string, bool) {
	switch ev.Kind {
	case nostr.KindBadgeAward:
		return extractUsernameProofFromAddressTag(ev.GetTag("a"))
	case nostr.KindBadgeDefinition:
		if dTag := ev.GetTag("d"); len(dTag) >= 2 {
			// For badge definition d-tag: username_proof_of_ownership~username.
			return strings.CutPrefix(dTag.Value(), model.TagSuffixUsernameProof+"~")
		}
	}
	return "", false
}

func checkProofOfOwnershipBadges(_ context.Context, _ *ruleSet, batch model.Events, username string, masterKey string) error {
	badgeDefinitionIndex := slices.IndexFunc(batch, func(e *model.Event) bool {
		return e.Kind == nostr.KindBadgeDefinition
	})
	badgeAwardIndex := slices.IndexFunc(batch, func(e *model.Event) bool {
		return e.Kind == nostr.KindBadgeAward
	})
	if badgeDefinitionIndex == -1 || badgeAwardIndex == -1 {
		return errors.Wrapf(ErrUsernameProofOfOwnershipFailed, "[proof-of-ownership] missing badge definition or award events for username %s", username)
	}
	badgeDefinition := batch[badgeDefinitionIndex]
	badgeUsername, _ := extractUsernameFromProofBadge(badgeDefinition)
	if badgeUsername == "" {
		return errors.Wrapf(ErrUsernameProofOfOwnershipFailed, "[proof-of-ownership] badge definition does not have username proof of ownership")
	}
	if badgeUsername != username {
		return errors.Wrapf(ErrUsernameProofOfOwnershipFailed, "[proof-of-ownership] username in badge definition (%s) doesn't match username (%s)", badgeUsername, username)
	}
	badgeAward := batch[badgeAwardIndex]
	badgeUsername, _ = extractUsernameFromProofBadge(badgeAward)
	if badgeUsername == "" {
		return errors.Wrapf(ErrUsernameProofOfOwnershipFailed, "[proof-of-ownership] badge award does not have username proof of ownership")
	}
	if badgeUsername != username {
		return errors.Wrapf(ErrUsernameProofOfOwnershipFailed, "[proof-of-ownership] username in badge award (%s) doesn't match username (%s)", badgeUsername, username)
	}
	pTag := badgeAward.GetTag("p")
	if pTag == nil || pTag.Value() == "" {
		return errors.Wrapf(ErrUsernameProofOfOwnershipFailed, "[proof-of-ownership] no p tag in badge award for username change")
	}
	if pTag.Value() != masterKey {
		return errors.Wrapf(ErrUsernameProofOfOwnershipFailed, "[proof-of-ownership] p tag in badge award doesn't point to the profile owner: username proof of ownership failed")
	}

	return nil
}

func (ev *eventValidator) getEvent(ctx context.Context, address string) (event *model.Event, err error) {
	for e, err := range ev.QueryFunc(ctx, model.Filter{Addresses: []string{address}}) {
		if err != nil {
			return nil, errors.Wrapf(err, "failed to fetch linked event for by filter %v ", address)
		}
		if e != nil {
			return e, nil
		}
	}
	return nil, nil
}

func (ev *eventValidator) validateProfileBadgeAward(ctx context.Context, _ *ruleSet, batch model.Events, badgeRef, badgeAwardID string, userPubkeys []string) error {
	for _, event := range batch {
		if event.Kind == nostr.KindBadgeAward && event.GetID() == badgeAwardID {
			if aTag := event.GetTag("a"); aTag == nil || aTag.Value() != badgeRef {
				return errors.Wrapf(ErrUserIsNotPresentedOnRelay, "badge award %s does not reference badge %s", badgeAwardID, badgeRef)
			}
			if pTag := event.GetTag("p"); pTag == nil || !slices.Contains(userPubkeys, pTag.Value()) {
				return errors.Wrapf(ErrUserIsNotPresentedOnRelay, "badge award %s is not for user %v", badgeAwardID, userPubkeys)
			}

			return nil
		}
	}

	it := ev.QueryFunc(ctx, model.Filter{
		IDs:   []string{badgeAwardID},
		Kinds: []int{nostr.KindBadgeAward},
		Limit: 1,
	})

	for event, err := range it {
		if err != nil {
			return errors.Wrapf(err, "failed to query badge award from database")
		}
		if event.Kind == nostr.KindBadgeAward && event.GetID() == badgeAwardID {
			if aTag := event.GetTag("a"); aTag == nil || aTag.Value() != badgeRef {
				return errors.Wrapf(ErrUserIsNotPresentedOnRelay, "badge award %s does not reference badge %s", badgeAwardID, badgeRef)
			}
			if pTag := event.GetTag("p"); pTag == nil || !slices.Contains(userPubkeys, pTag.Value()) {
				return errors.Wrapf(ErrUserIsNotPresentedOnRelay, "badge award %s is not for user %v", badgeAwardID, userPubkeys)
			}

			return nil
		}
	}

	return errors.Wrapf(ErrUserIsNotPresentedOnRelay, "badge award %s not found", badgeAwardID)
}
