package query

import (
	"context"
	"sync"

	"github.com/ice-blockchain/subzero/model"
)

var (
	globalDB struct {
		Client *dbClient
		Once   sync.Once
	}
)

func MustInit(url ...string) {
	target := ":memory:"

	if len(url) > 0 {
		target = url[0]
	}

	globalDB.Once.Do(func() {
		globalDB.Client = openDatabase(target, true)
	})
}

func AcceptEvent(ctx context.Context, event *model.Event) error {
	return globalDB.Client.AcceptEvent(ctx, event)
}

func GetStoredEvents(ctx context.Context, subscription *model.Subscription) EventIterator {
	return globalDB.Client.SelectEvents(ctx, subscription)
}
