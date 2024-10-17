// SPDX-License-Identifier: ice License 1.0

package command

import (
	"context"

	"github.com/hashicorp/go-multierror"

	"github.com/ice-blockchain/subzero/model"
)

func AcceptEvent(ctx context.Context, event *model.Event) error {
	var mErr *multierror.Error
	//TODO impl consensus
	return mErr.ErrorOrNil()
}
