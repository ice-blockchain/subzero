// SPDX-License-Identifier: ice License 1.0

package http

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr/nip11"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	wsserver "github.com/ice-blockchain/subzero/server/ws"
	"github.com/ice-blockchain/subzero/server/ws/fixture"
)

const (
	testDeadline = 30 * time.Second
	certPath     = "%v/../ws/fixture/.testdata/localhost.crt"
	keyPath      = "%v/../ws/fixture/.testdata/localhost.key"
)

var pubsubServer *fixture.MockService

func TestMain(m *testing.M) {
	serverCtx, serverCancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer serverCancel()
	wd, _ := os.Getwd()
	certFilePath := fmt.Sprintf(certPath, wd)
	keyFilePath := fmt.Sprintf(keyPath, wd)
	pubsubServer = fixture.NewTestServer(serverCtx, serverCancel, &wsserver.Config{
		CertPath: certFilePath,
		KeyPath:  keyFilePath,
		Port:     9997,
	}, nil, NewNIP11Handler())
	http.DefaultClient.Transport = &http.Transport{TLSClientConfig: fixture.LocalhostTLS()}
	time.Sleep(100 * time.Millisecond)
	m.Run()
	serverCancel()
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
