// SPDX-License-Identifier: ice License 1.0

package internal

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
	"github.com/ice-blockchain/subzero/server/ws/internal/config"
	"github.com/ice-blockchain/subzero/server/ws/internal/http2"
	"github.com/ice-blockchain/subzero/server/ws/internal/http3"
)

func NewWSServer(router RegisterRoutes, cfg *config.Config) Server {
	s := &Srv{cfg: cfg, routesSetup: router}
	if cfg.Debug {
		gin.SetMode(gin.DebugMode)
		s.router = gin.Default()
		pprof.Register(s.router, "subzero/pprof")
	} else {
		gin.SetMode(gin.ReleaseMode)
		s.router = gin.New()
	}
	s.router.Use(gin.Recovery())
	s.router.RemoteIPHeaders = []string{"cf-connecting-ip", "X-Real-IP", "X-Forwarded-For"}
	s.router.TrustedPlatform = gin.PlatformCloudflare
	s.router.HandleMethodNotAllowed = true
	s.router.RedirectFixedPath = true
	s.router.RemoveExtraSlash = true
	s.router.UseRawPath = true
	s.H3Server = http3.New(s.cfg, s.router, s.cfg.BindingPorts[0])
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

	portsMap := make(map[uint16]struct{})
	for _, port := range s.cfg.BindingPorts {
		if port > 0 {
			portsMap[port] = struct{}{}
		}
	}
	allPorts := make([]uint16, 0, len(portsMap))
	for port := range portsMap {
		allPorts = append(allPorts, port)
	}
	if len(allPorts) > 1 {
		log.Info().Uints16("ports", allPorts).Msg("starting servers on multiple ports")
	} else {
		log.Info().Uint16("port", allPorts[0]).Msg("starting server")
	}
	commonErrCh := make(chan error, len(allPorts)+1)
	h2Listeners := make([]net.Listener, 0, len(allPorts))
	for _, port := range allPorts {
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err != nil {
			log.Panic().Err(err).Uint16("port", port).Msg("failed to create HTTP2 listener")
		}
		h2Listeners = append(h2Listeners, listener)
		log.Info().Uint16("port", port).Msg("created HTTP2 listener")
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := s.H2Server.ListenAndServeTLS(ctx, h2Listeners...); err != nil && !errors.IsAny(err, io.EOF, http.ErrServerClosed, context.Canceled) {
			log.Error().Err(err).Msg("HTTP2 server error")
			commonErrCh <- err
		}
	}()

	type h3ServerInfo struct {
		server http3.Server
		port   uint16
	}
	h3Servers := make([]h3ServerInfo, 0, len(allPorts))

	for i, port := range allPorts {
		var h3Server http3.Server
		if i == 0 {
			h3Server = s.H3Server
		} else {
			h3Server = http3.New(s.cfg, s.router, port)
		}

		h3Servers = append(h3Servers, h3ServerInfo{
			server: h3Server,
			port:   port,
		})

		h3ErrCh := s.runServer(ctx, &wg, h3Server)
		go func(p uint16, ch chan error) {
			select {
			case err := <-ch:
				if err != nil {
					log.Error().Str("protocol", "HTTP3").Uint16("port", p).Err(err).Msg("server error")
					commonErrCh <- err
				}
			case <-ctx.Done():
				return
			}
		}(port, h3ErrCh)
	}
	select {
	case err := <-commonErrCh:
		log.Panic().Str("context", "HTTP2/HTTP3").Err(err).Msg("server failed to start")
	case <-ctx.Done():
	}
	log.Info().Msg("shutting down servers")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Minute)
	defer shutdownCancel()
	for _, listener := range h2Listeners {
		if err := listener.Close(); err != nil && !errors.Is(err, io.EOF) {
			log.Error().Err(err).Str("addr", listener.Addr().String()).Msg("failed to close HTTP2 listener")
		}
	}
	if err := s.H2Server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, io.EOF) {
		log.Error().Str("context", "HTTP2").Err(err).Msg("server shutdown failed")
	}
	for _, h3Info := range h3Servers {
		if err := h3Info.server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, io.EOF) {
			log.Error().Str("context", "HTTP3").Uint16("port", h3Info.port).Err(err).Msg("server shutdown failed")
		}
	}

	wg.Wait()
	log.Info().Msg("servers stopped")
}

func withServer(ctx context.Context, srv *Srv) context.Context {
	return context.WithValue(ctx, adapters.CtxKeyServer, srv)
}
