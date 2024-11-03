// SPDX-License-Identifier: ice License 1.0

package internal

import (
	"context"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"

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

	return s
}

func (s *Srv) setupRouter(ctx context.Context) {
	s.routesSetup.RegisterRoutes(ctx, s.router)
}

func (s *Srv) ListenAndServe(ctx context.Context) {
	s.setupRouter(ctx)
	group, ctx := errgroup.WithContext(withServer(ctx, s))

	log.Printf("starting servers on port %v...", s.cfg.Port)
	group.Go(func() error {
		return errors.Wrap(s.H2Server.ListenAndServeTLS(ctx), "cannot start HTTP2 server")
	})
	group.Go(func() error {
		return errors.Wrap(s.H3Server.ListenAndServeTLS(ctx), "cannot start HTTP3 server")
	})

	err := group.Wait()
	if err != nil && !errors.IsAny(err, io.EOF, http.ErrServerClosed, context.Canceled) {
		log.Printf("ERROR:%v", errors.Wrap(err, "server stopped unexpectedly"))
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	log.Println("shutting down servers ...")
	if err = s.H2Server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, io.EOF) {
		log.Printf("ERROR:%v", errors.Wrap(err, "HTTP2 server shutdown failed"))
	}
	if err = s.H3Server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, io.EOF) {
		log.Printf("ERROR:%v", errors.Wrap(err, "HTTP3 server shutdown failed"))
	}
}

func withServer(ctx context.Context, srv *Srv) context.Context {
	return context.WithValue(ctx, adapters.CtxKeyServer, srv)
}
