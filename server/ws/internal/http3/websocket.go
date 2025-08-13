// SPDX-License-Identifier: ice License 1.0

package http3

import (
	"context"
	"log"
	"net/http"

	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
	cws "github.com/ice-blockchain/subzero/server/ws/internal/connect-ws-upgrader"
)

func (s *srv) handleWebsocket(writer http.ResponseWriter, req *http.Request) (h3ws adapters.WSWithWriter, ctx context.Context, err error) {
	conn, _, hs, err := cws.New().Upgrade(req, writer)
	if err != nil {
		log.Printf("[http3] ERROR: upgrading http3/websocket failed: %v", err)
		writer.WriteHeader(http.StatusBadRequest)

		return
	}
	wsocket, ctx := adapters.NewWebSocketAdapter(req.Context(), conn, &adapters.WebtransportAdapterConfig{
		Handshake:    hs,
		ReadTimeout:  s.cfg.ReadTimeout,
		WriteTimeout: s.cfg.WriteTimeout,
		CloseChannel: s.shutdownCh,
	})

	return wsocket, ctx, nil
}
