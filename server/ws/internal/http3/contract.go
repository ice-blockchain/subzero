// SPDX-License-Identifier: ice License 1.0

package http3

import (
	"context"
	"github.com/ice-blockchain/subzero/server/ws/internal/config"
	"github.com/quic-go/webtransport-go"
	"net/http"
	stdlibtime "time"
)

type (
	Server interface {
		ListenAndServeTLS(ctx context.Context, certFile, keyFile string) error
		Shutdown(ctx context.Context) error
	}
)
type (
	srv struct {
		server  *webtransport.Server
		handler http.HandlerFunc
		cfg     *config.Config
	}
)

const (
	acceptStreamTimeout = 60 * stdlibtime.Second
	maxIdleTimeout      = 7 * 24 * stdlibtime.Hour
	maxStreamsCount     = 1<<60 - 1
)
