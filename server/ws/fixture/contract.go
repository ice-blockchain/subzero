package fixture

import (
	"context"
	_ "embed"
	h2ec "github.com/ice-blockchain/go/src/net/http"
	"github.com/ice-blockchain/subzero/server/ws/internal"
	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
	"io"
	"net"
	"sync"
	"sync/atomic"
	stdlibtime "time"
)

var (
	//go:embed .testdata/localhost.crt
	localhostCrt string
	//go:embed .testdata/localhost.key
	localhostKey string
)

type (
	MockService struct {
		server         internal.Server
		closed         bool
		closedMx       sync.Mutex
		handlersMx     sync.Mutex
		Handlers       map[adapters.WSWriter]struct{}
		processingFunc func(ctx context.Context, writer adapters.WSWriter, in []byte)
		ReaderExited   atomic.Uint64
	}
	Client interface {
		Received
		adapters.WSWriter
	}
	Received interface {
		Received() <-chan []byte
	}
)

const (
	applicationYamlKeyEcho   = "echo"
	applicationYamlKeyPubSub = "sub"
	wtCapsuleStream          = 0x190B4D3B
	wtCapsuleStreamFin       = 0x190B4D3C
)

type (
	wsocketClient struct {
		conn          net.Conn
		closeChannel  chan struct{}
		closed        bool
		closeMx       sync.Mutex
		writeTimeout  stdlibtime.Duration
		readTimeout   stdlibtime.Duration
		inputMessages chan []byte
	}
	wtransportClient struct {
		wt            *adapters.WebtransportAdapter
		inputMessages chan []byte
		closed        bool
		closedMx      sync.Mutex
	}
	http2ClientStream struct {
		w    *io.PipeWriter
		resp *h2ec.Response
	}
	http2WebtransportWrapper struct {
		conn     *http2ClientStream
		streamID uint32
	}
)
