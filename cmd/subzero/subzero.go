package main

import (
	"context"
	"flag"
	"github.com/ice-blockchain/subzero/database/command"
	"github.com/ice-blockchain/subzero/database/query"
	wsserver "github.com/ice-blockchain/subzero/server/ws"
	"github.com/ice-blockchain/wintr/log"
	"github.com/pkg/errors"
)

func init() {
	wsserver.RegisterWSEventListener(command.AcceptEvent)
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
