// SPDX-License-Identifier: ice License 1.0

package http2

import (
	"bytes"
	"context"
	"net"
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsflate"
	"github.com/rs/zerolog/log"

	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
	cws "github.com/ice-blockchain/subzero/server/ws/internal/connect-ws-upgrader"
)

func (s *srv) handleWebsocket(writer http.ResponseWriter, req *http.Request) (h2ws adapters.WSWithWriter, ctx context.Context, err error) {
	var conn net.Conn
	var hs ws.Handshake

	h2Upgrader := cws.New()
	if req.Header.Get("Upgrade") == websocketProtocol {
		conn, _, hs, err = ws.HTTPUpgrader{
			Negotiate: h2Upgrader.Negotiate,
			Protocol:  h2Upgrader.Protocol,
			Extension: h2Upgrader.Extension,
		}.Upgrade(req, writer)
	} else if req.Method == http.MethodConnect && req.Proto == websocketProtocol {
		conn, _, hs, err = h2Upgrader.Upgrade(req, writer)
	}
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to upgrade to websocket over http1/2: %v, upgrade: %v", req.Proto, req.Header.Get("Upgrade"))
	}

	{
		hasCompression := false
		for _, ext := range hs.Extensions {
			hasCompression = hasCompression || bytes.EqualFold(ext.Name, wsflate.ExtensionNameBytes)
		}
		if !hasCompression {
			conn.Write(ws.CompiledCloseProtocolError)
			log.Error().Str("remote_addr", conn.RemoteAddr().String()).Msg("websocket connection does not support compression, closing")
			conn.Close()
			return nil, nil, errNoCompression
		}
	}

	wsocket, ctx := adapters.NewWebSocketAdapter(req.Context(), conn, &adapters.WebtransportAdapterConfig{
		Handshake:    hs,
		ReadTimeout:  s.cfg.ReadTimeout,
		WriteTimeout: s.cfg.WriteTimeout,
		CloseChannel: s.shutdownCh,
	})

	return wsocket, ctx, nil
}
