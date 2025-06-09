// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/nbd-wtf/go-nostr"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

var (
	ErrUserIsNotPresentedOnRelay = errors.New("user is not presented on relay")
)

func validateKindBadgeDefinitionEvent(ctx context.Context, e *model.Event, imcomingEvents []*model.Event) error {
	if dTag := e.Tags.GetD(); dTag == "" {
		return errors.Wrapf(ErrWrongEventParams, "nip-58, no required d tag: %+v", e)
	}

	return nil
}

func validateKindBadgeAwardEvent(ctx context.Context, e *model.Event, incomingEvents []*model.Event) error {
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
	if err := validateKindBadgeAwardOwnership(ctx, e, incomingEvents); err != nil {
		return errors.Wrap(err, "can't validate badge award ownership")
	}

	return nil
}

func validateKindProfileBadgesEvent(ctx context.Context, e *model.Event, imcomingEvents []*model.Event) error {
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
	userPubkey := e.GetMasterPublicKey()

	for i, aTag := range aTags {
		if len(aTag) < 2 {
			continue
		}
		badgeRef := aTag[1]
		parts := strings.Split(badgeRef, ":")
		if len(parts) < 3 {
			return errors.Wrapf(ErrWrongEventParams, "invalid badge reference format: %s", badgeRef)
		}
		if i < len(eTags) && len(eTags[i]) >= 2 {
			badgeAwardID := eTags[i][1]
			if badgeAwardID != "" {
				if err := validateProfileBadgeAward(ctx, badgeRef, badgeAwardID, userPubkey, imcomingEvents); err != nil {
					return errors.Wrapf(err, "invalid badge award reference %s", badgeAwardID)
				}
			}
		}
	}

	return nil
}

func validateKindBadgeAwardOwnership(ctx context.Context, e *model.Event, incomingEvents []*model.Event) error {
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
	badgeDefinitionIndex := slices.IndexFunc(incomingEvents, func(event *model.Event) bool {
		if event.Kind != nostr.KindBadgeDefinition {
			return false
		}
		expectedDTag := parts[2]
		return event.GetMasterPublicKey() == expectedBadgeAuthor && event.Tags.GetD() == expectedDTag
	})

	if badgeDefinitionIndex == -1 {
		badgePubkey := parts[1]
		badgeDTag := parts[2]

		it := query.GetStoredEvents(ctx, &model.Subscription{
			Filters: nostr.Filters{
				model.Filter{
					Authors: []string{badgePubkey},
					Kinds:   []int{nostr.KindBadgeDefinition},
					Tags:    nostr.TagMap{}.SetLiterals("d", badgeDTag),
					Limit:   1,
				},
			},
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

func validateKindBadgeDefinitionOwnership(ctx context.Context, e *model.Event, incomingEvents []*model.Event) error {
	dTag := e.Tags.GetD()
	if dTag == "" {
		return errors.Wrapf(ErrUserIsNotPresentedOnRelay, "badge definition missing d tag %v", e.ID)
	}
	aTagRef := fmt.Sprintf("%d:%s:%s", nostr.KindBadgeDefinition, e.GetMasterPublicKey(), dTag)
	badgeAwardIndex := slices.IndexFunc(incomingEvents, func(event *model.Event) bool {
		if event.Kind != nostr.KindBadgeAward {
			return false
		}
		if aTag := event.GetTag("a"); aTag != nil && aTag.Value() == aTagRef {
			return true
		}
		return false
	})

	var masterKey string
	if badgeAwardIndex != -1 {
		badgeAward := incomingEvents[badgeAwardIndex]
		if pTag := badgeAward.GetTag("p"); pTag != nil && pTag.Value() != "" {
			masterKey = pTag.Value()
		}
	} else {
		it := query.GetStoredEvents(ctx, &model.Subscription{
			Filters: nostr.Filters{
				model.Filter{
					Kinds: []int{nostr.KindBadgeAward},
					Tags:  nostr.TagMap{}.SetLiterals("a", aTagRef),
					Limit: 1,
				},
			},
		})
		for event, err := range it {
			if err != nil {
				return errors.Wrapf(err, "failed to query badge award from database")
			}
			if event.Kind == nostr.KindBadgeAward {
				if pTag := event.GetTag("p"); pTag != nil && pTag.Value() != "" {
					masterKey = pTag.Value()

					break
				}
			}
		}

		if masterKey == "" {
			return errors.Wrapf(ErrUserIsNotPresentedOnRelay, "no badge award found in events or database for badge definition %v", e.ID)
		}
	}
	if masterKey == "" {
		return errors.Wrapf(ErrUserIsNotPresentedOnRelay, "no p tag found in badge award %v", e.ID)
	}

	return nil
}

func extractUsernameFromProofBadge(ev *model.Event) (bool, string) {
	const usernameProofOfOwnership = "username_proof_of_ownership"
	if ev.Kind == nostr.KindBadgeAward {
		if aTag := ev.GetTag("a"); aTag != nil && len(aTag) >= 2 {
			parts := strings.Split(aTag.Value(), ":")
			// For badge award: kind:pubkey:username_proof_of_ownership~username
			if len(parts) >= 3 && strings.HasPrefix(parts[2], usernameProofOfOwnership+"~") {
				username := strings.TrimPrefix(parts[2], usernameProofOfOwnership+"~")
				if username != "" {
					return true, username
				}
			}
		}
	} else if ev.Kind == nostr.KindBadgeDefinition {
		if dTag := ev.GetTag("d"); dTag != nil && len(dTag) >= 2 {
			// For badge definition d-tag: username_proof_of_ownership~username
			if strings.HasPrefix(dTag.Value(), usernameProofOfOwnership+"~") {
				username := strings.TrimPrefix(dTag.Value(), usernameProofOfOwnership+"~")
				if username != "" {
					return true, username
				}
			}
		}
	}

	return false, ""
}

func validateBadgeRestrictionsWithAck(settingsTag *model.Tag, ev *model.Event, acks []*model.EphemeralEmbeddingEvent) error {
	if settingsTag == nil || (*settingsTag).Value() != model.WhoCanReplySettings {
		return nil
	}

	values := strings.Split((*settingsTag)[2], ",")
	for _, value := range values {
		if !strings.HasPrefix(value, model.BadgeWhoCanReplySettingsPrefix) {
			continue
		}

		badgeATagRef := strings.TrimPrefix(value, model.BadgeWhoCanReplySettingsPrefix+"|")
		badgePubkey := strings.Split(badgeATagRef, ":")[1]
		badgeDTag := strings.Split(badgeATagRef, ":")[2]

		isBadgeDefinitionValid, isBadgeAwardValid := false, false
		for _, ack := range acks {
			if ack.ContentEvent.GetMasterPublicKey() != badgePubkey {
				continue
			}

			if ack.ContentEvent.Kind == nostr.KindBadgeDefinition {
				if ack.ContentEvent.Tags.GetD() != badgeDTag {
					continue
				}
				isBadgeDefinitionValid = true
			}

			if ack.ContentEvent.Kind == nostr.KindBadgeAward {
				aTag := ack.ContentEvent.GetTag("a")
				if aTag == nil || aTag.Value() != badgeATagRef {
					continue
				}
				pTag := ack.ContentEvent.GetTag("p")
				if pTag == nil || pTag.Value() != ev.GetMasterPublicKey() {
					continue
				}
				isBadgeAwardValid = true
			}
		}

		if !isBadgeDefinitionValid || !isBadgeAwardValid {
			return errors.Wrap(ErrCommentsForbidden, "comments are disabled for root post")
		}
	}

	return nil
}

func checkProofOfOwnershipBadges(username string, masterKey string, incomingEvents []*model.Event) error {
	badgeDefinitionIndex := slices.IndexFunc(incomingEvents, func(e *model.Event) bool {
		return e.Kind == nostr.KindBadgeDefinition
	})
	badgeAwardIndex := slices.IndexFunc(incomingEvents, func(e *model.Event) bool {
		return e.Kind == nostr.KindBadgeAward
	})
	if badgeDefinitionIndex == -1 || badgeAwardIndex == -1 {
		return errors.Wrapf(ErrUsernameProofOfOwnershipFailed, "[proof-of-ownership] missing badge definition or award events for username %s", username)
	}
	badgeDefinition := incomingEvents[badgeDefinitionIndex]
	_, badgeUsername := extractUsernameFromProofBadge(badgeDefinition)
	if badgeUsername == "" {
		return errors.Wrapf(ErrUsernameProofOfOwnershipFailed, "[proof-of-ownership] badge definition does not have username proof of ownership")
	}
	if badgeUsername != username {
		return errors.Wrapf(ErrUsernameProofOfOwnershipFailed, "[proof-of-ownership] username in badge definition (%s) doesn't match username (%s)", badgeUsername, username)
	}
	badgeAward := incomingEvents[badgeAwardIndex]
	_, badgeUsername = extractUsernameFromProofBadge(badgeAward)
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

func getEvent(ctx context.Context, address string) (event *model.Event, err error) {
	events := query.GetStoredEvents(ctx, &model.Subscription{
		Filters: []nostr.Filter{
			{
				Addresses: []string{address},
			},
		},
	})
	for e, err := range events {
		if err != nil {
			return nil, errors.Wrapf(err, "failed to fetch linked event for by filter %v ", address)
		}
		if e != nil {
			return e, nil
		}
	}
	return nil, nil
}

func validateProfileBadgeAward(ctx context.Context, badgeRef, badgeAwardID, userPubkey string, incomingEvents []*model.Event) error {
	for _, event := range incomingEvents {
		if event.Kind == nostr.KindBadgeAward && event.GetID() == badgeAwardID {
			if aTag := event.GetTag("a"); aTag == nil || aTag.Value() != badgeRef {
				return errors.Wrapf(ErrUserIsNotPresentedOnRelay, "badge award %s does not reference badge %s", badgeAwardID, badgeRef)
			}
			if pTag := event.GetTag("p"); pTag == nil || pTag.Value() != userPubkey {
				return errors.Wrapf(ErrUserIsNotPresentedOnRelay, "badge award %s is not for user %s", badgeAwardID, userPubkey)
			}

			return nil
		}
	}
	it := query.GetStoredEvents(ctx, &model.Subscription{
		Filters: nostr.Filters{
			model.Filter{
				IDs:   []string{badgeAwardID},
				Kinds: []int{nostr.KindBadgeAward},
				Limit: 1,
			},
		},
	})

	for event, err := range it {
		if err != nil {
			return errors.Wrapf(err, "failed to query badge award from database")
		}
		if event.Kind == nostr.KindBadgeAward && event.GetID() == badgeAwardID {
			if aTag := event.GetTag("a"); aTag == nil || aTag.Value() != badgeRef {
				return errors.Wrapf(ErrUserIsNotPresentedOnRelay, "badge award %s does not reference badge %s", badgeAwardID, badgeRef)
			}
			if pTag := event.GetTag("p"); pTag == nil || pTag.Value() != userPubkey {
				return errors.Wrapf(ErrUserIsNotPresentedOnRelay, "badge award %s is not for user %s", badgeAwardID, userPubkey)
			}

			return nil
		}
	}

	return errors.Wrapf(ErrUserIsNotPresentedOnRelay, "badge award %s not found", badgeAwardID)
}
