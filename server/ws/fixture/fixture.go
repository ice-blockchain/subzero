package fixture

import (
	"context"
	"github.com/ice-blockchain/subzero/server/ws/internal"
	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
	"github.com/ice-blockchain/subzero/server/ws/internal/config"
)

func NewTestServer(ctx context.Context, cancel context.CancelFunc, cfg *config.Config, processingFunc func(ctx context.Context, w adapters.WSWriter, in []byte)) *MockService {
	service := newMockService(processingFunc)
	server := internal.NewWSServer(service, cfg)
	service.server = server
	go service.server.ListenAndServe(ctx, cancel)

	return service
}
func newMockService(processingFunc func(ctx context.Context, w adapters.WSWriter, in []byte)) *MockService {
	return &MockService{processingFunc: processingFunc, Handlers: make(map[adapters.WSWriter]struct{})}
}

func (m *MockService) Reset() {
	m.handlersMx.Lock()
	for k, _ := range m.Handlers {
		delete(m.Handlers, k)
	}
	m.ReaderExited.Store(uint64(0))
	m.handlersMx.Unlock()

}
func (m *MockService) Read(ctx context.Context, w internal.WS) {
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
			m.processingFunc(ctx, w, msg)
		}
	}
}
func (m *MockService) Init(ctx context.Context, cancel context.CancelFunc) {
}

func (m *MockService) Close(ctx context.Context) error {
	return nil
}
