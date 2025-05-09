// SPDX-License-Identifier: ice License 1.0

//go:build test

package fixture

import (
	"context"
	_ "embed"
	"io"
	"net"
	"net/http"
	"sync"
	stdlibtime "time"

	"github.com/gin-gonic/gin"

	h2ec "github.com/ice-blockchain/go/src/net/http"
	"github.com/ice-blockchain/subzero/database/command"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/server/ws/internal"
	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
)

type (
	MockCallback func(ctx context.Context, w adapters.WSWriter, in []byte)
	TestDB       interface {
		AcceptEvents(ctx context.Context, events ...*model.Event) error
		RollbackEvents(ctx context.Context, events ...*model.Event) error
		SelectEvents(ctx context.Context, filters ...model.Filter) query.EventIterator
	}
	MockService struct {
		server            internal.Server
		handlersMx        sync.Mutex
		Handlers          map[adapters.WSWriter]struct{}
		processingFunc    MockCallback
		nip11Handler      http.Handler
		extraHttpHandlers map[string]gin.HandlerFunc
		readerWg          *sync.WaitGroup
		port              int
		DB                TestDB
		Consensus         command.TestConsensus
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
