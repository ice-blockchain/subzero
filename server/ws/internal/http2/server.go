// SPDX-License-Identifier: ice License 1.0

package http2

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"

	"github.com/gookit/goutil/errorx"

	h2ec "github.com/ice-blockchain/go/src/net/http"
	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
	"github.com/ice-blockchain/subzero/server/ws/internal/config"
)

func New(cfg *config.Config, router http.Handler) Server {
	s := &srv{cfg: cfg}
	s.router = router

	return s
}

func (s *srv) ListenAndServeTLS(ctx context.Context, certFile, keyFile string) error {
	s.server = &h2ec.Server{
		Addr:    fmt.Sprintf(":%v", s.cfg.Port),
		Handler: s.router,
		BaseContext: func(_ net.Listener) context.Context {
			return ctx
		},
	}
	isUnexpectedError := func(err error) bool {
		return err != nil &&
			!errors.Is(err, io.EOF) &&
			!errors.Is(err, h2ec.ErrServerClosed)
	}
	if err := s.server.ListenAndServeTLS(certFile, keyFile); isUnexpectedError(err) {
		return errorx.With(err, "failed to start http2/tcp server")
	}

	return nil
}

//nolint:funlen,revive // .
func (s *srv) HandleWS(wsHandler adapters.WSHandler, handler http.Handler, writer http.ResponseWriter, req *http.Request) {
	var wsocket adapters.WSWithWriter
	var ctx context.Context
	var err error
	if req.Header.Get("Upgrade") == websocketProtocol || (req.Method == http.MethodConnect && req.Proto == websocketProtocol) {
		wsocket, ctx, err = s.handleWebsocket(writer, req)
	} else if req.Method == http.MethodConnect && req.Proto == webtransportProtocol {
		wsocket, ctx, err = s.handleWebTransport(writer, req)
	}
	if err != nil {
		log.Printf("ERROR:%v", errorx.Withf(err, "upgrading failed (http2 / %v)", req.Proto))
		writer.WriteHeader(http.StatusBadRequest)

		return
	}
	if wsocket != nil {
		go func() {
			defer func() {
				if clErr := wsocket.Close(); clErr != nil {
					log.Printf("ERROR:%v", errorx.With(clErr, "failed to close websocket conn"))
				}
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

func (s *srv) Shutdown(_ context.Context) error {
	if err := s.server.Close(); err != nil {
		return errorx.Withf(err, "failed to close server")
	}
	return nil
}
