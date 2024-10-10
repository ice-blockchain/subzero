// SPDX-License-Identifier: ice License 1.0

package main

import (
	"context"
	"log"

	"github.com/cockroachdb/errors"
	"github.com/spf13/cobra"

	"github.com/ice-blockchain/subzero/database/command"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/server"
	wsserver "github.com/ice-blockchain/subzero/server/ws"
)

var (
	minLeadingZeroBits int
	port               uint16
	cert               string
	key                string
	databasePath       string
	subzero            = &cobra.Command{
		Use:   "subzero",
		Short: "subzero",
		Run: func(_ *cobra.Command, args []string) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if databasePath == ":memory:" {
				log.Print("using in-memory database")
			} else {
				log.Print("using database at ", databasePath)
			}
			query.MustInit(databasePath)

			server.ListenAndServe(ctx, cancel, &server.Config{
				CertPath:                cert,
				KeyPath:                 key,
				Port:                    port,
				NIP13MinLeadingZeroBits: minLeadingZeroBits,
			})
		},
	}
	initFlags = func() {
		subzero.Flags().StringVar(&databasePath, "database", ":memory:", "path to the database")
		subzero.Flags().StringVar(&cert, "cert", "", "path to tls certificate for the http/ws server (TLS)")
		subzero.Flags().StringVar(&key, "key", "", "path to tls certificate for the http/ws server (TLS)")
		subzero.Flags().Uint16Var(&port, "port", 0, "port to communicate with clients (http/websocket)")
		subzero.Flags().IntVar(&minLeadingZeroBits, "minLeadingZeroBits", 0, "min leading zero bits according NIP-13")
		if err := subzero.MarkFlagRequired("cert"); err != nil {
			log.Print(err)
		}
		if err := subzero.MarkFlagRequired("key"); err != nil {
			log.Print(err)
		}
		if err := subzero.MarkFlagRequired("port"); err != nil {
			log.Print(err)
		}
	}
)

func init() {
	initFlags()
	wsserver.RegisterWSEventListener(func(ctx context.Context, event *model.Event) error {
		if err := command.AcceptEvent(ctx, event); err != nil {
			return errors.Wrapf(err, "failed to command.AcceptEvent(%#v)", event)
		}
		if err := query.AcceptEvent(ctx, event); err != nil {
			return errors.Wrapf(err, "failed to query.AcceptEvent(%#v)", event)
		}

		return nil
	})
	wsserver.RegisterWSSubscriptionListener(query.GetStoredEvents)
}

func main() {
	if err := subzero.Execute(); err != nil {
		log.Panic(err)
	}
}
