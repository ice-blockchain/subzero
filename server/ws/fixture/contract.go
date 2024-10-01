// SPDX-License-Identifier: ice License 1.0

package fixture

import (
	"context"
	_ "embed"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	stdlibtime "time"

	h2ec "github.com/ice-blockchain/go/src/net/http"
	"github.com/ice-blockchain/subzero/server/ws/internal"
	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
	"github.com/ice-blockchain/subzero/server/ws/internal/config"
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
		processingFunc func(ctx context.Context, writer adapters.WSWriter, in []byte, cfg *config.Config)
		nip11Handler   http.Handler
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
	applicationYamlKeyEcho            = "echo"
	applicationYamlKeyPubSub          = "sub"
	wtCapsuleStream                   = 0x190B4D3B
	wtCapsuleStreamFin                = 0x190B4D3C
	wtCapsuleCloseWebtransportSession = 0x2843
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
