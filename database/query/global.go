// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"log"
	"sync"
	"time"

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

		go globalDB.Client.StartExpiredEventsCleanup(context.Background())
	})
}

func AcceptEvent(ctx context.Context, events *model.Event) error {
	return globalDB.Client.AcceptEvent(ctx, events)
}

func GetStoredEvents(ctx context.Context, subscription *model.Subscription) EventIterator {
	return globalDB.Client.SelectEvents(ctx, subscription)
}

func CountEvents(ctx context.Context, subscription *model.Subscription) (int64, error) {
	return globalDB.Client.CountEvents(ctx, subscription)
}

func (d *dbClient) StartExpiredEventsCleanup(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			reqCtx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
			if err := d.deleteExpiredEvents(reqCtx); err != nil {
				log.Printf("failed to delete expired events: %v", err)
			}

			cancel()
		case <-ctx.Done():
			return
		}
	}
}
