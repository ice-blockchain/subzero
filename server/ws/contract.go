// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"errors"

	"github.com/puzpuzpuz/xsync/v4"

	"github.com/ice-blockchain/subzero/model"
	pushnotifications "github.com/ice-blockchain/subzero/push-notifications"
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
		Challenge string
		model.UserDataContext
	}
	connSubscriptions struct {
		// SubscriptionID -> Subscription
		Subscriptions *xsync.Map[string, *model.Subscription]
	}
	handler struct {
		connSubs                *xsync.Map[Writer, connSubscriptions]
		connAuth                *xsync.Map[Writer, connAuthData]
		relayURL                string
		pushNotificationManager *pushnotifications.PushNotificationManager
	}
)

var (
	errAuthRequired = errors.New("auth-required: please authenticate first by sending AUTH message")
)
