// SPDX-License-Identifier: ice License 1.0

package internal

import (
	"context"

	"github.com/gin-gonic/gin"

	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
	"github.com/ice-blockchain/subzero/server/ws/internal/config"
	"github.com/ice-blockchain/subzero/server/ws/internal/http2"
	"github.com/ice-blockchain/subzero/server/ws/internal/http3"
)

type (
	Router = gin.IRoutes
	Server interface {
		// MustListenAndServe starts everything and blocks indefinitely.
		MustListenAndServe(ctx context.Context)
	}
	RegisterRoutes interface {
		RegisterRoutes(ctx context.Context, router Router)
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
		routesSetup RegisterRoutes
	}
	internalServer interface {
		ListenAndServeTLS(ctx context.Context) error
		Shutdown(ctx context.Context) error
	}
)
