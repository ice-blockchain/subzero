// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"fmt"
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
		PrivateKey string `yaml:"private-key"`
		RelayURL   string `yaml:"relay-url" validate:"required,url"`
	}
)

func MustInit(ctx context.Context, port string) {
	// TODO: take from config.
	var portValue string
	if port != "" {
		portValue = port
	}
	globalDB.Once.Do(func() {
		globalConfig = cfg.MustGet[config]()

		cfg := Config{
			Storage: StorageCfg{
				Credentials: struct {
					User     string `yaml:"user"`
					Password string `yaml:"password"`
				}{
					User:     "root",
					Password: "pass",
				},
				Timeout:    "30s",
				PrimaryURL: fmt.Sprintf("postgresql://root:pass@localhost:%v/subzero", portValue),
				ReplicaURLs: []string{
					fmt.Sprintf("postgresql://root:pass@localhost:%v/subzero", portValue),
				},
				RunDDL:       true,
				IgnoreGlobal: false,
			},
		}

		globalDB.Client = openPostgresDatabase(&cfg, true).
			WithPrivateKey(globalConfig.PrivateKey).
			WithRelayURL(globalConfig.RelayURL)

		// TODO:
		// if err := globalDB.Client.Ping(); err != nil {
		// 	log.Printf("can't ping the database: %v", err)

		// globalDB.Client.Close()
		// globalDB.Once = sync.Once{}
		// }

		go globalDB.Client.StartExpiredEventsCleanup(ctx)
		go func() {
			<-ctx.Done()
			globalDB.Client.Close()
			globalDB.Once = sync.Once{}
		}()
	})
}

func RegisterExpiredEventsProcessor(proc func(ctx context.Context, events ...*model.Event) error) {
	notifyExpiredEvents = proc
}

func AcceptEvents(ctx context.Context, events ...*model.Event) error {
	return globalDB.Client.AcceptEvents(ctx, events...)
}

func GetStoredEvents(ctx context.Context, subscription *model.Subscription) EventIterator {
	var filters model.Filters
	if subscription != nil {
		filters = subscription.Filters
	}
	return globalDB.Client.SelectEvents(ctx, filters...)
}

func CountEvents(ctx context.Context, subscription *model.Subscription) (int64, error) {
	var filters model.Filters
	if subscription != nil {
		filters = subscription.Filters
	}
	return globalDB.Client.CountEvents(ctx, filters...)
}

func CountGroupedEventReactions(ctx context.Context, subscription *model.Subscription) (string, error) {
	var filters model.Filters
	if subscription != nil {
		filters = subscription.Filters
	}
	return globalDB.Client.CountGroupedEventReactions(ctx, filters...)
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
