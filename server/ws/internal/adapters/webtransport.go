// SPDX-License-Identifier: ice License 1.0

package adapters

import (
	"bufio"
	"context"
	"net"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/quic-go/webtransport-go"
	"github.com/rs/zerolog/log"
)

func NewWebTransportAdapter(ctx context.Context, session *webtransport.Session, stream Stream, readTimeout, writeTimeout time.Duration, shutdownChannel <-chan struct{}) (WSWithWriter, context.Context) {
	wt := &WebtransportAdapter{
		stream:       stream,
		session:      session,
		reader:       bufio.NewReaderSize(stream, 1024),
		closeChannel: make(chan struct{}, 1),
		out:          make(chan []byte),
		readTimeout:  readTimeout,
		writeTimeout: writeTimeout,
	}

	return wt, NewCustomCancelContext(ctx, wt.closeChannel, shutdownChannel)
}

func (w *WebtransportAdapter) LocalAddr() net.Addr {
	return w.session.LocalAddr()
}

func (w *WebtransportAdapter) RemoteAddr() net.Addr {
	return w.session.RemoteAddr()
}

func (w *WebtransportAdapter) WriteMessage(ctx context.Context, _ int, data []byte) error {
	if w.Closed() {
		return nil
	}

	w.wrErrMx.Lock()
	if isConnClosedErr(w.wrErr) {
		w.wrErrMx.Unlock()

		return w.Close()
	}
	w.wrErrMx.Unlock()

	select {
	case w.out <- data:
		return nil
	case <-w.closeChannel:
	case <-ctx.Done():
		return errors.Wrap(ctx.Err(), "failed to write message to webtransport stream")
	}

	return nil
}

func (w *WebtransportAdapter) WriteMessageToStream(ctx context.Context, data []byte) error {
	if w.writeTimeout > 0 {
		_ = w.stream.SetWriteDeadline(time.Now().Add(w.writeTimeout)) //nolint:errcheck // .
	}
	data = append(data, 0x00)
	select {
	case <-w.closeChannel:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		if _, err := w.stream.Write(data); err != nil {
			w.wrErrMx.Lock()
			w.wrErr = err
			w.wrErrMx.Unlock()
			if isConnClosedErr(err) {
				return nil
			}
			return errors.Wrap(err, "failed to write data to webtransport stream")
		}
		return nil
	}
}

func (w *WebtransportAdapter) Write(ctx context.Context) {
	for msg := range w.out {
		if ctx.Err() != nil || isConnClosedErr(w.wrErr) {
			break
		}
		if err := w.WriteMessageToStream(ctx, msg); err != nil {
			log.Error().Err(err).Msg("failed to send message to webtransport")
		}
	}
}

func (w *WebtransportAdapter) Closed() bool {
	return w.closed.Load()
}

func (w *WebtransportAdapter) Close() error {
	if w.closed.Load() {
		return nil
	}

	if !w.closed.CompareAndSwap(false, true) {
		return nil
	}

	w.closed.Store(true)
	close(w.closeChannel)
	close(w.out)
	var clErr error
	if w.session != nil {
		clErr = w.session.CloseWithError(0, "")
	}
	if clErr != nil || w.session == nil {
		clErr = w.stream.Close()
		if clErr != nil {
			if strings.Contains(clErr.Error(), "close called for canceled stream") {
				return nil
			}
			return errors.Wrap(clErr, "failed to close http3/webtransport stream")
		}
	}
	return nil
}

func (w *WebtransportAdapter) ReadMessage() (messageType int, readValue []byte, err error) {
	if w.readTimeout > 0 {
		_ = w.stream.SetReadDeadline(time.Now().Add(w.readTimeout)) //nolint:errcheck // .
	}
	readValue, err = w.reader.ReadBytes(0x00)
	if err != nil {
		return 0, readValue, errors.Wrap(err, "failed to read data from webtransport stream")
	}
	if len(readValue) > 0 && readValue[len(readValue)-1] == 0x00 {
		readValue = readValue[0 : len(readValue)-1]
	}

	return 1, readValue, nil
}
