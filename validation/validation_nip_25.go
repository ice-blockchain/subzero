// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/model"
)

var (
	ErrUsernameProofOfOwnershipFailed = errors.New("username proof of ownership failed")
)

func validateKindReactionToWebsiteEvent(e *model.Event) error {
	if e.Content != "+" && e.Content != "-" && e.Content != "" {
		return errors.Wrapf(ErrWrongEventParams, "nip-25, wrong content value: %+v", e)
	}
	if rTag := e.Tags.GetFirst([]string{"r"}); rTag == nil || rTag.Value() == "" {
		return errors.Wrapf(ErrWrongEventParams, "nip-25, wrong r tag value: %+v", e)
	}

	return nil
}

func (ev *eventValidator) validateProfileMetadataNameChange(ctx context.Context, e *model.Event, masterKey string) (bool, string, error) {
	profileAddress := fmt.Sprintf("%d:%s:", nostr.KindProfileMetadata, masterKey)
	oldProfile, err := ev.getEvent(ctx, profileAddress)
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
