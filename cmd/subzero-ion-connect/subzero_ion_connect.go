// SPDX-License-Identifier: ice License 1.0

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"
	"github.com/panjf2000/ants/v2"
	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/ice-blockchain/subzero/appcontext"
	"github.com/ice-blockchain/subzero/cfg"
	"github.com/ice-blockchain/subzero/database/command"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/dvm"
	followerssender "github.com/ice-blockchain/subzero/followers-sender"
	hashtagssender "github.com/ice-blockchain/subzero/hashtags-sender"
	"github.com/ice-blockchain/subzero/log"
	"github.com/ice-blockchain/subzero/model"
	nftcontentsender "github.com/ice-blockchain/subzero/nft-content-sender"
	opentelemetry "github.com/ice-blockchain/subzero/open-telemetry"
	pushnotifications "github.com/ice-blockchain/subzero/push-notifications"
	"github.com/ice-blockchain/subzero/server"
	wsserver "github.com/ice-blockchain/subzero/server/ws"
	"github.com/ice-blockchain/subzero/storage"
	"github.com/ice-blockchain/subzero/validation"
)

type (
	Config struct {
		LogLevel string `yaml:"log-level" validate:"omitempty,oneof=trace debug info warn error fatal panic"`
		Debug    bool   `yaml:"debug"`
	}
)

func logInit() {
	config, err := cfg.Get[Config]()
	if config == nil || err != nil {
		fmt.Println("no log configuration found, using default values", err)
		config = &Config{}
	}

	if config.LogLevel == "" {
		config.LogLevel = "info"
	}

	level, err := zerolog.ParseLevel(config.LogLevel)
	if err != nil {
		panic(fmt.Sprintf("invalid log level: %v", err))
	}

	zerolog.SetGlobalLevel(level)
	if config.Debug {
		zlog.Logger = zlog.Output(zerolog.ConsoleWriter{
			Out:          os.Stdout,
			TimeFormat:   time.RFC3339Nano,
			TimeLocation: time.UTC,
			NoColor:      true,
		})
	}
}

var (
	configPath string
	antsPool   *ants.Pool
	webserver  server.Server
	subzero    = &cobra.Command{
		Use:     "subzero",
		Short:   "subzero",
		Version: getVersion(),
		Run: func(cmd *cobra.Command, _ []string) {
			cfg.MustInit(configPath)
			logInit()
			opentelemetry.MustInit(cmd.Context())
			validation.MustInit(cmd.Context())
			query.MustInit(cmd.Context())
			command.MustInit(cmd.Context())
			storage.MustInit(cmd.Context())
			dvm.MustInit(cmd.Context())
			pushnotifications.MustInit(cmd.Context(), antsPool)
			hashtagssender.MustInit(cmd.Context())
			nftcontentsender.MustInit(cmd.Context())
			followerssender.MustInit(cmd.Context())
			webserver = server.New(cmd.Context())
			webserver.MustListenAndServe(cmd.Context())
		},
	}
	initFlags = func() {
		subzero.Flags().StringVar(&configPath, "config", cfg.DefaultYAMLConfigurationFilePath, "absolute path to the service config yaml file")
	}

	// Do not require authentication for these kinds of events (publishing).
	eventKindsNoAuth = map[int]struct{}{
		nostr.KindGiftWrap:     {},
		nostr.KindFileMetadata: {},
	}
)

func getVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}

	var revision, commitDate string
	for _, v := range info.Settings {
		switch v.Key {
		case "vcs.revision":
			revision = v.Value
		case "vcs.time":
			commitDate = v.Value
		}
	}

	return fmt.Sprintf("%s: %v (%s / %s)", info.Main.Path, info.Main.Version, revision, commitDate)
}

func init() {
	initFlags()
	query.RegisterExpiredEventsProcessor(storage.DeleteExpiredFiles)
	command.RegisterRollbackListener(query.RollbackEvents)
	command.RegisterAcceptListener(func(ctx context.Context, events ...*model.Event) error {
		if err := query.AcceptEvents(ctx, events...); err != nil {
			return errors.Wrapf(err, "failed to query.AcceptEvent(%#v)", model.Events(events).String())
		}
		return nil
	})
	command.RegisterCommitListener(func(ctx context.Context, events ...*model.Event) error {
		if err := query.CommitEvents(ctx, events...); err != nil {
			return errors.Wrapf(err, "failed to delete outdated replaced events")
		}

		if err := storage.AcceptEvents(ctx, events...); err != nil {
			return errors.Wrapf(err, "storage.AcceptEvents failed: %s", model.Events(events).String())
		}

		antsPool.Submit(func() { webserver.BroadcastNewEvents(ctx, events...) })

		return nil
	})
	wsserver.RegisterReqMustAuthenticate(func(_ context.Context, sub *model.Subscription) (authRequired bool) {
		// Require authentication for all types/kinds of subscriptions.
		return true
	})
	wsserver.RegisterEventMustAuthenticate(func(_ context.Context, events ...*model.Event) (authRequired bool) {
		for _, e := range events {
			if _, exists := eventKindsNoAuth[e.Kind]; !exists {
				return true
			}
		}
		return false
	})
	wsserver.RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		for _, event := range events {
			if event.Kind == nostr.KindGiftWrap {
				if model.GetUserDataFromContext(ctx).Authenticated {
					return fmt.Errorf("%v: authenticated user is not allowed to send gift wrap events", event.ID)
				}
			}
		}
		if err := query.AcceptEvents(ctx, events...); err != nil {
			return errors.Wrap(err, "query.AcceptEvent failed")
		}

		antsPool.Submit(func() {
			if err := webserver.BroadcastUserEvents(context.WithoutCancel(ctx), events...); err != nil {
				log.Error(ctx, errors.Wrapf(err, "failed to webserver.BroadcastUserEvents"), "context", "MAIN",
					"events", model.Events(events).String())
			}
		})

		if ch, err := dvm.AcceptJob(ctx, events[0]); err == nil && ch != nil {
			antsPool.Submit(func() {
				result := <-ch
				if result != nil {
					webserver.BroadcastNewEvents(context.WithoutCancel(ctx), result)
				}
			})
		} else if err != nil {
			log.Error(ctx, errors.Wrapf(err, "dvm failed to accept job for event"), "context", "MAIN", "event_id", events[0].ID)
		}

		if err := command.AcceptEvents(ctx, events...); err != nil {
			return errors.Wrap(err, "command.AcceptEvent failed")
		}

		if err := storage.AcceptEvents(ctx, events...); err != nil {
			return errors.Wrap(err, "storage.AcceptEvents failed")
		}

		antsPool.Submit(func() {
			if err := pushnotifications.AcceptEvents(ctx, events...); err != nil {
				log.Error(ctx, errors.Wrapf(err, "failed to pushnotifications.AcceptEvents"), "events", model.Events(events).String())
			}
		})

		antsPool.Submit(func() {
			if err := storage.ReplicateFileOnPeers(ctx, events...); err != nil {
				log.Error(ctx, errors.Wrapf(err, "failed to storage.ReplicateFileOnPeers"), "events", model.Events(events).String())
			}
		})

		antsPool.Submit(func() {
			if err := hashtagssender.AcceptEvents(ctx, events...); err != nil {
				log.Error(ctx, errors.Wrapf(err, "failed to hashtagssender.AcceptEvents"), "events", model.Events(events).String())
			}
		})

		antsPool.Submit(func() {
			if err := nftcontentsender.AcceptEvents(ctx, events...); err != nil {
				log.Error(ctx, errors.Wrapf(err, "failed to nftcontentsender.AcceptEvents"), "events", model.Events(events).String())
			}
		})

		antsPool.Submit(func() {
			if err := followerssender.AcceptEvents(ctx, events...); err != nil {
				log.Error(ctx, errors.Wrapf(err, "failed to followerssender.AcceptEvents"), "events", model.Events(events).String())
			}
		})

		antsPool.Submit(func() { webserver.BroadcastNewEvents(context.WithoutCancel(ctx), events...) })

		return nil
	})
	wsserver.RegisterWSSubscriptionListener(query.GetStoredEvents, dvm.GetStoredEvents)
	wsserver.RegisterWSBroadcastEventListener(func(ctx context.Context, events ...*model.Event) error {
		antsPool.Submit(func() {
			start := time.Now()
			n := webserver.BroadcastNewEvents(context.WithoutCancel(ctx), events...)
			end := time.Since(start)
			log.Trace(ctx, "broadcast events",
				"event_count", len(events),
				"event_ids", model.Events(events).IDs(),
				"duration", end,
				"subscription_count", n)
		})
		antsPool.Submit(func() {
			if err := pushnotifications.AcceptEvents(ctx, events...); err != nil {
				log.Error(ctx, errors.Wrapf(err, "failed to pushnotifications.AcceptEvents"), "events", model.Events(events).String())
			}
		})

		return nil
	})
}

func newContext() appcontext.WaitForShutdown {
	ctx, cancel := appcontext.NewAppContext(context.Background())

	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		force := false
		for sig := range c {
			if force {
				log.Warn(ctx, "force shutdown", "signal", sig.String())
				os.Exit(2)
			} else {
				log.Info(ctx, "graceful shutdown", "signal", sig.String())
				cancel()
				force = true
			}
		}
	}()

	return ctx
}

func main() {
	appCtx := newContext()
	defer appcontext.GetAppContext(appCtx).Recover()
	pool, err := ants.NewPool(10_000 * runtime.NumCPU())
	if err != nil {
		log.Panic(errors.Wrapf(err, "failed to create ants pool"))
	}
	defer pool.Release()

	antsPool = pool
	if err = subzero.ExecuteContext(appCtx); err != nil {
		log.Panic().Err(err)
	}
	appCtx.WaitForShutdown()
}
