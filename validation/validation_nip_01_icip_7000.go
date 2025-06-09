// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"context"
	"strconv"

	"github.com/cockroachdb/errors"

	"github.com/ice-blockchain/subzero/model"
)

func validateTextNote(ctx context.Context, e *model.Event, incomingEvents ...*model.Event) error {
	richText := e.GetTag(model.CustomIONTagRichText)
	if richText != nil && len(e.Content) > 0 {
		return errors.Wrap(ErrWrongEventParams, "rich text tag is set, but content is not empty")
	}
	if len(e.Content) == 0 && richText == nil {
		pubAt := e.GetTag("published_at").Value()
		if val, err := strconv.ParseInt(pubAt, 10, 64); err != nil || val == int64(e.CreatedAt) {
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

		if err := validatePostCommunityEvent(ctx, e); err != nil {
			return errors.Wrap(err, "validate post community event")
		}
		if err := validateWhoCanReplySettings(ctx, e, incomingEvents...); err != nil {
			return errors.Wrap(err, "validate who can reply settings")
		}
	}

	return nil
}
