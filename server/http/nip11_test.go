// SPDX-License-Identifier: ice License 1.0

package http

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nbd-wtf/go-nostr/nip11"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"

	"github.com/ice-blockchain/subzero/cfg"
	"github.com/ice-blockchain/subzero/database/query"
	wsserver "github.com/ice-blockchain/subzero/server/ws"
	"github.com/ice-blockchain/subzero/server/ws/fixture"
)

const (
	testDeadline       = 30 * time.Second
	minLeadingZeroBits = 5
	storageRoot        = "../../.test-uploads"
)

var (
	pubsubServer *fixture.MockService
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
	}, nil, NewNIP11Handler(&Config{MinLeadingZeroBits: minLeadingZeroBits}), map[string]gin.HandlerFunc{
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

	handler := nip11handler{cfg: &Config{MinLeadingZeroBits: minLeadingZeroBits}}
	expected := handler.info()

	require.Equal(t, "subzero", info.Name)
	require.Equal(t, "subzero", info.Description)
	require.Equal(t, "~", info.PubKey)
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

	require.Equal(t, expected.FCMAndroidConfigs, fullResponse.FCMAndroidConfigs)
	require.Equal(t, expected.FCMIOSConfigs, fullResponse.FCMIOSConfigs)
	require.Equal(t, expected.FCMWebConfigs, fullResponse.FCMWebConfigs)
}
