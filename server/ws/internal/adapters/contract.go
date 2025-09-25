// SPDX-License-Identifier: ice License 1.0

package adapters

import (
	"bufio"
	"context"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gobwas/ws"
	"github.com/quic-go/quic-go"
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
		WriteMessage(ctx context.Context, messageType int, data []byte) error
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
	WebtransportAdapterConfig struct {
		CloseChannel <-chan struct{}
		Handshake    ws.Handshake
		WriteTimeout time.Duration
		ReadTimeout  time.Duration
	}
	WebtransportAdapter struct {
		stream       Stream
		wrErr        error
		session      *webtransport.Session
		reader       *bufio.Reader
		closeChannel chan struct{}
		out          chan []byte
		writeTimeout time.Duration
		readTimeout  time.Duration
		wrErrMx      sync.Mutex
		closed       atomic.Bool
	}

	WebsocketAdapter struct {
		conn         net.Conn
		wrErr        error
		out          chan wsWrite
		closeChannel chan struct{}
		framer       func(int, []byte) (ws.Frame, error)
		writeTimeout time.Duration
		readTimeout  time.Duration
		wrErrMx      sync.Mutex
		closed       atomic.Bool
	}
)

type Stream interface {
	io.Writer
	io.Reader
	io.Closer

	CancelWrite(webtransport.StreamErrorCode)

	SetWriteDeadline(time.Time) error
	StreamID() quic.StreamID
	CancelRead(webtransport.StreamErrorCode)

	SetReadDeadline(time.Time) error
	SetDeadline(time.Time) error
}

const CtxKeyServer = "ws-server"

type (
	customCancelContext struct {
		context.Context //nolint:containedctx // Custom implementation.
		ch              <-chan struct{}
		shutdownCh      <-chan struct{}
	}
	wsWrite struct {
		data   []byte
		opCode int
	}
)
