// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"context"
	"errors"

	"github.com/panjf2000/ants/v2"
	"github.com/puzpuzpuz/xsync/v4"

	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/server/ws/internal"
	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
	"github.com/ice-blockchain/subzero/server/ws/internal/config"
)

type (
	Writer           = adapters.WSWriter
	Config           = config.Config
	EventBroadcaster interface {
		BroadcastNewEvents(ctx context.Context, events ...*model.Event)
	}
	Handler interface {
		adapters.WSHandler
		EventBroadcaster
	}
	Server         = internal.Server
	RegisterRoutes = internal.RegisterRoutes
	Router         = internal.Router
)

var (
	WithWS = internal.WithWS
)

type (
	connAuthData struct {
		Challenge string
		model.UserDataContext
	}
	subscription struct {
		Source *model.Subscription
		Writer Writer
	}
	handler struct {
		Pool          *ants.Pool
		Subscriptions *xsync.Map[string, subscription] // Subscriptions ID -> subscription.
		ConnAuth      *xsync.Map[Writer, connAuthData]
		RelayURL      string
	}
)

var (
	errAuthRequired = errors.New("auth-required: please authenticate first by sending AUTH message")
)
