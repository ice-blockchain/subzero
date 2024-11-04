// SPDX-License-Identifier: ice License 1.0

package internal

import (
	"context"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

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

	return s
}

func (s *Srv) setupRouter(ctx context.Context) {
	s.routesSetup.RegisterRoutes(ctx, s.router)
}

func (s *Srv) runServer(ctx context.Context, wg *sync.WaitGroup, srv internalServer) chan error {
	ch := make(chan error, 1)

	wg.Add(1)
	go func() {
		defer wg.Done()
		err := srv.ListenAndServeTLS(ctx)
		if err != nil && !errors.IsAny(err, io.EOF, http.ErrServerClosed, context.Canceled) {
			ch <- err
		}
	}()

	return ch
}

func (s *Srv) MustListenAndServe(ctx context.Context) {
	var wg sync.WaitGroup

	ctx, cancel := context.WithCancel(withServer(ctx, s))
	s.setupRouter(ctx)
	defer cancel()

	log.Printf("starting servers on port %v...", s.cfg.Port)
	select {
	case err := <-s.runServer(ctx, &wg, s.H2Server):
		log.Panicf("ERROR:%v", errors.Wrap(err, "HTTP2 server start failed"))

	case err := <-s.runServer(ctx, &wg, s.H3Server):
		log.Panicf("ERROR:%v", errors.Wrap(err, "HTTP3 server start failed"))

	case <-ctx.Done():
	}

	log.Println("shutting down servers ...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Minute)
	defer shutdownCancel()

	if err := s.H2Server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, io.EOF) {
		log.Printf("ERROR:%v", errors.Wrap(err, "HTTP2 server shutdown failed"))
	}
	if err := s.H3Server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, io.EOF) {
		log.Printf("ERROR:%v", errors.Wrap(err, "HTTP3 server shutdown failed"))
	}

	wg.Wait()
	log.Println("servers stopped")
}

func withServer(ctx context.Context, srv *Srv) context.Context {
	return context.WithValue(ctx, adapters.CtxKeyServer, srv)
}
