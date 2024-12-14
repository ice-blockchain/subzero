// SPDX-License-Identifier: ice License 1.0

package broadcast

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/websocket"

	"github.com/ice-blockchain/subzero/cfg"
	"github.com/ice-blockchain/subzero/model"
)

type mockDatabaseAdapter struct {
	Data map[string][]string
}

func (m *mockDatabaseAdapter) ReadRelays(ctx context.Context, userPubkey string) ([]string, error) {
	return m.Data[userPubkey], nil
}

func helperNewWebsocketServer(handler func(*websocket.Conn)) *httptest.Server {
	return httptest.NewServer(&websocket.Server{
		Handshake: func(conf *websocket.Config, r *http.Request) error {
			return nil
		},
		Handler: handler,
	})
}

func TestBroadcastOnTwoRelays(t *testing.T) {
	t.Parallel()

	// Add one kind to broadcast.
	kindsToBroadcast[nostr.KindTextNote] = struct{}{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	priv, public := model.GenerateKeyPair()
	_, public2 := model.GenerateKeyPair()

	var originalEvent model.Event
	originalEvent.Kind = nostr.KindTextNote
	originalEvent.Content = "hello"
	originalEvent.CreatedAt = 1

	var wg sync.WaitGroup
	wg.Add(2)
	relay1 := helperNewWebsocketServer(func(conn *websocket.Conn) {
		defer wg.Done()

		var e nostr.EventEnvelope

		err := websocket.JSON.Receive(conn, &e)
		require.NoError(t, err)
		t.Logf("relay1: received event: %v", e)
		require.Len(t, e.Events, 1)
		require.Equal(t, originalEvent.Event, *e.Events[0])
	})
	defer relay1.Close()

	relay2 := helperNewWebsocketServer(func(conn *websocket.Conn) {
		defer wg.Done()

		var e nostr.EventEnvelope

		err := websocket.JSON.Receive(conn, &e)
		require.NoError(t, err)
		t.Logf("relay2: creceived event: %v", e)
		require.Len(t, e.Events, 1)
		require.Equal(t, originalEvent.Event, *e.Events[0])
	})
	defer relay2.Close()

	broadcaster, err := newBroadcaster(ctx,
		cfg.MustGet[config](),
		WithCustomDatabaseAdapter(&mockDatabaseAdapter{
			Data: map[string][]string{
				public:  {relay2.URL}, // Source relay.
				public2: {relay1.URL}, // Destination relay.
			},
		}),
	)
	require.NoError(t, err)
	require.NotNil(t, broadcaster)

	originalEvent.Tags = append(originalEvent.Tags, model.Tag{"p", public2})
	require.NoError(t, originalEvent.SignWithAlg(priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	err = broadcaster.Broadcast(ctx, public, &originalEvent)
	require.NoError(t, err)

	ch := make(chan struct{})
	go func() {
		wg.Wait()
		close(ch)
	}()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for relays to receive the event")
	}

	broadcaster.Close()
}
