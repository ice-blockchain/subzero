// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"context"
	"crypto/rand"
	"sync"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
	"github.com/ice-blockchain/subzero/rq"
)

func TestNotificationBroadcastRemoteEvents(t *testing.T) {
	// This test is NOT parallel because it relies on shared database state.

	mockNotificationClient := &mockNotificationClient{
		T:    t,
		Chan: make(chan *pn.Notification[*model.Event], 100),
	}

	const remoteRelayURL = "wss://remote-relay.example.com"

	pm := helperNewManager(t)
	pm.pushNotificationClient = mockNotificationClient

	pm.rq = rq.MustNewClient(t.Context(), rq.WithConfig(helperNewRiverConfig(t, pm, "pn-broadcaster-remote-e2e-test")))
	pm.registerWorkers()

	require.NoError(t, pm.rq.Start(t.Context()))
	defer pm.rq.Stop(t.Context())

	broadcasterForwardCtx, cancelBroadcaster := context.WithCancel(t.Context())
	var wg sync.WaitGroup
	wg.Go(func() {
		mockedBroadcaster, ok := pm.broadcaster.(*mockBroadcaster)
		require.True(t, ok, "Broadcaster should be of type *mockBroadcaster for this test")

	next:
		for broadcasterForwardCtx.Err() == nil {
			select {
			case <-broadcasterForwardCtx.Done():
				return
			case data, open := <-mockedBroadcaster.Chan:
				if !open {
					return
				}
				t.Logf("Broadcaster received %d events to forward from %s", len(data.Events), data.RelayURL)
				if got, expected := data.RelayURL, remoteRelayURL; got != expected {
					t.Logf("Ignoring events from relay %q since it doesn't match expected remote relay URL %q", got, expected)
					continue next
				}
				require.NoError(t, pm.AcceptEventsFromBroadcast(broadcasterForwardCtx, data.Events))
			}
		}
	})

	subscriber := helperCreateTestUser(t)

	storyAuthor := helperCreateTestUser(t)
	videoPostAuthor := helperCreateTestUser(t)
	articleAuthor := helperCreateTestUser(t)
	genericPostAuthor := helperCreateTestUser(t)

	t.Run("Register user for notifications", func(t *testing.T) {
		subscriberFilters := model.Filters{model.Filter{
			Kinds:   []int{nostr.KindArticle, nostr.KindTextNote, model.CustomIONKindEditableTextNote},
			Authors: []string{storyAuthor.PublicKey, videoPostAuthor.PublicKey, articleAuthor.PublicKey, genericPostAuthor.PublicKey},
		}}
		deviceID := rand.Text()

		t.Logf("subscriber: pubkey=%s, device=%s", subscriber.PublicKey, deviceID)

		t.Run("Remote", func(t *testing.T) {
			var deviceRegEventRemote model.Event

			deviceRegEventRemote.Kind = model.CustomIONKindDeviceRegistration
			deviceRegEventRemote.CreatedAt = nostr.Now()
			deviceRegEventRemote.Content = subscriberFilters.String()
			deviceRegEventRemote.Tags = model.Tags{
				{"d", subscriber.PublicKey + "_" + deviceID},
				{"relay", remoteRelayURL},
			}
			for _, relayURL := range subscriber.Relays {
				deviceRegEventRemote.Tags = append(deviceRegEventRemote.Tags, model.Tag{"relay", relayURL})
			}
			require.NoError(t, deviceRegEventRemote.SignWithAlg(subscriber.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

			require.NoError(t, pm.ManageDeviceRegistrationEvents(t.Context(), model.Events{&deviceRegEventRemote}))
			require.Equal(t, 1, pm.devicesFilterIndex.Size())
			for dev := range pm.devicesFilterIndex.Range() {
				require.True(t, dev.Remote)
				require.Equal(t, deviceRegEventRemote.ID, dev.Event.ID)
				require.Equal(t, subscriber.PublicKey, dev.Event.PubKey)
			}
		})
		t.Run("Local", func(t *testing.T) {
			var deviceRegEventLocal model.Event

			deviceRegEventLocal.Kind = model.CustomIONKindDeviceRegistration
			deviceRegEventLocal.CreatedAt = nostr.Now()
			deviceRegEventLocal.Content = subscriberFilters.String()
			deviceRegEventLocal.Tags = model.Tags{
				{"d", deviceID},
				{"t", model.DeviceTokenOSIOS},
				{"relay", pm.relayURL},
			}
			require.NoError(t, deviceRegEventLocal.SignWithAlg(subscriber.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

			require.NoError(t, pm.ManageDeviceRegistrationEvents(t.Context(), model.Events{&deviceRegEventLocal}))
			require.Equal(t, 4, pm.devicesEventMap.Size())
			require.Equal(t, 2, pm.devicesFilterIndex.Size())
			for dev := range pm.devicesFilterIndex.Range() {
				require.Equal(t, subscriber.PublicKey, dev.Event.PubKey)
				if dev.Remote {
					continue
				}
				require.Equal(t, deviceRegEventLocal.ID, dev.Event.ID)
			}
		})
	})
	var storyEvent, videoPostEvent, articleEvent, genericPostEvent model.Event
	t.Run("Create events to trigger notifications", func(t *testing.T) {
		storyEvent.Kind, videoPostEvent.Kind, genericPostEvent.Kind = model.CustomIONKindEditableTextNote, model.CustomIONKindEditableTextNote, model.CustomIONKindEditableTextNote
		articleEvent.Kind = nostr.KindArticle
		storyEvent.CreatedAt, videoPostEvent.CreatedAt, articleEvent.CreatedAt, genericPostEvent.CreatedAt = nostr.Now(), nostr.Now(), nostr.Now(), nostr.Now()
		storyEvent.Content, videoPostEvent.Content, articleEvent.Content, genericPostEvent.Content = "Story content", "Video post content", "Article content", "Generic post content"

		storyEvent.Tags = model.Tags{
			{"expiration", storyEvent.CreatedAt.Add(time.Hour).String()},
		}

		videoPostEvent.Tags = model.Tags{
			{"imeta", "url https://example.com", "m video/mp4"},
		}

		require.NoError(t, storyEvent.SignWithAlg(storyAuthor.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, videoPostEvent.SignWithAlg(videoPostAuthor.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, articleEvent.SignWithAlg(articleAuthor.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, genericPostEvent.SignWithAlg(genericPostAuthor.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		require.True(t, storyEvent.IsStory())
		require.True(t, videoPostEvent.HasVideoIMeta())
	})

	var cases = []struct {
		Type          NotificationType
		ExpectedEvent *model.Event
	}{
		{Type: NotificationTypeSomeoneStory, ExpectedEvent: &storyEvent},
		{Type: NotificationTypeSomeoneVideo, ExpectedEvent: &videoPostEvent},
		{Type: NotificationTypeSomeoneArticle, ExpectedEvent: &articleEvent},
		{Type: NotificationTypeSomeonePost, ExpectedEvent: &genericPostEvent},
	}
	for _, c := range cases {
		t.Run("Triggering notifications for "+string(c.Type), func(t *testing.T) {
			require.NoError(t, pm.AcceptEventsForRemotePush(t.Context(), model.Events{c.ExpectedEvent}))
			helperWaitForNotifications(t, mockNotificationClient, c.Type, c.ExpectedEvent)
		})
	}

	cancelBroadcaster()
	wg.Wait()
}
