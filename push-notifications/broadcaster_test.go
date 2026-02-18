// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"context"
	"crypto/rand"
	"fmt"
	mathRand "math/rand/v2"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/push-notifications/internal"
	"github.com/ice-blockchain/subzero/rq"
)

type (
	testUser struct {
		PrivateKey string
		PublicKey  string
		Relays     []string
	}
	mockedBroadcastEvent struct {
		RelayURL string
		Events   model.Events
	}
	mockBroadcaster struct {
		T    testing.TB
		Chan chan mockedBroadcastEvent
	}
	mockNotificationClient struct {
		T    testing.TB
		Chan chan *internal.Notification[*model.Event]
	}
)

func (m *mockBroadcaster) Reset() {
	for {
		select {
		case e := <-m.Chan:
			m.T.Logf("Draining broadcast event to relay %s with %d events", e.RelayURL, len(e.Events))
		default:
			return
		}
	}
}
func (m *mockNotificationClient) Reset() {
	for {
		select {
		case n := <-m.Chan:
			m.T.Logf("Draining notification to device %s for master public key %s", n.Target.PubKey, n.Target.GetMasterPublicKey())
		default:
			return
		}
	}
}

func (m *mockNotificationClient) SendSingle(ctx context.Context, notification *internal.Notification[*model.Event]) error {
	select {
	case m.Chan <- notification:
		m.T.Logf("Mock SendSingle to device %s for master public key %s", notification.Target.PubKey, notification.Target.GetMasterPublicKey())
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *mockNotificationClient) SendTopic(ctx context.Context, notification *internal.Notification[internal.SubscriptionTopic]) error {
	m.T.Fatalf("unexpected call to SendTopic with topic: %v", notification.Target)
	return nil
}

func (m *mockBroadcaster) BroadcastTo(ctx context.Context, target string, events model.Events) error {
	select {
	case m.Chan <- mockedBroadcastEvent{RelayURL: target, Events: events}:
		m.T.Logf("Mock BroadcastTo %d events to relay %s", len(events), target)
		return nil

	case <-time.After(time.Second * 5):
		m.T.Fatalf("timed out broadcasting to relay %s", target)
		return nil

	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *mockBroadcaster) Close() {}

func helperCreateTestUser(t *testing.T) *testUser {
	t.Helper()

	privateKey, publicKey := model.GenerateKeyPair()
	relayCount := mathRand.IntN(5) + 1 // 1 to 5 relays.
	relays := make([]string, relayCount)
	for i := range relayCount {
		relays[i] = fmt.Sprintf("wss://relay-%s-%d.example.com", rand.Text()[:8], i)
	}
	return &testUser{
		PrivateKey: privateKey,
		PublicKey:  publicKey,
		Relays:     relays,
	}
}

func helperCreateRelayListEvent(t *testing.T, user *testUser) *model.Event {
	t.Helper()

	var ev model.Event
	ev.Kind = nostr.KindRelayListMetadata
	ev.CreatedAt = nostr.Now()
	ev.Tags = make(model.Tags, 0, len(user.Relays))
	for _, relay := range user.Relays {
		ev.Tags = append(ev.Tags, model.Tag{"r", relay})
	}
	require.NoError(t, ev.SignWithAlg(user.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	return &ev
}

func helperCreateDeviceRegistrationEvent(
	tb testing.TB,
	user *testUser,
	subscribeToAuthors []string,
	kinds []int,
	relayURL string,
) *model.Event {
	tb.Helper()

	deviceID := rand.Text()
	filters := model.Filters{
		{
			Kinds:   kinds,
			Authors: subscribeToAuthors,
		},
	}

	var ev model.Event
	ev.Kind = model.CustomIONKindDeviceRegistration
	ev.CreatedAt = nostr.Now()
	ev.Content = filters.String()
	ev.Tags = model.Tags{
		{"d", deviceID},
		{"t", "ios"},
		{"relay", relayURL},
		{"token", "encrypted-token-" + rand.Text()[:8]},
	}
	require.NoError(tb, ev.SignWithAlg(user.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	return &ev
}

func helperCreateEditableTextNoteEvent(t *testing.T, user *testUser, content string, pTags ...string) *model.Event {
	t.Helper()

	var ev model.Event
	ev.Kind = model.CustomIONKindEditableTextNote
	ev.CreatedAt = nostr.Now()
	ev.Content = content
	ev.Tags = model.Tags{
		{"d", rand.Text()},
	}
	for _, p := range pTags {
		ev.Tags = append(ev.Tags, model.Tag{"p", p})
	}

	require.NoError(t, ev.SignWithAlg(user.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	return &ev
}

func helperCreateTokenizedCommunityDefinitionEvent(t *testing.T, user *testUser, buyerPubKey string) *model.Event {
	t.Helper()

	var ev model.Event
	ev.Kind = model.CustomIONKindTokenizedCommunityDefinition
	ev.CreatedAt = nostr.Now()
	ev.Content = "First buy event"
	ev.Tags = model.Tags{
		{"d", rand.Text()},
		{"p", buyerPubKey},
		{"t", "community_token_action"},
	}
	require.NoError(t, ev.SignWithAlg(user.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	return &ev
}

func helperVerifyBroadcastEvents(t *testing.T, broadcastedEvents []mockedBroadcastEvent, expectedLength int) {
	t.Helper()

	for i := range broadcastedEvents {
		require.Len(t, broadcastedEvents[i].Events, expectedLength)
		require.Equal(t, model.CustomIONKindEphemeralEmbedding, broadcastedEvents[i].Events[0].Kind)
		ok, err := broadcastedEvents[i].Events[0].CheckSignature()
		require.NoError(t, err)
		require.True(t, ok)
	}
}

func helperWaitForNotifications(t *testing.T, mockClient *mockNotificationClient, notificationType NotificationType, expectedEvent *model.Event) {
	t.Helper()

	select {
	case n := <-mockClient.Chan:
		t.Logf("Received push notification %#v", n)
		require.Equal(t, expectedEvent, n.SourceEvent)
		require.Equal(t, defaultTranslations[notificationType].Title, n.Title)
		require.Equal(t, defaultTranslations[notificationType].Body, n.Body)

	case <-time.After(time.Second * 5):
		t.Fatalf("Timed out waiting for push notification")
	}
}

func TestNotificationBroadcastEndToEnd(t *testing.T) {
	t.Parallel()

	addr, release := query.NewTestDatabase(t.Context())
	defer release()

	mockNotificationClient := &mockNotificationClient{
		T:    t,
		Chan: make(chan *internal.Notification[*model.Event], 100),
	}

	pm := helperNewManager(t)
	pm.pushNotificationClient = internal.Client(mockNotificationClient)

	dbConf := query.Config{
		PrivateKey: pm.privateKey,
		RelayURL:   pm.relayURL,
		WriteURLs:  []string{addr},
	}
	query.MustInit(t.Context(), query.WithConfig(&dbConf))

	pm.rq = rq.MustNewClient(t.Context(), rq.WithConfig(&rq.Config{
		Config: dbConf,
		ID:     "pn-test-e2e",
	}))
	pm.registerWorkers()

	require.NoError(t, pm.rq.Start(t.Context()))
	defer pm.rq.Stop(t.Context())

	user1 := helperCreateTestUser(t) // Author 1.
	user2 := helperCreateTestUser(t) // Author 2.
	user3 := helperCreateTestUser(t) // Subscriber 1.
	user4 := helperCreateTestUser(t) // Subscriber 2.

	t.Logf("User1 (Author): %s with %d relays", user1.PublicKey, len(user1.Relays))
	t.Logf("User2 (Author): %s with %d relays", user2.PublicKey, len(user2.Relays))
	t.Logf("User3 (Subscriber): %s with %d relays", user3.PublicKey, len(user3.Relays))
	t.Logf("User4 (Subscriber): %s with %d relays", user4.PublicKey, len(user4.Relays))

	t.Run("Setup users and RelayLists", func(t *testing.T) {
		relayEvents := []*model.Event{
			helperCreateRelayListEvent(t, user1),
			helperCreateRelayListEvent(t, user2),
			helperCreateRelayListEvent(t, user3),
			helperCreateRelayListEvent(t, user4),
		}
		require.NoError(t, query.AcceptEvents(t.Context(), relayEvents...))

		for _, user := range []*testUser{user1, user2, user3, user4} {
			var found bool
			for ev, err := range query.GetStoredEvents(t.Context(), model.Filter{
				Authors: []string{user.PublicKey},
				Kinds:   []int{nostr.KindRelayListMetadata},
				Limit:   1,
			}) {
				require.NoError(t, err)
				found = true
				relays := model.CollectRelaysFromRelayEvent(ev)
				require.ElementsMatch(t, user.Relays, relays, "Relay list should match for user %s", user.PublicKey)
			}
			require.True(t, found, "Relay list should exist for user %s", user.PublicKey)
		}
	})

	t.Run("Setup_Device_Registrations", func(t *testing.T) {
		t.Logf("subscribing user3 to user1 with relay %s", pm.relayURL)
		deviceEvent3 := helperCreateDeviceRegistrationEvent(
			t, user3,
			[]string{user1.PublicKey},
			[]int{model.CustomIONKindEditableTextNote, model.CustomIONKindTokenizedCommunityDefinition},
			pm.relayURL,
		)

		t.Logf("subscribing user4 to user2 with relay %s", pm.relayURL)
		deviceEvent4 := helperCreateDeviceRegistrationEvent(
			t, user4,
			[]string{user2.PublicKey},
			[]int{model.CustomIONKindEditableTextNote, model.CustomIONKindTokenizedCommunityDefinition},
			pm.relayURL,
		)

		require.NoError(t, query.AcceptEvents(t.Context(), deviceEvent3, deviceEvent4))

		// Register devices in the manager.
		require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent3))
		require.NoError(t, pm.processDeviceRegistrationEvent(deviceEvent4))

		// Verify devices are registered.
		pm.deviceMutex.RLock()
		_, hasUser3 := pm.userDevicesMap[user3.PublicKey]
		_, hasUser4 := pm.userDevicesMap[user4.PublicKey]
		pm.deviceMutex.RUnlock()

		require.True(t, hasUser3, "User3 should have registered devices")
		require.True(t, hasUser4, "User4 should have registered devices")
	})

	t.Run("Publish EditableTextNote from user1", func(t *testing.T) {
		postEvent := helperCreateEditableTextNoteEvent(t, user1, "Hello from User1!", user3.PublicKey)
		t.Logf("Publishing 30175 event from User1: %s", postEvent.ID)

		err := pm.AcceptEventsForBroadcast(t.Context(), []*model.Event{postEvent})
		require.NoError(t, err)

		var receivedBroadcasts []mockedBroadcastEvent
		for range user3.Relays {
			select {
			case b := <-pm.broadcaster.(*mockBroadcaster).Chan:
				t.Logf("Received broadcast to relay %s with %d events", b.RelayURL, len(b.Events))
				require.Contains(t, user3.Relays, b.RelayURL)
				receivedBroadcasts = append(receivedBroadcasts, b)
			case <-time.After(time.Second * 5):
				t.Fatal("Timed out waiting for broadcasts to User3's relays")
			}
		}

		helperVerifyBroadcastEvents(t, receivedBroadcasts, 1)
		for _, b := range receivedBroadcasts {
			require.NoError(t, pm.AcceptEventsFromBroadcast(t.Context(), b.Events))
		}
		// Wait just for a single event since we have deduplication inside RQ.
		helperWaitForNotifications(t, mockNotificationClient, NotificationTypeMentionReply, postEvent)
	})

	t.Run("Publish_TokenizedCommunityDefinition_From_User2", func(t *testing.T) {
		pm.broadcaster.(*mockBroadcaster).Reset()
		mockNotificationClient.Reset()

		postEvent := helperCreateTokenizedCommunityDefinitionEvent(t, user2, user4.PublicKey)
		t.Logf("Publishing 31175 (first buy) event from User2: %s", postEvent.ID)

		err := pm.AcceptEventsForBroadcast(t.Context(), []*model.Event{postEvent})
		require.NoError(t, err)

		var receivedBroadcasts []mockedBroadcastEvent
		for range user4.Relays {
			select {
			case b := <-pm.broadcaster.(*mockBroadcaster).Chan:
				t.Logf("Received broadcast to relay %s with %d events", b.RelayURL, len(b.Events))
				require.Contains(t, user4.Relays, b.RelayURL)
				receivedBroadcasts = append(receivedBroadcasts, b)

			case <-time.After(time.Second * 5):
				t.Fatal("Timed out waiting for broadcasts to User4's relays")
			}
		}

		helperVerifyBroadcastEvents(t, receivedBroadcasts, 1)
		for _, b := range receivedBroadcasts {
			require.NoError(t, pm.AcceptEventsFromBroadcast(t.Context(), b.Events))
		}
		// Wait just for a single event since we have deduplication inside RQ.
		helperWaitForNotifications(t, mockNotificationClient, NotificationTypeContentTokenCreated, postEvent)
	})
}
