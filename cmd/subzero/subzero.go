package main

import (
	"context"
	"log"

	"github.com/gookit/goutil/errorx"
	"github.com/spf13/cobra"

	"github.com/ice-blockchain/subzero/database/command"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/server"
	wsserver "github.com/ice-blockchain/subzero/server/ws"
)

var (
	port    int16
	cert    string
	key     string
	subzero = &cobra.Command{
		Use:   "subzero",
		Short: "subzero",
		Run: func(cmd *cobra.Command, args []string) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			query.MustInit()
			server.ListenAndServe(ctx, cancel, &server.Config{
				CertPath: cert,
				KeyPath:  key,
				Port:     uint16(port),
			})
		},
	}
	initFlags = func() {
		subzero.Flags().StringVar(&cert, "cert", "", "path to tls certificate for the http/ws server (TLS)")
		subzero.Flags().StringVar(&key, "key", "", "path to tls certificate for the http/ws server (TLS)")
		subzero.Flags().Int16Var(&port, "port", 0, "port to communicate with clients (http/websocket)")
		subzero.MarkFlagRequired("cert")
		subzero.MarkFlagRequired("key")
		subzero.MarkFlagRequired("port")
	}
)

func init() {
	initFlags()
	wsserver.RegisterWSEventListener(func(ctx context.Context, event *model.Event) error {
		if err := command.AcceptEvent(ctx, event); err != nil {
			return errorx.Withf(err, "failed to command.AcceptEvent(%#v)", event)
		}
		if err := query.AcceptEvent(ctx, event); err != nil {
			return errorx.Withf(err, "failed to query.AcceptEvent(%#v)", event)
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
