// SPDX-License-Identifier: ice License 1.0

package server

import (
	"context"
	"crypto/tls"
	"log"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"

	"github.com/ice-blockchain/subzero/cfg"
	"github.com/ice-blockchain/subzero/database/command"
	"github.com/ice-blockchain/subzero/model"
	pushnotifications "github.com/ice-blockchain/subzero/push-notifications"
	"github.com/ice-blockchain/subzero/server/http/nip11"
	"github.com/ice-blockchain/subzero/server/http/nip96"
	wsserver "github.com/ice-blockchain/subzero/server/ws"
)

type (
	Server interface {
		wsserver.Server
		wsserver.EventBroadcaster
	}
	Config struct {
		TLSCert            string `yaml:"tls-cert"`
		TLSKey             string `yaml:"tls-key"`
		RelayURL           string `yaml:"relay-url"     validate:"required,url"`
		Port               uint16 `yaml:"port"          validate:"required,min=1,max=65535"`
		IONLibertyDisabled bool   `yaml:"ion-liberty-disabled"`
		Debug              bool   `yaml:"debug"`
		PrivateKey         string `yaml:"private-key"`
		ACME               struct {
			APIKey string `yaml:"api-key"`
		} `yaml:"acme"`
	}
	Option func(*router)

	router struct {
		Config  *Config
		Handler wsserver.Handler
		Server  wsserver.Server
	}
)

func extractServerNameFromRelayURL(relayURL string) string {
	parsed, err := url.Parse(relayURL)
	if err != nil {
		log.Panic(err)
	}
	return parsed.Hostname()
}

func WithConfig(cfg *Config) Option {
	return func(s *router) {
		if cfg == nil {
			log.Panic("config cannot be nil")
		}
		s.Config = cfg
	}
}

func New(ctx context.Context, opts ...Option) Server {
	var r router

	if cfg, err := cfg.Get[Config](); err == nil {
		r.Config = cfg
	} else {
		log.Printf("[WARN] failed to load config: %v", err)
	}

	for _, opt := range opts {
		opt(&r)
	}

	if r.Config == nil {
		log.Panic("server: config cannot be nil")
	} else if err := cfg.Validate(r.Config); err != nil {
		log.Panicf("failed to validate config: %v", err)
	}

	var tls *tls.Config
	switch {
	case (r.Config.TLSCert == "" && r.Config.TLSKey == "") || (r.Config.TLSCert == "-" && r.Config.TLSKey == "-"):
		log.Printf("using ACME to obtain TLS certificate for %v", r.Config.RelayURL)
		if r.Config.ACME.APIKey == "" {
			log.Panic("API key is required for ACME DNS challenge")
		}
		tls = MustLoadTLSConfigFromACMEWithDNS(ctx, extractServerNameFromRelayURL(r.Config.RelayURL), r.Config.ACME.APIKey)

	case r.Config.TLSCert == "selfsigned" || r.Config.TLSKey == "selfsigned":
		log.Printf("using self-signed TLS certificate for %v", r.Config.RelayURL)
		tls = MustGenerateTLSConfigSelfSigned(extractServerNameFromRelayURL(r.Config.RelayURL))

	default:
		log.Println("using provided TLS certificate and key")
		tls = wsserver.LoadTLSConfig(r.Config.TLSCert, r.Config.TLSKey)
	}

	r.Handler = wsserver.NewHandler(r.Config.RelayURL)
	r.Server = wsserver.New(
		&wsserver.Config{
			Port:      r.Config.Port,
			Debug:     r.Config.Debug,
			TLSConfig: tls,
		},
		&r,
	)

	return &r
}

func (r *router) MustListenAndServe(ctx context.Context) {
	r.Server.MustListenAndServe(ctx)
}

func (r *router) BroadcastNewEvents(ctx context.Context, events ...*model.Event) {
	r.Handler.BroadcastNewEvents(ctx, events...)
}

func (r *router) RegisterRoutes(ctx context.Context, wsroutes wsserver.Router) {
	nip11Fetcher := nip11.NewFetcher(ctx)
	uploader := nip96.NewUploadHandler(ctx, r.Config.IONLibertyDisabled, nip11Fetcher)
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
		})
}
