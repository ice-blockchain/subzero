// SPDX-License-Identifier: ice License 1.0

package command

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/validation"
	"github.com/nbd-wtf/go-nostr"
)

func (c *consensus) findRootPost(ctx context.Context, ev *model.Event) (*model.Event, error) {
	aTags := ev.GetTags("a")
	rootTagIndex := slices.IndexFunc(aTags, func(a nostr.Tag) bool {
		return a.Value() != "" && len(a) >= 4 && a[3] == model.TagMarkerRoot
	})
	if rootTagIndex != -1 {
		rootPost, _, err := c.getEvent(ctx, aTags[rootTagIndex].Value())
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get root post by address")
		}
		if rootPost != nil {
			return rootPost, nil
		}
	}
	eTags := ev.GetTags("e")
	rootTagIndex = slices.IndexFunc(eTags, func(e nostr.Tag) bool {
		return e.Value() != "" && len(e) >= 4 && e[3] == model.TagMarkerRoot
	})
	if rootTagIndex == -1 || ev.ID == eTags[rootTagIndex].Value() {
		return nil, nil
	}
	rootPost, _, err := c.getEvent(ctx, eTags[rootTagIndex].Value())
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get root post")
	}

	return rootPost, nil
}

func (c *consensus) handleTextNoteWithoutAck(ctx context.Context, ev *model.Event) (string, error) {
	rootPost, err := c.findRootPost(ctx, ev)
	if err != nil {
		return "", err
	}

	if rootPost == nil {
		return ev.GetMasterPublicKey(), nil
	}

	return c.checkReplyPermissions(ctx, ev, rootPost)
}

func (c *consensus) handleTextNoteWithAck(ctx context.Context, ev *model.Event, acks []*model.EphemeralEmbeddingEvent) (string, error) {
	if err := c.validateRootPostReplyWithAck(ctx, ev, acks); err != nil {
		return "", err
	}

	return ev.GetMasterPublicKey(), nil
}

func (c *consensus) checkReplyPermissions(ctx context.Context, ev *model.Event, rootPost *model.Event) (string, error) {
	if ev.GetMasterPublicKey() == rootPost.GetMasterPublicKey() {
		return ev.GetMasterPublicKey(), nil
	}

	settingsTag := validation.GetLatestSettingsTag(rootPost, model.WhoCanReplySettings)
	if settingsTag == nil || (*settingsTag).Value() != model.WhoCanReplySettings {
		return ev.GetMasterPublicKey(), nil
	}

	values := strings.Split((*settingsTag)[2], ",")
	for _, value := range values {
		if strings.HasPrefix(value, model.BadgeWhoCanReplySettingsPrefix) {
			badgeATagRef := strings.TrimPrefix(value, model.BadgeWhoCanReplySettingsPrefix+"|")
			badgePubkey := strings.Split(badgeATagRef, ":")[1]

			it := c.Query(ctx, model.Filter{
				Authors: []string{badgePubkey},
				Kinds:   []int{nostr.KindBadgeDefinition},
				Tags:    nostr.TagMap{}.SetLiterals("d", "verified"),
				Limit:   1,
			}, model.Filter{
				Authors: []string{badgePubkey},
				Kinds:   []int{nostr.KindBadgeAward},
				Tags:    nostr.TagMap{}.SetLiterals("a", badgeATagRef).SetLiterals("p", ev.GetMasterPublicKey()),
				Limit:   1,
			})

			badgeDefinitionFound, badgeAwardFound := false, false
			for e, iErr := range it {
				if iErr != nil {
					return "", errors.Wrapf(iErr, "failed to fetch linked event for by filter %v %v", badgePubkey, badgeATagRef)
				}
				if e.Kind == nostr.KindBadgeDefinition {
					badgeDefinitionFound = true
				}
				if e.Kind == nostr.KindBadgeAward {
					badgeAwardFound = true
				}
			}

			if !badgeDefinitionFound || !badgeAwardFound {
				return "", errors.Wrapf(ErrCommentsForbidden, "comments are disabled for root post %v", rootPost.ID)
			}

			return ev.GetMasterPublicKey(), nil
		}
	}

	return "", errors.Wrapf(ErrCommentsForbidden, "comments are disabled for root post %v", rootPost.ID)
}

func (c *consensus) validateRootPostReplyWithAck(ctx context.Context, ev *model.Event, acks []*model.EphemeralEmbeddingEvent) error {
	rootPost, err := c.findRootPost(ctx, ev)
	if err != nil {
		return err
	}
	if rootPost == nil {
		return nil
	}
	if ev.GetMasterPublicKey() == rootPost.GetMasterPublicKey() {
		return nil
	}

	return c.validateBadgeRestrictionsWithAck(rootPost, ev, acks)
}

func (c *consensus) validateBadgeRestrictionsWithAck(rootPost *model.Event, ev *model.Event, acks []*model.EphemeralEmbeddingEvent) error {
	settingsTag := validation.GetLatestSettingsTag(rootPost, model.WhoCanReplySettings)
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

		isBadgeDefinitionValid, isBadgeAwardValid := false, false
		for _, ack := range acks {
			if ack.ContentEvent.GetMasterPublicKey() != badgePubkey {
				continue
			}

			if ack.ContentEvent.Kind == nostr.KindBadgeDefinition {
				if ack.ContentEvent.Tags.GetD() != "verified" {
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
			return errors.Wrapf(ErrCommentsForbidden, "comments are disabled for root post %v", rootPost.ID)
		}
	}

	return nil
}

func (c *consensus) findMasterKeyFromReferences(ctx context.Context, ev *model.Event) (string, error) {
	var masterKey string
	var linkedEvent *model.Event
	var err error
	if pTag := ev.GetTag("p"); pTag != nil && pTag.Value() != "" {
		relays, rErr := c.fetchUserRelays(ctx, pTag.Value()) // Mentioned user is presented on relay
		if rErr == nil && len(relays) > 0 {
			masterKey = pTag.Value()
		}
	}
	if eTag := ev.GetTag("e"); eTag != nil && eTag.Value() != "" && len(eTag) >= 4 && eTag[3] == model.TagMarkerReply {
		linkedEvent, masterKey, err = c.getEvent(ctx, eTag.Value())
		if err != nil {
			return "", errors.Wrapf(err, "failed to get referenced event")
		}
	}
	refTags := []string{"q", "Q", "a"}
	for _, tagName := range refTags {
		if tag := ev.GetTag(tagName); tag != nil && tag.Value() != "" {
			linkedEvent, masterKey, err = c.getEvent(ctx, tag.Value())
			if err != nil {
				return "", errors.Wrapf(err, "failed to get referenced event")
			}
			if linkedEvent != nil && masterKey != "" {
				break
			}
		}
	}

	return masterKey, nil
}

func extractUsernameFromProofBadge(ev *model.Event) (bool, string) {
	const usernameProofOfOwnership = "username_proof_of_ownership"
	if ev.Kind == nostr.KindBadgeAward {
		if aTag := ev.GetTag("a"); aTag != nil && len(aTag) >= 2 {
			parts := strings.Split(aTag.Value(), ":")
			if parts[2] != usernameProofOfOwnership {
				return false, ""
			}
			if len(parts) == 4 {
				return true, parts[3]
			}
		}
	} else if ev.Kind == nostr.KindBadgeDefinition {
		if dTag := ev.GetTag("d"); dTag != nil && len(dTag) >= 2 {
			parts := strings.Split(dTag.Value(), ":")
			if parts[0] != usernameProofOfOwnership {
				return false, ""
			}
			if len(parts) == 2 {
				return true, parts[1]
			}
		}
	}

	return false, ""
}

func (c *consensus) validateProfileMetadataNameChange(ctx context.Context, ev *model.Event, masterKey string) (bool, string, error) {
	profileAddress := fmt.Sprintf("%d:%s:", nostr.KindProfileMetadata, masterKey)
	oldProfile, _, err := c.getEvent(ctx, profileAddress)
	if err != nil {
		return false, "", errors.Wrapf(err, "[proof-of-ownership] failed to get old profile metadata")
	}
	var oldProfileMetadata model.ProfileMetadataContent
	var newProfileMetadata model.ProfileMetadataContent
	oldProfileFound := oldProfile != nil && oldProfile.ID != ev.ID
	if oldProfileFound {
		if err := json.Unmarshal([]byte(oldProfile.Content), &oldProfileMetadata); err != nil {
			return false, "", errors.Wrapf(err, "[proof-of-ownership] failed to unmarshal old profile metadata")
		}
	}
	if err := json.Unmarshal([]byte(ev.Content), &newProfileMetadata); err != nil {
		return false, "", errors.Wrapf(err, "[proof-of-ownership] failed to unmarshal new profile metadata")
	}
	if !oldProfileFound || oldProfileMetadata.Name != newProfileMetadata.Name {
		return true, newProfileMetadata.Name, nil
	}

	return false, "", nil
}

func (c *consensus) findProfileEventInEventsBatch(ctx context.Context, masterKey string, username string, incomingEvents []*model.Event) bool {
	profileIndex := slices.IndexFunc(incomingEvents, func(e *model.Event) bool {
		if e.Kind != nostr.KindProfileMetadata {
			return false
		}
		if e.GetMasterPublicKey() != masterKey {
			return false
		}
		var metadata model.ProfileMetadataContent
		if err := json.Unmarshal([]byte(e.Content), &metadata); err != nil {
			log.Printf("failed to unmarshal profile metadata: %v", err)

			return false
		}

		return metadata.Name == username
	})

	return profileIndex != -1
}

func (c *consensus) checkProofOfOwnershipBadges(username string, masterKey string, incomingEvents []*model.Event) error {
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
		return errors.Wrapf(ErrUsernameProofOfOwnershipFailed, "[proof-of-ownership] p tag in badge award doesn't point to the profile owner")
	}

	return nil
}
