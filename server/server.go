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
	httpserver "github.com/ice-blockchain/subzero/server/http"
	wsserver "github.com/ice-blockchain/subzero/server/ws"
)

type (
	config struct {
		TLSCert            string `yaml:"tls-cert"`
		TLSKey             string `yaml:"tls-key"`
		RelayURL           string `yaml:"relay-url"     validate:"required,url"`
		Port               uint16 `yaml:"port"          validate:"required,min=1,max=65535"`
		Debug              bool   `yaml:"debug"`
		IONLibertyDisabled bool   `yaml:"ion-liberty-disabled"`
		ACME               struct {
			APIKey string `yaml:"api-key"`
		} `yaml:"acme"`
		FCMAndroidConfigs []string `yaml:"fcm-android-configs"`
		FCMIOSConfigs     []string `yaml:"fcm-ios-configs"`
		FCMWebConfigs     []string `yaml:"fcm-web-configs"`
	}
	router struct {
	}
)

var (
	globalConfig *config
	globalRouter *router
)

func extractServerNameFromRelayURL(relayURL string) string {
	pared, err := url.Parse(relayURL)
	if err != nil {
		log.Panic(err)
	}
	return pared.Hostname()
}

func MustListenAndServe(ctx context.Context) {
	var serverTLS *tls.Config

	globalConfig = cfg.MustGet[config]()
	if (globalConfig.TLSCert == "" && globalConfig.TLSKey == "") || (globalConfig.TLSCert == "-" && globalConfig.TLSKey == "-") {
		log.Printf("using ACME to obtain TLS certificate for %v", globalConfig.RelayURL)
		if globalConfig.ACME.APIKey == "" {
			log.Panic("API key is required for ACME DNS challenge")
		}
		serverTLS = MustLoadTLSConfigFromACMEWithDNS(ctx, extractServerNameFromRelayURL(globalConfig.RelayURL), globalConfig.ACME.APIKey)
	} else {
		serverTLS = wsserver.LoadTLSConfig(globalConfig.TLSCert, globalConfig.TLSKey)
	}

	globalRouter = &router{}
	internalCfg := &wsserver.Config{
		Port:      globalConfig.Port,
		Debug:     globalConfig.Debug,
		TLSConfig: serverTLS,
	}
	wsserver.New(internalCfg, globalRouter).
		MustListenAndServe(ctx)
}

func (r *router) RegisterRoutes(ctx context.Context, wsroutes wsserver.Router) {
	uploader := httpserver.NewUploadHandler(ctx, globalConfig.IONLibertyDisabled)
	wsroutes.Any("/", wsserver.WithWS(wsserver.NewHandler(globalConfig.RelayURL), httpserver.NewNIP11Handler(&httpserver.Config{
		MinLeadingZeroBits: 1111,
		FCMAndroidConfigs:  globalConfig.FCMAndroidConfigs,
		FCMIOSConfigs:      globalConfig.FCMIOSConfigs,
		FCMWebConfigs:      globalConfig.FCMWebConfigs,
	}))).
		POST("/files", uploader.Upload()).
		GET("/files", uploader.ListFiles()).
		GET("/files/:file", uploader.Download()).
		DELETE("/files/:file", uploader.Delete()).
		GET("/.well-known/nostr/nip96.json", uploader.NIP96Info()).
		GET("/health-check", func(c *gin.Context) {
			c.JSON(http.StatusOK, map[string]any{})
		})
}
