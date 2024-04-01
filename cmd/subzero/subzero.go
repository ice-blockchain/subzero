package main

import (
	"context"
	"github.com/gookit/goutil/errorx"
	"github.com/ice-blockchain/subzero/database/command"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	wsserver "github.com/ice-blockchain/subzero/server/ws"
	"github.com/spf13/cobra"

	"log"
)

var (
	wsPort  int16
	wsCert  string
	wsKey   string
	subzero = &cobra.Command{
		Use:   "subzero",
		Short: "subzero",
		Run: func(cmd *cobra.Command, args []string) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			query.MustInit()
			wsserver.ListenAndServe(ctx, cancel, &wsserver.Config{
				CertPath: wsCert,
				KeyPath:  wsKey,
				Port:     uint16(wsPort),
			})
		},
	}
	initFlags = func() {
		subzero.Flags().StringVar(&wsCert, "wscert", "", "path to tls certificate for the http/ws server (TLS)")
		subzero.Flags().StringVar(&wsKey, "wskey", "", "path to tls certificate for the http/ws server (TLS)")
		subzero.Flags().Int16Var(&wsPort, "wsport", 0, "port to communicate with clients (http/websocket)")
		subzero.MarkFlagRequired("wscert")
		subzero.MarkFlagRequired("wskey")
		subzero.MarkFlagRequired("wsport")
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
