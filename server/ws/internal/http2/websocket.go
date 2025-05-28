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

//nolint:gochecknoglobals,grouper // We need single instance to avoid spending extra mem
var h2Upgrader = &cws.ConnectUpgrader{}

func (s *srv) handleWebsocket(writer http.ResponseWriter, req *http.Request) (h2ws adapters.WSWithWriter, ctx context.Context, err error) {
	var conn net.Conn
	if req.Header.Get("Upgrade") == websocketProtocol {
		conn, _, _, err = ws.DefaultHTTPUpgrader.Upgrade(req, writer)
	} else if req.Method == http.MethodConnect && req.Proto == websocketProtocol {
		conn, _, _, err = h2Upgrader.Upgrade(req, writer)
	}
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to upgrade to websocket over http1/2: %v, upgrade: %v", req.Proto, req.Header.Get("Upgrade"))
	}
	wsocket, ctx := adapters.NewWebSocketAdapter(req.Context(), conn, s.cfg.ReadTimeout, s.cfg.WriteTimeout, s.shutdownCh)
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
