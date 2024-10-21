// SPDX-License-Identifier: ice License 1.0

package http3

import (
	"context"
	"net/http"
	stdlibtime "time"

	"github.com/quic-go/webtransport-go"

	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
	"github.com/ice-blockchain/subzero/server/ws/internal/config"
)

type (
	Server interface {
		ListenAndServeTLS(ctx context.Context) error
		Shutdown(ctx context.Context) error
		HandleWS(wsHandler adapters.WSHandler, handler http.Handler, writer http.ResponseWriter, req *http.Request)
	}
)
type (
	srv struct {
		server *webtransport.Server
		router http.Handler
		cfg    *config.Config
	}
)

const (
	acceptStreamTimeout = 60 * stdlibtime.Second
	maxIdleTimeout      = 7 * 24 * stdlibtime.Hour
	maxStreamsCount     = 1<<60 - 1
)
