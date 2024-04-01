// SPDX-License-Identifier: ice License 1.0

package http3

import (
	"context"
	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
	"net/http"

	"github.com/pkg/errors"

	cws "github.com/ice-blockchain/subzero/server/ws/internal/connect-ws-upgrader"
	"github.com/ice-blockchain/wintr/log"
)

//nolint:gochecknoglobals // We need single instance.
var (
	//nolint:gochecknoglobals // We need single instance.
	websocketupgrader = cws.ConnectUpgrader{}
)

func (s *srv) handleWebsocket(writer http.ResponseWriter, req *http.Request) (h3ws adapters.WSWithWriter, ctx context.Context, err error) {
	conn, _, _, err := websocketupgrader.Upgrade(req, writer)
	if err != nil {
		err = errors.Wrapf(err, "upgrading http3/websocket failed")
		log.Error(err)
		writer.WriteHeader(http.StatusBadRequest)

		return
	}
	wsocket, ctx := adapters.NewWebSocketAdapter(req.Context(), conn, s.cfg.ReadTimeout, s.cfg.WriteTimeout)

	return wsocket, ctx, nil
}
