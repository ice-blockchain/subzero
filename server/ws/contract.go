// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"context"
	"errors"

	"github.com/puzpuzpuz/xsync/v4"

	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/server/ws/internal"
	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
	"github.com/ice-blockchain/subzero/server/ws/internal/config"
)

type (
	Writer  = adapters.WSWriter
	Config  = config.Config
	Handler interface {
		adapters.WSHandler
		BroadcastNewEvents(ctx context.Context, events ...*model.Event) error
	}
	Server         = internal.Server
	RegisterRoutes = internal.RegisterRoutes
	Router         = internal.Router
)

var (
	ErrNotifyFailed = errors.New("failed to notify about new events")

	WithWS = internal.WithWS
)

type (
	connAuthData struct {
		Challenge string
		model.UserDataContext
	}
	connSubscriptions struct {
		// SubscriptionID -> Subscription
		Subscriptions *xsync.Map[string, *model.Subscription]
	}
	router struct {
	}
	handler struct {
		ConnSubs *xsync.Map[Writer, connSubscriptions]
		ConnAuth *xsync.Map[Writer, connAuthData]
		RelayURL string
	}
)

var (
	errAuthRequired = errors.New("auth-required: please authenticate first by sending AUTH message")
)
