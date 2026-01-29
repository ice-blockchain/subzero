// SPDX-License-Identifier: ice License 1.0

package nip11

import (
	"context"
	"crypto/tls"
	"net/http"
	"os"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nbd-wtf/go-nostr/nip11"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/appcontext"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/server/cert"
	wsserver "github.com/ice-blockchain/subzero/server/ws"
	"github.com/ice-blockchain/subzero/server/ws/fixture"
)

const (
	minLeadingZeroBits = 5
	testRelayURL       = "wss://localhost:9996"
)

var (
	pubsubServer    *fixture.MockService
	privKey, pubKey = model.GenerateKeyPair()
)

func TestMain(m *testing.M) {
	serverCtx, serverCancel := context.WithTimeout(context.Background(), 10*time.Minute)
	serverCtx, _ = appcontext.NewAppContext(serverCtx)
	addr, release := query.NewTestDatabase(serverCtx)
	query.MustInit(serverCtx, query.WithConfig(&query.Config{
		WriteURLs:       []string{addr},
		RunDDL:          true,
		DisableSelfTest: true,
	}))

	initServer(serverCtx, 9996)
	code := m.Run()
	serverCancel()
	release()
	os.Exit(code)
}

func initServer(serverCtx context.Context, port uint16) {
	pubsubServer = fixture.NewTestServer(serverCtx, &wsserver.Config{
		TLSConfig:    cert.MustGenerateTLSConfigSelfSigned("localhost"),
		BindingPorts: []uint16{port},
	}, nil, NewNIP11Handler(serverCtx, &Config{
		MinLeadingZeroBits: minLeadingZeroBits,
		PrivateKey:         privKey,
	}, os.TempDir(), os.TempDir()), map[string]gin.HandlerFunc{})
	time.Sleep(100 * time.Millisecond)
}

func TestNIP11(t *testing.T) {
	t.Parallel()

	handler := nip11handler{cfg: &Config{MinLeadingZeroBits: minLeadingZeroBits}, systemMetrics: new(atomic.Pointer[SystemMetrics])}
	expected := handler.info()
	require.NotNil(t, expected)

	t.Run("Fetch via standard nip11 fetcher", func(t *testing.T) {
		defClient := http.DefaultClient
		defer func() {
			http.DefaultClient = defClient
		}()

		http.DefaultClient.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
		info, err := nip11.Fetch(t.Context(), "wss://localhost:9996")
		require.NoError(t, err)
		require.NotNil(t, info)
		require.Equal(t, "subzero", info.Name)
		require.Equal(t, pubKey, info.PubKey)
		require.Equal(t, testRelayURL, info.URL)

		require.Zero(t, slices.CompareFunc(info.SupportedNIPs, expected.RelayInformationDocument.SupportedNIPs, func(a, b any) int {
			require.EqualValues(t, a, b)
			return 0
		}))
	})
	t.Run("Fetch via custom fetcher", func(t *testing.T) {
		ctx := appcontext.TestContext(t)
		fetcher := NewFetcher(ctx, WithInsecureFetch())
		require.NotNil(t, fetcher)

		data, err := fetcher.Fetch(ctx, "wss://localhost:9996")
		require.NoError(t, err)
		require.NotNil(t, data)

		require.Equal(t, "subzero", data.Name)
		require.Equal(t, pubKey, data.PubKey)
		require.Equal(t, testRelayURL, data.URL)
		require.NotEmpty(t, data.SystemStatus)
		require.Equal(t, SystemStatusStateOK, data.SystemStatus.EventsRead)
		require.Equal(t, SystemStatusStateOK, data.SystemStatus.EventsWrite)

		require.Len(t, data.FCMAndroidConfigs, len(expected.FCMAndroidConfigs))
		require.Len(t, data.FCMIOSConfigs, len(expected.FCMIOSConfigs))
		require.Len(t, data.FCMWebConfigs, len(expected.FCMWebConfigs))
		require.NotNil(t, data.SystemMetrics)
	})
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
