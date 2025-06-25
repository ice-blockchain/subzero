// SPDX-License-Identifier: ice License 1.0

package connectwsupgrader

import (
	"net"
	"strings"
	stdlibtime "time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

func NewHttp3Proxy(stream quic.Stream, conn quic.Connection) net.Conn {
	return &http3StreamProxy{stream: stream, connection: conn}
}
func (h *http3StreamProxy) Read(b []byte) (n int, err error) {
	return h.stream.Read(b) //nolint:wrapcheck // Proxy.
}

func (h *http3StreamProxy) Write(b []byte) (n int, err error) {
	return h.stream.Write(b) //nolint:wrapcheck // Proxy.
}

func (h *http3StreamProxy) Close() error {
	h.stream.CancelRead(quic.StreamErrorCode(http3.ErrCodeNoError))
	err := h.stream.Close()
	if err != nil && strings.Contains(err.Error(), "close called for canceled stream") {
		err = nil
	}
	return err
}

func (h *http3StreamProxy) LocalAddr() net.Addr {
	return h.connection.LocalAddr()
}

func (h *http3StreamProxy) RemoteAddr() net.Addr {
	return h.connection.RemoteAddr()
}

func (h *http3StreamProxy) SetDeadline(t stdlibtime.Time) error {
	return h.stream.SetDeadline(t) //nolint:wrapcheck // Proxy.
}

func (h *http3StreamProxy) SetReadDeadline(t stdlibtime.Time) error {
	return h.stream.SetReadDeadline(t) //nolint:wrapcheck // Proxy.
}

func (h *http3StreamProxy) SetWriteDeadline(t stdlibtime.Time) error {
	return h.stream.SetWriteDeadline(t) //nolint:wrapcheck // Proxy.
}
