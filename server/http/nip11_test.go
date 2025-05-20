// SPDX-License-Identifier: ice License 1.0

package http

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nbd-wtf/go-nostr/nip11"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"

	"github.com/ice-blockchain/subzero/cfg"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	wsserver "github.com/ice-blockchain/subzero/server/ws"
	"github.com/ice-blockchain/subzero/server/ws/fixture"
)

const (
	testDeadline       = 30 * time.Second
	minLeadingZeroBits = 5
	storageRoot        = "../../.test-uploads"
)

var (
	pubsubServer    *fixture.MockService
	privKey, pubKey = model.GenerateKeyPair()
)

func TestMain(m *testing.M) {
	serverCtx, serverCancel := context.WithTimeout(context.Background(), 10*time.Minute)

	addr, release := query.NewTestDatabase(serverCtx)
	query.MustInit(serverCtx, query.WithConfig(&query.Config{
		URL: addr,
	}))

	initServer(serverCtx, 9996)
	http.DefaultClient.Transport = &http2.Transport{TLSClientConfig: fixture.ClientTLS()}
	code := m.Run()
	serverCancel()
	release()
	os.Exit(code)
}

func initServer(serverCtx context.Context, port uint16) {
	initStorage(serverCtx)
	type globalCfg struct {
		TLSCert string `yaml:"tls-cert"`
		TLSKey  string `yaml:"tls-key"`
	}
	globalConfig := cfg.MustGet[globalCfg]()
	uploader := NewUploadHandler(serverCtx, false)
	pubsubServer = fixture.NewTestServer(serverCtx, &wsserver.Config{
		TLSConfig: wsserver.LoadTLSConfig(globalConfig.TLSCert, globalConfig.TLSKey),
		Port:      port,
	}, nil, NewNIP11Handler(serverCtx, &Config{MinLeadingZeroBits: minLeadingZeroBits, PrivateKey: privKey}, uploader.RootPath(), os.TempDir()), map[string]gin.HandlerFunc{
		"POST /files":         uploader.Upload(),
		"GET /files":          uploader.ListFiles(),
		"GET /files/:file":    uploader.Download(),
		"DELETE /files/:file": uploader.Delete(),
	})
	time.Sleep(100 * time.Millisecond)
}

func TestNIP11(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()

	info, err := nip11.Fetch(ctx, "wss://localhost:9996")
	require.NoError(t, err)
	require.NotNil(t, info)

	handler := nip11handler{cfg: &Config{MinLeadingZeroBits: minLeadingZeroBits}, systemMetrics: new(atomic.Pointer[SystemMetrics])}
	expected := handler.info()

	require.Equal(t, "subzero", info.Name)
	require.Equal(t, "subzero", info.Description)
	require.Equal(t, pubKey, info.PubKey)
	require.Equal(t, "~", info.Contact)
	require.Equal(t, "subzero", info.Software)
	require.Equal(t, minLeadingZeroBits, info.Limitation.MinPowDifficulty)
	require.Equal(t, "wss://localhost:9996", info.URL)

	require.Zero(t, slices.CompareFunc(info.SupportedNIPs, expected.RelayInformationDocument.SupportedNIPs, func(a, b any) int {
		require.EqualValues(t, a, b)

		return 0
	}))

	req, err := http.NewRequestWithContext(ctx, "GET", "https://localhost:9996", nil)
	require.NoError(t, err)
	req.Header.Add("Accept", "application/nostr+json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var fullResponse RelayInformationDocument
	err = json.Unmarshal(body, &fullResponse)
	require.NoError(t, err)

	require.Len(t, fullResponse.FCMAndroidConfigs, len(expected.FCMAndroidConfigs))
	require.Len(t, fullResponse.FCMIOSConfigs, len(expected.FCMIOSConfigs))
	require.Len(t, fullResponse.FCMWebConfigs, len(expected.FCMWebConfigs))
	require.NotNil(t, fullResponse.SystemMetrics)
}

func TestFCMConfigParsing(t *testing.T) {
	t.Parallel()

	androidConfig := `{"apiKey":"android-key","appId":"android-app-id","messagingSenderId":"android-messaging-sender","projectId":"android-project"}`
	iosConfig := `{"apiKey":"ios-key","appId":"ios-app-id","messagingSenderId":"ios-messaging-sender","projectId":"ios-project"}`
	webConfig := `{"apiKey":"web-key","appId":"web-app-id","messagingSenderId":"web-messaging-sender","projectId":"web-project"}`

	handler := nip11handler{
		cfg: &Config{
			MinLeadingZeroBits: minLeadingZeroBits,
			FCMAndroidConfigs:  []string{androidConfig},
			FCMIOSConfigs:      []string{iosConfig},
			FCMWebConfigs:      []string{webConfig},
		},
		systemMetrics: new(atomic.Pointer[SystemMetrics]),
	}
	handler.systemMetrics.Store(&SystemMetrics{})
	info := handler.info()

	require.Len(t, info.FCMAndroidConfigs, 1)
	androidCfg := info.FCMAndroidConfigs[0]
	require.Equal(t, "android-key", androidCfg.ApiKey)
	require.Equal(t, "android-app-id", androidCfg.AppID)
	require.Equal(t, "android-messaging-sender", androidCfg.MessagingSenderID)
	require.Equal(t, "android-project", androidCfg.ProjectID)

	require.Len(t, info.FCMIOSConfigs, 1)
	iosCfg := info.FCMIOSConfigs[0]
	require.Equal(t, "ios-key", iosCfg.ApiKey)
	require.Equal(t, "ios-app-id", iosCfg.AppID)
	require.Equal(t, "ios-messaging-sender", iosCfg.MessagingSenderID)
	require.Equal(t, "ios-project", iosCfg.ProjectID)

	require.Len(t, info.FCMWebConfigs, 1)
	webCfg := info.FCMWebConfigs[0]
	require.Equal(t, "web-key", webCfg.ApiKey)
	require.Equal(t, "web-app-id", webCfg.AppID)
	require.Equal(t, "web-messaging-sender", webCfg.MessagingSenderID)
	require.Equal(t, "web-project", webCfg.ProjectID)

	handlerWithInvalidJSON := nip11handler{
		cfg: &Config{
			MinLeadingZeroBits: minLeadingZeroBits,
			FCMAndroidConfigs:  []string{`invalid json`},
		},
		systemMetrics: new(atomic.Pointer[SystemMetrics]),
	}
	handlerWithInvalidJSON.systemMetrics.Store(&SystemMetrics{})
	infoWithInvalidJSON := handlerWithInvalidJSON.info()
	require.Empty(t, infoWithInvalidJSON.FCMAndroidConfigs)
}
