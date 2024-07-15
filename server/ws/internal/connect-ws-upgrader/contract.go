// SPDX-License-Identifier: ice License 1.0

package connectwsupgrader

import (
	"errors"
	"net"

	"github.com/gobwas/httphead"
	"github.com/quic-go/quic-go"
)

// Implements PFC 8441.
type (
	ConnectUpgrader struct {
		Protocol  func(string) bool
		Extension func(httphead.Option) bool
		Negotiate func(httphead.Option) (httphead.Option, error)
	}
)

//nolint:grouper // .
var ErrBadProtocol = errors.New(":protocol must be websocket")

const (
	headerSecVersionCanonical    = "Sec-Websocket-Version"
	headerSecProtocolCanonical   = "Sec-Websocket-Protocol"
	headerSecExtensionsCanonical = "Sec-Websocket-Extensions"
)

type (
	conn interface {
		LocalAddr() net.Addr
		RemoteAddr() net.Addr
	}
	http3StreamProxy struct {
		stream     quic.Stream
		connection conn
	}
)
