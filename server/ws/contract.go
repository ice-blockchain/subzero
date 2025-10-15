// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"context"
	"errors"

	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/server/ws/internal"
	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
	"github.com/ice-blockchain/subzero/server/ws/internal/config"
)

type (
	Writer           = adapters.WSWriter
	Config           = config.Config
	EventBroadcaster interface {
		BroadcastNewEvents(ctx context.Context, events ...*model.Event) int
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
	syncMap[K comparable, V any] interface {
		Store(key K, data V)

		Load(key K) (V, bool)
		LoadOrCompute(key K, f func() (V, bool)) (V, bool)
		LoadAndStore(key K, value V) (V, bool)
		LoadAndDelete(key K) (V, bool)

		Delete(key K)

		Range(f func(key K, value V) bool)
	}
	handler struct {
		Subscriptions      syncMap[string, subscription] // Subscriptions ID -> subscription.
		ConnAuth           syncMap[Writer, connAuthData]
		RelayURL           string
		BroadcastPublicKey string
	}
)

var (
	errAuthRequired  = errors.New("auth-required: please authenticate first by sending AUTH message")
	errRelayReadOnly = errors.New("relay-is-read-only: read only")
	errDuplicate     = errors.New("duplicate: event with that data already exists")
)
