// SPDX-License-Identifier: ice License 1.0

package broadcaster

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/database/query/fixture"
	"github.com/ice-blockchain/subzero/model"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func helperNewServer(t *testing.T, cb func(p []byte, err error)) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		for {
			messageType, data, err := conn.ReadMessage()
			if err != nil {
				break
			}
			t.Logf("Received message: type %d: data: %s", messageType, string(data))
			if messageType == websocket.TextMessage {
				cb(data, err)
			} else {
				t.Logf("unhandled message type: %d", messageType)
			}
		}
	}))
	t.Cleanup(server.Close)

	return server
}

func TestBroadcaster_Broadcast(t *testing.T) {
	t.Parallel()

	testPrivateKey, testPublicKey := model.GenerateKeyPair()

	t.Run("successful broadcast", func(t *testing.T) {
		var memdb fixture.MemDB
		var envelope model.BroadcastEnvelope

		received := make(chan struct{})
		server := helperNewServer(t, func(p []byte, err error) {
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(p, &envelope))
			close(received)
		})

		relayURL := "ws" + strings.TrimPrefix(server.URL, "http")
		config := Config{
			RelayURL:   "ws://bob.localhost",
			PrivateKey: testPrivateKey,
			QueryFunc:  memdb.SelectEvents,
		}
		broadcaster := New(config)
		defer broadcaster.Close()

		var relayEvent model.Event
		relayEvent.PubKey = "alice"
		relayEvent.Kind = nostr.KindRelayListMetadata
		relayEvent.Tags = model.Tags{
			{"r", relayURL, "read"},
			{"r", config.RelayURL, "read"},
			{"r", "wss://example.com", "write"},
		}
		require.NoError(t, memdb.AcceptEvents(t.Context(), &relayEvent))

		var newEvent model.Event
		newEvent.PubKey = "alice"
		newEvent.Kind = nostr.KindTextNote
		newEvent.Content = "Hello, world!"

		err := broadcaster.Broadcast(t.Context(), &newEvent)
		require.NoError(t, err)
		<-received

		require.Equal(t, config.RelayURL, envelope.Relay)
		require.Equal(t, model.CustomIONKindEphemeralBatch, envelope.Event.Kind)
		require.Equal(t, testPublicKey, envelope.Event.PubKey)
	})

	t.Run("filtered events not broadcasted", func(t *testing.T) {
		var memdb fixture.MemDB

		config := Config{
			RelayURL:   "ws://bob.localhost",
			PrivateKey: testPrivateKey,
			QueryFunc:  memdb.SelectEvents,
		}
		broadcaster := New(config)
		defer broadcaster.Close()

		filteredEvent := &model.Event{
			Event: nostr.Event{
				Kind: model.CustomIONKindEphemeralEmbedding,
			},
		}

		err := broadcaster.Broadcast(t.Context(), filteredEvent)
		require.NoError(t, err)
	})

	t.Run("query function error", func(t *testing.T) {
		testErr := errors.New("something went wrong")
		config := Config{
			RelayURL:   "ws://bob.localhost",
			PrivateKey: testPrivateKey,
			QueryFunc: func(ctx context.Context, filters ...model.Filter) query.EventIterator {
				return func(yield func(*model.Event, error) bool) {
					yield(nil, testErr)
				}
			},
		}
		broadcaster := New(config)
		defer broadcaster.Close()

		testEvent := &model.Event{
			Event: nostr.Event{
				PubKey: "test-pubkey",
				Kind:   nostr.KindTextNote,
			},
		}

		err := broadcaster.Broadcast(t.Context(), testEvent)
		t.Logf("broadcast error: %v", err)
		require.ErrorIs(t, err, testErr)
	})

	t.Run("relay connection failure", func(t *testing.T) {
		var memdb fixture.MemDB
		config := Config{
			RelayURL:   "wss://test-relay.com",
			PrivateKey: testPrivateKey,
			QueryFunc:  memdb.SelectEvents,
		}
		broadcaster := New(config)
		defer broadcaster.Close()

		var relayEvent model.Event
		relayEvent.PubKey = "alice"
		relayEvent.Kind = nostr.KindRelayListMetadata
		relayEvent.Tags = model.Tags{
			{"r", "ws://127.0.0.1:1", "read"},
			{"r", config.RelayURL, "read"},
		}
		require.NoError(t, memdb.AcceptEvents(t.Context(), &relayEvent))

		testEvent := &model.Event{
			Event: nostr.Event{
				PubKey: "alice",
				Kind:   nostr.KindTextNote,
			},
		}

		err := broadcaster.Broadcast(t.Context(), testEvent)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to broadcast")
	})
}
