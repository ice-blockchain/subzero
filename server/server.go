// SPDX-License-Identifier: ice License 1.0

package server

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"github.com/ice-blockchain/subzero/appcontext"
	"github.com/ice-blockchain/subzero/cfg"
	"github.com/ice-blockchain/subzero/database/command"
	"github.com/ice-blockchain/subzero/model"
	pushnotifications "github.com/ice-blockchain/subzero/push-notifications"
	"github.com/ice-blockchain/subzero/server/broadcaster"
	"github.com/ice-blockchain/subzero/server/cert"
	"github.com/ice-blockchain/subzero/server/http/events"
	"github.com/ice-blockchain/subzero/server/http/nip11"
	"github.com/ice-blockchain/subzero/server/http/nip96"
	wsserver "github.com/ice-blockchain/subzero/server/ws"
)

type (
	Server interface {
		wsserver.Server
		wsserver.EventBroadcaster
		BroadcastUserEvents(ctx context.Context, events ...*model.Event) error
	}
	Config struct {
		TLSCert             string `yaml:"tls-cert"`
		TLSKey              string `yaml:"tls-key"`
		RelayURL            string `yaml:"relay-url"     validate:"required,url"`
		PrivateKey          string `yaml:"private-key"`
		BroadcastPrivateKey string `yaml:"broadcast-private-key" validate:"required"`
		ACME                struct {
			APIKey string `yaml:"api-key"`
		} `yaml:"acme"`
		Port               uint16 `yaml:"port"          validate:"required,min=1,max=65535"`
		IONLibertyDisabled bool   `yaml:"ion-liberty-disabled"`
		Debug              bool   `yaml:"debug"`
	}
	Option func(*router)

	router struct {
		Config      *Config
		Broadcaster *broadcaster.Broadcaster
		Handler     wsserver.Handler
		Server      wsserver.Server
	}
)

func extractServerNameFromRelayURL(relayURL string) string {
	parsed, err := url.Parse(relayURL)
	if err != nil {
		log.Panic().Str("context", "SERVER").Err(err).Msg("failed to parse relay URL")
	}
	return parsed.Hostname()
}

func WithConfig(cfg *Config) Option {
	return func(s *router) {
		if cfg == nil {
			log.Fatal().Msg("config cannot be nil")
		}
		s.Config = cfg
	}
}

func mustLoadTLSConfig(ctx context.Context, conf *Config) (tls *tls.Config) {
	target := extractServerNameFromRelayURL(conf.RelayURL)
	isIP := net.ParseIP(target) != nil

	switch {
	case (conf.TLSCert == "" && conf.TLSKey == "") || (conf.TLSCert == "-" && conf.TLSKey == "-"):
		log.Trace().Str("target", target).Msg("using ACME to obtain TLS certificate")

		if conf.ACME.APIKey == "" {
			log.Info().Str("target", target).Msg("API key is required for ACME, falling back to self-signed TLS certificate")
			return cert.MustGenerateTLSConfigSelfSigned(target)
		}

		var err error
		if isIP {
			log.Trace().Str("target", target).Msg("using HTTP challenge for IP address")
			tls, err = cert.LoadTLSConfigFromACMEWithHTTP(ctx, target, conf.ACME.APIKey)
		} else {
			log.Trace().Str("target", target).Msg("using DNS challenge for domain")
			tls, err = cert.LoadTLSConfigFromACMEWithDNS(ctx, target, conf.ACME.APIKey)
		}
		if err != nil {
			log.Panic().Err(err).Msg("failed to load TLS config from ACME")
		}

	case conf.TLSCert == "selfsigned" || conf.TLSKey == "selfsigned":
		log.Info().Str("target", target).Msg("using self-signed TLS certificate")
		tls = cert.MustGenerateTLSConfigSelfSigned(target)

	default:
		log.Info().Msg("using provided TLS certificate and key")
		tls = wsserver.LoadTLSConfig(conf.TLSCert, conf.TLSKey)
	}

	return tls
}

func New(ctx context.Context, opts ...Option) Server {
	var r router

	if cfg, err := cfg.Get[Config](); err == nil {
		r.Config = cfg
	} else {
		log.Warn().Err(err).Msg("failed to load config")
	}

	for _, opt := range opts {
		opt(&r)
	}

	if r.Config == nil {
		log.Fatal().Msg("server: config cannot be nil")
	}

	if r.Config.BroadcastPrivateKey == "" && r.Config.PrivateKey != "" {
		log.Warn().Msg("broadcastPrivateKey is empty, using privateKey for broadcasting")
		r.Config.BroadcastPrivateKey = r.Config.PrivateKey
	}

	if err := cfg.Validate(r.Config); err != nil {
		log.Panic().Err(err).Msg("failed to validate config")
	}

	r.Broadcaster = broadcaster.New(broadcaster.Config{
		RelayURL:   r.Config.RelayURL,
		PrivateKey: r.Config.BroadcastPrivateKey,
	})

	appcontext.GetAppContext(ctx).OnShutdown(func() error {
		r.Broadcaster.Close()
		return nil
	})

	public, err := model.GetPublicKey(r.Config.BroadcastPrivateKey)
	if err != nil {
		log.Panic().Err(err).Msg("failed to get public key from private key")
	}
	r.Handler = wsserver.NewHandler(r.Config.RelayURL, public)
	r.Server = wsserver.New(
		&wsserver.Config{
			Port:      r.Config.Port,
			Debug:     r.Config.Debug,
			TLSConfig: mustLoadTLSConfig(ctx, r.Config),
		},
		&r,
	)

	return &r
}

func (r *router) MustListenAndServe(ctx context.Context) {
	r.Server.MustListenAndServe(ctx)
}

func (r *router) BroadcastUserEvents(ctx context.Context, events ...*model.Event) error {
	return r.Broadcaster.Broadcast(ctx, events...)
}

func (r *router) BroadcastNewEvents(ctx context.Context, events ...*model.Event) int {
	return r.Handler.BroadcastNewEvents(ctx, events...)
}

func (r *router) RegisterRoutes(ctx context.Context, wsroutes wsserver.Router) {
	nip11Fetcher := nip11.NewFetcher(ctx)
	uploader := nip96.NewUploadHandler(ctx, r.Config.IONLibertyDisabled, nip11Fetcher)
	tus := uploader.LargeFiles()
	androidConfigs, iosConfigs, webConfigs := pushnotifications.GetFCMConfigs()
	nip11Handler := nip11.NewNIP11Handler(ctx, &nip11.Config{
		MinLeadingZeroBits: 1111,
		FCMAndroidConfigs:  androidConfigs,
		FCMIOSConfigs:      iosConfigs,
		FCMWebConfigs:      webConfigs,
		PrivateKey:         r.Config.PrivateKey,
	}, uploader.RootPath(), command.RootPath())
	wsroutes.Any("/", wsserver.WithWS(r.Handler, nip11Handler)).
		POST("/files", uploader.Upload()).
		GET("/files", uploader.ListFiles()).
		GET("/files/:file", uploader.Download()).
		DELETE("/files/:file", uploader.Delete()).
		HEAD("/files/:file", uploader.CrossRelayDownload()).
		GET("/.well-known/nostr/nip96.json", uploader.NIP96Info()).
		GET("/health-check", func(c *gin.Context) {
			c.JSON(http.StatusOK, map[string]any{})
		}).
		GET("/v1/events/:eventAddress", events.GetEventByAddress).
		Any("/xfiles/* tus-handler", gin.WrapH(http.StripPrefix("/xfiles/", tus)))
}
