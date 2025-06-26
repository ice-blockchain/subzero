// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"context"
	"encoding/json"

	"github.com/cockroachdb/errors"

	"github.com/ice-blockchain/subzero/model"
)

func (ev *eventValidator) validateKindProfileMetadataEvent(ctx context.Context, e *model.Event, incomingEvents []*model.Event) error {
	if !json.Valid([]byte(e.Content)) {
		return errors.Wrapf(ErrWrongEventParams, "nip-01: content field should be stringified json: %+v", e)
	}
	var parsedContent model.ProfileMetadataContent
	if err := json.Unmarshal([]byte(e.Content), &parsedContent); err != nil {
		return errors.Wrapf(ErrWrongEventParams, "nip-01,nip-24: wrong json fields for: %+v", e)
	}
	if parsedContent.Name == "" || parsedContent.DisplayName == "" {
		return errors.Wrapf(ErrWrongEventParams, "nip-01: there are no required content fields: %+v", e)
	}
	masterKey := e.GetMasterPublicKey()
	nameChanged, username, err := ev.validateProfileMetadataNameChange(ctx, e, masterKey)
	if err != nil {
		return errors.Wrapf(err, "failed to validate profile metadata name change")
	}
	if !nameChanged {
		return nil
	}
	if err := checkProofOfOwnershipBadges(username, masterKey, incomingEvents); err != nil {
		return errors.Wrapf(err, "failed to check proof of ownership for badges for username %s", username)
	}

	return nil
}
