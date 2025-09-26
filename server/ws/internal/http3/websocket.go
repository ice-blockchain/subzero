// SPDX-License-Identifier: ice License 1.0

package http3

import (
	"context"
	"net/http"

	"github.com/rs/zerolog/log"

	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
	cws "github.com/ice-blockchain/subzero/server/ws/internal/connect-ws-upgrader"
)

func (s *srv) handleWebsocket(writer http.ResponseWriter, req *http.Request) (h3ws adapters.WSWithWriter, ctx context.Context, err error) {
	conn, _, hs, err := cws.New().Upgrade(req, writer)
	if err != nil {
		log.Error().Str("context", "http3").Err(err).Msg("upgrading http3/websocket failed")
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
