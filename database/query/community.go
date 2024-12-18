// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"

	"github.com/cockroachdb/errors"
	"github.com/ice-blockchain/subzero/model"
	"github.com/nbd-wtf/go-nostr"
)

type (
	Role string
)

const (
	moderatorRole = "moderator"
	adminRole     = "admin"
	ownerRole     = "owner"
	othersRole    = ""
)

func handleBanUserEvent(event, communityDefinitionEvent *model.Event) error {
	authorRole := getCommunityRoleByPubkey(event.PubKey, communityDefinitionEvent)
	if authorRole != adminRole && authorRole != moderatorRole && authorRole != ownerRole {
		return errors.Wrap(ErrCommunityActionForbidden, "only admin, owner or moderator can ban user")
	}
	pTags := event.Tags.GetAll([]string{"p"})
	for _, pTag := range pTags {
		if pTag.Key() == "p" {
			if pTag.Value() == event.PubKey {
				return errors.Wrap(ErrCommunityActionForbidden, "admin/moderator can't ban himself")
			}
			if pTag.Value() == communityDefinitionEvent.PubKey {
				return errors.Wrap(ErrCommunityActionForbidden, "owner of the community can't be banned")
			}
			bannedRole := getCommunityRoleByPubkey(pTag.Value(), communityDefinitionEvent)
			if bannedRole == adminRole && authorRole == moderatorRole {
				return errors.Wrap(ErrCommunityActionForbidden, "moderator can't ban admin")
			}
		}
	}

	return nil
}

func handleChangeCommunityDefinitionEvent(event, communityDefinitionEvent *model.Event) error {
	authorRole := getCommunityRoleByPubkey(event.PubKey, communityDefinitionEvent)
	if authorRole != moderatorRole && authorRole != adminRole && authorRole != ownerRole {
		return errors.Wrap(ErrCommunityActionForbidden, "only admin, owner or moderator can change community definition")
	}
	if authorRole == moderatorRole {
		pTags := event.Tags.GetAll([]string{"p"})
		for _, pTag := range pTags {
			if pTag.Key() == "p" {
				if pTag[3] == adminRole {
					return errors.Wrap(ErrCommunityActionForbidden, "moderator can't promote user to admin")
				}
				for _, tag := range communityDefinitionEvent.Tags.GetAll([]string{"p"}) {
					if tag.Key() == "p" && tag.Value() == pTag.Value() && tag[3] == adminRole {
						return errors.Wrap(ErrCommunityActionForbidden, "moderator can't demote admin")
					}
				}
			}
		}
		if name := event.GetTag("name"); name != nil {
			return errors.Wrap(ErrCommunityActionForbidden, "moderator can't change the name of the community")
		}
		if description := event.GetTag("description"); description != nil {
			return errors.Wrap(ErrCommunityActionForbidden, "moderator can't change the description of the community")
		}
		if closed := event.GetTag("closed"); closed != nil {
			return errors.Wrap(ErrCommunityActionForbidden, "moderator can't change the open/closed status of the community")
		}
		if open := event.GetTag("open"); open != nil {
			return errors.Wrap(ErrCommunityActionForbidden, "moderator can't change the open/closed status of the community")
		}
		if public := event.GetTag("public"); public != nil {
			return errors.Wrap(ErrCommunityActionForbidden, "moderator can't change the public/private status of the community")
		}
		if private := event.GetTag("private"); private != nil {
			return errors.Wrap(ErrCommunityActionForbidden, "moderator can't change the public/private status of the community")
		}
		if imeta := event.GetTag("imeta"); imeta != nil {
			return errors.Wrap(ErrCommunityActionForbidden, "moderator can't change the picture of the community")
		}
		if settings := event.Tags.GetAll([]string{"settings"}); settings != nil {
			return errors.Wrap(ErrCommunityActionForbidden, "moderator can't change the settings of the community")
		}
	}

	return nil
}

func checkDeleteRights(event, communityDefinitionEvent *model.Event) error {
	authorRole := getCommunityRoleByPubkey(event.PubKey, communityDefinitionEvent)
	if authorRole == ownerRole {
		return nil
	} else if authorRole == "" {
		allowed := false
		pTags := event.Tags.GetAll([]string{"p"})
		for _, pTag := range pTags {
			if pTag.Key() == "p" {
				if pTag.Value() == event.PubKey {
					allowed = true
				}
			}
		}
		if !allowed || (allowed && len(pTags) > 1) {
			return errors.Wrap(ErrCommunityActionForbidden, "only admin, owner or moderator can remove user/post/comment/repost from the community")
		}
	} else {
		fmt.Printf("authorRole: %v\n", authorRole)
		if authorRole != adminRole && authorRole != moderatorRole && event.PubKey != communityDefinitionEvent.PubKey {
			return errors.Wrap(ErrCommunityActionForbidden, "only admin, owner or moderator can remove user/post/comment/repost from the community")
		}
		if authorRole == moderatorRole {
			pTags := event.Tags.GetAll([]string{"p"})
			for _, pTag := range pTags {
				if pTag.Key() == "p" {
					if pTag[3] == adminRole || pTag.Key() == communityDefinitionEvent.PubKey {
						return errors.Wrap(ErrCommunityActionForbidden, "moderator can't remove admin or community owner user/post/comment/repost")
					}
				}
			}
		}
	}

	return nil
}

func getLatestSettingsTag(event *model.Event, settingsName string) *model.Tag {
	var latestSettingsTag *model.Tag
	latestTimestamp := int64(0)

	for _, tag := range event.Tags.GetAll([]string{"settings"}) {
		if tag.Value() == settingsName && len(tag) > 3 {
			timestamp, err := strconv.ParseInt(tag[3], 10, 64)
			if err != nil {
				log.Printf("error parsing timestamp: %v", err)

				continue
			}

			if timestamp > latestTimestamp {
				latestSettingsTag = &tag
				latestTimestamp = timestamp
			}
		}
	}

	return latestSettingsTag
}

func isCommunityCommentsEnabled(event *model.Event) bool {
	if settings := getLatestSettingsTag(event, "comments_enabled"); settings != nil && len(*settings) > 3 {
		val, err := strconv.ParseBool((*settings)[2])
		if err != nil {
			return false
		}

		return val
	}

	return true
}

func roleRequiredForPosting(event *model.Event) string {
	if settings := getLatestSettingsTag(event, "role_required_for_posting"); settings != nil && len(*settings) > 3 && ((*settings)[2] == adminRole || (*settings)[2] == moderatorRole) {
		return (*settings)[2]
	}

	return ""
}

func getCommunityRoleByPubkey(pubkey string, communityDefinitionEvent *model.Event) string {
	if communityDefinitionEvent.PubKey == pubkey {
		return "owner"
	}
	pTags := communityDefinitionEvent.Tags.GetAll([]string{"p"})
	if pTags == nil {
		return ""
	}
	for _, pTag := range pTags {
		if pTag.Key() == "p" && len(pTag) > 3 && pTag.Value() == pubkey {
			return pTag[3]
		}
	}

	return ""
}

func handleCommunityJoinEvent(event, communityDefinitionEvent *model.Event) error {
	closedTag := communityDefinitionEvent.GetTag("closed")
	authorizationTag := event.GetTag("authorization")
	if closedTag != nil {
		if role := getCommunityRoleByPubkey(event.PubKey, communityDefinitionEvent); role == ownerRole || role == adminRole || role == moderatorRole {
			return nil
		}
		if authorizationTag == nil {
			return errors.Wrap(ErrCommunityActionForbidden, "can't join closed community")
		}
		var parsedAuthorizationEvent model.Event
		if err := json.Unmarshal([]byte(authorizationTag.Value()), &parsedAuthorizationEvent); err != nil {
			return errors.Wrap(ErrCommunityActionForbidden, "wrong authorization event")
		}
		if parsedAuthorizationEvent.PubKey != communityDefinitionEvent.PubKey {
			pTags := communityDefinitionEvent.Tags.GetAll([]string{"p"})
			if pTags == nil {
				return errors.Wrap(ErrCommunityActionForbidden, "community definition has no moderator/admins")
			}
			passed := false
			for _, pTag := range pTags {
				if pTag.Key() == "p" {
					if pTag.Value() == parsedAuthorizationEvent.PubKey && len(pTag) == 4 && (pTag[3] == "moderator" || pTag[3] == "admin") {
						passed = true

						break
					}
				}
			}
			if !passed {
				return errors.Wrap(ErrCommunityActionForbidden, "user not authorized to join this community")
			}
		}
	}

	return nil
}

func (db *dbClient) handleDeletionEvents(ctx context.Context, deleteEvent *model.Event, communityEventsToDelete []*model.Event) (filters []databaseFilterDelete, err error) {
	for _, ev := range communityEventsToDelete {
		communityDefinitionEvent := db.getCommunityDefinition(ctx, ev.GetTag("h").Value())
		if communityDefinitionEvent == nil {
			return nil, errors.Newf("community definition:%v not found", ev.GetTag("h").Value())
		}
		if deleteEventRole := getCommunityRoleByPubkey(deleteEvent.PubKey, communityDefinitionEvent); deleteEventRole == "" {
			if ev.PubKey != deleteEvent.PubKey {
				return nil, errors.Wrap(ErrCommunityActionForbidden, "only admin, owner, moderator or owner of the post can remove user/post/comment/repost from the community")
			}
		} else {
			if deleteEventRole != adminRole && deleteEventRole != moderatorRole && deleteEventRole != ownerRole {
				return nil, errors.Wrap(ErrCommunityActionForbidden, "only admin, owner or moderator can remove user/post/comment/repost from the community")
			}
			postRole := getCommunityRoleByPubkey(ev.PubKey, communityDefinitionEvent)
			if deleteEventRole == moderatorRole && (postRole == adminRole || ev.PubKey == communityDefinitionEvent.PubKey) {
				return nil, errors.Wrap(ErrCommunityActionForbidden, "moderator can't remove admin or community owner user/post/comment/repost")
			} else if deleteEventRole == adminRole && ev.PubKey == communityDefinitionEvent.PubKey {
				return nil, errors.Wrap(ErrCommunityActionForbidden, "admin can't remove owner community user/post/comment/repost")
			}
		}
		filters = append(filters, databaseFilterDelete{
			IDs:    []string{ev.ID},
			Author: ev.PubKey,
		})
	}

	return filters, nil
}

func (db *dbClient) handleCommunityEvents(ctx context.Context, incomingEvent *model.Event) error {
	if incomingEvent.Kind != model.KindCommunityDefinition {
		if hTag := incomingEvent.GetTag("h"); hTag != nil {
			var communityDefinitionEvent *model.Event
			if communityDefinitionEvent = db.getCommunityDefinition(ctx, hTag.Value()); communityDefinitionEvent == nil {
				return errors.New("community definition not found")
			}
			switch incomingEvent.Kind {
			case model.KindCommunityChangeDefinition:
				if err := handleChangeCommunityDefinitionEvent(incomingEvent, communityDefinitionEvent); err != nil {
					return err
				}
			case model.KindCommunityBanUser:
				if err := handleBanUserEvent(incomingEvent, communityDefinitionEvent); err != nil {
					return err
				}
			case model.KindCommunityJoin:
				if err := handleCommunityJoinEvent(incomingEvent, communityDefinitionEvent); err != nil {
					return err
				}
			case model.KindCommunityOwnershipTransferring:
				if getAuthorRole := getCommunityRoleByPubkey(incomingEvent.PubKey, communityDefinitionEvent); getAuthorRole != ownerRole {
					return errors.Wrap(ErrCommunityActionForbidden, "only owner of the community can transfer ownership")
				}
			case nostr.KindArticle, nostr.KindDraftArticle, nostr.KindTextNote,
				nostr.KindRepost, nostr.KindGenericRepost:
				requiredRole := roleRequiredForPosting(communityDefinitionEvent)
				postRole := getCommunityRoleByPubkey(incomingEvent.PubKey, communityDefinitionEvent)
				if requiredRole == moderatorRole && (postRole != adminRole && postRole != ownerRole && postRole != moderatorRole) {
					return errors.Wrapf(ErrCommunityActionForbidden, "only moderator, admin or owner can post in this community", requiredRole)
				} else if requiredRole == adminRole && postRole != ownerRole {
					return errors.Wrapf(ErrCommunityActionForbidden, "only admin or owner can post in this community", requiredRole)
				}
			case model.KindComment:
				if !isCommunityCommentsEnabled(communityDefinitionEvent) {
					return errors.Wrap(ErrCommunityActionForbidden, "comments are disabled in this community")
				}
			}
		}
	}

	return nil
}

func (db *dbClient) gatherCommunityEventsForDeletion(ctx context.Context, incomingEvent *model.Event) []*model.Event {
	eTags := incomingEvent.Tags.GetAll([]string{"e"})
	var filterETags []string
	for _, eTag := range eTags {
		if eTag.Key() == "e" {
			filterETags = append(filterETags, eTag.Value())
		}
	}
	eventIterator := db.SelectEvents(ctx, nostr.Filter{IDs: filterETags})
	var communityEventsToDelete []*model.Event
	for ev := range eventIterator {
		if ev != nil {
			hTags := ev.Tags.GetAll([]string{"h"})
			for _, hTag := range hTags {
				if hTag.Key() == "h" {
					communityEventsToDelete = append(communityEventsToDelete, ev)
				}
			}
		}
	}

	return communityEventsToDelete
}

func (db *dbClient) getCommunityDefinition(ctx context.Context, hTag string) *model.Event {
	eventIterator := db.SelectEvents(ctx, nostr.Filter{Kinds: []int{model.KindCommunityDefinition}, Tags: model.TagMap{}.SetLiterals("h", hTag)})
	var lastEventDefinition *model.Event
	for ev := range eventIterator {
		if lastEventDefinition == nil {
			lastEventDefinition = ev

			continue
		}
		if ev.CreatedAt > lastEventDefinition.CreatedAt {
			lastEventDefinition = ev
		}
	}

	return lastEventDefinition
}
