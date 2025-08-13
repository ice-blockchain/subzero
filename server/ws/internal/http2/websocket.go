// SPDX-License-Identifier: ice License 1.0

package http2

import (
	"context"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gobwas/ws"

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
	wsocket, ctx := adapters.NewWebSocketAdapter(req.Context(), conn, &adapters.WebtransportAdapterConfig{
		Handshake:    hs,
		ReadTimeout:  s.cfg.ReadTimeout,
		WriteTimeout: s.cfg.WriteTimeout,
		CloseChannel: s.shutdownCh,
	})
	go s.ping(ctx, wsocket)

	return wsocket, ctx, nil
}

func (s *srv) ping(ctx context.Context, writer adapters.WSWithWriter) {
	ticker := time.NewTicker(time.Minute)
	defer func() {
		ticker.Stop()
		writer.Close()
	}()
	for ctx.Err() == nil {
		select {
		case <-ticker.C:
			var dErr error
			if err := errors.Join(
				dErr,
				writer.WriteMessage(ctx, int(ws.OpPing), nil),
			); err != nil {
				log.Printf("ERROR:%v", errors.Wrap(err, "failed to send ping message"))
			}
		case <-ctx.Done():
			return
		case <-s.shutdownCh:
			return
		}
	}
}
