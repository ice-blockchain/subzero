package internal

import (
	"context"
	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
	"github.com/ice-blockchain/subzero/server/ws/internal/config"
	"github.com/ice-blockchain/subzero/server/ws/internal/http2"
	"github.com/ice-blockchain/subzero/server/ws/internal/http3"
	httpserver "github.com/ice-blockchain/wintr/server"
	"os"
)

type (
	Router = httpserver.Router
	Server interface {
		// ListenAndServe starts everything and blocks indefinitely.
		ListenAndServe(ctx context.Context, cancel context.CancelFunc)
	}

	WSHandler = adapters.WSHandler
	WS        = adapters.WS
	Service   interface {
		WSHandler
		RegisterRoutes(r *Router)
	}
)
type (
	srv struct {
		h3server http3.Server
		wsServer http2.Server
		router   *Router
		cfg      *config.Config
		quit     chan<- os.Signal
		service  Service
	}
)
