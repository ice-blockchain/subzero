// SPDX-License-Identifier: ice License 1.0

package broadcaster

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip42"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/ice-blockchain/subzero/appcontext"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/database/query/fixture"
	"github.com/ice-blockchain/subzero/model"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func helperNewServer(t *testing.T, cb func(conn *websocket.Conn, p []byte, err error)) *httptest.Server {
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
				cb(conn, data, err)
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

	testPrivateKey := model.GeneratePrivateKey()

	t.Run("successful broadcast", func(t *testing.T) {
		var memdb fixture.MemDB

		received := make(chan model.BroadcastEnvelope, 2)
		handler := func(conn *websocket.Conn, p []byte, err error) {
			const authChallenge = "test-challenge"

			msg, err := nostr.ParseMessage(p, new(model.BroadcastEnvelope))
			require.NoError(t, err)

			switch msg := msg.(type) {
			case *model.BroadcastEnvelope:
				received <- *msg

			case *nostr.EventEnvelope:
				require.Len(t, msg.Events, 1)
				require.Equal(t, nostr.KindClientAuthentication, msg.Events[0].Kind)
				require.NoError(t, conn.WriteJSON(&nostr.AuthEnvelope{
					Challenge: model.PointerOf(authChallenge),
				}))
				require.NoError(t, conn.WriteJSON(&nostr.OKEnvelope{
					EventID: msg.Events[0].ID,
					OK:      false,
					Reason:  "auth-required: please authenticate first by sending AUTH message",
				}))

			case *nostr.AuthEnvelope:
				_, err := nip42.ValidateAuthEvent(
					&msg.Event,
					authChallenge,
					msg.Event.Tags.GetFirst([]string{"relay"}).Value(),
					nip42.WithCustomVerificator(func(nostrEvent *nostr.Event) (bool, error) {
						return (&model.Event{Event: *nostrEvent}).CheckSignature()
					}))
				require.NoError(t, err, "failed to validate auth event")
				require.NoError(t, conn.WriteJSON(&nostr.OKEnvelope{
					EventID: msg.Event.ID,
					OK:      true,
				}))

			default:
				t.Fatalf("unexpected message type: %T", msg)
			}
		}
		server := helperNewServer(t, handler)
		server2 := helperNewServer(t, handler)

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
			{"r", "ws" + strings.TrimPrefix(server2.URL, "http"), "write"},
		}
		require.NoError(t, memdb.AcceptEvents(t.Context(), &relayEvent))

		var newEvent model.Event
		newEvent.ID = "alice_id1"
		newEvent.PubKey = "alice"
		newEvent.Kind = nostr.KindTextNote
		newEvent.Content = "Hello, world!"
		newEvent.Tags = model.Tags{}

		var newEvent2 model.Event
		newEvent2.ID = "alice_id2"
		newEvent2.PubKey = "alice"
		newEvent2.Kind = nostr.KindArticle
		newEvent2.Content = "Hello"
		newEvent2.Tags = model.Tags{}

		err := broadcaster.Broadcast(appcontext.TestContext(t), &newEvent, &newEvent2)
		require.NoError(t, err)

		for range 2 {
			e := <-received
			require.Equal(t, config.RelayURL, e.Relay)
			require.Len(t, e.Events, 2)
			require.Equal(t, &newEvent, e.Events[0])
			require.Equal(t, &newEvent2, e.Events[1])
		}
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

		err := broadcaster.Broadcast(appcontext.TestContext(t), testEvent)
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

		err := broadcaster.Broadcast(appcontext.TestContext(t), testEvent)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to broadcast")
	})
}
func TestBroadcaster_collectTargets(t *testing.T) {
	t.Parallel()

	testPrivateKey := model.GeneratePrivateKey()

	t.Run("authoritative mode with regular events", func(t *testing.T) {
		var memdb fixture.MemDB
		config := Config{
			RelayURL:   "ws://test.localhost",
			PrivateKey: testPrivateKey,
			QueryFunc:  memdb.SelectEvents,
		}
		broadcaster := New(config)
		defer broadcaster.Close()

		// Setup relay metadata for author
		var relayEvent model.Event
		relayEvent.PubKey = "author1"
		relayEvent.Kind = nostr.KindRelayListMetadata
		relayEvent.Tags = model.Tags{
			{"r", "wss://relay1.com", "read"},
			{"r", "wss://relay2.com", "write"},
		}
		require.NoError(t, memdb.AcceptEvents(t.Context(), &relayEvent))

		// Create test events
		var event1 model.Event
		event1.PubKey = "author1"
		event1.Kind = nostr.KindTextNote
		event1.Content = "test content"

		var event2 model.Event
		event2.PubKey = "author2"
		event2.Kind = nostr.KindTextNote
		event2.Content = "test content 2"

		targets, err := broadcaster.collectTargets(t.Context(), []*model.Event{&event1, &event2})
		require.NoError(t, err)

		// Should find relays for author1 but not author2
		require.Contains(t, targets, "author1")
		require.Equal(t, []string{"wss://relay1.com", "wss://relay2.com"}, targets["author1"])
		require.NotContains(t, targets, "author2")
	})

	t.Run("non-authoritative mode with embedding events", func(t *testing.T) {
		var memdb fixture.MemDB
		config := Config{
			RelayURL:   "ws://test.localhost",
			PrivateKey: testPrivateKey,
			QueryFunc:  memdb.SelectEvents,
		}
		broadcaster := New(config)
		defer broadcaster.Close()

		// Create embedding event with relay list metadata
		var relayContent model.Event
		relayContent.Kind = nostr.KindRelayListMetadata
		relayContent.Tags = model.Tags{
			{"r", "wss://embedded-relay.com", "read"},
		}
		contentJSON, err := relayContent.MarshalJSON()
		require.NoError(t, err)

		var embeddingEvent model.Event
		embeddingEvent.PubKey = "embedder"
		embeddingEvent.Kind = model.CustomIONKindEphemeralEmbedding
		embeddingEvent.Content = string(contentJSON)

		// Create regular event with references
		var regularEvent model.Event
		regularEvent.PubKey = "author1"
		regularEvent.Kind = nostr.KindTextNote
		regularEvent.Tags = model.Tags{
			{"e", "event123"},
			{"a", "address456"},
		}

		targets, err := broadcaster.collectTargets(t.Context(), []*model.Event{&embeddingEvent, &regularEvent})
		require.NoError(t, err)

		// Should find relays from embedding event
		require.Contains(t, targets, "embedder")
		require.Equal(t, []string{"wss://embedded-relay.com"}, targets["embedder"])
	})

	t.Run("gift wrap events", func(t *testing.T) {
		var memdb fixture.MemDB
		config := Config{
			RelayURL:   "ws://test.localhost",
			PrivateKey: testPrivateKey,
			QueryFunc:  memdb.SelectEvents,
		}
		broadcaster := New(config)
		defer broadcaster.Close()

		// Setup relay metadata for wrap receiver
		var relayEvent model.Event
		relayEvent.PubKey = "receiver1"
		relayEvent.Kind = nostr.KindRelayListMetadata
		relayEvent.Tags = model.Tags{
			{"r", "wss://receiver-relay.com", "read"},
		}
		require.NoError(t, memdb.AcceptEvents(t.Context(), &relayEvent))

		// Create gift wrap event
		var giftWrapEvent model.Event
		giftWrapEvent.Kind = nostr.KindGiftWrap
		giftWrapEvent.Tags = model.Tags{
			{"p", "receiver1"},
		}

		targets, err := broadcaster.collectTargets(t.Context(), []*model.Event{&giftWrapEvent})
		require.NoError(t, err)

		// Should find relays for receiver
		require.Contains(t, targets, "receiver1")
		require.Equal(t, []string{"wss://receiver-relay.com"}, targets["receiver1"])
	})

	t.Run("badge award events", func(t *testing.T) {
		var memdb fixture.MemDB
		config := Config{
			RelayURL:   "ws://test.localhost",
			PrivateKey: testPrivateKey,
			QueryFunc:  memdb.SelectEvents,
		}
		broadcaster := New(config)
		defer broadcaster.Close()

		var relayEvent model.Event
		relayEvent.PubKey = "badge-receiver1"
		relayEvent.Kind = nostr.KindRelayListMetadata
		relayEvent.Tags = model.Tags{
			{"r", "wss://badge-receiver-relay.com", "read"},
		}
		require.NoError(t, memdb.AcceptEvents(t.Context(), &relayEvent))

		var badgeAwardEvent model.Event
		badgeAwardEvent.Kind = nostr.KindBadgeAward
		badgeAwardEvent.Tags = model.Tags{
			{"p", "badge-receiver1"},
		}

		targets, err := broadcaster.collectTargets(t.Context(), []*model.Event{&badgeAwardEvent})
		require.NoError(t, err)

		require.Contains(t, targets, "badge-receiver1")
		require.Equal(t, []string{"wss://badge-receiver-relay.com"}, targets["badge-receiver1"])
	})

	t.Run("mixed event types", func(t *testing.T) {
		var memdb fixture.MemDB
		config := Config{
			RelayURL:   "ws://test.localhost",
			PrivateKey: testPrivateKey,
			QueryFunc:  memdb.SelectEvents,
		}
		broadcaster := New(config)
		defer broadcaster.Close()

		// Setup relay metadata
		var relayEvent1 model.Event
		relayEvent1.PubKey = "author1"
		relayEvent1.Kind = nostr.KindRelayListMetadata
		relayEvent1.Tags = model.Tags{
			{"r", "wss://author-relay.com", "read"},
		}

		var relayEvent2 model.Event
		relayEvent2.PubKey = "receiver1"
		relayEvent2.Kind = nostr.KindRelayListMetadata
		relayEvent2.Tags = model.Tags{
			{"r", "wss://receiver-relay.com", "read"},
		}

		require.NoError(t, memdb.AcceptEvents(t.Context(), &relayEvent1, &relayEvent2))

		// Create mixed events
		var giftWrap model.Event
		giftWrap.Kind = nostr.KindGiftWrap
		giftWrap.Tags = model.Tags{{"p", "receiver1"}}

		var regularEvent model.Event
		regularEvent.PubKey = "author1"
		regularEvent.Kind = nostr.KindTextNote

		targets, err := broadcaster.collectTargets(t.Context(), []*model.Event{&giftWrap, &regularEvent})
		require.NoError(t, err)

		// Should find relays for both
		require.Contains(t, targets, "author1")
		require.Contains(t, targets, "receiver1")
		require.Equal(t, []string{"wss://author-relay.com"}, targets["author1"])
		require.Equal(t, []string{"wss://receiver-relay.com"}, targets["receiver1"])
	})

	t.Run("no events", func(t *testing.T) {
		var memdb fixture.MemDB
		config := Config{
			RelayURL:   "ws://test.localhost",
			PrivateKey: testPrivateKey,
			QueryFunc:  memdb.SelectEvents,
		}
		broadcaster := New(config)
		defer broadcaster.Close()

		targets, err := broadcaster.collectTargets(t.Context(), []*model.Event{})
		require.NoError(t, err)
		require.Empty(t, targets)
	})

	t.Run("malformed embedding event", func(t *testing.T) {
		var memdb fixture.MemDB
		config := Config{
			RelayURL:   "ws://test.localhost",
			PrivateKey: testPrivateKey,
			QueryFunc:  memdb.SelectEvents,
		}
		broadcaster := New(config)
		defer broadcaster.Close()

		var embeddingEvent model.Event
		embeddingEvent.Kind = model.CustomIONKindEphemeralEmbedding
		embeddingEvent.Content = "invalid json"

		targets, err := broadcaster.collectTargets(t.Context(), []*model.Event{&embeddingEvent})
		require.Error(t, err)
		require.Contains(t, err.Error(), "malformed")
		require.Nil(t, targets)
	})

	t.Run("database query error", func(t *testing.T) {
		testErr := errors.New("database error")
		config := Config{
			RelayURL:   "ws://test.localhost",
			PrivateKey: testPrivateKey,
			QueryFunc: func(ctx context.Context, filters ...model.Filter) query.EventIterator {
				return func(yield func(*model.Event, error) bool) {
					yield(nil, testErr)
				}
			},
		}
		broadcaster := New(config)
		defer broadcaster.Close()

		var event model.Event
		event.PubKey = "author1"
		event.Kind = nostr.KindTextNote

		targets, err := broadcaster.collectTargets(t.Context(), []*model.Event{&event})
		require.Error(t, err)
		require.ErrorIs(t, err, testErr)
		require.Nil(t, targets)
	})

	t.Run("skip non-relay metadata events", func(t *testing.T) {
		var memdb fixture.MemDB
		config := Config{
			RelayURL:   "ws://test.localhost",
			PrivateKey: testPrivateKey,
			QueryFunc:  memdb.SelectEvents,
		}
		broadcaster := New(config)
		defer broadcaster.Close()

		// Add non-relay metadata event
		var nonRelayEvent model.Event
		nonRelayEvent.PubKey = "author1"
		nonRelayEvent.Kind = nostr.KindTextNote
		nonRelayEvent.Content = "not relay metadata"
		require.NoError(t, memdb.AcceptEvents(t.Context(), &nonRelayEvent))

		var event model.Event
		event.PubKey = "author1"
		event.Kind = nostr.KindTextNote

		targets, err := broadcaster.collectTargets(t.Context(), []*model.Event{&event})
		require.NoError(t, err)
		require.Empty(t, targets) // Should not find any relays
	})
}
