// SPDX-License-Identifier: ice License 1.0

package http

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nbd-wtf/go-nostr/nip11"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"

	"github.com/ice-blockchain/subzero/database/query"
	wsserver "github.com/ice-blockchain/subzero/server/ws"
	"github.com/ice-blockchain/subzero/server/ws/fixture"
)

const (
	testDeadline       = 30 * time.Second
	minLeadingZeroBits = 5
	storageRoot        = "../../.test-uploads"
)

var pubsubServer *fixture.MockService

func TestMain(m *testing.M) {
	serverCtx, serverCancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer serverCancel()
	query.MustInit()
	initServer(serverCtx, serverCancel, 9997, storageRoot)
	http.DefaultClient.Transport = &http2.Transport{TLSClientConfig: fixture.LocalhostTLS()}
	code := m.Run()
	serverCancel()
	os.Exit(code)
}

func initServer(serverCtx context.Context, serverCancel context.CancelFunc, port uint16, storageRoot string) {
	initStorage(serverCtx, storageRoot)
	uploader := NewUploadHandler(serverCtx)
	pubsubServer = fixture.NewTestServer(serverCtx, serverCancel, &wsserver.Config{
		TLSConfig: fixture.LocalhostTLS(),
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
	ctx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
	info, err := nip11.Fetch(ctx, "wss://localhost:9997")
	require.NoError(t, err)
	require.NotNil(t, info)
	handler := nip11handler{cfg: &Config{MinLeadingZeroBits: minLeadingZeroBits}}
	expected := handler.info()
	expected.URL = "wss://localhost:9997"
	assert.Equal(t, expected, info)
}
