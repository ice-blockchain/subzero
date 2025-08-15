// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"context"
	"encoding/json"

	"github.com/cockroachdb/errors"

	"github.com/ice-blockchain/subzero/model"
)

func (ev *eventValidator) validateKindProfileMetadataEvent(ctx context.Context, rules *ruleSet, batch model.Events, e *model.Event) error {
	var parsedContent model.ProfileMetadataContent
	if err := json.Unmarshal([]byte(e.Content), &parsedContent); err != nil {
		return errors.Wrapf(ErrWrongEventParams, "nip-01,nip-24: wrong json fields for: %+v", e)
	}
	if parsedContent.Name == "" {
		return errors.Wrapf(ErrWrongEventParams, "nip-01: there are no required content fields: %+v", e)
	}
	for collectionName, collectionMetadata := range parsedContent.IONContentNFTCollections {
		if collectionName == "" {
			return errors.Wrapf(ErrWrongEventParams, "icip-01: ion_content_nft_collections: collection name cannot be empty: %s", e.ID)
		}
		if collectionMetadata.Address == "" {
			return errors.Wrapf(ErrWrongEventParams, "icip-01: ion_content_nft_collections: collection address cannot be empty for collection '%s': %s", collectionName, e.ID)
		}
		if collectionMetadata.CreatedBy == "" {
			return errors.Wrapf(ErrWrongEventParams, "icip-01: ion_content_nft_collections: created_by cannot be empty for collection '%s': %s", collectionName, e.ID)
		}
	}
	if !rules.SkipKindProfileProofEventsVerify {
		masterKey := e.GetMasterPublicKey()
		nameChanged, username, err := ev.validateProfileMetadataNameChange(ctx, e, masterKey)
		if err != nil {
			return errors.Wrapf(err, "failed to validate profile metadata name change")
		}
		if !nameChanged {
			return nil
		}
		if err := checkProofOfOwnershipBadges(ctx, rules, batch, username, masterKey); err != nil {
			return errors.Wrapf(err, "failed to check proof of ownership for badges for username %s", username)
		}
	}

	return nil
}
