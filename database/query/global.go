// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/ice-blockchain/subzero/cfg"
	"github.com/ice-blockchain/subzero/model"
)

var (
	globalDB struct {
		Client *dbClient
		Once   sync.Once
	}
	globalConfig *config
)

type (
	config struct {
		URL        string `yaml:"url"`
		PrivateKey string `yaml:"private_key"`
	}
)

func MustInit(ctx context.Context) {
	globalDB.Once.Do(func() {
		globalConfig = cfg.MustGet[config]()
		globalDB.Client = openDatabase(globalConfig.URL, true).
			WithPrivateKey(globalConfig.PrivateKey)

		go globalDB.Client.StartExpiredEventsCleanup(ctx)
		go func() {
			<-ctx.Done()
			globalDB.Client.Close()
			globalDB.Once = sync.Once{}
		}()
	})
}

func AcceptEvents(ctx context.Context, events ...*model.Event) error {
	return globalDB.Client.AcceptEvents(ctx, events...)
}

func GetStoredEvents(ctx context.Context, subscription *model.Subscription) EventIterator {
	return globalDB.Client.SelectEvents(ctx, subscription)
}

func CountEvents(ctx context.Context, subscription *model.Subscription) (int64, error) {
	return globalDB.Client.CountEvents(ctx, subscription)
}

func (db *dbClient) StartExpiredEventsCleanup(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			reqCtx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
			if err := db.deleteExpiredEvents(reqCtx); err != nil {
				log.Printf("failed to delete expired events: %v", err)
			}

			cancel()
		case <-ctx.Done():
			return
		}
	}
}
