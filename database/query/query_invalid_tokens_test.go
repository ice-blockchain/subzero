// SPDX-License-Identifier: ice License 1.0

package query

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ice-blockchain/subzero/model"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
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

		var initialInvalidToken *bool
		err := db.DB.QueryRowContext(t.Context(), "SELECT invalid_token FROM events WHERE id = $1", deviceEvent.ID).Scan(&initialInvalidToken)
		require.NoError(t, err)
		require.Nil(t, initialInvalidToken, "invalid_token should be NULL initially")

		require.NoError(t, db.markTokenAsInvalidInEvents(t.Context(), []*model.Event{events[0]}))

		eventsAfterUpdate := helperSelectEvents(t, db, model.Filter{
			Kinds: []int{model.CustomIONKindDeviceRegistration},
			Tags:  model.TagMap{}.Set("d", model.PointerOf(deviceID)),
		})
		require.Len(t, eventsAfterUpdate, 1, "Device registration event should still exist")

		t.Logf("After update - Event: %+v, Tags: %+v", eventsAfterUpdate[0], eventsAfterUpdate[0].Tags)

		var updatedInvalidToken bool
		err = db.DB.QueryRowContext(t.Context(), "SELECT invalid_token FROM events WHERE id = $1", deviceEvent.ID).Scan(&updatedInvalidToken)
		require.NoError(t, err)
		require.True(t, updatedInvalidToken, "invalid_token should be TRUE after update")
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

		var invalid1 bool
		err := db.DB.QueryRowContext(t.Context(), "SELECT invalid_token FROM events WHERE id = $1", deviceEvent1.ID).Scan(&invalid1)
		require.NoError(t, err)
		require.True(t, invalid1, "invalid_token should be TRUE for device1")

		var invalid2 *bool
		err = db.DB.QueryRowContext(t.Context(), "SELECT invalid_token FROM events WHERE id = $1", deviceEvent2.ID).Scan(&invalid2)
		require.NoError(t, err)
		require.Nil(t, invalid2, "invalid_token should remain NULL for device2")
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

		var invalid bool
		err := db.DB.QueryRowContext(t.Context(), "SELECT invalid_token FROM events WHERE id = $1", deviceEvent.ID).Scan(&invalid)
		require.NoError(t, err)
		require.True(t, invalid, "invalid_token should be TRUE after update")
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
			var invalid bool
			err := db.DB.QueryRowContext(t.Context(), "SELECT invalid_token FROM events WHERE id = $1", event.ID).Scan(&invalid)
			require.NoError(t, err)
			require.True(t, invalid, "invalid_token should be TRUE after batch update for event %s", event.ID)
		}
	})

	t.Run("mark tokens as invalid with empty array should not fail", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()

		require.NoError(t, db.markTokenAsInvalidInEvents(t.Context(), []*model.Event{}),
			"Should not return error when updating with empty array")
	})
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
