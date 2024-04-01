package command

import (
	"context"
	"github.com/gookit/goutil/errorx"
	"github.com/ice-blockchain/subzero/model"
)

var syncQuery func(context.Context, *model.Event) error

func RegisterQuerySyncer(sync func(context.Context, *model.Event) error) {
	syncQuery = sync
}

func AcceptEvent(ctx context.Context, event *model.Event) error {
	//TODO impl consensus

	return errorx.Withf(syncQuery(ctx, event), "failed to sync event to query storage: %#v", event)
}
