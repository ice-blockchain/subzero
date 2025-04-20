// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
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

		require.True(t, helperCheckTokenTag(t, events[0], token, false), "Token tag should exist")

		tagValue3, err := helperGetTokenValue3FromEventTags(t, t.Context(), db, pubKey)
		t.Logf("Before update - token value3 from event_tags: %s, error: %v", tagValue3, err)

		require.NoError(t, db.markTokenAsInvalid(t.Context(), deviceEvent.ID))

		eventsAfterUpdate := helperSelectEvents(t, db, model.Filter{
			Kinds: []int{model.CustomIONKindDeviceRegistration},
			Tags:  model.TagMap{}.Set("d", model.PointerOf(deviceID)),
		})
		require.Len(t, eventsAfterUpdate, 1, "Device registration event should still exist")

		t.Logf("After update - Event: %+v, Tags: %+v", eventsAfterUpdate[0], eventsAfterUpdate[0].Tags)

		tagValue3, err = helperGetTokenValue3FromEventTags(t, t.Context(), db, pubKey)
		t.Logf("After update - token value3 from event_tags: %s, error: %v", tagValue3, err)

		require.True(t, helperCheckTokenTag(t, eventsAfterUpdate[0], token, true), "Invalid token tag should exist")
	})

	t.Run("mark token as invalid for non-existing device", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()

		nonExistingEventID := "non-existing-event-" + uuid.NewString()
		require.Error(t, db.markTokenAsInvalid(t.Context(), nonExistingEventID), "Should return error for non-existing event")
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

		require.NoError(t, db.markTokenAsInvalid(t.Context(), deviceEvent1.ID))

		event1AfterUpdate := helperSelectEvents(t, db, model.Filter{
			Kinds: []int{model.CustomIONKindDeviceRegistration},
			Tags:  model.TagMap{}.Set("d", model.PointerOf(deviceID1)),
		})
		require.Len(t, event1AfterUpdate, 1, "First device event should still exist")

		require.True(t, helperCheckTokenTag(t, event1AfterUpdate[0], token1, true), "Invalid token tag should exist for device1")

		event2AfterUpdate := helperSelectEvents(t, db, model.Filter{
			Kinds: []int{model.CustomIONKindDeviceRegistration},
			Tags:  model.TagMap{}.Set("d", model.PointerOf(deviceID2)),
		})
		require.Len(t, event2AfterUpdate, 1, "Second device event should still exist")
		require.True(t, helperCheckTokenTag(t, event2AfterUpdate[0], token2, false), "Valid token tag should exist for device2")
	})
}

func TestMarkTokenAsInvalidInEventTags(t *testing.T) {
	t.Parallel()

	t.Run("mark token in event_tags", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()

		deviceID := "test-etags-device-" + uuid.NewString()
		pubKey := "test-etags-pubkey-" + uuid.NewString()
		token := "fcm-etags-token-" + uuid.NewString()

		deviceEvent := helperCreateDeviceRegistrationEvent(deviceID, pubKey, token)

		require.NoError(t, db.AcceptEvents(t.Context(), deviceEvent))
		require.NoError(t, db.markTokenAsInvalidInEventTags(t.Context(), deviceEvent.ID))
		require.NoError(t, db.markTokenAsInvalidInEvents(t.Context(), deviceEvent.ID))
		events := helperSelectEvents(t, db, model.Filter{IDs: []string{deviceEvent.ID}})
		require.Len(t, events, 1, "Should find the event")

		require.True(t, helperCheckTokenTag(t, events[0], token, true), "Invalid token tag should exist")
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
		require.NoError(t, db.markTokenAsInvalidInEvents(t.Context(), deviceEvent.ID), "Should not return error when updating token in events")
		events := helperSelectEvents(t, db, model.Filter{IDs: []string{deviceEvent.ID}})
		require.Len(t, events, 1, "Should find the event")

		require.True(t, helperCheckTokenTag(t, events[0], token, true), "Invalid token tag should exist in events table")
	})
}

func TestRollbackTokenInEventTags(t *testing.T) {
	t.Parallel()

	t.Run("rollback token in event_tags", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()

		deviceID := "test-rollback-device-" + uuid.NewString()
		pubKey := "test-rollback-pubkey-" + uuid.NewString()
		token := "fcm-rollback-token-" + uuid.NewString()

		deviceEvent := helperCreateDeviceRegistrationEvent(deviceID, pubKey, token)
		require.NoError(t, db.AcceptEvents(t.Context(), deviceEvent))
		require.NoError(t, db.markTokenAsInvalid(t.Context(), deviceEvent.ID))
		eventsBeforeRollback := helperSelectEvents(t, db, model.Filter{IDs: []string{deviceEvent.ID}})
		require.Len(t, eventsBeforeRollback, 1, "Should find the event")
		require.True(t, helperCheckTokenTag(t, eventsBeforeRollback[0], token, true), "Invalid token tag should exist before rollback")
		require.Error(t, db.rollbackTokenInEventTags(t.Context(), deviceEvent.ID), "Should not return error when rolling back")

		helperSyncEventsWithEventTags(t, db, deviceEvent.ID)

		eventsAfterRollback := helperSelectEvents(t, db, model.Filter{IDs: []string{deviceEvent.ID}})
		require.Len(t, eventsAfterRollback, 1, "Should find the event after rollback")

		invalidTokenAfterRollback := false
		for _, tag := range eventsAfterRollback[0].Tags {
			if tag.Key() == "token" && len(tag) >= 3 && tag[2] == "invalid" {
				invalidTokenAfterRollback = true
				break
			}
		}
		require.False(t, invalidTokenAfterRollback, "Token should not be marked as invalid after rollback")
	})

	t.Run("rollback token for non-existing event", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()

		nonExistingEventID := "non-existing-rollback-event-" + uuid.NewString()
		require.Error(t, db.rollbackTokenInEventTags(t.Context(), nonExistingEventID), "Should return error for non-existing event")
	})
}

func helperGetTokenValue3FromEventTags(t *testing.T, ctx context.Context, db *dbClient, pubKey string) (string, error) {
	t.Helper()

	var tagValue3 string
	err := db.DB.QueryRowContext(ctx, `
		SELECT et.event_tag_value3
		FROM event_tags et
		JOIN events e ON e.id = et.event_id
		WHERE e.pubkey = $1 AND et.event_tag_key = 'token'
	`, pubKey).Scan(&tagValue3)

	return tagValue3, err
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

func helperCheckTokenTag(t *testing.T, event *model.Event, expectedToken string, expectInvalid bool) bool {
	t.Helper()

	for _, tag := range event.Tags {
		if tag.Key() == "token" && len(tag) >= 2 {
			require.Equal(t, expectedToken, tag.Value(), "Token value should match")
			if expectInvalid {
				if len(tag) >= 3 {
					require.Equal(t, "invalid", tag[2], "Token should be marked as invalid")

					return true
				}

				return false
			} else {
				if len(tag) >= 3 {
					require.NotEqual(t, "invalid", tag[2], "Token should not be marked as invalid")
				}

				return true
			}
		}
	}

	if expectInvalid {
		t.Fail()

		t.Log("Expected to find invalid token tag, but none found")
	}

	return false
}

func helperSyncEventsWithEventTags(t *testing.T, db *dbClient, eventID string) {
	t.Helper()

	_, err := db.DB.ExecContext(t.Context(), `
		UPDATE events e
		SET tags = (
			SELECT json_agg(
				CASE
					WHEN elem->0 ? 'token' THEN
						(SELECT jsonb_build_array(elem->0, elem->1, et.event_tag_value3)
						 FROM event_tags et
						 WHERE et.event_id = e.id AND et.event_tag_key = 'token')
					ELSE
						elem
				END
			)
			FROM jsonb_array_elements(tags) AS elem
		)
		WHERE id = $1
	`, eventID)

	require.NoError(t, err, "Should be able to sync events with event_tags")
}
