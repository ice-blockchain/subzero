// SPDX-License-Identifier: ice License 1.0

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"
	"github.com/panjf2000/ants/v2"
	"github.com/spf13/cobra"

	"github.com/ice-blockchain/subzero/cfg"
	"github.com/ice-blockchain/subzero/database/command"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/dvm"
	hashtagssender "github.com/ice-blockchain/subzero/hashtags-sender"
	"github.com/ice-blockchain/subzero/model"
	pushnotifications "github.com/ice-blockchain/subzero/push-notifications"
	"github.com/ice-blockchain/subzero/server"
	wsserver "github.com/ice-blockchain/subzero/server/ws"
	"github.com/ice-blockchain/subzero/storage"
	"github.com/ice-blockchain/subzero/validation"
)

var (
	configPath string
	webserver  server.Server
	subzero    = &cobra.Command{
		Use:     "subzero",
		Short:   "subzero",
		Version: getVersion(),
		Run: func(cmd *cobra.Command, _ []string) {
			cfg.MustInit(configPath)
			validation.MustInit()
			query.MustInit(cmd.Context())
			command.MustInit(cmd.Context())
			storage.MustInit(cmd.Context())
			dvm.MustInit(cmd.Context())
			pushnotifications.MustInit()
			hashtagssender.MustInit(cmd.Context())
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
		if err := storage.AcceptEvents(ctx, events...); err != nil {
			return errors.Wrapf(err, "storage.AcceptEvents failed: %s", model.Events(events).String())
		}
		if err := query.CommitEvents(ctx, events...); err != nil {
			return errors.Wrapf(err, "failed to delete outdated replaced events")
		}

		ants.Submit(func() {
			webserver.BroadcastNewEvents(ctx, events...)
		})

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
				_, _, authenticated, _ := model.GetUserDataFromContext(ctx)
				if authenticated {
					return fmt.Errorf("%v: authenticated user is not allowed to send gift wrap events", event.ID)
				}
			}
		}
		if err := query.AcceptEvents(ctx, events...); err != nil {
			return errors.Wrap(err, "query.AcceptEvent failed")
		}
		if err := dvm.AcceptJob(ctx, events[0]); err != nil {
			return errors.Wrap(err, "dvm.AcceptEvent failed")
		}
		if err := command.AcceptEvents(ctx, events...); err != nil {
			return errors.Wrap(err, "command.AcceptEvent failed")
		}

		if err := storage.AcceptEvents(ctx, events...); err != nil {
			return errors.Wrap(err, "storage.AcceptEvents failed")
		}

		ants.Submit(func() {
			if err := pushnotifications.AcceptEvents(ctx, events); err != nil {
				log.Printf("failed to pushnotifications.AcceptEvents(%s): %v", model.Events(events).String(), err)
			}
		})
		ants.Submit(func() {
			if err := hashtagssender.AcceptEvents(ctx, events...); err != nil {
				log.Printf("failed to hashtagssender.AcceptEvents(%s): %v", model.Events(events).String(), err)
			}
		})
		return nil
	})
	wsserver.RegisterWSSubscriptionListener(query.GetStoredEvents, dvm.GetStoredEvents)
}

func newContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())

	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		force := false
		for sig := range c {
			if force {
				log.Println("force shutdown", "signal", sig.String())
				os.Exit(2)
			} else {
				log.Println("graceful shutdown", "signal", sig.String())
				cancel()
				force = true
			}
		}
	}()

	return ctx
}

func main() {
	err := subzero.ExecuteContext(newContext())
	if err != nil {
		log.Panic(err)
	}
}
