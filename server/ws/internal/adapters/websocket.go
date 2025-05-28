// SPDX-License-Identifier: ice License 1.0

package adapters

import (
	"context"
	"log"
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"

	h2ec "github.com/ice-blockchain/go/src/net/http"
)

func NewWebSocketAdapter(ctx context.Context, conn net.Conn, readTimeout, writeTimeout time.Duration, shutdownChannel <-chan struct{}) (WSWithWriter, context.Context) {
	wt := &WebsocketAdapter{
		conn:         conn,
		closeChannel: make(chan struct{}, 1),
		out:          make(chan wsWrite),
		readTimeout:  readTimeout,
		writeTimeout: writeTimeout,
	}

	return wt, NewCustomCancelContext(ctx, wt.closeChannel, shutdownChannel)
}

func (w *WebsocketAdapter) writeMessageToWebsocket(messageType int, data []byte) (err error) {
	if w.Closed() {
		return nil
	}

	select {
	case <-w.closeChannel:
		return nil
	default:
		if w.writeTimeout > 0 {
			err = w.conn.SetWriteDeadline(time.Now().Add(w.writeTimeout))
		}
		wErr := wsutil.WriteServerMessage(w.conn, ws.OpCode(messageType), data)
		w.wrErrMx.Lock()
		w.wrErr = wErr
		w.wrErrMx.Unlock()
		if isConnClosedErr(wErr) {
			wErr = nil
		}

		if err = errors.Join(err, wErr); err != nil {
			return errors.Wrap(err, "failed to write data to websocket")
		}

		if flusher, ok := w.conn.(http.Flusher); ok {
			flusher.Flush()
		}

		return nil
	}
}

func (w *WebsocketAdapter) WriteMessage(ctx context.Context, messageType int, data []byte) error {
	select {
	case <-w.closeChannel:
		return nil

	case <-ctx.Done():
		return ctx.Err()

	default:
		w.wrErrMx.Lock()
		if isConnClosedErr(w.wrErr) {
			w.wrErrMx.Unlock()
			return w.Close()
		}
		w.wrErrMx.Unlock()
		select {
		case w.out <- wsWrite{
			opCode: messageType,
			data:   data,
		}:
		case <-ctx.Done():
			return errors.Wrapf(ctx.Err(), "cannot write message type %d with size %d to websocket",
				messageType, len(data))
		}
	}

	return nil
}

func (w *WebsocketAdapter) Write(ctx context.Context) {
	for ctx.Err() == nil {
		select {
		case <-w.closeChannel:
			return

		case <-ctx.Done():
			return

		case msg := <-w.out:
			if isConnClosedErr(w.wrErr) {
				return
			}

			if err := w.writeMessageToWebsocket(msg.opCode, msg.data); err != nil {
				log.Printf("ERROR:%v", errors.Wrap(err, "failed to send message to websocket"))
			}
		}
	}
}

func (w *WebsocketAdapter) ReadMessage() (messageType int, p []byte, err error) {
	if w.readTimeout > 0 {
		_ = w.conn.SetReadDeadline(time.Now().Add(w.readTimeout)) //nolint:errcheck // It is not crucial if we ignore it here.
	}
	msgBytes, typ, err := wsutil.ReadClientData(w.conn)
	if err != nil {
		return int(typ), msgBytes, err
	}
	if typ == ws.OpPing {
		err = wsutil.WriteServerMessage(w.conn, ws.OpPong, nil)
		if err == nil {
			return w.ReadMessage()
		}

		return int(typ), msgBytes, err
	}

	return int(typ), msgBytes, err
}

func (w *WebsocketAdapter) Closed() bool {
	return w.closed.Load()
}

func (w *WebsocketAdapter) Close() error {
	if w.closed.Load() {
		return nil
	}

	if !w.closed.CompareAndSwap(false, true) {
		return nil
	}

	close(w.closeChannel)

	var wErr error
	if w.wrErr == nil || !isConnClosedErr(w.wrErr) {
		wErr = wsutil.WriteServerMessage(w.conn, ws.OpClose, ws.NewCloseFrameBody(ws.StatusNormalClosure, ""))
		if wErr != nil && isConnClosedErr(wErr) {
			wErr = nil
		}
	}
	clErr := w.conn.Close()
	if clErr != nil && isConnClosedErr(clErr) {
		clErr = nil
	}

	return errors.Join(wErr, clErr)
}

func isConnClosedErr(err error) bool {
	return err != nil &&
		(errors.Is(err, syscall.EPIPE) ||
			errors.Is(err, syscall.ECONNRESET) ||
			errors.Is(err, h2ec.Http2errClientDisconnected) ||
			errors.Is(err, h2ec.Http2errStreamClosed) ||
			strings.Contains(err.Error(), "convert stream error 386759528") ||
			strings.Contains(err.Error(), "canceled by remote with error code 256") ||
			strings.Contains(err.Error(), "use of closed network connection"))
}
