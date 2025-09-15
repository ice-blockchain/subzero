// SPDX-License-Identifier: ice License 1.0

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"
	"github.com/panjf2000/ants/v2"
	"github.com/spf13/cobra"

	"github.com/ice-blockchain/subzero/cfg"
	"github.com/ice-blockchain/subzero/database/command"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/dvm"
	followerssender "github.com/ice-blockchain/subzero/followers-sender"
	hashtagssender "github.com/ice-blockchain/subzero/hashtags-sender"
	"github.com/ice-blockchain/subzero/model"
	nftcontentsender "github.com/ice-blockchain/subzero/nft-content-sender"
	pushnotifications "github.com/ice-blockchain/subzero/push-notifications"
	"github.com/ice-blockchain/subzero/server"
	wsserver "github.com/ice-blockchain/subzero/server/ws"
	"github.com/ice-blockchain/subzero/storage"
	"github.com/ice-blockchain/subzero/validation"
)

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
			validation.MustInit(cmd.Context())
			query.MustInit(cmd.Context())
			command.MustInit(cmd.Context())
			storage.MustInit(cmd.Context())
			dvm.MustInit(cmd.Context())
			pushnotifications.MustInit(cmd.Context())
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
				log.Printf("failed to webserver.BroadcastUserEvents(%s): %v", model.Events(events).String(), err)
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
			log.Printf("DVM: failed to accept job for event %s: %v", events[0].ID, err)
		}

		if err := command.AcceptEvents(ctx, events...); err != nil {
			return errors.Wrap(err, "command.AcceptEvent failed")
		}

		if err := storage.AcceptEvents(ctx, events...); err != nil {
			return errors.Wrap(err, "storage.AcceptEvents failed")
		}

		antsPool.Submit(func() {
			if err := pushnotifications.AcceptEvents(ctx, events...); err != nil {
				log.Printf("failed to pushnotifications.AcceptEvents(%s): %v", model.Events(events).String(), err)
			}
		})

		antsPool.Submit(func() {
			if err := storage.ReplicateFileOnPeers(ctx, events...); err != nil {
				log.Printf("failed to storage.ReplicateFileOnPeers(%s): %v", model.Events(events).String(), err)
			}
		})

		antsPool.Submit(func() {
			if err := hashtagssender.AcceptEvents(ctx, events...); err != nil {
				log.Printf("failed to hashtagssender.AcceptEvents(%s): %v", model.Events(events).String(), err)
			}
		})

		antsPool.Submit(func() {
			if err := nftcontentsender.AcceptEvents(ctx, events...); err != nil {
				log.Printf("failed to nftcontentsender.AcceptEvents(%s): %v", model.Events(events).String(), err)
			}
		})

		antsPool.Submit(func() {
			if err := followerssender.AcceptEvents(ctx, events...); err != nil {
				log.Printf("failed to followerssender.AcceptEvents(%s): %v", model.Events(events).String(), err)
			}
		})

		antsPool.Submit(func() { webserver.BroadcastNewEvents(context.WithoutCancel(ctx), events...) })

		return nil
	})
	wsserver.RegisterWSSubscriptionListener(query.GetStoredEvents, dvm.GetStoredEvents)
	wsserver.RegisterWSBroadcastEventListener(func(ctx context.Context, events ...*model.Event) error {
		antsPool.Submit(func() {
			start := time.Now()
			webserver.BroadcastNewEvents(context.WithoutCancel(ctx), events...)
			end := time.Since(start)
			log.Printf("INFO: broadcast %d events (%v) [duration %s]", len(events), model.Events(events).IDs(), end)
		})
		antsPool.Submit(func() {
			var ids []string
			for _, event := range events {
				ids = append(ids, event.ID)
			}
			log.Printf("[push-notifications-broadcast] accepting events for pushes: %s", strings.Join(ids, ", "))
			if err := pushnotifications.AcceptEvents(ctx, events...); err != nil {
				log.Printf("failed to pushnotifications.AcceptEvents(%s): %v", model.Events(events).String(), err)
			}
		})

		return nil
	})
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
	pool, err := ants.NewPool(10_000 * runtime.NumCPU())
	if err != nil {
		log.Panicf("failed to create ants pool: %v", err)
	}
	defer pool.Release()

	antsPool = pool
	err = subzero.ExecuteContext(newContext())
	if err != nil {
		log.Panic(err)
	}
}
