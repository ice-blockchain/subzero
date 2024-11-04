// SPDX-License-Identifier: ice License 1.0

package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/cockroachdb/errors"
	"github.com/spf13/cobra"

	"github.com/ice-blockchain/subzero/cfg"
	"github.com/ice-blockchain/subzero/database/command"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/dvm"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/server"
	wsserver "github.com/ice-blockchain/subzero/server/ws"
	"github.com/ice-blockchain/subzero/storage"
)

var (
	configPath string
	subzero    = &cobra.Command{
		Use:   "subzero",
		Short: "subzero",
		Run: func(cmd *cobra.Command, _ []string) {
			cfg.MustInit(configPath)
			query.MustInit(cmd.Context())
			storage.MustInit(cmd.Context())
			dvm.MustInit()
			server.MustListenAndServe(cmd.Context())
		},
	}
	initFlags = func() {
		subzero.Flags().StringVar(&configPath, "config", cfg.DefaultYAMLConfigurationFilePath, "absolute path to the service config yaml file")
	}
)

func init() {
	initFlags()
	wsserver.RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		if err := command.AcceptEvents(ctx, events...); err != nil {
			return errors.Wrapf(err, "failed to command.AcceptEvent(%#v)", events)
		}
		if err := query.AcceptEvents(ctx, events...); err != nil {
			return errors.Wrapf(err, "failed to query.AcceptEvent(%#v)", events)
		}
		if sErr := storage.AcceptEvents(ctx, events...); sErr != nil {
			return errors.Wrapf(sErr, "failed to process NIP-94 events")
		}
		if err := dvm.AcceptJob(ctx, events[0]); err != nil {
			return errors.Wrapf(err, "failed to dvm.AcceptEvent(%#v)", events[0])
		}

		return nil
	})
	wsserver.RegisterWSSubscriptionListener(query.GetStoredEvents)
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
