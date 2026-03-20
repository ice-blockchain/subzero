// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/ice-blockchain/subzero/appcontext"
	"github.com/ice-blockchain/subzero/cfg"
	"github.com/ice-blockchain/subzero/model"
)

var (
	globalDB struct {
		Client  *dbClient
		Tracker *statusTracker
		Once    sync.Once
	}
	UsedDatabaseStorage atomic.Uint64
)

type (
	Config struct {
		Username                 string        `yaml:"username,omitempty"`
		Password                 string        `yaml:"password,omitempty"`
		PrivateKey               string        `yaml:"private-key"          validate:"required"`
		RelayURL                 string        `yaml:"relay-url"            validate:"required,url"`
		WriteURLs                []string      `yaml:"write-urls"`
		ReadURLs                 []string      `yaml:"read-urls"`
		PeriodicSelfTestInterval time.Duration `yaml:"periodic-self-test-interval"`
		RunDDL                   bool          `yaml:"run-ddl"`
		DisableSelfTest          bool          `yaml:"disable-self-test"`
	}
	Option func(*Config)
)

func WithConfig(cfg *Config) Option {
	return func(in *Config) {
		if cfg == nil {
			return
		}
		if cfg.Username != "" {
			in.Username = cfg.Username
		}
		if cfg.Password != "" {
			in.Password = cfg.Password
		}
		if cfg.PrivateKey != "" {
			in.PrivateKey = cfg.PrivateKey
		}
		if cfg.RelayURL != "" {
			in.RelayURL = cfg.RelayURL
		}
		if len(cfg.ReadURLs) > 0 {
			in.ReadURLs = cfg.ReadURLs
		}
		if len(cfg.WriteURLs) > 0 {
			in.WriteURLs = cfg.WriteURLs
		}
		if cfg.PeriodicSelfTestInterval != 0 {
			in.PeriodicSelfTestInterval = cfg.PeriodicSelfTestInterval
		}
		in.RunDDL = cfg.RunDDL
		in.DisableSelfTest = cfg.DisableSelfTest
	}
}

func createPgURL(username, password, target string) (string, error) {
	if !strings.HasPrefix(target, "postgres://") && !strings.HasPrefix(target, "postgresql://") {
		target = "postgres://" + target
	}

	parsed, err := url.Parse(target)
	if err != nil {
		return "", err
	}

	if parsed.User.Username() == "" {
		parsed.User = url.UserPassword(username, password)
	}

	return parsed.String(), nil
}

func mustLoadConfig(opts ...Option) *Config {
	conf, err := cfg.Get[Config]()
	if err != nil {
		conf = &Config{}
	}

	for _, opt := range opts {
		opt(conf)
	}
	if conf.PeriodicSelfTestInterval == 0 {
		conf.PeriodicSelfTestInterval = 1 * time.Minute
	}

	for i := range conf.WriteURLs {
		conf.WriteURLs[i], err = createPgURL(conf.Username, conf.Password, conf.WriteURLs[i])
		if err != nil {
			log.Panic().Err(err).Str("url", conf.WriteURLs[i]).Msg("failed to create write URL")
		}
	}
	for i := range conf.ReadURLs {
		conf.ReadURLs[i], err = createPgURL(conf.Username, conf.Password, conf.ReadURLs[i])
		if err != nil {
			log.Panic().Err(err).Str("url", conf.ReadURLs[i]).Msg("failed to create read URL")
		}
	}

	if len(conf.WriteURLs) == 0 && len(conf.ReadURLs) == 0 {
		log.Fatal().Msg("no database URLs provided, at least one read or write URL is required")
	}

	if err := cfg.Validate(conf); err != nil {
		log.Panic().Err(err)
	}

	return conf
}

func MustInit(ctx context.Context, opts ...Option) {
	globalDB.Once.Do(func() {
		conf := mustLoadConfig(opts...)

		if !conf.RunDDL {
			log.Warn().Msg("database DDL execution is disabled")
		}

		globalDB.Tracker = newStatusTracker()
		globalDB.Client = openDatabase(ctx, conf.WriteURLs, conf.ReadURLs, conf.RunDDL).
			WithPrivateKey(conf.PrivateKey).
			WithOperationReporter(globalDB.Tracker.Submit).
			WithRelayURL(conf.RelayURL)

		if !conf.DisableSelfTest {
			if err := doSelfTest(ctx, conf.WriteURLs, conf.ReadURLs); err != nil {
				log.Fatal().Err(err).Msg("database self-test failed")
			}
			if conf.PeriodicSelfTestInterval > 0 {
				startPeriodicSelfTest(ctx, conf.WriteURLs, conf.ReadURLs, conf.PeriodicSelfTestInterval)
			}
		}

		globalDB.Tracker.Start(ctx)
		if !globalDB.Client.hasReadURLs {
			go globalDB.Client.StartExpiredEventsCleanup(ctx)
		} else {
			log.Info().Msg("expired events cleanup is disabled on read-preferred database instances")
		}
		go globalDB.Client.StartCollectingUsedDatabaseStorage(ctx)
		appcontext.GetAppContext(ctx).OnShutdown(func() error {
			err := globalDB.Client.Close()
			globalDB.Once = sync.Once{}
			return err
		})
	})
}

func RegisterExpiredEventsProcessor(proc func(ctx context.Context, events ...*model.Event) error) {
	notifyExpiredEvents = proc
}

func AcceptEvents(ctx context.Context, events ...*model.Event) error {
	if globalDB.Client.hasReadURLs {
		log.Trace().Str("relay_url", globalDB.Client.relayURL).Str("events", model.Events(events).String()).Msg("acceptEvents called on read-preferred instance")
	}
	return globalDB.Client.AcceptEvents(ctx, events...)
}

func RollbackEvents(ctx context.Context, events ...*model.Event) error {
	return globalDB.Client.RollbackEvents(ctx, events...)
}

func CommitEvents(ctx context.Context, events ...*model.Event) error {
	return globalDB.Client.CommitEvents(ctx, events...)
}

func GetStoredEvents(ctx context.Context, filters ...model.Filter) EventIterator {
	return globalDB.Client.SelectEvents(ctx, filters...)
}

func MarkTokenAsInvalidInEventTags(ctx context.Context, events []*model.Event) error {
	return globalDB.Client.markTokenAsInvalidInEventTags(ctx, events)
}

func CountEvents(ctx context.Context, filters ...model.Filter) (int64, error) {
	return globalDB.Client.CountEvents(ctx, filters...)
}

func CountGroupedEventReactions(ctx context.Context, filters ...model.Filter) (string, error) {
	return globalDB.Client.CountGroupedEventReactions(ctx, filters...)
}

func (db *dbClient) StartExpiredEventsCleanup(ctx context.Context) {
	ticks := make(chan struct{}, 1)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		defer appcontext.GetAppContext(ctx).Recover()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		defer close(ticks)

		for {
			select {
			case <-ticker.C:
				select {
				case ticks <- struct{}{}:
				default:
					log.Warn().
						Str("context", "DATABASE").
						Msg("skipping expired events cleanup tick because the previous one is still running")
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	for range ticks {
		deleteCtx, cancel := context.WithTimeout(ctx, time.Minute)
		err := db.deleteExpiredEvents(deleteCtx)
		if err != nil {
			if errors.Is(err, ErrReadOnly) {
				log.Info().Err(err).Msg("expired events cleanup skipped because the database is in read-only mode")
				cancel()
				return
			}
			log.Error().Err(err).Msg("failed to delete expired events")
		}
		cancel()
	}
}

func (db *dbClient) StartCollectingUsedDatabaseStorage(ctx context.Context) {
	ticks := make(chan struct{}, 1)
	ticks <- struct{}{}
	go func() {
		defer appcontext.GetAppContext(ctx).Recover()
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
			log.Error().Err(err).Msg("failed to query database size")
		} else {
			UsedDatabaseStorage.Store(usedDatabaseStorage)
		}
		cancel()
	}
}

func CollectDeviceRegistrationEvents(ctx context.Context) EventIterator {
	return globalDB.Client.collectDeviceRegistrationEvents(ctx)
}

func GetStatusReport(ctx context.Context) (*Status, error) {
	if globalDB.Tracker == nil {
		return nil, errors.New("database status tracker is not initialized")
	}
	return globalDB.Tracker.Get(ctx)
}

func FetchAndUpdatePriceChangeNotification(ctx context.Context, ev *model.Event, targetDevices []string) ([]PriceChangeData, error) {
	return globalDB.Client.FetchAndUpdatePriceChangeNotification(ctx, ev, targetDevices)
}

func DeletePriceChangeSubscriber(ctx context.Context, devicePubKey, deviceUUID, token, eventID string) error {
	return globalDB.Client.DeletePriceChangeSubscriber(ctx, devicePubKey, deviceUUID, token, eventID)
}

func RegisterPriceChangeSubscriber(ctx context.Context, deviceUUID string, ev *model.Event) error {
	return globalDB.Client.RegisterPriceChangeSubscriber(ctx, deviceUUID, ev)
}

func CollectPriceChangeSubscribersCandidates(ctx context.Context, ev *model.Event, startID, limit uint64) ([]string, uint64, error) {
	return globalDB.Client.CollectPriceChangeSubscribersCandidates(ctx, ev, startID, limit)
}

func CollectTokenActivityCandidates(ctx context.Context, now time.Time, startID, limit uint64) ([]string, uint64, error) {
	return globalDB.Client.CollectTokenActivityCandidates(ctx, now, startID, limit)
}

func FetchAndUpdateTokenActivityNotification(ctx context.Context, now time.Time, tokens []string) ([]*TokenActivityData, error) {
	return globalDB.Client.FetchAndUpdateTokenActivityNotification(ctx, now, tokens)
}
