// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"
	"github.com/rs/zerolog/log"

	"github.com/ice-blockchain/subzero/model"
)

func (ev *eventValidator) validateWhoCanReplySettings(ctx context.Context, rules *ruleSet, batch model.Events, e *model.Event) error {
	rootPost, err := ev.findRootPost(ctx, e)
	if err != nil {
		return err
	}
	if rootPost == nil || rootPost.GetMasterPublicKey() == e.GetMasterPublicKey() {
		return nil
	}
	settingsTag := getLatestSettingsTag(rootPost, model.WhoCanReplySettings)
	if settingsTag == nil || (settingsTag)[1] != model.WhoCanReplySettings {
		return nil
	}

	values := strings.Split((settingsTag)[2], ",")
	passed := false

	for _, value := range values {
		if passed, err = ev.checkWhoCanReplySettings(ctx, rules, batch, e, value, rootPost, settingsTag); err != nil {
			return err
		}
		if passed {
			break
		}
	}
	if !passed {
		return errors.Wrapf(ErrActionForbidden, "reply can be added only by users with settings %+v for event: %v", settingsTag, e.ID)
	}

	return nil
}

func (ev *eventValidator) checkWhoCanReplySettings(
	ctx context.Context,
	rules *ruleSet,
	batch model.Events,
	e *model.Event,
	value string,
	rootPost *model.Event,
	settingsTag model.Tag,
) (bool, error) {
	switch {
	case value == model.FollowingWhoCanReplySettings:
		return ev.checkFollowingWhoCanReplySettings(ctx, rootPost, e)

	case value == model.MentionWhoCanReplySettings:
		return checkMentionWhoCanReplySettings(rootPost, e)

	case strings.HasPrefix(value, model.BadgeWhoCanReplySettingsPrefix):
		if err := ev.handleTextNoteVerifiedOnlyReply(ctx, rules, batch, e, settingsTag); err != nil {
			return false, err
		}

	default:
		return false, nil
	}

	return true, nil
}

func (ev *eventValidator) checkFollowingWhoCanReplySettings(ctx context.Context, rootPost, e *model.Event) (bool, error) {
	for _, err := range ev.QueryFunc(ctx, model.Filter{
		Authors: []string{rootPost.GetMasterPublicKey()},
		Kinds:   []int{nostr.KindFollowList},
		Tags:    model.TagMap{}.SetLiterals("p", e.GetMasterPublicKey()),
	}) {
		if err != nil {
			return false, err
		}

		return true, nil
	}
	return false, nil
}

func checkMentionWhoCanReplySettings(rootPost, e *model.Event) (bool, error) {
	mentionedPubkeys, err := model.ExtractMentionedPubkeys(rootPost)
	if err != nil {
		return false, err
	}
	for _, pubkey := range mentionedPubkeys {
		if pubkey == e.GetMasterPublicKey() {
			return true, nil
		}
	}

	return false, nil
}

func (ev *eventValidator) findRootPost(ctx context.Context, e *model.Event) (*model.Event, error) {
	filter, err := createRootPostFilter(e)
	if err != nil {
		return nil, err
	}
	if filter == nil {
		return nil, nil
	}
	rootPosts := ev.QueryFunc(ctx, *filter)
	for ev, err := range rootPosts {
		if err != nil {
			return nil, err
		}
		if e.Kind == ev.Kind {
			return ev, nil
		}
	}

	return nil, nil
}

func createRootPostFilter(e *model.Event) (*model.Filter, error) {
	for _, tag := range e.GetTags("a") {
		if !isRootTag(tag) {
			continue
		}
		parts := strings.Split(tag.Value(), ":")
		if len(parts) != 3 {
			return nil, errors.Wrapf(ErrWrongEventParams, "invalid tag value: %v", tag.Value())
		}
		kind, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, err
		}

		return &nostr.Filter{
			Kinds:   []int{kind},
			Authors: []string{parts[1]},
			Tags:    nostr.TagMap{}.SetLiterals("d", parts[2]),
		}, nil
	}
	for _, tag := range e.GetTags("e") {
		if !isRootTag(tag) {
			continue
		}

		return &nostr.Filter{
			IDs:   []string{tag.Value()},
			Kinds: []int{nostr.KindTextNote, nostr.KindArticle, nostr.KindDraftArticle, nostr.KindRepost, model.CustomIONKindEditableTextNote},
		}, nil
	}
	return nil, nil
}

func isRootTag(tag nostr.Tag) bool {
	return len(tag) >= 4 && (tag)[3] == model.TagMarkerRoot
}

func (ev *eventValidator) validatePostCommunityEvent(ctx context.Context, incomingEvent *model.Event) error {
	hTag := incomingEvent.GetTag(model.CustomIONTagCommunity)
	if hTag == nil {
		return nil
	}

	communityDefinitionEvent, err := ev.getCommunityDefinition(ctx, hTag.Value())
	if err != nil {
		return err
	}
	if err := ev.IsUserBanned(ctx, incomingEvent.GetMasterPublicKey(), hTag.Value()); err != nil {
		return errors.Wrapf(err, "user:%v banned", incomingEvent.GetMasterPublicKey())
	}

	requiredRole := roleRequiredForPosting(communityDefinitionEvent)
	replyRole := model.GetCommunityRoleByPubkey(incomingEvent.GetMasterPublicKey(), communityDefinitionEvent)
	if requiredRole == model.ModeratorRole && (replyRole != model.AdminRole && replyRole != model.OwnerRole && replyRole != model.ModeratorRole) {
		return errors.Wrapf(ErrActionForbidden, "only %v can post in this community", cmp.Or(requiredRole, "moderator, admin or owner"))
	} else if requiredRole == model.AdminRole && replyRole != model.OwnerRole && replyRole != model.AdminRole {
		return errors.Wrapf(ErrActionForbidden, "only %v can post in this community", cmp.Or(requiredRole, "admin or owner"))
	} else if requiredRole == model.RegularRole && replyRole == model.RegularRole {
		if err := ev.IsUserPartOfCommunity(ctx, communityDefinitionEvent, incomingEvent.GetMasterPublicKey()); err != nil {
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

func (ev *eventValidator) validateDeleteCommunityEvents(ctx context.Context, e *model.Event) error {
	var ids []string
	for _, eTag := range e.GetTags("e") {
		ids = append(ids, eTag.Value())
	}
	if len(ids) == 0 {
		return nil
	}

	var communityEventsToCheck []*model.Event
	for ev, err := range ev.QueryFunc(ctx, model.Filter{
		IDs:  ids,
		Tags: model.TagMap{}.SetLiterals(model.CustomIONTagCommunity),
	}) {
		if err != nil {
			return errors.Wrap(err, "failed to get stored events")
		}
		communityEventsToCheck = append(communityEventsToCheck, ev)
	}
	for _, event := range communityEventsToCheck {
		if err := ev.validateCommunityDeleteEvent(ctx, event, e); err != nil {
			return errors.Wrap(err, "failed to validate delete event")
		}
	}

	return nil
}

func (ev *eventValidator) validateCommunityDeleteEvent(ctx context.Context, event, deleteEvent *model.Event) error {
	communityDefinitionEvent, err := ev.getCommunityDefinition(ctx, event.GetTag(model.CustomIONTagCommunity).Value())
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

func (ev *eventValidator) IsUserBanned(ctx context.Context, pubkey, communityID string) error {
	eventIterator := ev.QueryFunc(ctx, model.Filter{
		Kinds: []int{model.CustomIONKindCommunityBanUser},
		Tags:  model.TagMap{}.SetLiterals("p", pubkey).SetLiterals(model.CustomIONTagCommunity, communityID),
	})
	for _, err := range eventIterator {
		if err != nil {
			return errors.Wrap(err, "failed to get stored events")
		}

		return errors.Wrap(ErrActionForbidden, "user was banned")
	}

	return nil
}

func (ev *eventValidator) IsUserPartOfCommunity(ctx context.Context, communityDefinitionEvent *model.Event, masterPubkey string) error {
	eventIterator := ev.QueryFunc(ctx, model.Filter{
		Kinds: []int{model.CustomIONKindCommunityJoin},
		Tags: model.TagMap{}.
			SetLiterals("p", masterPubkey).
			SetLiterals(model.CustomIONTagCommunity, communityDefinitionEvent.GetHTag()),
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

func getLatestSettingsTag(event *model.Event, settingsName string) model.Tag {
	var latestSettingsTag model.Tag
	latestTimestamp := nostr.Timestamp(0)

	for _, tag := range event.Tags.GetAll([]string{"settings"}) {
		if tag.Value() == settingsName && len(tag) > 3 {
			timestamp, err := nostr.ParseTimestamp(tag[3])
			if err != nil {
				log.Error().Err(err).Str("event", event.String()).Str("settings_name", settingsName).Msg("error parsing timestamp")

				continue
			}

			if timestamp.After(latestTimestamp) {
				latestSettingsTag = tag
				latestTimestamp = timestamp
			}
		}
	}

	return latestSettingsTag
}

func isCommunityCommentsEnabled(event *model.Event) bool {
	if settings := getLatestSettingsTag(event, "comments_enabled"); len(settings) > 3 {
		val, err := strconv.ParseBool(settings[2])
		if err != nil {
			return false
		}

		return val
	}

	return true
}

func roleRequiredForPosting(event *model.Event) model.Role {
	if settings := getLatestSettingsTag(event, model.RoleRequiredForPostingSettings); len(settings) > 3 && (model.Role(settings[2]) == model.AdminRole || model.Role(settings[2]) == model.ModeratorRole) {
		return model.Role(settings[2])
	}

	return ""
}

func (ev *eventValidator) getCommunityDefinition(ctx context.Context, hTag string) (*model.Event, error) {
	eventIterator := ev.QueryFunc(ctx, model.Filter{
		Kinds: []int{model.CustomIONKindCommunityDefinition, model.CustomIONKindCommunityChangeDefinition},
		Tags:  model.TagMap{}.SetLiterals(model.CustomIONTagCommunity, hTag),
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
		if ev.CreatedAt.After(lastDefinitionEvent.CreatedAt) {
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
	slices.SortStableFunc(patches, func(a, b *model.Event) int {
		if a.CreatedAt.Before(b.CreatedAt) {
			return -1
		}
		if a.CreatedAt.After(b.CreatedAt) {
			return 1
		}

		return 0
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

func validateSettingsTag(e *model.Event, tag nostr.Tag) error {
	if hasReplyTag(e) {
		return errors.Wrapf(ErrWrongEventParams, "reply events cannot set settings: %+v", tag)
	}

	if len(tag) < 4 {
		return errors.Wrapf(ErrWrongEventParams, "settings tag is incomplete: %+v", tag)
	}
	settingType := tag[1]
	value := tag[2]
	timestamp := tag[3]
	if _, err := nostr.ParseTimestamp(timestamp); err != nil {
		return errors.Wrapf(err, "invalid timestamp in settings tag: %+v", tag)
	}
	switch settingType {
	case model.CommentsEnabledSettings:
		if e.Kind != model.CustomIONKindCommunityDefinition && e.Kind != model.CustomIONKindCommunityChangeDefinition {
			return errors.Wrapf(ErrWrongEventParams, "comments_enabled can be set only for 31750 kind: %+v", tag)
		}
		if value != "true" && value != "false" {
			return errors.Wrapf(ErrWrongEventParams, "comments_enabled must be true or false: %+v", tag)
		}
	case model.RoleRequiredForPostingSettings:
		if e.Kind != model.CustomIONKindCommunityDefinition && e.Kind != model.CustomIONKindCommunityChangeDefinition {
			return errors.Wrapf(ErrWrongEventParams, "role_required_for_posting can be set only for 31750 kind: %+v", tag)
		}
		if value != string(model.AdminRole) && value != string(model.ModeratorRole) && value != "" {
			return errors.Wrapf(ErrWrongEventParams, "role_required_for_posting must be admin or moderator: %+v", tag)
		}
	case model.WhoCanReplySettings:
		accept := map[int]struct{}{
			nostr.KindTextNote:                  {},
			nostr.KindArticle:                   {},
			nostr.KindDraftArticle:              {},
			model.CustomIONKindEditableTextNote: {},
		}
		if _, ok := accept[e.Kind]; !ok {
			return errors.Wrapf(ErrWrongEventParams, "who_can_reply cannot be set for kind: %d: %+v", e.Kind, tag)
		}
		values := strings.Split(value, ",")
		for _, v := range values {
			if !strings.HasPrefix(v, "following") && !strings.HasPrefix(v, "mentioned") && !strings.HasPrefix(v, "badge|") {
				return errors.Wrapf(ErrWrongEventParams, "who_can_reply contains invalid value: %s", v)
			}
			if strings.HasPrefix(v, "badge|") {
				parts := strings.Split(v, "|")
				if len(parts) != 2 {
					return errors.Wrapf(ErrWrongEventParams, "invalid badge format in who_can_reply: %s", v)
				}
			}
		}
	default:
		return errors.Wrapf(ErrUnsupportedTag, "unsupported settings tag: %s", settingType)
	}

	return nil
}

func hasReplyTag(e *model.Event) bool {
	for _, tag := range e.GetTags("e") {
		if len(tag) >= 4 && tag[3] == model.TagMarkerReply {
			return true
		}
	}
	for _, tag := range e.GetTags("a") {
		if len(tag) >= 4 && tag[3] == model.TagMarkerReply {
			return true
		}
	}

	return false
}

func (ev *eventValidator) handleTextNoteVerifiedOnlyReply(ctx context.Context, _ *ruleSet, batch model.Events, e *model.Event, settingsTag model.Tag) error {
	ephemeralAckEvents, err := model.ParseEphemeralEmbeddingEvents(batch...)
	if err != nil {
		return errors.Wrap(err, "failed to parse ephemeral ack events")
	}

	var acks []*model.EphemeralEmbeddingEvent
	var hasAck bool
	if acks, hasAck = ephemeralAckEvents[e.Address()]; !hasAck || len(acks) == 0 {
		return ev.checkReplyPermissions(ctx, settingsTag, e, nil)
	}

	return ev.checkReplyPermissions(ctx, settingsTag, e, acks)
}

func checkBadgeInEphemeralEvents(acks []*model.EphemeralEmbeddingEvent, badgePubkey, badgeDTag, userPubkey string) bool {
	if len(acks) == 0 {
		return false
	}

	var badgeDefinitionFound, badgeAwardFound bool
	expectedBadgeRef := fmt.Sprintf("%d:%s:%s", nostr.KindBadgeDefinition, badgePubkey, badgeDTag)

	for _, ack := range acks {
		if ack.ContentEvent == nil {
			continue
		}

		switch ack.ContentEvent.Kind {
		case nostr.KindBadgeDefinition:
			dTag := ack.ContentEvent.Tags.GetD()
			if ack.ContentEvent.GetMasterPublicKey() == badgePubkey &&
				dTag == badgeDTag {
				badgeDefinitionFound = true
			}

		case nostr.KindBadgeAward:
			aTags := ack.ContentEvent.GetTags("a")
			pTags := ack.ContentEvent.GetTags("p")

			for _, aTag := range aTags {
				if len(aTag) >= 2 {
					if aTag[1] == expectedBadgeRef {
						for _, pTag := range pTags {
							if len(pTag) >= 2 {
								if pTag[1] == userPubkey {
									badgeAwardFound = true
									break
								}
							}
						}
						if badgeAwardFound {
							break
						}
					}
				}
			}
		}

		if badgeDefinitionFound && badgeAwardFound {
			return true
		}
	}

	return badgeDefinitionFound && badgeAwardFound
}

func (ev *eventValidator) hasUserBadge(ctx context.Context, badgePubkey, badgeDTag, userPubkey string) bool {
	it := ev.QueryFunc(ctx, model.Filter{
		Authors: []string{badgePubkey},
		Kinds:   []int{nostr.KindBadgeDefinition},
		Tags:    nostr.TagMap{}.SetLiterals("d", badgeDTag),
		Limit:   1,
	})

	badgeDefinitionFound := false
	for badgeDefinition, err := range it {
		if err != nil {
			return false
		}
		if badgeDefinition.Kind == nostr.KindBadgeDefinition &&
			badgeDefinition.GetMasterPublicKey() == badgePubkey &&
			badgeDefinition.Tags.GetD() == badgeDTag {
			badgeDefinitionFound = true
			break
		}
	}

	if !badgeDefinitionFound {
		return false
	}

	badgeATagRef := fmt.Sprintf("%d:%s:%s", nostr.KindBadgeDefinition, badgePubkey, badgeDTag)
	it = ev.QueryFunc(ctx, model.Filter{
		Authors: []string{badgePubkey},
		Kinds:   []int{nostr.KindBadgeAward},
		Tags:    nostr.TagMap{}.SetLiterals("a", badgeATagRef).SetLiterals("p", userPubkey),
		Limit:   1,
	})

	for badgeAward, err := range it {
		if err != nil {
			return false
		}
		if badgeAward.Kind == nostr.KindBadgeAward {
			if aTag := badgeAward.GetTag("a"); aTag != nil && aTag.Value() == badgeATagRef {
				if pTag := badgeAward.GetTag("p"); pTag != nil && pTag.Value() == userPubkey {
					return true
				}
			}
		}
	}

	return false
}

func (ev *eventValidator) checkReplyPermissions(ctx context.Context, settingsTag model.Tag, e *model.Event, acks []*model.EphemeralEmbeddingEvent) error {
	if len(settingsTag) < 3 {
		return errors.New("invalid settings tag format")
	}
	settingsConfig := (settingsTag)[2]
	settings := strings.Split(settingsConfig, ",")

	userPubkey := e.GetMasterPublicKey()

	for _, setting := range settings {
		if strings.HasPrefix(setting, model.BadgeWhoCanReplySettingsPrefix) {
			badgeRef := strings.TrimPrefix(setting, model.BadgeWhoCanReplySettingsPrefix)
			badgeRef = strings.TrimPrefix(badgeRef, "badge|")
			parts := strings.Split(badgeRef, ":")
			if len(parts) != 3 {
				continue
			}

			badgePubkey := parts[1]
			dtag := parts[2]
			if len(acks) > 0 {
				if checkBadgeInEphemeralEvents(acks, badgePubkey, dtag, userPubkey) {
					return nil
				}
			}
			if ev.hasUserBadge(ctx, badgePubkey, dtag, userPubkey) {
				return nil
			}
		}
	}

	return errors.New("comments are disabled for root post")
}
