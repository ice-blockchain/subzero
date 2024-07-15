// SPDX-License-Identifier: ice License 1.0

package http2

import (
	"context"
	"log"
	"net/http"

	"github.com/gookit/goutil/errorx"

	h2ec "github.com/ice-blockchain/go/src/net/http"
	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
)

func (s *srv) handleWebTransport(writer http.ResponseWriter, req *http.Request) (h2wt adapters.WSWithWriter, ctx context.Context, err error) {
	w := writer
	if unwrapper, unwrap := w.(Unwrapper); unwrap {
		w = unwrapper.Unwrap()
	}
	if upgrader, ok := w.(h2ec.WebTransportUpgrader); ok {
		var session h2ec.Session
		session, err = upgrader.UpgradeWebTransport()
		if err != nil {
			err = errorx.Withf(err, "upgrading http2/webtransport stream failed")
			log.Printf("ERROR:%v", err)
			writer.WriteHeader(http.StatusBadRequest)

			return nil, nil, err
		}
		writer.(http.Flusher).Flush()
		acceptCtx, acceptCancel := context.WithTimeout(req.Context(), acceptStreamTimeout)
		stream := session.AcceptStream(acceptCtx)
		acceptCancel()
		h2wt, ctx = adapters.NewWebTransportAdapter(req.Context(), nil, stream, s.cfg.ReadTimeout, s.cfg.WriteTimeout)

		return h2wt, ctx, nil
	}
	err = errorx.Withf(err, "upgrading webtransport is not implemented for http2")
	log.Printf("ERROR:%v", err)

	return nil, nil, err
}
