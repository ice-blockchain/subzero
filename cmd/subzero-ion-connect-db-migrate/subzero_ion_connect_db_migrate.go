// SPDX-License-Identifier: ice License 1.0

package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/cockroachdb/errors"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
	"spheric.cloud/xiter"

	sqlite "github.com/ice-blockchain/subzero/cmd/subzero-ion-connect-db-migrate/internal"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

var (
	sqliteDatabasePath    string
	postgresConnectionURL string

	subzero = &cobra.Command{
		Use:   "subzero",
		Short: "subzero",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if sqliteDatabasePath == "" {
				return errors.Errorf("missing --source flag")
			}
			if postgresConnectionURL == "" {
				return errors.Errorf("missing --target flag")
			}

			query.MustInit(cmd.Context(), query.WithConfig(&query.Config{
				URL:        postgresConnectionURL,
				PrivateKey: model.GeneratePrivateKey(),
				RelayURL:   "http://localhost:8080",
			}))

			sourceDB := sqlite.MustOpen(sqliteDatabasePath)
			defer sourceDB.Close()

			count, err := sourceDB.Count(cmd.Context())
			if err != nil {
				return errors.Wrap(err, "count")
			}
			log.Printf("%s: found %d records", sqliteDatabasePath, count)

			bar := progressbar.Default(count, "migrating records")
			defer bar.Close()

			it := xiter.Concat2(
				sourceDB.SelectManagementEvents(cmd.Context()),
				sourceDB.SelectDataEvents(cmd.Context()),
			)

			for ev, err := range it {
				bar.Add(1)

				if cmd.Context().Err() != nil {
					break
				}

				if err != nil {
					return errors.Wrap(err, "select events")
				}

				err = query.AcceptEvents(cmd.Context(), ev)
				if err != nil {
					log.Printf("ERROR: %s: %s", err, ev.String())
					if errors.IsAny(err, query.ErrOnBehalfAccessDenied, query.ErrInvalidEvent, query.ErrRepostOfDeletedPost) {
						log.Printf("    skipping")
						continue
					}
					return errors.Wrapf(err, "insert event %s", ev.String())
				}
			}

			return nil
		},
	}
)

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
	subzero.Flags().StringVar(&sqliteDatabasePath, "source", "", "Path to SQLite database")
	subzero.Flags().StringVar(&postgresConnectionURL, "target", "", "Postgres connection URL")

	err := subzero.ExecuteContext(newContext())
	if err != nil {
		log.Fatal(err)
	}
}
