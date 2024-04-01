// SPDX-License-Identifier: ice License 1.0

package http2

import (
	"context"
	"github.com/hashicorp/go-multierror"
	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
	"net"
	"net/http"
	stdlibtime "time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/pkg/errors"

	cws "github.com/ice-blockchain/subzero/server/ws/internal/connect-ws-upgrader"
	"github.com/ice-blockchain/wintr/log"
	"github.com/ice-blockchain/wintr/time"
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
	wsocket, ctx := adapters.NewWebSocketAdapter(req.Context(), conn, s.cfg.WSServer.ReadTimeout, s.cfg.WSServer.WriteTimeout)
	go s.ping(ctx, conn)

	return wsocket, ctx, nil
}

func (s *srv) ping(ctx context.Context, conn net.Conn) {
	ticker := stdlibtime.NewTicker(stdlibtime.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			var dErr error
			if (s.cfg.WSServer.WriteTimeout) > 0 {
				dErr = conn.SetWriteDeadline(time.Now().Add(s.cfg.WSServer.WriteTimeout))
			}
			if err := multierror.Append(
				dErr,
				wsutil.WriteServerMessage(conn, ws.OpPing, nil),
			).ErrorOrNil(); err != nil {
				log.Error(errors.Wrapf(err, "failed to send ping message"))
			}
		case <-ctx.Done():
			return
		}
	}
}
