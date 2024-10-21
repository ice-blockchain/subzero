// SPDX-License-Identifier: ice License 1.0

package fixture

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ice-blockchain/subzero/server/ws/internal"
	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
	"github.com/ice-blockchain/subzero/server/ws/internal/config"
)

func NewTestServer(ctx context.Context, cancel context.CancelFunc, cfg *config.Config, processingFunc func(ctx context.Context, w adapters.WSWriter, in []byte, cfg *config.Config), nip11 http.Handler, extraHttpHandlers map[string]gin.HandlerFunc) *MockService {
	service := newMockService(processingFunc, nip11, extraHttpHandlers)
	server := internal.NewWSServer(service, cfg)
	service.server = server
	go service.server.ListenAndServe(ctx, cancel)

	return service
}

func newMockService(processingFunc func(ctx context.Context, w adapters.WSWriter, in []byte, cfg *config.Config), nip11Handler http.Handler, extraHttpHandlers map[string]gin.HandlerFunc) *MockService {
	return &MockService{processingFunc: processingFunc, Handlers: make(map[adapters.WSWriter]struct{}), nip11Handler: nip11Handler, extraHttpHandlers: extraHttpHandlers}
}

func (m *MockService) Reset() {
	m.handlersMx.Lock()
	for k := range m.Handlers {
		delete(m.Handlers, k)
	}
	m.ReaderExited.Store(uint64(0))
	m.handlersMx.Unlock()
}

func (m *MockService) Read(ctx context.Context, w internal.WS, cfg *config.Config) {
	defer func() {
		m.ReaderExited.Add(1)
	}()
	for ctx.Err() == nil {
		_, msg, err := w.ReadMessage()
		if err != nil {
			break
		}
		if len(msg) > 0 {
			m.handlersMx.Lock()
			m.Handlers[w] = struct{}{}
			m.handlersMx.Unlock()
			m.processingFunc(ctx, w, msg, cfg)
		}
	}
}

func (m *MockService) RegisterRoutes(ctx context.Context, r internal.Router, cfg *config.Config) {
	for route, handler := range m.extraHttpHandlers {
		parts := strings.Split(route, " ")
		method, path := parts[0], parts[1]
		r = r.Handle(method, path, handler)
	}
	r.Any("/", internal.WithWS(m, m.nip11Handler))
}

func (m *MockService) Close(ctx context.Context) error {
	return nil
}
