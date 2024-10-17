// SPDX-License-Identifier: ice License 1.0

package internal

import (
	"context"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
	"github.com/ice-blockchain/subzero/server/ws/internal/config"
	"github.com/ice-blockchain/subzero/server/ws/internal/http2"
	"github.com/ice-blockchain/subzero/server/ws/internal/http3"
)

type (
	Router = gin.IRoutes
	Server interface {
		// ListenAndServe starts everything and blocks indefinitely.
		ListenAndServe(ctx context.Context, cancel context.CancelFunc)
	}
	RegisterRoutes interface {
		RegisterRoutes(ctx context.Context, router Router, cfg *config.Config)
	}

	WSHandler = adapters.WSHandler
	WS        = adapters.WS
)
type (
	Srv struct {
		H3Server    http3.Server
		H2Server    http2.Server
		router      *gin.Engine
		cfg         *config.Config
		quit        chan<- os.Signal
		routesSetup RegisterRoutes
	}
)
