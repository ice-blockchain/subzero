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
		Subscriptions      *xsync.Map[string, subscription] // Subscriptions ID -> subscription.
		ConnAuth           *xsync.Map[Writer, connAuthData]
		RelayURL           string
		BroadcastPublicKey string
	}
)

var (
	errAuthRequired          = errors.New("auth-required: please authenticate first by sending AUTH message")
	errRelayReadOnly         = errors.New("relay-is-read-only: read only")
	errRelayNotAuthoritative = errors.New("relay-is-not-authoritative: relay is not authoritative for the user")
	errRelayAuthoritative    = errors.New("relay-is-authoritative: relay is authoritative for the user")
	errDuplicate             = errors.New("duplicate: event with that data already exists")
)
