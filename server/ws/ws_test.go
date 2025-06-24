// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gobwas/ws"
	"github.com/google/uuid"
	"github.com/panjf2000/ants/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/ice-blockchain/subzero/cfg"
	"github.com/ice-blockchain/subzero/database/command"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/dvm"
	"github.com/ice-blockchain/subzero/server/ws/fixture"
	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
	"github.com/ice-blockchain/subzero/validation"
)

const (
	testDeadline            = time.Minute
	NIP13MinLeadingZeroBits = 5
)

var (
	pubsubServers      []*fixture.MockService
	pubsubServersExtra []*fixture.MockService // For `TestConsensusEvents`.
)

type globalCfg struct {
	TLSCert string `yaml:"tls-cert"`
	TLSKey  string `yaml:"tls-key"`
}

func TestMain(m *testing.M) {
	var closeFuncs []func() error

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	container := query.NewTestContainer(ctx)
	closeFuncs = append(closeFuncs, func() error {
		return container.Close(context.WithoutCancel(ctx))
	})
	tempDB, tempDBClose := container.MustTempDB(ctx)
	query.MustInit(ctx, query.WithConfig(&query.Config{
		URL: tempDB,
	}))
	closeFuncs = append(closeFuncs, func() error {
		tempDBClose()
		return nil
	})

	validation.MustInit()
	dvm.MustInit(ctx)

	for _, wsPort := range []uint16{9988, 9977, 9966} {
		const discoveryPortDelta = 10_000
		log.Printf("Starting server on port %d / %d", wsPort, wsPort+discoveryPortDelta)
		server, release := helperCreateWsInstance(ctx, container, wsPort, wsPort+discoveryPortDelta)
		pubsubServers = append(pubsubServers, server)
		closeFuncs = append(closeFuncs, release)
	}

	// Used in `TestConsensusEvents`.
	for _, wsPort := range []uint16{9955, 9944} {
		const discoveryPortDelta = 10_000
		log.Printf("Starting server on port %d / %d", wsPort, wsPort+discoveryPortDelta)
		server, release := helperCreateWsInstance(ctx, container, wsPort, wsPort+discoveryPortDelta)
		pubsubServersExtra = append(pubsubServersExtra, server)
		closeFuncs = append(closeFuncs, release)
	}

	code := m.Run()

	// Close all servers in reverse order.
	cancel()
	for i := len(closeFuncs) - 1; i >= 0; i-- {
		closeFuncs[i]()
	}

	ants.Release()

	if code == 0 {
		time.Sleep(15 * time.Second)
		if err := goleak.Find(); err != nil {
			log.Printf("goleak: %v", err)
			code = 1
		}
	}

	os.Exit(code)
}

func helperCreateWsInstance(
	ctx context.Context,
	databaseContainer *query.Container,
	wsPort uint16,
	consensusPort uint16,
) (*fixture.MockService, func() error) {
	tlsData := cfg.MustGet[globalCfg]()
	tlsConfig := LoadTLSConfig(tlsData.TLSCert, tlsData.TLSKey)

	db, releaseDB := query.NewTestDatabaseClient(ctx, databaseContainer)
	node, releaseNode := command.NewConsensusNode(ctx, nil, consensusPort,
		command.WithQuery(db.SelectEvents))

	srv := fixture.NewTestServer(ctx,
		&Config{
			Port:      wsPort,
			TLSConfig: tlsConfig,
		},
		newHandler(ctx, fmt.Sprintf("wss://localhost:%v", wsPort)).Handle,
		nil,
		map[string]gin.HandlerFunc{},
	)
	srv.Consensus = node
	srv.DB = db

	return srv, func() error {
		return errors.Join(releaseNode(), releaseDB())
	}
}

func TestSimpleEchoDifferentTransports(t *testing.T) {
	const (
		connCountTCP = 100
		connCountUDP = 100
	)

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

	ctx, cancel := context.WithTimeout(t.Context(), testDeadline)
	defer cancel()

	tlsConfig := cfg.MustGet[globalCfg]()
	echoServer := fixture.NewTestServer(
		t.Context(),
		&Config{
			Port:      9999,
			TLSConfig: LoadTLSConfig(tlsConfig.TLSCert, tlsConfig.TLSKey),
		},
		func(ctx context.Context, w Writer, in []byte) {
			if wErr := w.WriteMessage(ctx, int(ws.OpText), []byte("server reply:"+string(in))); wErr != nil {
				log.Panic(wErr)
			}
		},
		nil,
		map[string]gin.HandlerFunc{},
	)

	var wg sync.WaitGroup
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
				err := clientConn.WriteMessage(ctx, int(ws.OpText), []byte(msg))
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
