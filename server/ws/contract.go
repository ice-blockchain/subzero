// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"errors"
	"sync"

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

var WithWS = internal.WithWS

type (
	connAuthData struct {
		Challenge     string
		Authenticated bool
		PublicKey     string
	}
	handler struct {
		subListenersMx sync.Mutex
		subListeners   map[adapters.WSWriter]map[string]*model.Subscription
		connAuth       *xsync.MapOf[adapters.WSWriter, connAuthData]
		relayURL       string
	}
)

var (
	errAuthRequired = errors.New("auth-required: please authenticate first by sending AUTH message")
)
