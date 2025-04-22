// SPDX-License-Identifier: ice License 1.0

package query

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func TestMarkTokenAsInvalid(t *testing.T) {
	t.Parallel()

	t.Run("mark token as invalid for existing device", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()

		deviceID := "test-device-" + uuid.NewString()
		pubKey := "test-pubkey-" + uuid.NewString()
		token := "fcm-token-" + uuid.NewString()

		deviceEvent := helperCreateDeviceRegistrationEvent(deviceID, pubKey, token)

		require.NoError(t, db.AcceptEvents(t.Context(), deviceEvent))

		events := helperSelectEvents(t, db, model.Filter{
			Kinds: []int{model.CustomIONKindDeviceRegistration},
			Tags:  model.TagMap{}.Set("d", model.PointerOf(deviceID)),
		})
		require.Len(t, events, 1, "Device registration event should be stored")

		t.Logf("Before update - Event: %+v, Tags: %+v", events[0], events[0].Tags)

		initialInvalidToken, err := helperGetNotificationTokenInvalid(t, db, deviceEvent.ID)
		require.NoError(t, err)
		require.Nil(t, initialInvalidToken, "notification_token_invalid should be NULL initially")

		require.NoError(t, db.markTokenAsInvalidInEvents(t.Context(), []*model.Event{events[0]}))

		eventsAfterUpdate := helperSelectEvents(t, db, model.Filter{
			Kinds: []int{model.CustomIONKindDeviceRegistration},
			Tags:  model.TagMap{}.Set("d", model.PointerOf(deviceID)),
		})
		require.Len(t, eventsAfterUpdate, 1, "Device registration event should still exist")

		t.Logf("After update - Event: %+v, Tags: %+v", eventsAfterUpdate[0], eventsAfterUpdate[0].Tags)

		updatedInvalidToken, err := helperGetNotificationTokenInvalid(t, db, deviceEvent.ID)
		require.NoError(t, err)
		require.NotNil(t, updatedInvalidToken, "notification_token_invalid should not be NULL after update")
		require.True(t, *updatedInvalidToken, "notification_token_invalid should be TRUE after update")
	})

	t.Run("mark token as invalid for non-existing device", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()

		nonExistingEvent := &model.Event{
			Event: nostr.Event{
				ID: "non-existing-event-" + uuid.NewString(),
			},
		}
		require.Error(t, db.markTokenAsInvalidInEvents(t.Context(), []*model.Event{nonExistingEvent}), "Should return error for non-existing event")
	})

	t.Run("mark token as invalid for multiple devices with same pubkey", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()

		pubKey := "multi-device-pubkey-" + uuid.NewString()
		deviceID1 := "test-device1-" + uuid.NewString()
		deviceID2 := "test-device2-" + uuid.NewString()
		token1 := "fcm-token1-" + uuid.NewString()
		token2 := "fcm-token2-" + uuid.NewString()

		deviceEvent1 := helperCreateDeviceRegistrationEvent(deviceID1, pubKey, token1)
		deviceEvent2 := helperCreateDeviceRegistrationEvent(deviceID2, pubKey, token2)

		require.NoError(t, db.AcceptEvents(t.Context(), deviceEvent1))
		require.NoError(t, db.AcceptEvents(t.Context(), deviceEvent2))

		events1 := helperSelectEvents(t, db, model.Filter{
			Kinds: []int{model.CustomIONKindDeviceRegistration},
			Tags:  model.TagMap{}.Set("d", model.PointerOf(deviceID1)),
		})
		require.Len(t, events1, 1, "First device event should exist")

		events2 := helperSelectEvents(t, db, model.Filter{
			Kinds: []int{model.CustomIONKindDeviceRegistration},
			Tags:  model.TagMap{}.Set("d", model.PointerOf(deviceID2)),
		})
		require.Len(t, events2, 1, "Second device event should exist")

		require.NoError(t, db.markTokenAsInvalidInEvents(t.Context(), []*model.Event{events1[0]}))

		invalid1, err := helperGetNotificationTokenInvalid(t, db, deviceEvent1.ID)
		require.NoError(t, err)
		require.NotNil(t, invalid1, "notification_token_invalid should not be NULL for device1")
		require.True(t, *invalid1, "notification_token_invalid should be TRUE for device1")

		invalid2, err := helperGetNotificationTokenInvalid(t, db, deviceEvent2.ID)
		require.NoError(t, err)
		require.Nil(t, invalid2, "notification_token_invalid should remain NULL for device2")
	})
}

func TestMarkTokenAsInvalidInEvents(t *testing.T) {
	t.Parallel()
	t.Run("mark token in events table", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()

		deviceID := "test-events-device-" + uuid.NewString()
		pubKey := "test-events-pubkey-" + uuid.NewString()
		token := "fcm-events-token-" + uuid.NewString()

		deviceEvent := helperCreateDeviceRegistrationEvent(deviceID, pubKey, token)
		require.NoError(t, db.AcceptEvents(t.Context(), deviceEvent))
		require.NoError(t, db.markTokenAsInvalidInEvents(t.Context(), []*model.Event{deviceEvent}), "Should not return error when updating token in events")

		invalid, err := helperGetNotificationTokenInvalid(t, db, deviceEvent.ID)
		require.NoError(t, err)
		require.NotNil(t, invalid, "notification_token_invalid should not be NULL after update")
		require.True(t, *invalid, "notification_token_invalid should be TRUE after update")
	})

	t.Run("mark token for non-device event should fail", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()

		nonDeviceEvent := &model.Event{
			Event: nostr.Event{
				ID:        "non-device-event-" + uuid.NewString(),
				PubKey:    "pubkey-" + uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindTextNote,
				Tags:      model.Tags{},
			},
		}

		require.NoError(t, db.AcceptEvents(t.Context(), nonDeviceEvent))
		require.Error(t, db.markTokenAsInvalidInEvents(t.Context(), []*model.Event{nonDeviceEvent}),
			"Should return error when trying to mark non-device event as invalid")
	})

	t.Run("mark multiple tokens as invalid", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()

		pubKey := "batch-test-pubkey-" + uuid.NewString()
		deviceCount := 3
		deviceEvents := make([]*model.Event, deviceCount)

		for i := 0; i < deviceCount; i++ {
			deviceID := "batch-device-" + uuid.NewString()
			token := "batch-token-" + uuid.NewString()
			deviceEvents[i] = helperCreateDeviceRegistrationEvent(deviceID, pubKey, token)
			require.NoError(t, db.AcceptEvents(t.Context(), deviceEvents[i]))
		}

		require.NoError(t, db.markTokenAsInvalidInEvents(t.Context(), deviceEvents),
			"Should not return error when updating multiple tokens")

		for _, event := range deviceEvents {
			invalid, err := helperGetNotificationTokenInvalid(t, db, event.ID)
			require.NoError(t, err)
			require.NotNil(t, invalid, "notification_token_invalid should not be NULL after batch update")
			require.True(t, *invalid, "notification_token_invalid should be TRUE after batch update for event %s", event.ID)
		}
	})

	t.Run("mark tokens as invalid with empty array should not fail", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()

		require.NoError(t, db.markTokenAsInvalidInEvents(t.Context(), []*model.Event{}),
			"Should not return error when updating with empty array")
	})
}

func TestGetStoredEventsWithNotificationTokenInvalid(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	deviceID := "test-device-token-" + uuid.NewString()
	pubKey := "test-pubkey-token-" + uuid.NewString()
	token := "fcm-token-test-" + uuid.NewString()

	deviceEvent := helperCreateDeviceRegistrationEvent(deviceID, pubKey, token)

	require.NoError(t, db.AcceptEvents(t.Context(), deviceEvent))
	require.NoError(t, db.markTokenAsInvalidInEvents(t.Context(), []*model.Event{deviceEvent}))

	subscription := &model.Subscription{
		Filters: model.Filters{
			model.Filter{
				IDs: []string{deviceEvent.ID},
			},
		},
	}

	var foundEvents []*model.Event
	for event, err := range db.SelectEvents(t.Context(), subscription.Filters...) {
		require.NoError(t, err, "Unexpected error from SelectEvents")
		foundEvents = append(foundEvents, event)
	}

	require.Len(t, foundEvents, 1, "Should find exactly one event")
	require.Equal(t, deviceEvent.ID, foundEvents[0].ID, "Should find our device event")
	require.True(t, foundEvents[0].NotificationTokenInvalid, "NotificationTokenInvalid field should be true in the Event struct")
}

func helperCreateDeviceRegistrationEvent(deviceID, pubKey, token string) *model.Event {
	return &model.Event{
		Event: nostr.Event{
			ID:        "device-event-" + uuid.NewString(),
			PubKey:    pubKey,
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.CustomIONKindDeviceRegistration,
			Tags: model.Tags{
				{"d", deviceID},
				{"token", token},
			},
		},
	}
}

func helperGetNotificationTokenInvalid(t *testing.T, db *dbClient, eventID string) (*bool, error) {
	t.Helper()
	var invalidToken *bool
	err := db.DB.QueryRowContext(t.Context(), "SELECT notification_token_invalid FROM events WHERE id = $1", eventID).Scan(&invalidToken)

	return invalidToken, err
}
