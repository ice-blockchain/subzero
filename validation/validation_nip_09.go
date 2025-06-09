// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"context"

	"github.com/cockroachdb/errors"

	"github.com/ice-blockchain/subzero/model"
)

func validateKindDeletionEvent(ctx context.Context, e *model.Event) error {
	if len(e.Tags) == 0 {
		// Account deletion request.
		return nil
	}

	if eTags, kTags := e.GetTags("e"), e.GetTags("k"); len(eTags) != len(kTags) {
		return errors.Wrapf(ErrWrongEventParams, "nip-09: deletion request should include k tag for the each event: found %d e tags and %d k tags", len(eTags), len(kTags))
	}

	if err := validateDeleteCommunityEvents(ctx, e); err != nil {
		return err
	}

	return nil
}
