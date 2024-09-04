// SPDX-License-Identifier: ice License 1.0

package command

import (
	"context"

	"github.com/gookit/goutil/errorx"
	"github.com/hashicorp/go-multierror"

	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/storage"
)

func AcceptEvent(ctx context.Context, event *model.Event) error {
	var mErr *multierror.Error
	//TODO impl consensus
	// After consensus pass it to storage
	if sErr := storage.AcceptEvent(ctx, event); sErr != nil {
		mErr = multierror.Append(mErr, errorx.Withf(sErr, "failed to process NIP-94 event"))
	}
	return mErr.ErrorOrNil()
}
