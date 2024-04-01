// SPDX-License-Identifier: ice License 1.0

package http2

import (
	"context"
	"fmt"
	h2ec "github.com/ice-blockchain/go/src/net/http"
	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
	"github.com/ice-blockchain/subzero/server/ws/internal/config"
	"github.com/pkg/errors"
	"net"
	"net/http"

	"github.com/ice-blockchain/wintr/log"
)

func New(cfg *config.Config, wshandler adapters.WSHandler, handler http.Handler) Server {
	s := &srv{cfg: cfg}
	s.handler = s.handle(wshandler, handler)

	return s
}

func (s *srv) ListenAndServeTLS(ctx context.Context, certFile, keyFile string) error {
	s.server = &h2ec.Server{
		Addr:    fmt.Sprintf(":%v", s.cfg.WSServer.Port),
		Handler: s.handler,
		BaseContext: func(_ net.Listener) context.Context {
			return ctx
		},
	}

	return errors.Wrap(s.server.ListenAndServeTLS(certFile, keyFile), "failed to start http2/tcp server")
}

//nolint:funlen,revive // .
func (s *srv) handle(wsHandler adapters.WSHandler, handler http.Handler) http.HandlerFunc {
	return func(writer http.ResponseWriter, req *http.Request) {
		var wsocket adapters.WSWithWriter
		var ctx context.Context
		var err error
		if req.Header.Get("Upgrade") == websocketProtocol || (req.Method == http.MethodConnect && req.Proto == websocketProtocol) {
			wsocket, ctx, err = s.handleWebsocket(writer, req)
		} else if req.Method == http.MethodConnect && req.Proto == webtransportProtocol {
			wsocket, ctx, err = s.handleWebTransport(writer, req)
		}
		if err != nil {
			log.Error(errors.Wrapf(err, "upgrading failed (http2 / %v)", req.Proto))
			writer.WriteHeader(http.StatusBadRequest)

			return
		}
		if wsocket != nil {
			go func() {
				defer func() {
					log.Error(wsocket.Close(), "failed to close websocket conn")
				}()
				go wsocket.Write(ctx)
				wsHandler.Read(ctx, wsocket)
			}()

			return
		} else if handler != nil {
			handler.ServeHTTP(writer, req)

			return
		}
		writer.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *srv) Shutdown(_ context.Context) error {
	return errors.Wrapf(s.server.Close(), "failed to close server")
}
