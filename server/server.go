// SPDX-License-Identifier: ice License 1.0

package server

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ice-blockchain/subzero/cfg"
	httpserver "github.com/ice-blockchain/subzero/server/http"
	wsserver "github.com/ice-blockchain/subzero/server/ws"
)

type (
	config struct {
		TLSCert            string `yaml:"tls-cert"`
		TLSKey             string `yaml:"tls-key"`
		Port               uint16 `yaml:"port"`
		Debug              bool   `yaml:"debug"`
		IONLibertyDisabled bool   `yaml:"ion-liberty-disabled"`
	}
	router struct {
	}
)

var (
	globalConfig *config
	globalRouter *router
)

func MustListenAndServe(ctx context.Context) {
	globalConfig = cfg.MustGet[config]()
	globalRouter = &router{}
	internalCfg := &wsserver.Config{
		Port:      globalConfig.Port,
		Debug:     globalConfig.Debug,
		TLSConfig: wsserver.LoadTLSConfig(globalConfig.TLSCert, globalConfig.TLSKey),
	}
	wsserver.New(internalCfg, globalRouter).
		MustListenAndServe(ctx)
}

func (r *router) RegisterRoutes(ctx context.Context, wsroutes wsserver.Router) {
	uploader := httpserver.NewUploadHandler(ctx, globalConfig.IONLibertyDisabled)
	wsroutes.Any("/", wsserver.WithWS(wsserver.NewHandler(), httpserver.NewNIP11Handler(&httpserver.Config{MinLeadingZeroBits: 1111}))).
		POST("/files", uploader.Upload()).
		GET("/files", uploader.ListFiles()).
		GET("/files/:file", uploader.Download()).
		DELETE("/files/:file", uploader.Delete()).
		GET("/.well-known/nostr/nip96.json", uploader.NIP96Info()).
		GET("/health-check", func(c *gin.Context) {
			c.JSON(http.StatusOK, map[string]any{})
			return
		})
}
