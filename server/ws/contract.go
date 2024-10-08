// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"sync"

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

var WithWS = internal.WithWS

type (
	handler struct {
		subListenersMx sync.Mutex
		subListeners   map[adapters.WSWriter]map[string]*subscription
	}
	subscription struct {
		*model.Subscription
		SubscriptionID string
	}
)
