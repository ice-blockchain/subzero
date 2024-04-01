package ws

import (
	"context"
	"github.com/gobwas/ws"
	"github.com/google/uuid"
	"github.com/ice-blockchain/subzero/server/ws/fixture"
	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
	"github.com/ice-blockchain/wintr/log"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
	stdlibtime "time"
)

const (
	connCountTCP = 100
	connCountUDP = 100
	testDeadline = 30 * stdlibtime.Second
)

var echoServer *fixture.MockService
var pubsubServer *fixture.MockService

func TestMain(m *testing.M) {
	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()
	echoFunc := func(_ context.Context, w Writer, in []byte) {
		log.Panic(w.WriteMessage(int(ws.OpText), []byte("server reply:"+string(in))))
	}
	echoServer = fixture.NewTestServer(serverCtx, serverCancel, "echo", echoFunc)
	hdl = new(handler)
	pubsubServer = fixture.NewTestServer(serverCtx, serverCancel, "pubsub", hdl.Handle)
	m.Run()
	serverCancel()
}
func TestSimpleEchoDifferentTransports(t *testing.T) {
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
			}
			assert.GreaterOrEqual(t, len(receivedBackOnClient), 0)
		}(i)
	}
	wg.Wait()
	shutdownCtx, _ := context.WithTimeout(context.Background(), testDeadline)
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
