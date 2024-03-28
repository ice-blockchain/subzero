package fixture

import (
	"context"
	"github.com/ice-blockchain/subzero/server/ws/internal"
	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
)

func NewTestServer(ctx context.Context, cancel context.CancelFunc, processingFunc func(ctx context.Context, w adapters.WSWriter, in []byte)) *MockService {
	service := newMockService(processingFunc)
	server := internal.NewWSServer(service, applicationYamlKey)
	service.server = server
	go service.server.ListenAndServe(ctx, cancel)

	return service
}
func newMockService(processingFunc func(ctx context.Context, w adapters.WSWriter, in []byte)) *MockService {
	return &MockService{processingFunc: processingFunc}
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
			m.processingFunc(ctx, w, msg)
		}
	}
}
func (m *MockService) Init(ctx context.Context, cancel context.CancelFunc) {
}

func (m *MockService) Close(ctx context.Context) error {
	return nil
}

func (m *MockService) RegisterRoutes(r *internal.Router) {

}
