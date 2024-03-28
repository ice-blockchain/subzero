// SPDX-License-Identifier: ice License 1.0

package http2

import (
	"context"
	"github.com/ice-blockchain/subzero/server/ws/internal/config"
	"net/http"
	stdlibtime "time"

	h2ec "github.com/ice-blockchain/go/src/net/http"
)

type (
	Server interface {
		ListenAndServeTLS(ctx context.Context, certFile, keyFile string) error
		Shutdown(ctx context.Context) error
	}
)
type (
	srv struct {
		server  *h2ec.Server
		handler http.HandlerFunc
		cfg     *config.Config
	}
)

const (
	websocketProtocol    = "websocket"
	webtransportProtocol = "webtransport"
	acceptStreamTimeout  = 30 * stdlibtime.Second
)
