package main

import (
	"context"
	"fmt"
	"github.com/ice-blockchain/subzero/database/command"
	"github.com/ice-blockchain/subzero/model"
	wsserver "github.com/ice-blockchain/subzero/server/ws"
	"github.com/ice-blockchain/wintr/log"
	"github.com/nbd-wtf/go-nostr"
	"time"
)

func init() {
	wsserver.RegisterWSEventListener(acceptEvent)
	wsserver.RegisterWSSubscriptionListener(acceptSubscription)
	command.RegisterSubscriptionsNotifier(wsserver.NotifySubscriptions)
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wsserver.ListenAndServe(ctx, cancel)
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
