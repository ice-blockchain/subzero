// SPDX-License-Identifier: ice License 1.0

package server

import (
	"context"
	"crypto/tls"
	"log"

	"github.com/ice-blockchain/subzero/cfg"
	httpserver "github.com/ice-blockchain/subzero/server/http"
	wsserver "github.com/ice-blockchain/subzero/server/ws"
)

type (
	config struct {
		TLSCert string `yaml:"tls-cert"`
		TLSKey  string `yaml:"tls-key"`
		Port    uint16 `yaml:"port"`
		Debug   bool   `yaml:"debug"`
	}
	router struct {
	}
)

var (
	globalConfig *config
	globalRouter *router
)

func ListenAndServe(ctx context.Context, cancel context.CancelFunc) {
	globalConfig = cfg.MustGet[config]()
	globalRouter = &router{}
	internalCfg := &wsserver.Config{
		Port:      globalConfig.Port,
		Debug:     globalConfig.Debug,
		TLSConfig: loadTLSConfig(globalConfig.TLSCert, globalConfig.TLSKey),
	}
	wsserver.New(internalCfg, globalRouter).ListenAndServe(ctx, cancel)
}

func loadTLSConfig(certFileName, keyFileName string) *tls.Config {
	cert, err := tls.LoadX509KeyPair(certFileName, keyFileName)
	if err != nil {
		log.Panic(err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
	}
}

func (r *router) RegisterRoutes(ctx context.Context, wsroutes wsserver.Router) {
	uploader := httpserver.NewUploadHandler(ctx)
	wsroutes.Any("/", wsserver.WithWS(wsserver.NewHandler(), httpserver.NewNIP11Handler(&httpserver.Config{MinLeadingZeroBits: 1111}))).
		POST("/files", uploader.Upload()).
		GET("/files", uploader.ListFiles()).
		GET("/files/:file", uploader.Download()).
		DELETE("/files/:file", uploader.Delete()).
		GET("/.well-known/nostr/nip96.json", uploader.NIP96Info())
}
