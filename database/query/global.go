// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ice-blockchain/subzero/cfg"
	"github.com/ice-blockchain/subzero/model"
)

var (
	globalDB struct {
		Client *dbClient
		Once   sync.Once
	}
	UsedDatabaseStorage atomic.Uint64
)

type (
	Config struct {
		URL         string   `yaml:"url"`
		ReplicaURLs []string `yaml:"replicas"   validate:"omitempty,dive,url"`
		PrivateKey  string   `yaml:"private-key"`
		RelayURL    string   `yaml:"relay-url" validate:"required,url"`
	}
	Option func(*Config)
)

func WithConfig(cfg *Config) Option {
	return func(in *Config) {
		if cfg == nil {
			return
		}
		if cfg.URL != "" {
			in.URL = cfg.URL
		}
		if cfg.PrivateKey != "" {
			in.PrivateKey = cfg.PrivateKey
		}
		if cfg.RelayURL != "" {
			in.RelayURL = cfg.RelayURL
		}
		if len(cfg.ReplicaURLs) > 0 {
			in.ReplicaURLs = cfg.ReplicaURLs
		}
	}
}

func mustLoadConfig(opts ...Option) *Config {
	if len(opts) == 0 {
		return cfg.MustGet[Config]()
	}

	conf, err := cfg.Get[Config]()
	if err != nil {
		conf = &Config{}
	}

	for _, opt := range opts {
		opt(conf)
	}

	if err := cfg.Validate(conf); err != nil {
		log.Panic(err)
	}

	return conf
}

func MustInit(ctx context.Context, opts ...Option) {
	globalDB.Once.Do(func() {
		conf := mustLoadConfig(opts...)
		globalDB.Client = openDatabase(ctx, conf.URL, true, conf.ReplicaURLs...).
			WithPrivateKey(conf.PrivateKey).
			WithRelayURL(conf.RelayURL)

		go globalDB.Client.StartExpiredEventsCleanup(ctx)
		go globalDB.Client.StartCollectingUsedDatabaseStorage(ctx)

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

func RollbackEvents(ctx context.Context, events ...*model.Event) error {
	return globalDB.Client.RollbackEvents(ctx, events...)
}

func CommitEvents(ctx context.Context, events ...*model.Event) error {
	return globalDB.Client.CommitEvents(ctx, events...)
}

func GetStoredEvents(ctx context.Context, subscription *model.Subscription) EventIterator {
	var filters model.Filters
	if subscription != nil {
		filters = subscription.Filters
	}
	return globalDB.Client.SelectEvents(ctx, filters...)
}

func MarkTokenAsInvalidInEventTags(ctx context.Context, events []*model.Event) error {
	return globalDB.Client.markTokenAsInvalidInEventTags(ctx, events)
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
	ticks := make(chan struct{}, 1)

	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		defer close(ticks)

		for {
			select {
			case <-ticker.C:
				select {
				case ticks <- struct{}{}:
				default:
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	for range ticks {
		deleteCtx, cancel := context.WithTimeout(ctx, time.Minute)
		if err := db.deleteExpiredEvents(deleteCtx); err != nil {
			log.Printf("failed to delete expired events: %v", err)
		}
		cancel()
	}
}

func (db *dbClient) StartCollectingUsedDatabaseStorage(ctx context.Context) {
	ticks := make(chan struct{}, 1)
	ticks <- struct{}{}
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		defer close(ticks)

		for {
			select {
			case <-ticker.C:
				select {
				case ticks <- struct{}{}:
				default:
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	for range ticks {
		queryCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		if usedDatabaseStorage, err := db.queryDatabaseSize(queryCtx); err != nil {
			log.Printf("failed to query database size: %v", err)
		} else {
			UsedDatabaseStorage.Store(usedDatabaseStorage)
		}
		cancel()
	}
}

func CollectDeviceRegistrationEvents(ctx context.Context) EventIterator {
	return globalDB.Client.collectDeviceRegistrationEvents(ctx)
}
