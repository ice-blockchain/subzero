// SPDX-License-Identifier: ice License 1.0

package fixture

import (
	"context"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ice-blockchain/subzero/server/ws/internal"
	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
	"github.com/ice-blockchain/subzero/server/ws/internal/config"
)

func NewTestServer(ctx context.Context, cfg *config.Config, cb MockCallback, nip11 http.Handler, extraHttpHandlers map[string]gin.HandlerFunc) *MockService {
	service := newMockService(cb, nip11, extraHttpHandlers)
	service.server = internal.NewWSServer(service, cfg)
	service.readerWg = new(sync.WaitGroup)
	service.port = int(cfg.Port)

	go service.server.MustListenAndServe(ctx)

	return service
}

func newMockService(cb MockCallback, nip11Handler http.Handler, extraHttpHandlers map[string]gin.HandlerFunc) *MockService {
	return &MockService{
		processingFunc:    cb,
		Handlers:          make(map[adapters.WSWriter]struct{}),
		nip11Handler:      nip11Handler,
		extraHttpHandlers: extraHttpHandlers,
	}
}

func (m *MockService) Reset() {
	m.handlersMx.Lock()
	clear(m.Handlers)
	m.readerWg = new(sync.WaitGroup)
	m.handlersMx.Unlock()
}

func (m *MockService) Read(ctx context.Context, w internal.WS, cfg *config.Config) {
	m.readerWg.Add(1)
	defer m.readerWg.Done()

	for ctx.Err() == nil {
		_, msg, err := w.ReadMessage()
		if err != nil {
			break
		}
		if len(msg) > 0 {
			m.handlersMx.Lock()
			m.Handlers[w] = struct{}{}
			m.handlersMx.Unlock()
			go m.processingFunc(ctx, w, msg, cfg)
		}
	}
}

func (m *MockService) WaitForReaders(timeout time.Duration) error {
	if timeout == 0 {
		m.readerWg.Wait()

		return nil
	}

	done := make(chan struct{}, 1)
	go func() {
		m.readerWg.Wait()
		done <- struct{}{}
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-done:
		return nil

	case <-timer.C:
		return os.ErrDeadlineExceeded
	}
}

func (m *MockService) Endpoint() string {
	return "wss://" + net.JoinHostPort("localhost", strconv.Itoa(m.port))
}

func (m *MockService) RegisterRoutes(ctx context.Context, r internal.Router) {
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
