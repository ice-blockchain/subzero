package server

import (
	"context"
	httpserver "github.com/ice-blockchain/subzero/server/http"
	wsserver "github.com/ice-blockchain/subzero/server/ws"
)

type Config = wsserver.Config

func ListenAndServe(ctx context.Context, cancel context.CancelFunc, config *wsserver.Config) {
	wsserver.New(config, httpserver.NewNIP11Handler()).ListenAndServe(ctx, cancel)
}
