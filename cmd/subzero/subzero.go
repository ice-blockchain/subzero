package main

import (
	"context"
	"flag"
	"github.com/gookit/goutil/errorx"
	"github.com/ice-blockchain/subzero/database/command"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	wsserver "github.com/ice-blockchain/subzero/server/ws"
	"github.com/ice-blockchain/wintr/log"
	"github.com/pkg/errors"
)

func init() {
	wsserver.RegisterWSEventListener(func(ctx context.Context, event *model.Event) error {
		if err := command.AcceptEvent(ctx, event); err != nil {
			return errorx.Withf(err, "failed to command.AcceptEvent(%#v)", event)
		}

		return errorx.Withf(query.AcceptEvent(ctx, event), "failed to query.AcceptEvent(%#v)", event)
	})
	wsserver.RegisterWSSubscriptionListener(query.GetStoredEvents)
}

var (
	wsPort = flag.Int("wsport", -1, "port to communicate with clients (http/websocket)")
	wsCert = flag.String("wscert", "", "path to tls certificate for the http/ws server (TLS)")
	wsKey  = flag.String("wskey", "", "path to key for the http/ws server (TLS)")
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	flag.Parse()
	if wsPort == nil || *wsPort <= 0 {
		log.Panic(errors.Errorf("wsport required"))
	}
	if wsCert == nil || *wsCert == "" {
		log.Panic(errors.Errorf("wscert required"))
	}
	if wsKey == nil || *wsKey == "" {
		log.Panic(errors.Errorf("wskey required"))
	}
	query.MustInit()
	wsserver.ListenAndServe(ctx, cancel, &wsserver.Config{
		CertPath: *wsCert,
		KeyPath:  *wsKey,
		Port:     uint16(*wsPort),
	})
}
