package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/server"
)

func init() {
	server.RegisterWSEventListener(acceptEvent)
	server.RegisterWSSubscriptionListener(acceptSubscription)
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-ctx.Done():
	case <-quit:
	}
}

func acceptEvent(event *model.Event) error {
	return nil
}

func acceptSubscription(subscription *model.Subscription) ([]*model.Event, error) {
	return nil, nil
}
