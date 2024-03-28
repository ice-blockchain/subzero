// SPDX-License-Identifier: ice License 1.0

package connectwsupgrader

import (
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"net"
	"strings"
	stdlibtime "time"
)

func NewHttp3Proxy(stream http3.Stream, sc http3.StreamCreator) net.Conn {
	return &http3StreamProxy{stream: stream, streamCreator: sc}
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
	return h.streamCreator.LocalAddr()
}

func (h *http3StreamProxy) RemoteAddr() net.Addr {
	return h.streamCreator.RemoteAddr()
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
