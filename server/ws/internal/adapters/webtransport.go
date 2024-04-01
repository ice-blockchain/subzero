// SPDX-License-Identifier: ice License 1.0

package adapters

import (
	"bufio"
	"context"
	"strings"

	"github.com/gookit/goutil/errorx"
	"github.com/quic-go/webtransport-go"

	"log"
	"time"
)

func NewWebTransportAdapter(ctx context.Context, session *webtransport.Session, stream webtransport.Stream, readTimeout, writeTimeout time.Duration) (WSWithWriter, context.Context) {
	wt := &WebtransportAdapter{
		stream:       stream,
		session:      session,
		reader:       bufio.NewReaderSize(stream, 1024),
		closeChannel: make(chan struct{}, 1),
		out:          make(chan []byte),
		readTimeout:  readTimeout,
		writeTimeout: writeTimeout,
	}

	return wt, NewCustomCancelContext(ctx, wt.closeChannel)
}

func (w *WebtransportAdapter) WriteMessage(_ int, data []byte) (err error) {
	w.closeMx.Lock()
	if w.closed {
		w.closeMx.Unlock()

		return nil
	}
	w.closeMx.Unlock()
	w.wrErrMx.Lock()
	if isConnClosedErr(w.wrErr) {
		w.wrErrMx.Unlock()

		return w.Close()
	}
	w.wrErrMx.Unlock()
	w.out <- data

	return nil
}

func (w *WebtransportAdapter) WriteMessageToStream(data []byte) error {
	if w.writeTimeout > 0 {
		_ = w.stream.SetWriteDeadline(time.Now().Add(w.writeTimeout)) //nolint:errcheck // .
	}
	data = append(data, 0x00)
	select {
	case <-w.closeChannel:
		return nil
	default:
		if _, err := w.stream.Write(data); err != nil {
			w.wrErrMx.Lock()
			w.wrErr = err
			w.wrErrMx.Unlock()
			if isConnClosedErr(err) {
				return nil
			}
			return errorx.Withf(err, "failed to write data to webtransport stream")
		}
		return nil
	}
}

func (w *WebtransportAdapter) Write(ctx context.Context) {
	for msg := range w.out {
		if ctx.Err() != nil || isConnClosedErr(w.wrErr) {
			break
		}
		if err := w.WriteMessageToStream(msg); err != nil {
			log.Printf("ERROR:%v", errorx.Withf(err, "failed to send message to webtransport"))
		}
	}
}

func (w *WebtransportAdapter) Closed() bool {
	w.closeMx.Lock()
	closed := w.closed
	w.closeMx.Unlock()

	return closed
}

func (w *WebtransportAdapter) Close() error {
	w.closeMx.Lock()
	if w.closed {
		w.closeMx.Unlock()

		return nil
	}
	w.closed = true
	close(w.closeChannel)
	close(w.out)
	w.closeMx.Unlock()
	var clErr error
	if w.session != nil {
		clErr = w.session.CloseWithError(0, "")
	}
	if clErr != nil {
		clErr = w.stream.Close()
		if clErr != nil && strings.Contains(clErr.Error(), "close called for canceled stream") {
			return nil
		}
		return errorx.With(clErr, "failed to close http3/webtransport stream")
	}
	return nil
}

func (w *WebtransportAdapter) ReadMessage() (messageType int, readValue []byte, err error) {
	if w.readTimeout > 0 {
		_ = w.stream.SetReadDeadline(time.Now().Add(w.readTimeout)) //nolint:errcheck // .
	}
	readValue, err = w.reader.ReadBytes(0x00)
	if err != nil {
		return 0, readValue, errorx.Withf(err, "failed to read data from webtransport stream")
	}
	if len(readValue) > 0 && readValue[len(readValue)-1] == 0x00 {
		readValue = readValue[0 : len(readValue)-1]
	}

	return 1, readValue, nil
}
