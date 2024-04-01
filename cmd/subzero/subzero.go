package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/ice-blockchain/subzero/database/command"
	"github.com/ice-blockchain/subzero/model"
	wsserver "github.com/ice-blockchain/subzero/server/ws"
	"github.com/ice-blockchain/wintr/log"
	"github.com/nbd-wtf/go-nostr"
	"github.com/pkg/errors"
	"time"
)

func init() {
	wsserver.RegisterWSEventListener(acceptEvent)
	wsserver.RegisterWSSubscriptionListener(acceptSubscription)
	command.RegisterSubscriptionsNotifier(wsserver.NotifySubscriptions)
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
	wsserver.ListenAndServe(ctx, cancel, &wsserver.Config{
		CertPath: *wsCert,
		KeyPath:  *wsKey,
		Port:     uint16(*wsPort),
	})
}

func acceptEvent(event *model.Event) error {
	log.Debug(fmt.Sprintf("accepted event %+v", event))
	return nil
}

func acceptSubscription(subscription *model.Subscription) ([]*model.Event, error) {
	return []*model.Event{
		{
			Event: nostr.Event{
				ID:        "bogus",
				PubKey:    "bogus",
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindTextNote,
				Tags:      nil,
				Content:   "bogus note",
				Sig:       "bogus",
			},
		},
	}, nil
}
