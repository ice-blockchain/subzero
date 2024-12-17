// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"errors"

	"github.com/puzpuzpuz/xsync/v3"

	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/server/ws/internal"
	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
	"github.com/ice-blockchain/subzero/server/ws/internal/config"
)

type (
	Writer         = adapters.WSWriter
	Config         = config.Config
	WSHandler      = adapters.WSHandler
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
		Challenge       string
		PublicKey       string
		MasterPublicKey string
		Authenticated   bool
	}
	connSubscriptions struct {
		// SubscriptionID -> Subscription
		Subscriptions *xsync.MapOf[string, *model.Subscription]
	}
	handler struct {
		connSubs *xsync.MapOf[Writer, connSubscriptions]
		connAuth *xsync.MapOf[Writer, connAuthData]
		relayURL string
	}
)

var (
	errAuthRequired = errors.New("auth-required: please authenticate first by sending AUTH message")
)
