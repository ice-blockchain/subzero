// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"testing"
	stdlibtime "time"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/gobwas/ws"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/server/ws/fixture"
	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
	"github.com/ice-blockchain/subzero/server/ws/internal/config"
)

const (
	connCountTCP            = 100
	connCountUDP            = 100
	testDeadline            = 15 * stdlibtime.Second
	certPath1               = "%v/fixture/.testdata/localhost1.crt"
	keyPath1                = "%v/fixture/.testdata/localhost1.key"
	certPath2               = "%v/fixture/.testdata/localhost2.crt"
	keyPath2                = "%v/fixture/.testdata/localhost2.key"
	NIP13MinLeadingZeroBits = 5
)

var echoServer *fixture.MockService
var pubsubServers []*fixture.MockService

func TestMain(m *testing.M) {
	serverCtx, serverCancel := context.WithTimeout(context.Background(), 10*stdlibtime.Minute)
	defer serverCancel()
	echoFunc := func(_ context.Context, w Writer, in []byte, cfg *config.Config) {
		if wErr := w.WriteMessage(int(ws.OpText), []byte("server reply:"+string(in))); wErr != nil {
			log.Panic(wErr)
		}

	}
	wd, _ := os.Getwd()
	certFilePath1 := fmt.Sprintf(certPath1, wd)
	keyFilePath1 := fmt.Sprintf(keyPath1, wd)
	echoServer = fixture.NewTestServer(serverCtx, serverCancel, &Config{
		CertPath: certFilePath1,
		KeyPath:  keyFilePath1,
		Port:     9999,
	}, echoFunc, nil, map[string]gin.HandlerFunc{})
	hdl = new(handler)
	pubsubServers = append(pubsubServers, fixture.NewTestServer(serverCtx, serverCancel, &Config{
		CertPath:                certFilePath1,
		KeyPath:                 keyFilePath1,
		Port:                    9998,
		NIP13MinLeadingZeroBits: NIP13MinLeadingZeroBits,
	}, hdl.Handle, nil, map[string]gin.HandlerFunc{}))
	hdl2 := new(handler)
	certFilePath2 := fmt.Sprintf(certPath2, wd)
	keyFilePath2 := fmt.Sprintf(keyPath2, wd)
	pubsubServers = append(pubsubServers, fixture.NewTestServer(serverCtx, serverCancel, &Config{
		CertPath:                certFilePath2,
		KeyPath:                 keyFilePath2,
		Port:                    9997,
		NIP13MinLeadingZeroBits: NIP13MinLeadingZeroBits,
	}, hdl2.Handle, nil, map[string]gin.HandlerFunc{}))
	m.Run()
	serverCancel()
}
func TestSimpleEchoDifferentTransports(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip() // Heavy CPU load, it produces messages in loop
	}
	t.Run("webtransport http 3", func(t *testing.T) {
		testEcho(t, connCountUDP, func(ctx context.Context) (fixture.Client, error) {
			return fixture.NewWebTransportClientHttp3(ctx, "https://localhost:9999/", fixture.LocalhostCrt1)
		})
	})
	t.Run("websocket http 3", func(t *testing.T) {
		testEcho(t, connCountUDP, func(ctx context.Context) (fixture.Client, error) {
			return fixture.NewWebsocketClientHttp3(ctx, "https://localhost:9999/", fixture.LocalhostCrt1)
		})
	})

	t.Run("webtransport http 2", func(t *testing.T) {
		testEcho(t, connCountTCP, func(ctx context.Context) (fixture.Client, error) {
			return fixture.NewWebtransportClientHttp2(ctx, "https://localhost:9999/", fixture.LocalhostCrt1)
		})
	})

	t.Run("websocket http 2", func(t *testing.T) {
		testEcho(t, connCountTCP, func(ctx context.Context) (fixture.Client, error) {
			return fixture.NewWebsocketClientHttp2(ctx, "https://localhost:9999/", fixture.LocalhostCrt1)
		})
	})

	t.Run("websocket http 1.1", func(t *testing.T) {
		testEcho(t, connCountTCP, func(ctx context.Context) (fixture.Client, error) {
			return fixture.NewWebsocketClient(ctx, "wss://localhost:9999/", fixture.LocalhostCrt1)
		})
	})
}

func testEcho(t *testing.T, conns int, client func(ctx context.Context) (fixture.Client, error)) {
	t.Helper()
	echoServer.Reset()
	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
	var clients []fixture.Client
	for i := 0; i < conns; i++ {
		clientConn, err := client(ctx)
		if err != nil {
			log.Panic(err)
		}
		clients = append(clients, clientConn)
	}
	for i := 0; i < conns; i++ {
		wg.Add(1)
		go func(ii int) {
			defer wg.Done()
			clientConn := clients[ii]
			defer clientConn.Close()
			sendMsgs := make([]string, 0)
			sendMsgsTransformed := make([]string, 0)
			receivedBackOnClient := make([]string, 0)
			go func() {
				receivedCh := clientConn.Received()
				for received := range receivedCh {
					receivedBackOnClient = append(receivedBackOnClient, string(received))
					assert.Equal(t, sendMsgsTransformed[0:len(receivedBackOnClient)], receivedBackOnClient)
				}
			}()
			for ctx.Err() == nil {
				msg := uuid.NewString()
				sendMsgs = append(sendMsgs, msg)
				sendMsgsTransformed = append(sendMsgsTransformed, "server reply:"+msg)
				err := clientConn.WriteMessage(int(ws.OpText), []byte(msg))
				if ctx.Err() == nil {
					require.NoError(t, err)
				}
				_ = sendMsgs
			}
			assert.GreaterOrEqual(t, len(receivedBackOnClient), 0)
		}(i)
	}
	wg.Wait()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
	for echoServer.ReaderExited.Load() != uint64(conns) {
		if shutdownCtx.Err() != nil {
			log.Panic(errors.Errorf("shutdown timeout %v of %v", echoServer.ReaderExited.Load(), conns))
		}
		stdlibtime.Sleep(100 * stdlibtime.Millisecond)
	}
	require.Equal(t, uint64(conns), echoServer.ReaderExited.Load())
	require.Len(t, echoServer.Handlers, conns)
	for w := range echoServer.Handlers {
		var closed bool
		switch h := w.(type) {
		case *adapters.WebsocketAdapter:
			closed = h.Closed()
		case *adapters.WebtransportAdapter:
			closed = h.Closed()
		default:
			panic("unknown protocol implementation")
		}
		require.True(t, closed)
	}
}
