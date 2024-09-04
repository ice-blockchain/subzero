// SPDX-License-Identifier: ice License 1.0

package http

import (
	"context"
	"fmt"
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
	testDeadline = 30 * time.Second
	certPath     = "%v/../ws/fixture/.testdata/localhost.crt"
	keyPath      = "%v/../ws/fixture/.testdata/localhost.key"
	storageRoot  = "../../.test-uploads"
)

var pubsubServer *fixture.MockService

func TestMain(m *testing.M) {
	serverCtx, serverCancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer serverCancel()
	initServer(serverCtx, serverCancel, 9997, storageRoot)
	http.DefaultClient.Transport = &http2.Transport{TLSClientConfig: fixture.LocalhostTLS()}
	m.Run()
	serverCancel()
}

func initServer(serverCtx context.Context, serverCancel context.CancelFunc, port uint16, storageRoot string) {
	wd, _ := os.Getwd()
	certFilePath := fmt.Sprintf(certPath, wd)
	keyFilePath := fmt.Sprintf(keyPath, wd)
	query.MustInit()
	initStorage(serverCtx, storageRoot)
	uploader := NewUploadHandler(serverCtx)
	pubsubServer = fixture.NewTestServer(serverCtx, serverCancel, &wsserver.Config{
		CertPath: certFilePath,
		KeyPath:  keyFilePath,
		Port:     port,
	}, nil, NewNIP11Handler(), map[string]gin.HandlerFunc{
		"POST /media":         uploader.Upload(),
		"GET /media":          uploader.ListFiles(),
		"GET /media/:file":    uploader.Download(),
		"DELETE /media/:file": uploader.Delete(),
	})
	time.Sleep(100 * time.Millisecond)
}

func TestNIP11(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
	info, err := nip11.Fetch(ctx, "wss://localhost:9997")
	require.NoError(t, err)
	require.NotNil(t, info)
	expected := new(nip11handler).info()
	expected.URL = "wss://localhost:9997"
	assert.Equal(t, expected, info)
}
