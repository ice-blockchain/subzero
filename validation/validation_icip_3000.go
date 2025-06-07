// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/ice-blockchain/subzero/model"
)

func validateCustomIONKindCommunityDefinitionEvent(ctx context.Context, e *model.Event) error {
	hTag := e.GetTag(model.CustomIONTagCommunity)
	if hTag == nil {
		return errors.Wrap(ErrWrongEventParams, "community must have h tag")
	}

	pTags := e.GetTags("p")
	for _, tag := range pTags {
		if len(tag) < 4 || (tag[3] != string(model.ModeratorRole) && tag[3] != string(model.AdminRole) && tag[3] != "") {
			return errors.Wrapf(ErrWrongEventParams, "p tag must specify a valid role (moderator or admin): %v", tag)
		}
	}

	if e.GetTag("open") != nil && e.GetTag("closed") != nil {
		return errors.Wrap(ErrWrongEventParams, "community cannot be open and closed at the same time")
	}

	if e.GetTag("public") != nil && e.GetTag("private") != nil {
		return errors.Wrap(ErrWrongEventParams, "community cannot be public and private at the same time")
	}

	for _, aTag := range e.GetTags("a") {
		if splitted := strings.Split(aTag.Value(), ":"); len(splitted) != 3 || splitted[0] != strconv.Itoa(model.CustomIONKindCommunityDefinition) {
			return errors.Wrapf(ErrWrongEventParams, "community ownership must have a valid a tag: %v", aTag)
		}
	}

	// Here we have two possible cases:
	// - CustomIONKindCommunityDefinition -- addressable community definition, only owner can create/modify it.
	// - CustomIONKindCommunityChangeDefinition -- regular event, with some updates to the already existing community definition, moderators/admins can send it.
	communityDefinitionEvent, err := GetCommunityDefinition(ctx, hTag.Value())
	if e.Kind == model.CustomIONKindCommunityDefinition {
		// Must not exist OR must be owned by the same pubkey/master pubkey.
		if err == nil {
			// Existing community definition found, check if the owner is the same.
			if communityDefinitionEvent.GetMasterPublicKey() != e.GetMasterPublicKey() {
				return errors.Wrap(ErrActionForbidden, "community already exists")
			}
		} else if errors.Is(err, ErrNotFound) {
			// Not found, allow the creation.
			return nil
		}
		return err
	}
	// Change definition event, community must exist.
	if err != nil {
		return err
	}
	authorRole := model.GetCommunityRoleByPubkey(e.GetMasterPublicKey(), communityDefinitionEvent)
	if authorRole == model.RegularRole {
		return errors.Wrap(ErrActionForbidden, "only admin, owner or moderator can change community definition")
	}
	if authorRole == model.ModeratorRole {
		for _, pTag := range pTags {
			if pTag.Key() == "p" {
				if model.Role(pTag[3]) == model.AdminRole {
					return errors.Wrap(ErrActionForbidden, "moderator can't promote user to admin")
				}
				for _, tag := range communityDefinitionEvent.Tags.GetAll([]string{"p"}) {
					if tag.Key() == "p" && tag.Value() == pTag.Value() && model.Role(tag[3]) == model.AdminRole {
						return errors.Wrap(ErrActionForbidden, "moderator can't demote admin")
					}
				}
			}
		}
		if name := e.GetTag("name"); name != nil {
			return errors.Wrap(ErrActionForbidden, "moderator can't change the name of the community")
		}
		if description := e.GetTag("description"); description != nil {
			return errors.Wrap(ErrActionForbidden, "moderator can't change the description of the community")
		}
		if closed := e.GetTag("closed"); closed != nil {
			return errors.Wrap(ErrActionForbidden, "moderator can't change the open/closed status of the community")
		}
		if open := e.GetTag("open"); open != nil {
			return errors.Wrap(ErrActionForbidden, "moderator can't change the open/closed status of the community")
		}
		if public := e.GetTag("public"); public != nil {
			return errors.Wrap(ErrActionForbidden, "moderator can't change the public/private status of the community")
		}
		if private := e.GetTag("private"); private != nil {
			return errors.Wrap(ErrActionForbidden, "moderator can't change the public/private status of the community")
		}
		if imeta := e.GetTag("imeta"); imeta != nil {
			return errors.Wrap(ErrActionForbidden, "moderator can't change the picture of the community")
		}
		if settings := e.Tags.GetAll([]string{"settings"}); settings != nil {
			return errors.Wrap(ErrActionForbidden, "moderator can't change the settings of the community")
		}
	}

	return nil
}

func validateCustomIONKindCommunityJoinEvent(ctx context.Context, e *model.Event) error {
	var (
		hTag             = e.GetTag(model.CustomIONTagCommunity)
		authorizationTag = e.GetTag("authorization")
	)
	if hTag == nil {
		return errors.Wrapf(ErrWrongEventParams, "community join must have h tag: %+v", e)
	}
	if authorizationTag != nil {
		var parsedContent model.Event
		if err := json.Unmarshal([]byte(authorizationTag.Value()), &parsedContent); err != nil {
			return errors.Wrapf(ErrWrongEventParams, "wrong authorization content: %+v", e)
		}
		if parsedContent.Kind != model.CustomIONKindCommunityJoin {
			return errors.Wrapf(ErrWrongEventParams, "wrong authorization content kind: %+v", e)
		}
		expirationTag := parsedContent.GetTag("expiration")
		if expirationTag == nil {
			return errors.Wrapf(ErrWrongEventParams, "community join must have an expiration tag for authorization event: %+v", e)
		}
		expirationTime, err := strconv.ParseInt(expirationTag.Value(), 10, 64)
		if err != nil {
			return errors.Wrapf(ErrWrongEventParams, "wrong expiration tag value, %v", err.Error())
		}
		currentTime := time.Now().Unix()
		if currentTime > expirationTime {
			return errors.Wrapf(ErrWrongEventParams, "authorization event has expired: %v", expirationTime)
		}
		if ok, err := parsedContent.CheckSignature(); err != nil || !ok {
			return errors.Wrapf(ErrWrongEventParams, "wrong authorization signature: %v", err.Error())
		}
	}
	communityDefinitionEvent, err := GetCommunityDefinition(ctx, hTag.Value())
	if err != nil {
		return err
	}
	openTag := communityDefinitionEvent.GetTag("open")
	publicTag := communityDefinitionEvent.GetTag("public")
	if openTag != nil && publicTag != nil && e.GetTag("authorization") != nil {
		return errors.Wrap(ErrWrongEventParams, "wrong join event, authorization tag is not required for public open community")
	}
	if closedTag := communityDefinitionEvent.GetTag("closed"); closedTag != nil {
		if authorRole := model.GetCommunityRoleByPubkey(e.GetMasterPublicKey(), communityDefinitionEvent); authorRole != model.RegularRole {
			return nil
		}
		if authorizationTag == nil {
			return errors.Wrap(ErrActionForbidden, "can't join closed community")
		}
		var parsedAuthorizationEvent model.Event
		if err := json.Unmarshal([]byte(authorizationTag.Value()), &parsedAuthorizationEvent); err != nil {
			return errors.Wrap(ErrActionForbidden, "wrong authorization event")
		}
		if err := Validate(ctx, &parsedAuthorizationEvent); err != nil {
			return err
		}
		if authorizationRole := model.GetCommunityRoleByPubkey(parsedAuthorizationEvent.GetMasterPublicKey(), communityDefinitionEvent); authorizationRole == model.RegularRole {
			return errors.Wrap(ErrActionForbidden, "user not authorized to join this community")
		}
	}

	return nil
}

func validateCustomIONKindCommunityOwnershipTransferringEvent(ctx context.Context, e *model.Event) error {
	var (
		aTags         = e.Tags.GetAll([]string{"a"})
		hTag          = e.GetTag(model.CustomIONTagCommunity)
		pTags         = e.Tags.GetAll([]string{"p"})
		expirationTag = e.GetTag("expiration")
		currentTime   = time.Now().Unix()
	)
	for _, aTag := range aTags {
		if len(aTag) < 2 {
			return errors.Wrapf(ErrWrongEventParams, "community ownership must have a valid a tag: %+v", e)
		}
		if splitted := strings.Split(aTag.Value(), ":"); len(splitted) != 3 || splitted[0] != fmt.Sprint(model.CustomIONKindCommunityDefinition) {
			return errors.Wrapf(ErrWrongEventParams, "community ownership must have a valid a tag: %+v", e)
		}
	}
	if hTag == nil {
		return errors.Wrapf(ErrWrongEventParams, "community ownership must have a valid h tag: %+v", e)
	}
	if len(pTags) == 0 {
		return errors.Wrapf(ErrWrongEventParams, "community ownership must have at least one p tag: %+v", e)
	}
	if expirationTag == nil {
		return errors.Wrapf(ErrWrongEventParams, "community ownership must have an expiration tag: %+v", e)
	}
	expirationTime, err := strconv.ParseInt(expirationTag.Value(), 10, 64)
	if err != nil {
		return errors.Wrapf(ErrWrongEventParams, "wrong expiration tag value: %+v", e)
	}
	if currentTime > expirationTime {
		return errors.Wrapf(ErrWrongEventParams, "community ownership transferring event has expired: %+v", e)
	}
	communityDefinitionEvent, err := GetCommunityDefinition(ctx, hTag.Value())
	if err != nil {
		return err
	}
	if authorRole := model.GetCommunityRoleByPubkey(e.GetMasterPublicKey(), communityDefinitionEvent); authorRole != model.OwnerRole {
		return errors.Wrap(ErrActionForbidden, "only owner of the community can transfer ownership")
	}

	return nil
}

func validateCustomIONKindCommunityBanUserEvent(ctx context.Context, e *model.Event) error {
	var (
		hTag  = e.GetTag(model.CustomIONTagCommunity)
		pTags = e.Tags.GetAll([]string{"p"})
	)
	if hTag == nil {
		return errors.Wrap(ErrWrongEventParams, "community ban must have h tag")
	}
	if len(pTags) == 0 {
		return errors.Wrap(ErrWrongEventParams, "community ban must have at least one p tag")
	}
	communityDefinitionEvent, err := GetCommunityDefinition(ctx, hTag.Value())
	if err != nil {
		return err
	}
	authorRole := model.GetCommunityRoleByPubkey(e.GetMasterPublicKey(), communityDefinitionEvent)
	if authorRole == model.RegularRole {
		return errors.Wrap(ErrActionForbidden, "only admin, owner or moderator can ban user")
	}
	for _, pTag := range pTags {
		if pTag.Key() == "p" {
			if pTag.Value() == e.GetMasterPublicKey() {
				return errors.Wrap(ErrActionForbidden, "admin/moderator can't ban himself")
			}
			if pTag.Value() == communityDefinitionEvent.GetMasterPublicKey() {
				return errors.Wrap(ErrActionForbidden, "owner of the community can't be banned")
			}
			toBanRole := model.GetCommunityRoleByPubkey(pTag.Value(), communityDefinitionEvent)
			if toBanRole == model.AdminRole && authorRole == model.ModeratorRole {
				return errors.Wrap(ErrActionForbidden, "moderator can't ban admin")
			}
		}
	}

	return nil
}
