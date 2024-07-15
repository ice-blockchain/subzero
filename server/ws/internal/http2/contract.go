// SPDX-License-Identifier: ice License 1.0

package http2

import (
	"context"
	"net/http"
	stdlibtime "time"

	h2ec "github.com/ice-blockchain/go/src/net/http"
	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
	"github.com/ice-blockchain/subzero/server/ws/internal/config"
)

type (
	Server interface {
		ListenAndServeTLS(ctx context.Context, certFile, keyFile string) error
		Shutdown(ctx context.Context) error

		HandleWS(wsHandler adapters.WSHandler, handler http.Handler, writer http.ResponseWriter, req *http.Request)
	}
	Unwrapper interface {
		Unwrap() http.ResponseWriter
	}
)
type (
	srv struct {
		server *h2ec.Server
		router http.Handler
		cfg    *config.Config
	}
)

const (
	websocketProtocol    = "websocket"
	webtransportProtocol = "webtransport"
	acceptStreamTimeout  = 30 * stdlibtime.Second
)
