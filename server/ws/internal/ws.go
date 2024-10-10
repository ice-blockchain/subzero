// SPDX-License-Identifier: ice License 1.0

package internal

import (
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"

	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
	"github.com/ice-blockchain/subzero/server/ws/internal/config"
	"github.com/ice-blockchain/subzero/server/ws/internal/http2"
	"github.com/ice-blockchain/subzero/server/ws/internal/http3"
)

func NewWSServer(router RegisterRoutes, cfg *config.Config) Server {
	s := &Srv{cfg: cfg, routesSetup: router}
	gin.SetMode(gin.ReleaseMode)
	s.router = gin.Default()
	s.router.Use(gin.Recovery())
	s.router.RemoteIPHeaders = []string{"cf-connecting-ip", "X-Real-IP", "X-Forwarded-For"}
	s.router.TrustedPlatform = gin.PlatformCloudflare
	s.router.HandleMethodNotAllowed = true
	s.router.RedirectFixedPath = true
	s.router.RemoveExtraSlash = true
	s.router.UseRawPath = true
	s.H3Server = http3.New(s.cfg, s.router)
	s.H2Server = http2.New(s.cfg, s.router)
	s.setupRouter()
	return s
}

func (s *Srv) setupRouter() {
	s.routesSetup.RegisterRoutes(s.router)
}

func (s *Srv) ListenAndServe(ctx context.Context, cancel context.CancelFunc) {
	ctx = withServer(ctx, s)
	go s.startServer(ctx, s.H3Server)
	go s.startServer(ctx, s.H2Server)
	s.wait(ctx)
	s.shutDown() //nolint:contextcheck // Nope, we want to gracefully shutdown on a different context.
}

func (s *Srv) startServer(ctx context.Context, server interface {
	ListenAndServeTLS(ctx context.Context, certFile, keyFile string) error
}) {
	defer log.Printf("server stopped listening")
	log.Printf("server started listening on %v...", s.cfg.Port)

	isUnexpectedError := func(err error) bool {
		return err != nil &&
			!errors.Is(err, io.EOF) &&
			!errors.Is(err, http.ErrServerClosed)
	}

	if err := server.ListenAndServeTLS(ctx, s.cfg.CertPath, s.cfg.KeyPath); isUnexpectedError(err) {
		s.quit <- syscall.SIGTERM
		log.Printf("ERROR:%v", errors.Wrap(err, "server.ListenAndServeTLS failed"))
	}
}

func (s *Srv) wait(ctx context.Context) {
	quit := make(chan os.Signal, 1)
	s.quit = quit
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-ctx.Done():
	case <-quit:
	}
}

func (s *Srv) shutDown() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.shutdownServer(ctx, s.H3Server)
	go s.shutdownServer(ctx, s.H2Server)
}

func (*Srv) shutdownServer(ctx context.Context, server interface {
	Shutdown(ctx context.Context) error
}) {
	log.Printf("shutting down server...")

	if err := server.Shutdown(ctx); err != nil && !errors.Is(err, io.EOF) {
		log.Printf("ERROR:%v", errors.Wrap(err, "server shutdown failed"))
	} else {
		log.Printf("server shutdown succeeded")
	}
}

func withServer(ctx context.Context, srv *Srv) context.Context {
	return context.WithValue(ctx, adapters.CtxKeyServer, srv)
}
