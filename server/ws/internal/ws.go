// SPDX-License-Identifier: ice License 1.0

package internal

import (
	"context"
	"fmt"
	"github.com/gookit/goutil/errorx"
	"github.com/ice-blockchain/subzero/server/ws/internal/config"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"errors"

	"github.com/ice-blockchain/subzero/server/ws/internal/http2"
	"github.com/ice-blockchain/subzero/server/ws/internal/http3"
	"log"
)

func NewWSServer(service WSHandler, cfg *config.Config) Server {
	s := &srv{cfg: cfg, wsHandler: service}
	s.h3server = http3.New(s.cfg, s.wsHandler, nil)
	s.wsServer = http2.New(s.cfg, s.wsHandler, nil)

	return s
}
func NewWSServerWithExtraHandler(service WSHandler, cfg *config.Config, handler http.Handler) Server {
	s := &srv{cfg: cfg, wsHandler: service}
	s.h3server = http3.New(s.cfg, s.wsHandler, handler)
	s.wsServer = http2.New(s.cfg, s.wsHandler, handler)

	return s
}

func (s *srv) ListenAndServe(ctx context.Context, cancel context.CancelFunc) {
	go s.startServer(ctx, s.h3server)
	go s.startServer(ctx, s.wsServer)
	s.wait(ctx)
	s.shutDown() //nolint:contextcheck // Nope, we want to gracefully shutdown on a different context.
}

func (s *srv) startServer(ctx context.Context, server interface {
	ListenAndServeTLS(ctx context.Context, certFile, keyFile string) error
}) {
	defer log.Printf("server stopped listening")
	log.Printf(fmt.Sprintf("server started listening on %v...", s.cfg.Port))

	isUnexpectedError := func(err error) bool {
		return err != nil &&
			!errors.Is(err, io.EOF) &&
			!errors.Is(err, http.ErrServerClosed)
	}

	if err := server.ListenAndServeTLS(ctx, s.cfg.CertPath, s.cfg.KeyPath); isUnexpectedError(err) {
		s.quit <- syscall.SIGTERM
		log.Printf("ERROR:%v", errorx.With(err, "server.ListenAndServeTLS failed"))
	}
}

func (s *srv) wait(ctx context.Context) {
	quit := make(chan os.Signal, 1)
	s.quit = quit
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-ctx.Done():
	case <-quit:
	}
}

func (s *srv) shutDown() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.shutdownServer(ctx, s.h3server)
	go s.shutdownServer(ctx, s.wsServer)
}

func (*srv) shutdownServer(ctx context.Context, server interface {
	Shutdown(ctx context.Context) error
}) {
	log.Printf("shutting down server...")

	if err := server.Shutdown(ctx); err != nil && !errors.Is(err, io.EOF) {
		log.Printf("ERROR:%v", errorx.With(err, "server shutdown failed"))
	} else {
		log.Printf("server shutdown succeeded")
	}
}
