package server

import (
	"github.com/ice-blockchain/subzero/model"
)

var wsEventListener func(*model.Event) error
var wsSubscriptionListener func(*model.Subscription) ([]*model.Event, error)

func RegisterWSEventListener(listen func(*model.Event) error) {
	wsEventListener = listen
}

func RegisterWSSubscriptionListener(listen func(*model.Subscription) ([]*model.Event, error)) {
	wsSubscriptionListener = listen
}
