package internal

import (
	"context"
	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
	"github.com/ice-blockchain/subzero/server/ws/internal/config"
	"github.com/ice-blockchain/subzero/server/ws/internal/http2"
	"github.com/ice-blockchain/subzero/server/ws/internal/http3"
	"os"
)

type (
	Server interface {
		// ListenAndServe starts everything and blocks indefinitely.
		ListenAndServe(ctx context.Context, cancel context.CancelFunc)
	}

	WSHandler = adapters.WSHandler
	WS        = adapters.WS
)
type (
	srv struct {
		h3server  http3.Server
		wsServer  http2.Server
		cfg       *config.Config
		quit      chan<- os.Signal
		wsHandler WSHandler
	}
)
