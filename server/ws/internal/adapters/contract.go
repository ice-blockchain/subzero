// SPDX-License-Identifier: ice License 1.0

package adapters

import (
	"bufio"
	"context"
	"io"
	"net"
	"sync"
	stdlibtime "time"

	"github.com/quic-go/webtransport-go"
)

type (
	WSHandler interface {
		Read(ctx context.Context, reader WS)
	}
	WSReader interface {
		ReadMessage() (messageType int, p []byte, err error)
		io.Closer
	}
	WSWriter interface {
		WriteMessage(messageType int, data []byte) error
		io.Closer
	}
	WS interface {
		WSWriter
		WSReader
	}
	WSWithWriter interface {
		WS
		WSWriterRoutine
	}
	WSWriterRoutine interface {
		Write(ctx context.Context)
	}
	WebtransportAdapter struct {
		stream       webtransport.Stream
		session      *webtransport.Session
		reader       *bufio.Reader
		closeChannel chan struct{}
		closed       bool
		closeMx      sync.Mutex
		wrErr        error
		wrErrMx      sync.Mutex
		out          chan []byte
		writeTimeout stdlibtime.Duration
		readTimeout  stdlibtime.Duration
	}

	WebsocketAdapter struct {
		conn         net.Conn
		out          chan wsWrite
		closeChannel chan struct{}
		wrErr        error
		wrErrMx      sync.Mutex
		closed       bool
		closeMx      sync.Mutex
		writeTimeout stdlibtime.Duration
		readTimeout  stdlibtime.Duration
	}
)

const CtxKeyServer = "ws-server"

type (
	customCancelContext struct {
		context.Context //nolint:containedctx // Custom implementation.
		ch              <-chan struct{}
	}
	wsWrite struct {
		data   []byte
		opCode int
	}
)
