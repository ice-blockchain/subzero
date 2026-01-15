// SPDX-License-Identifier: ice License 1.0

package adapters

import (
	"bytes"
	"compress/flate"
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsflate"
	"github.com/gobwas/ws/wsutil"
	"github.com/rs/zerolog/log"

	h2ec "github.com/ice-blockchain/go/src/net/http"
	"github.com/ice-blockchain/subzero/appcontext"
)

const (
	// Buffer up to 50 messages before WriteMessage calls start to block.
	WebSocketWriteBufferSize = 50
	// Compress messages larger than this threshold.
	WebSocketCompressThresholdBytes = 256
	// Interval between pings to the client to keep the connection alive.
	WebSocketPingInterval = time.Minute
)

func NewWebSocketAdapter(ctx context.Context, conn net.Conn, conf *WebtransportAdapterConfig) (WSWithWriter, context.Context) {
	wt := &WebsocketAdapter{
		conn:         conn,
		closeChannel: make(chan struct{}, 1),
		out:          make(chan wsWrite, WebSocketWriteBufferSize),
		readTimeout:  conf.ReadTimeout,
		writeTimeout: conf.WriteTimeout,
		framer: func(opCode int, data []byte) (ws.Frame, error) {
			return ws.NewFrame(ws.OpCode(opCode), true, data), nil
		},
	}

	wt.initExtensions(conf.Handshake)

	return wt, NewCustomCancelContext(ctx, wt.closeChannel, conf.CloseChannel)
}

func (w *WebsocketAdapter) initCompression() {
	w.framer = func(opCode int, data []byte) (ws.Frame, error) {
		frame := ws.NewFrame(ws.OpCode(opCode), true, data)
		if opCode == int(ws.OpText) || opCode == int(ws.OpBinary) && len(data) > WebSocketCompressThresholdBytes {
			return compressFrame(frame)
		}
		return frame, nil
	}
}

func (w *WebsocketAdapter) initExtensions(handshake ws.Handshake) {
	var hasCompression bool

	for _, ext := range handshake.Extensions {
		if bytes.Equal(ext.Name, wsflate.ExtensionNameBytes) {
			hasCompression = true
		}
	}

	if hasCompression {
		w.initCompression()
	}
}

func (w *WebsocketAdapter) RemoteAddr() net.Addr {
	return w.conn.RemoteAddr()
}

func (w *WebsocketAdapter) writeMessageToWebsocket(messageType int, data []byte) (err error) {
	if w.Closed() {
		return nil
	}

	select {
	case <-w.closeChannel:
		return nil
	default:
		frame, err := w.framer(messageType, data)
		if err != nil {
			return errors.Wrap(err, "failed to create websocket frame")
		}

		if w.writeTimeout > 0 {
			err = w.conn.SetWriteDeadline(time.Now().Add(w.writeTimeout))
		}
		wErr := ws.WriteFrame(w.conn, frame)
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

// Write listens on the out channel and writes messages to the websocket connection.
// It's lanched as a separate goroutine from the server's HandleWS method.
func (w *WebsocketAdapter) Write(ctx context.Context) {
	defer appcontext.GetAppContext(ctx).Recover()
	pingTicker := time.NewTicker(WebSocketPingInterval)
	defer pingTicker.Stop()

	for ctx.Err() == nil {
		select {
		case <-w.closeChannel:
			return

		case <-ctx.Done():
			return

		case <-pingTicker.C:
			select {
			case w.out <- wsWrite{
				opCode: int(ws.OpPing),
				data:   nil,
			}:
			default:
				// If the out channel is full, we skip sending the ping to avoid blocking.
			}

		case msg := <-w.out:
			if isConnClosedErr(w.wrErr) {
				return
			}

			if err := w.writeMessageToWebsocket(msg.opCode, msg.data); err != nil {
				log.Error().Err(err).Msg("failed to send message to websocket")
			}
		}
	}
}

func (w *WebsocketAdapter) readFrame() ([]byte, ws.OpCode, error) {
	const want = ws.OpText | ws.OpBinary
	var msg wsflate.MessageState
	controlHandler := wsutil.ControlFrameHandler(w.conn, ws.StateServerSide)
	rd := wsutil.Reader{
		Source:         w.conn,
		State:          ws.StateServerSide | ws.StateExtended,
		OnIntermediate: controlHandler,
		Extensions: []wsutil.RecvExtension{
			&msg,
		},
	}
	for !w.closed.Load() {
		hdr, err := rd.NextFrame()
		if err != nil {
			return nil, 0, err
		}
		if hdr.OpCode.IsControl() {
			if err := controlHandler(hdr, &rd); err != nil {
				return nil, 0, err
			}
			continue // Continue to the next frame if control frame is received.
		}
		if hdr.OpCode&want == 0 {
			if err := rd.Discard(); err != nil {
				return nil, 0, err
			}
			continue // Continue to the next frame if the received frame is not of the expected type.
		}

		var payloadReader io.Reader = &rd
		if msg.IsCompressed() {
			payloadReader = wsflate.NewReader(&rd, func(r io.Reader) wsflate.Decompressor {
				return flate.NewReader(r)
			})
		}

		bts, err := io.ReadAll(payloadReader)

		return bts, hdr.OpCode, err
	}

	return nil, 0, errors.New("websocket connection closed")
}

func (w *WebsocketAdapter) ReadMessage() (messageType int, p []byte, err error) {
	if w.readTimeout > 0 {
		_ = w.conn.SetReadDeadline(time.Now().Add(w.readTimeout)) //nolint:errcheck // It is not crucial if we ignore it here.
	}
	msgBytes, typ, err := w.readFrame()
	if err != nil {
		return int(typ), msgBytes, err
	}
	if typ == ws.OpPing {
		_, err = w.conn.Write(ws.CompiledPong)
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
		_, wErr = w.conn.Write(ws.CompiledCloseNormalClosure)
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
