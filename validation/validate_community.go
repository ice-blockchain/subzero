// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"cmp"
	"context"
	"log"
	"sort"
	"strconv"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

func validatePostCommunityEvent(ctx context.Context, incomingEvent *model.Event) error {
	hTag := incomingEvent.GetTag(model.CustomIONTagCommunity)
	if hTag == nil {
		return nil
	}

	communityDefinitionEvent, err := GetCommunityDefinition(ctx, hTag.Value())
	if err != nil {
		return err
	}
	if err := IsUserBanned(ctx, incomingEvent.GetMasterPublicKey(), hTag.Value()); err != nil {
		return errors.Wrapf(err, "user:%v banned", incomingEvent.GetMasterPublicKey())
	}

	requiredRole := roleRequiredForPosting(communityDefinitionEvent)
	replyRole := model.GetCommunityRoleByPubkey(incomingEvent.GetMasterPublicKey(), communityDefinitionEvent)
	if requiredRole == model.ModeratorRole && (replyRole != model.AdminRole && replyRole != model.OwnerRole && replyRole != model.ModeratorRole) {
		return errors.Wrapf(ErrActionForbidden, "only %v can post in this community", cmp.Or(requiredRole, "moderator, admin or owner"))
	} else if requiredRole == model.AdminRole && replyRole != model.OwnerRole && replyRole != model.AdminRole {
		return errors.Wrapf(ErrActionForbidden, "only %v can post in this community", cmp.Or(requiredRole, "admin or owner"))
	} else if requiredRole == model.RegularRole && replyRole == model.RegularRole {
		if err := IsUserPartOfCommunity(ctx, communityDefinitionEvent, incomingEvent.GetMasterPublicKey()); err != nil {
			return errors.Wrapf(err, "user:%v not part of the community", incomingEvent.GetMasterPublicKey())
		}
	}
	if incomingEvent.Kind == nostr.KindRepost || incomingEvent.Kind == nostr.KindGenericRepost {
		if !isCommunityCommentsEnabled(communityDefinitionEvent) {
			return errors.Wrap(ErrActionForbidden, "comments are disabled in this community")
		}
	}

	return nil
}

func validateDeleteCommunityEvents(ctx context.Context, e *model.Event) error {
	var ids []string
	for _, eTag := range e.GetTags("e") {
		ids = append(ids, eTag.Value())
	}
	if len(ids) == 0 {
		return nil
	}

	var communityEventsToCheck []*model.Event
	for ev, err := range query.GetStoredEvents(ctx, &model.Subscription{Filters: model.Filters{{
		IDs:  ids,
		Tags: model.TagMap{}.SetLiterals(model.CustomIONTagCommunity),
	}}}) {
		if err != nil {
			return errors.Wrap(err, "failed to get stored events")
		}
		communityEventsToCheck = append(communityEventsToCheck, ev)
	}
	for _, ev := range communityEventsToCheck {
		if err := ValidateCommunityDeleteEvent(ctx, ev, e); err != nil {
			return errors.Wrap(err, "failed to validate delete event")
		}
	}

	return nil
}

func ValidateCommunityDeleteEvent(ctx context.Context, event, deleteEvent *model.Event) error {
	communityDefinitionEvent, err := GetCommunityDefinition(ctx, event.GetTag(model.CustomIONTagCommunity).Value())
	if err != nil {
		return err
	}

	communityEventRole := model.GetCommunityRoleByPubkey(event.GetMasterPublicKey(), communityDefinitionEvent)
	if deleteEventIssuerRole := model.GetCommunityRoleByPubkey(deleteEvent.GetMasterPublicKey(), communityDefinitionEvent); deleteEventIssuerRole == model.ModeratorRole {
		if communityEventRole == model.AdminRole || communityEventRole == model.OwnerRole {
			return errors.Wrap(ErrActionForbidden, "moderator can't remove admin or community owner user/post/comment/repost")
		}
	} else if deleteEventIssuerRole == model.AdminRole && communityEventRole == model.OwnerRole {
		return errors.Wrap(ErrActionForbidden, "admin can't remove owner user/post/comment/repost")
	} else if deleteEventIssuerRole == model.RegularRole && event.GetMasterPublicKey() != deleteEvent.GetMasterPublicKey() {
		return errors.Wrap(ErrActionForbidden, "only admin, owner or moderator can remove user/post/comment/repost from the community")
	}

	return nil
}

func IsUserBanned(ctx context.Context, pubkey, communityID string) error {
	eventIterator := query.GetStoredEvents(ctx, &model.Subscription{
		Filters: model.Filters{
			model.Filter{
				Kinds: []int{model.CustomIONKindCommunityBanUser},
				Tags:  model.TagMap{}.SetLiterals("p", pubkey).SetLiterals(model.CustomIONTagCommunity, communityID),
			},
		},
	})
	for _, err := range eventIterator {
		if err != nil {
			return errors.Wrap(err, "failed to get stored events")
		}

		return errors.Wrap(ErrActionForbidden, "user was banned")
	}

	return nil
}

func IsUserPartOfCommunity(ctx context.Context, communityDefinitionEvent *model.Event, masterPubkey string) error {
	eventIterator := query.GetStoredEvents(ctx, &model.Subscription{
		Filters: model.Filters{
			model.Filter{
				Kinds: []int{model.CustomIONKindCommunityJoin},
				Tags:  model.TagMap{}.SetLiterals("p", masterPubkey),
			},
		},
	})
	for ev, err := range eventIterator {
		if err != nil {
			return errors.Wrap(err, "failed to get stored events")
		}
		if communityDefinitionEvent.GetTag("open") != nil {
			return nil
		}
		if communityDefinitionEvent.GetTag("closed") != nil && ev.GetTag("authorization") != nil {
			return nil
		}
	}

	return errors.Wrap(ErrActionForbidden, "user is not part of the community")
}

func getLatestSettingsTag(event *model.Event, settingsName string) *model.Tag {
	var latestSettingsTag *model.Tag
	latestTimestamp := int64(0)

	for _, tag := range event.Tags.GetAll([]string{"settings"}) {
		if tag.Value() == settingsName && len(tag) > 3 {
			timestamp, err := strconv.ParseInt(tag[3], 10, 64)
			if err != nil {
				log.Printf("%v: error parsing timestamp: %v: %v", event.String(), settingsName, err)

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

func roleRequiredForPosting(event *model.Event) model.Role {
	if settings := getLatestSettingsTag(event, model.RoleRequiredForPostingSettings); settings != nil && len(*settings) > 3 && (model.Role((*settings)[2]) == model.AdminRole || model.Role((*settings)[2]) == model.ModeratorRole) {
		return model.Role((*settings)[2])
	}

	return ""
}

func GetCommunityDefinition(ctx context.Context, hTag string) (*model.Event, error) {
	eventIterator := query.GetStoredEvents(ctx, &model.Subscription{
		Filters: model.Filters{
			model.Filter{
				Kinds: []int{model.CustomIONKindCommunityDefinition, model.CustomIONKindCommunityChangeDefinition},
				Tags:  model.TagMap{}.SetLiterals(model.CustomIONTagCommunity, hTag),
			},
		},
	})

	var (
		lastDefinitionEvent *model.Event
		patches             []*model.Event
	)
	for ev, err := range eventIterator {
		if err != nil {
			return nil, errors.Wrapf(err, "%v: cannot fetch community definition", hTag)
		}
		if ev.Kind == model.CustomIONKindCommunityChangeDefinition {
			patches = append(patches, ev)

			continue
		}
		if lastDefinitionEvent == nil {
			lastDefinitionEvent = ev

			continue
		}
		if ev.CreatedAt > lastDefinitionEvent.CreatedAt {
			lastDefinitionEvent = ev
		}
	}
	if lastDefinitionEvent == nil {
		return nil, errors.Wrapf(ErrNotFound, "%v: community definition not found", hTag)
	}

	tags := applyChangeCommunityPatch(patches, lastDefinitionEvent)
	if tags != nil {
		lastDefinitionEvent.Tags = tags
	}

	return lastDefinitionEvent, nil
}

func applyChangeCommunityPatch(patches []*model.Event, communityDefEvent *model.Event) model.Tags {
	if len(patches) == 0 {
		return nil
	}
	sort.Slice(patches, func(i, j int) bool {
		return patches[i].CreatedAt < patches[j].CreatedAt
	})
	tags := communityDefEvent.Tags
	for _, patch := range patches {
		for _, patchTag := range patch.Tags {
			switch patchTag.Key() {
			case "public":
				if privateTag := tags.GetFirst([]string{"private"}); privateTag != nil {
					tags = removeTag(tags, privateTag)
				}
				tags = tags.AppendUnique(model.Tag{"public"})
			case "private":
				if publicTag := tags.GetFirst([]string{"public"}); publicTag != nil {
					tags = removeTag(tags, publicTag)
				}
				tags = tags.AppendUnique(model.Tag{"private"})
			case "open":
				if closedTag := tags.GetFirst([]string{"closed"}); closedTag != nil {
					tags = removeTag(tags, closedTag)
				}
				tags = tags.AppendUnique(model.Tag{"open"})
			case "closed":
				if openTag := tags.GetFirst([]string{"open"}); openTag != nil {
					tags = removeTag(tags, openTag)
				}
				tags = tags.AppendUnique(model.Tag{"closed"})
			case "name":
				if existingNameTag := tags.GetFirst([]string{"name"}); existingNameTag != nil {
					tags = removeTag(tags, existingNameTag)
				}
				tags = tags.AppendUnique(patchTag)
			case "description":
				if existingDescriptionTag := tags.GetFirst([]string{"description"}); existingDescriptionTag != nil {
					tags = removeTag(tags, existingDescriptionTag)
				}
				tags = tags.AppendUnique(patchTag)
			case "p":
				found := false
				for _, existingPTag := range tags.GetAll([]string{"p"}) {
					if existingPTag.Key() == "p" && existingPTag.Value() == patchTag.Value() && (existingPTag[2] != patchTag[2] || existingPTag[3] != patchTag[3]) {
						tags = removeTag(tags, &existingPTag)
						tags = append(tags, patchTag)
						found = true
					}
				}
				if !found {
					tags = append(tags, patchTag)
				}
			default:
				if patchTag.Key() != model.CustomIONTagCommunity {
					tags = append(tags, patchTag)
				}
			}
		}
	}

	return tags
}

func removeTag(tags []nostr.Tag, tag *nostr.Tag) []nostr.Tag {
	for i, t := range tags {
		if t.Key() == tag.Key() && t.Value() == tag.Value() {
			return append(tags[:i], tags[i+1:]...)
		}
	}

	return tags
}
