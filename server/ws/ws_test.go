// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gobwas/ws"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/ice-blockchain/subzero/cfg"
	"github.com/ice-blockchain/subzero/database/command"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/dvm"
	"github.com/ice-blockchain/subzero/server/ws/fixture"
	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
	"github.com/ice-blockchain/subzero/server/ws/internal/config"
	"github.com/ice-blockchain/subzero/validation"
)

const (
	connCountTCP            = 100
	connCountUDP            = 100
	testDeadline            = 15 * time.Second
	NIP13MinLeadingZeroBits = 5
)

var (
	echoServer    *fixture.MockService
	pubsubServers []*fixture.MockService
)

type globalCfg struct {
	TLSCert string `yaml:"tls-cert"`
	TLSKey  string `yaml:"tls-key"`
}

func TestMain(m *testing.M) {
	globalConfig := cfg.MustGet[globalCfg]()
	serverCtx, serverCancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer serverCancel()
	validation.MustInit()
	addr, release := query.NewTestDatabase(serverCtx)
	log.Println(addr)
	closeFuncs := []func() error{release}
	dvm.MustInit(serverCtx)
	echoFunc := func(_ context.Context, w Writer, in []byte, cfg *config.Config) {
		if wErr := w.WriteMessage(int(ws.OpText), []byte("server reply:"+string(in))); wErr != nil {
			log.Panic(wErr)
		}
	}

	echoServer = fixture.NewTestServer(
		serverCtx,
		&Config{
			Port:      9999,
			TLSConfig: LoadTLSConfig(globalConfig.TLSCert, globalConfig.TLSKey),
		},
		echoFunc,
		nil,
		map[string]gin.HandlerFunc{},
	)

	server, release := helperCreateWsInstance(serverCtx, globalConfig,
		9988, 19988,
		"./../database/command/.testdata/node_key.json",
		"../../.cometbft",
	)
	pubsubServers = append(pubsubServers, server)
	closeFuncs = append(closeFuncs, release)

	server2, release2 := helperCreateWsInstance(serverCtx, globalConfig,
		9977, 19977,
		"./../database/command/.testdata/node_key2.json",
		"../../.cometbft2",
	)
	pubsubServers = append(pubsubServers, server2)
	closeFuncs = append(closeFuncs, release2)

	server3, release3 := helperCreateWsInstance(serverCtx, globalConfig,
		9966, 19966,
		"./../database/command/.testdata/node_key3.json",
		"../../.cometbft3",
	)
	pubsubServers = append(pubsubServers, server3)
	closeFuncs = append(closeFuncs, release3)
	code := m.Run()
	serverCancel()
	for _, closeDb := range closeFuncs {
		closeDb()
	}
	os.RemoveAll("../../.cometbft")
	os.RemoveAll("../../.cometbft2")
	os.RemoveAll("../../.cometbft3")
	if code == 0 {
		if err := goleak.Find(); err != nil {
			log.Printf("goleak: %v", err)
			code = 1
		}
	}

	os.Exit(code)
}

func helperCreateWsInstance(serverCtx context.Context, globalConfig *globalCfg, wsPort, consensusPort uint16, consensusKey, consensusStorage string) (*fixture.MockService, func() error) {
	addr, release := query.NewTestDatabase(serverCtx)
	log.Println(addr)
	hdl := newHandler(fmt.Sprintf("wss://localhost:%v", wsPort))
	srv := fixture.NewTestServer(serverCtx, &Config{
		Port:      wsPort,
		TLSConfig: LoadTLSConfig(globalConfig.TLSCert, globalConfig.TLSKey),
	}, hdl.Handle, nil, map[string]gin.HandlerFunc{})
	srv.DB = query.GetDB(serverCtx, query.WithConfig(&query.Config{
		URL: addr,
	}))

	srv.Consensus = command.GetConsensus(serverCtx, command.WithConfig(&command.Config{
		AbsoluteRootPath:           consensusStorage,
		AbsoluteNodePrivateKeyPath: consensusKey,
		DiscoveryPort:              consensusPort,
	}))
	return srv, release
}

func TestSimpleEchoDifferentTransports(t *testing.T) {
	if os.Getenv("TEST_ECHO") != "y" {
		t.Skip("set TEST_ECHO=y to run this test")
	}
	t.Run("webtransport http 3", func(t *testing.T) {
		testEcho(t, connCountUDP, func(ctx context.Context) (fixture.Client, error) {
			return fixture.NewWebTransportClientHttp3(ctx, "https://localhost:9999/")
		})
	})
	t.Run("websocket http 3", func(t *testing.T) {
		testEcho(t, connCountUDP, func(ctx context.Context) (fixture.Client, error) {
			return fixture.NewWebsocketClientHttp3(ctx, "https://localhost:9999/")
		})
	})

	t.Run("webtransport http 2", func(t *testing.T) {
		testEcho(t, connCountTCP, func(ctx context.Context) (fixture.Client, error) {
			return fixture.NewWebtransportClientHttp2(ctx, "https://localhost:9999/")
		})
	})

	t.Run("websocket http 2", func(t *testing.T) {
		testEcho(t, connCountTCP, func(ctx context.Context) (fixture.Client, error) {
			return fixture.NewWebsocketClientHttp2(ctx, "https://localhost:9999/")
		})
	})

	t.Run("websocket http 1.1", func(t *testing.T) {
		testEcho(t, connCountTCP, func(ctx context.Context) (fixture.Client, error) {
			return fixture.NewWebsocketClient(ctx, "wss://localhost:9999/")
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
	require.NoError(t, echoServer.WaitForReaders(testDeadline))
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
