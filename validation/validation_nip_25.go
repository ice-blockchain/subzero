// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"context"
	"encoding/json"

	"github.com/cockroachdb/errors"

	"github.com/ice-blockchain/subzero/model"
)

var (
	ErrUsernameProofOfOwnershipFailed = errors.New("username proof of ownership failed")
)

func validateKindReactionToWebsiteEvent(e *model.Event) error {
	if e.Content != "+" && e.Content != "-" && e.Content != "" {
		return errors.Wrap(ErrWrongEventParams, "nip-25: wrong content value")
	}
	if rTag := e.GetTag("r").Value(); rTag == "" {
		return errors.Wrap(ErrWrongEventParams, "nip-25: 'r' tag is missing or empty")
	}

	return nil
}

func (ev *eventValidator) validateProfileMetadataNameChange(ctx context.Context, e *model.Event) (bool, string, error) {
	oldProfile, err := ev.getEvent(ctx, e.Address())
	if err != nil {
		return false, "", errors.Wrapf(err, "[proof-of-ownership] failed to get old profile metadata")
	}
	var oldProfileMetadata model.ProfileMetadataContent
	var newProfileMetadata model.ProfileMetadataContent
	oldProfileFound := oldProfile != nil && oldProfile.ID != e.ID
	if oldProfileFound {
		if err := json.Unmarshal([]byte(oldProfile.Content), &oldProfileMetadata); err != nil {
			return false, "", errors.Wrapf(err, "[proof-of-ownership] failed to unmarshal old profile metadata")
		}
	}
	if err := json.Unmarshal([]byte(e.Content), &newProfileMetadata); err != nil {
		return false, "", errors.Wrapf(err, "[proof-of-ownership] failed to unmarshal new profile metadata")
	}
	if !oldProfileFound || oldProfileMetadata.Name != newProfileMetadata.Name {
		return true, newProfileMetadata.Name, nil
	}

	return false, "", nil
}
