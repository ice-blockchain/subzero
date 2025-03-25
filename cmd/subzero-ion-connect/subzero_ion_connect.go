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
	"github.com/spf13/cobra"

	"github.com/ice-blockchain/subzero/cfg"
	"github.com/ice-blockchain/subzero/database/command"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/dvm"
	"github.com/ice-blockchain/subzero/model"
	pushnotifications "github.com/ice-blockchain/subzero/push-notifications"
	"github.com/ice-blockchain/subzero/server"
	wsserver "github.com/ice-blockchain/subzero/server/ws"
	"github.com/ice-blockchain/subzero/storage"
	"github.com/ice-blockchain/subzero/validation"
)

var (
	configPath  string
	showVersion bool
	subzero     = &cobra.Command{
		Use:   "subzero",
		Short: "subzero",
		Run: func(cmd *cobra.Command, _ []string) {
			if showVersion {
				printVersion()
				return
			}

			cfg.MustInit(configPath)
			validation.MustInit()
			command.MustInit(cmd.Context())
			query.MustInit(cmd.Context())
			storage.MustInit(cmd.Context())
			dvm.MustInit(cmd.Context())
			pushnotifications.MustInit()
			server.MustListenAndServe(cmd.Context())
		},
	}
	initFlags = func() {
		subzero.Flags().StringVar(&configPath, "config", cfg.DefaultYAMLConfigurationFilePath, "absolute path to the service config yaml file")
		subzero.Flags().BoolVar(&showVersion, "version", false, "show version")
	}

	// Do not require authentication for these kinds of events (publishing).
	eventKindsNoAuth = map[int]struct{}{
		nostr.KindGiftWrap:     {},
		nostr.KindFileMetadata: {},
	}
)

func printVersion() {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		fmt.Println("no build info")
		return
	}

	fmt.Println("Package:", info.Main.Path)
	fmt.Println("Version:", info.Main.Version)
	for _, v := range info.Settings {
		switch v.Key {
		case "vcs.revision":
			fmt.Println("Revision:", v.Value)
		case "vcs.time":
			fmt.Println("Build Time:", v.Value)
		}
	}
}

func init() {
	initFlags()
	query.RegisterExpiredEventsProcessor(storage.DeleteExpiredFiles)
	command.RegisterRollbackListener(query.RollbackEvents)
	command.RegisterAcceptListener(query.AcceptEvents)
	wsserver.RegisterReqMustAuthenticate(func(_ context.Context, sub *model.Subscription) (authRequired bool) {
		// Require authentication for all types/kinds of subscriptions.
		return false
	})
	wsserver.RegisterEventMustAuthenticate(func(_ context.Context, events ...*model.Event) (authRequired bool) {
		for _, e := range events {
			if _, exists := eventKindsNoAuth[e.Kind]; !exists {
				return false
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
			return errors.Wrapf(err, "failed to query.AcceptEvent(%#v)", events)
		}
		if err := command.AcceptEvents(ctx, events...); err != nil {
			return errors.Wrapf(err, "failed to command.AcceptEvent(%#v)", events)
		}
		if sErr := storage.AcceptEvents(ctx, events...); sErr != nil {
			return errors.Wrapf(sErr, "failed to process NIP-94 events")
		}
		if err := dvm.AcceptJob(ctx, events[0]); err != nil {
			return errors.Wrapf(err, "failed to dvm.AcceptEvent(%#v)", events[0])
		}
		go func() {
			if err := pushnotifications.AcceptEvents(ctx, events); err != nil {
				log.Printf("failed to pushnotifications.AcceptEvents(%#v): %v", events, err)
			}
		}()

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
