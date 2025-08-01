// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"context"
	"encoding/json"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/model"
)

const (
	IONNFTCollectionName = "ion"
)

func (ev *eventValidator) validateTextNote(ctx context.Context, e *model.Event, incomingEvents ...*model.Event) error {
	richText := e.GetTag(model.CustomIONTagRichText)
	if richText != nil && len(e.Content) > 0 {
		return errors.Wrap(ErrWrongEventParams, "rich text tag is set, but content is not empty")
	}
	if len(e.Content) == 0 && richText == nil {
		pubAt := e.GetTag("published_at").Value()
		if val, err := nostr.ParseTimestamp(pubAt); err != nil || val.Equal(e.CreatedAt) {
			return errors.Wrap(ErrWrongEventParams, "content is empty or too short")
		}
		for _, tag := range e.Tags {
			switch tag.Key() {
			case "a", model.CustomIONTagOnBehalfOf, "d", "e", "published_at":
			default:
				return errors.Wrapf(ErrWrongEventParams, "tag %q is not allowed", tag.Key())
			}
		}
		// This is a `soft delete`, accept empty content.
	} else {
		if err := ev.validatePostCommunityEvent(ctx, e); err != nil {
			return errors.Wrap(err, "validate post community event")
		}
		if err := ev.validateWhoCanReplySettings(ctx, e, incomingEvents...); err != nil {
			return errors.Wrap(err, "validate who can reply settings")
		}
		if err := ev.validateRootContentNFTCollections(ctx, e); err != nil {
			return errors.Wrap(err, "validate root content NFT collections")
		}
	}

	return nil
}

func (ev *eventValidator) validateRootContentNFTCollections(ctx context.Context, e *model.Event) error {
	if e.IsComment() || e.IsStory() || e.IsCommunityPost() {
		return nil
	}
	if e.Kind != nostr.KindTextNote && e.Kind != nostr.KindArticle && e.Kind != model.CustomIONKindEditableTextNote {
		return nil
	}
	var profileMetadata *model.Event
	queryIterator := ev.QueryFunc(ctx, model.Filter{
		Authors: []string{e.GetMasterPublicKey()},
		Kinds:   []int{nostr.KindProfileMetadata},
		Limit:   1,
	})
	for event, err := range queryIterator {
		if err != nil {
			return errors.Wrapf(err, "failed to query profile metadata for user %s", e.GetMasterPublicKey())
		}
		if event != nil {
			profileMetadata = event

			break
		}
	}

	if profileMetadata == nil {
		return errors.Wrapf(ErrActionForbidden,
			"profile metadata not found for user %s creating root %d content",
			e.GetMasterPublicKey(), e.Kind)
	}
	var parsedContent model.ProfileMetadataContent
	if err := json.Unmarshal([]byte(profileMetadata.Content), &parsedContent); err != nil {
		return errors.Wrapf(ErrWrongEventParams, "invalid profile metadata content for user %s: %v", e.GetMasterPublicKey(), err)
	}
	if len(parsedContent.IONContentNFTCollections) == 0 {
		return errors.Wrapf(ErrActionForbidden,
			"user %s cannot create root %d content without ion_content_nft_collections in profile",
			e.GetMasterPublicKey(), e.Kind)
	}
	if _, exists := parsedContent.IONContentNFTCollections[IONNFTCollectionName]; !exists {
		return errors.Wrapf(ErrActionForbidden,
			"user %s cannot create root %d content: user doesn't have ion collection in profile",
			e.GetMasterPublicKey(), e.Kind)
	}

	return nil
}
