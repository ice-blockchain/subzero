// SPDX-License-Identifier: ice License 1.0

package http3

import (
	"context"
	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
	"net/http"

	"github.com/pkg/errors"

	"github.com/ice-blockchain/wintr/log"
)

func (s *srv) handleWebTransport(writer http.ResponseWriter, req *http.Request) (ws adapters.WSWithWriter, ctx context.Context, err error) {
	conn, err := s.server.Upgrade(writer, req)
	if err != nil {
		err = errors.Wrapf(err, "upgrading http3/webtransport failed")
		log.Error(err)
		writer.WriteHeader(http.StatusBadRequest)

		return nil, nil, err
	}
	acceptCtx, acceptCancel := context.WithTimeout(req.Context(), acceptStreamTimeout)
	stream, err := conn.AcceptStream(acceptCtx)
	if err != nil {
		acceptCancel()
		err = errors.Wrapf(err, "getting http3/webtransport stream failed")
		log.Error(err)
		writer.WriteHeader(http.StatusBadRequest)

		return nil, nil, err
	}
	acceptCancel()
	wt, ctx := adapters.NewWebTransportAdapter(conn.Context(), conn, stream, s.cfg.WSServer.ReadTimeout, s.cfg.WSServer.WriteTimeout)

	return wt, ctx, nil
}
