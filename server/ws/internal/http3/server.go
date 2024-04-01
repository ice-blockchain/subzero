// SPDX-License-Identifier: ice License 1.0

package http3

import (
	"context"
	"fmt"
	"github.com/gookit/goutil/errorx"
	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
	"github.com/ice-blockchain/subzero/server/ws/internal/config"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/quic-go/qlog"
	"github.com/quic-go/webtransport-go"
	"net/http"

	"log"
)

func New(cfg *config.Config, wshandler adapters.WSHandler, handler http.Handler) Server {
	s := &srv{cfg: cfg}
	s.handler = s.handle(wshandler, handler)

	return s
}

func (s *srv) ListenAndServeTLS(_ context.Context, certFile, keyFile string) error {
	wtserver := &webtransport.Server{
		H3: http3.Server{
			Addr:    fmt.Sprintf(":%v", s.cfg.Port),
			Port:    int(s.cfg.Port),
			Handler: s.handler,
			QuicConfig: &quic.Config{
				Tracer:                qlog.DefaultTracer,
				HandshakeIdleTimeout:  acceptStreamTimeout,
				MaxIdleTimeout:        maxIdleTimeout,
				MaxIncomingStreams:    maxStreamsCount,
				MaxIncomingUniStreams: maxStreamsCount,
			},
		},
	}
	if /*s.cfg.Development*/ true {
		noCors := func(_ *http.Request) bool {
			return true
		}
		wtserver.CheckOrigin = noCors
	}
	s.server = wtserver
	if err := s.server.ListenAndServeTLS(certFile, keyFile); err != nil {
		return errorx.With(err, "failed to start http3/udp server")
	}
	return nil
}

//nolint:revive // .
func (s *srv) handle(wsHandler adapters.WSHandler, handler http.Handler) http.HandlerFunc {
	return func(writer http.ResponseWriter, req *http.Request) {
		var ws adapters.WSWithWriter
		var err error
		var ctx context.Context
		if req.Method == http.MethodConnect && req.Proto == "webtransport" {
			ws, ctx, err = s.handleWebTransport(writer, req)
		} else if req.Method == http.MethodConnect && req.Proto == "websocket" {
			ws, ctx, err = s.handleWebsocket(writer, req)
		}
		if err != nil {
			log.Printf("ERROR:%v", errorx.Withf(err, "http3: upgrading failed for %v", req.Proto))
			writer.WriteHeader(http.StatusBadRequest)

			return
		}
		if ws != nil {
			go func() {
				defer func() {
					if clErr := ws.Close(); clErr != nil {
						log.Printf("ERROR:%v", errorx.Withf(clErr, "failed to close http3 stream"))
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
}

func (s *srv) Shutdown(_ context.Context) error {
	return errorx.With(s.server.Close(), "failed to close http3 server")
}
