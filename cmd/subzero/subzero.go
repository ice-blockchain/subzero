// SPDX-License-Identifier: ice License 1.0

package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"log"
	"net"

	"github.com/cockroachdb/errors"
	"github.com/spf13/cobra"

	"github.com/ice-blockchain/subzero/database/command"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/server"
	wsserver "github.com/ice-blockchain/subzero/server/ws"
	"github.com/ice-blockchain/subzero/storage"
)

var (
	minLeadingZeroBits int
	port               uint16
	cert               string
	key                string
	databasePath       string
	externalIP         string
	adnlPort           uint16
	storageRootDir     string
	globalConfigUrl    string
	adnlNodeKey        []byte
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
			storage.MustInit(ctx, adnlNodeKey, globalConfigUrl, storageRootDir, net.ParseIP(externalIP), int(adnlPort))
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
		subzero.Flags().StringVar(&externalIP, "adnl-external-ip", "", "external ip for storage service")
		subzero.Flags().Uint16Var(&adnlPort, "adnl-port", 0, "port to open adnl-gateway for storage service")
		subzero.Flags().StringVar(&storageRootDir, "storage-root", "./.uploads", "root storage directory")
		subzero.Flags().StringVar(&globalConfigUrl, "global-config-url", storage.DefaultConfigUrl, "global config for ION storage")
		subzero.Flags().BytesHexVar(&adnlNodeKey, "adnl-node-key", func() []byte {
			_, nodeKey, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				log.Panic(errorx.Wrapf(err, "failed to generate node key"))
			}
			return nodeKey
		}(), "adnl node key in hex")
		if err := subzero.MarkFlagRequired("cert"); err != nil {
			log.Print(err)
		}
		if err := subzero.MarkFlagRequired("key"); err != nil {
			log.Print(err)
		}
		if err := subzero.MarkFlagRequired("port"); err != nil {
			log.Print(err)
		}
		if err := subzero.MarkFlagRequired("adnl-external-ip"); err != nil {
			log.Print(err)
		}
		if err := subzero.MarkFlagRequired("adnl-port"); err != nil {
			log.Print(err)
		}
		if err := subzero.MarkFlagRequired("storage-root"); err != nil {
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
