// SPDX-License-Identifier: ice License 1.0

package adapters

import (
	"bufio"
	"context"
	"io"
	"iter"
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
		Metadata() WSMetaData
		ReadMessage() (messageType int, p []byte, err error)
		RemoteAddr() net.Addr
		LocalAddr() net.Addr
		io.Closer
	}
	WSWriter interface {
		Metadata() WSMetaData
		WriteMessage(ctx context.Context, messageType int, data []byte) error
		RemoteAddr() net.Addr
		LocalAddr() net.Addr
		io.Closer
	}
	WSMetaData interface {
		Set(key string, value any)
		Get(key string) (value any, exists bool)
		GetOrSet(key string, value any) (actual any, loaded bool)
		Range() iter.Seq2[string, any]
		Delete(key string) (oldValue any, loaded bool)
		Clear()
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
		MetadataHander
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
		MetadataHander
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
	MetadataHander struct {
		m sync.Map
	}
)
