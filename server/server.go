// SPDX-License-Identifier: ice License 1.0

package server

import (
	"context"

	httpserver "github.com/ice-blockchain/subzero/server/http"
	wsserver "github.com/ice-blockchain/subzero/server/ws"
)

type (
	Config = wsserver.Config
	router struct{}
)

func ListenAndServe(ctx context.Context, cancel context.CancelFunc, config *wsserver.Config) {
	wsserver.New(config, &router{}).ListenAndServe(ctx, cancel)
}

func (r *router) RegisterRoutes(wsroutes *wsserver.Router) {
	wsroutes.Any("/", wsserver.WithWS(wsserver.NewHandler(), httpserver.NewNIP11Handler()))
}
