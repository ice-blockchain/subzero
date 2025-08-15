// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"context"
	"encoding/json"

	"github.com/cockroachdb/errors"

	"github.com/ice-blockchain/subzero/model"
)

func (ev *eventValidator) validateKindTextNoteEvent(ctx context.Context, rules *ruleSet, batch model.Events, e *model.Event) error {
	if json.Valid([]byte(e.Content)) {
		return errors.Wrapf(ErrWrongEventParams, "nip-01: content field should be plain text: %q", e.Content)
	}

	if err := validateLabelTags(e); err != nil {
		return errors.Wrap(err, "nip-32: label tags are invalid for event")
	}

	pTags := e.GetTags("p")
	eTags := e.GetTags("e")
	if len(eTags) > 0 {
		for _, tag := range eTags {
			if len(tag) < 2 {
				return errors.Wrap(ErrWrongEventParams, "nip-10: 'e' tag does not contain any event id")
			}
			if len(tag) >= 3 {
				if tag[3] != model.TagMarkerRoot && tag[3] != model.TagMarkerReply && tag[3] != model.TagMarkerMention {
					return errors.Wrapf(ErrWrongEventParams, "nip-10: wrong tag marker param: %v, want root/reply/mention", tag[3])
				}
			}
		}
	}
	if len(pTags) > 0 {
		if len(eTags) == 0 {
			return errors.Wrap(ErrWrongEventParams, "wrong nip-10: no 'e' tags while p tag exist")
		}
		for _, tag := range pTags {
			if len(tag) == 1 {
				return errors.Wrap(ErrWrongEventParams, "nip-10: 'p' tag does not contain any pubkey who is involved in reply thread")
			}
		}
	}
	if err := ev.validatePostCommunityEvent(ctx, e); err != nil {
		return err
	}
	if err := ev.validateWhoCanReplySettings(ctx, rules, batch, e); err != nil {
		return err
	}

	return nil
}
