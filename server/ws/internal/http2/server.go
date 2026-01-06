// SPDX-License-Identifier: ice License 1.0

package http2

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/rs/zerolog/log"

	h2ec "github.com/ice-blockchain/go/src/net/http"
	"github.com/ice-blockchain/subzero/appcontext"
	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
	"github.com/ice-blockchain/subzero/server/ws/internal/config"
)

func New(cfg *config.Config, router http.Handler) Server {
	s := &srv{cfg: cfg, shutdownCh: make(chan struct{})}
	s.router = router

	return s
}

func (s *srv) ListenAndServeTLS(ctx context.Context, listeners ...net.Listener) error {
	isUnexpectedError := func(err error) bool {
		return err != nil &&
			!errors.Is(err, io.EOF) &&
			!errors.Is(err, h2ec.ErrServerClosed)
	}

	tlsConfig := s.cfg.TLSConfig.Clone()
	if len(tlsConfig.NextProtos) == 0 {
		tlsConfig.NextProtos = []string{"h2", "http/1.1"}
	}

	s.server = &h2ec.Server{
		Handler: s.router,
		BaseContext: func(l net.Listener) context.Context {
			return context.WithValue(ctx, "serverPort", uint16(l.Addr().(*net.TCPAddr).Port))
		},
		TLSConfig: tlsConfig,
	}
	var wg sync.WaitGroup
	errCh := make(chan error, len(listeners))
	for _, l := range listeners {
		wg.Add(1)
		go func(listener net.Listener) {
			defer wg.Done()
			log.Info().Str("protocol", "HTTP2").Str("addr", listener.Addr().String()).Msg("HTTP2 server listening")
			tlsListener := tls.NewListener(listener, tlsConfig)
			if err := s.server.Serve(tlsListener); isUnexpectedError(err) {
				errCh <- errors.Wrapf(err, "failed to serve http2/tcp on %s", listener.Addr().String())
			}
		}(l)
	}

	go func() {
		wg.Wait()
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
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
		if !errors.Is(err, errNoCompression) {
			log.Error().
				Err(err).
				Str("protocol", req.Proto).
				Msg("upgrading failed (http2)")
			writer.WriteHeader(http.StatusBadRequest)
		}
		return
	}
	if wsocket != nil {
		go func() {
			defer appcontext.GetAppContext(ctx).Recover()
			defer func() {
				if clErr := wsocket.Close(); clErr != nil {
					log.Error().
						Err(clErr).
						Msg("failed to close websocket conn")
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

func (s *srv) Shutdown(ctx context.Context) error {
	close(s.shutdownCh)
	return errors.Wrap(s.server.Shutdown(ctx), "failed to close server")
}
