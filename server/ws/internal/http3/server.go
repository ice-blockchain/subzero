// SPDX-License-Identifier: ice License 1.0

package http3

import (
	"context"
	"fmt"
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/webtransport-go"
	"github.com/rs/zerolog/log"

	"github.com/ice-blockchain/subzero/appcontext"
	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
	"github.com/ice-blockchain/subzero/server/ws/internal/config"
)

func New(cfg *config.Config, router http.Handler, port uint16) Server {
	s := &srv{cfg: cfg, port: port, shutdownCh: make(chan struct{})}
	s.router = router

	return s
}

func (s *srv) ListenAndServeTLS(ctx context.Context) error {
	wtserver := &webtransport.Server{
		H3: http3.Server{
			Addr:    fmt.Sprintf(":%v", s.port),
			Port:    int(s.port),
			Handler: s.router,
			ConnContext: func(connCtx context.Context, c *quic.Conn) context.Context {
				wsserver := ctx.Value(adapters.CtxKeyServer)
				ctx = context.WithValue(connCtx, adapters.CtxKeyServer, wsserver)
				ctx = context.WithValue(ctx, "serverPort", s.port)
				return ctx
			},
			QUICConfig: &quic.Config{
				HandshakeIdleTimeout:  acceptStreamTimeout,
				MaxIdleTimeout:        maxIdleTimeout,
				MaxIncomingStreams:    maxStreamsCount,
				MaxIncomingUniStreams: maxStreamsCount,
			},
			TLSConfig: s.cfg.TLSConfig,
		},
	}
	if s.cfg.Debug {
		noCors := func(_ *http.Request) bool {
			return true
		}
		wtserver.CheckOrigin = noCors
	}
	s.server = wtserver
	if err := s.server.ListenAndServe(); err != nil {
		return errors.Wrap(err, "failed to start http3/udp server")
	}

	return nil
}

//nolint:revive // .
func (s *srv) HandleWS(wsHandler adapters.WSHandler, handler http.Handler, writer http.ResponseWriter, req *http.Request) {
	var ws adapters.WSWithWriter
	var err error
	var ctx context.Context
	if req.Method == http.MethodConnect && req.Proto == "webtransport" {
		ws, ctx, err = s.handleWebTransport(writer, req)
	} else if req.Method == http.MethodConnect && req.Proto == "websocket" {
		ws, ctx, err = s.handleWebsocket(writer, req)
	}
	if err != nil {
		log.Error().
			Err(err).
			Str("protocol", req.Proto).
			Msg("http3 upgrading failed")
		writer.WriteHeader(http.StatusBadRequest)

		return
	}
	if ws != nil {
		go func() {
			defer appcontext.GetAppContext(ctx).Recover()
			defer func() {
				if clErr := ws.Close(); clErr != nil {
					log.Error().
						Err(clErr).
						Msg("failed to close http3 stream")
				}

			}()
			go ws.Write(ctx)        //nolint:contextcheck // It is new context.
			wsHandler.Read(ctx, ws) //nolint:contextcheck // It is new context.
		}()
		return
	} else if handler != nil {
		handler.ServeHTTP(writer, req)

		return
	}
	writer.WriteHeader(http.StatusMethodNotAllowed)
}

func (s *srv) Shutdown(_ context.Context) error {
	close(s.shutdownCh)
	return errors.Wrap(s.server.Close(), "failed to close http3 server")
}
