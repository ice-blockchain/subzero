// SPDX-License-Identifier: ice License 1.0

package query

import (
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func TestCollectDeviceRegistrationEvents(t *testing.T) {
	t.Parallel()

	t.Run("no events", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()

		it := db.collectDeviceRegistrationEvents(t.Context())
		events := helperCollectAllEvents(t, it)

		require.Empty(t, events, "Have no events")
	})

	t.Run("with events", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()

		const eventsCount = 3
		baseTimestamp := time.Now().Unix()
		expectedEvents := helperCreateBatchEvents(t, db, 0, eventsCount, baseTimestamp)

		it := db.collectDeviceRegistrationEvents(t.Context())
		events := helperCollectAllEvents(t, it)

		require.Len(t, events, eventsCount, "Have all created events")

		eventMap := make(map[string]*model.Event)
		for _, event := range events {
			eventMap[event.ID] = event
		}

		for _, expected := range expectedEvents {
			actual, exists := eventMap[expected.ID]
			require.True(t, exists, "Event with ID %s must be in results", expected.ID)
			require.Equal(t, expected.ID, actual.ID)
			require.Equal(t, expected.PubKey, actual.PubKey)
			require.Equal(t, expected.Kind, actual.Kind)
			require.Equal(t, expected.Tags.GetD(), actual.Tags.GetD())
		}
	})

	t.Run("pagination", func(t *testing.T) {
		const (
			batchSize        = 1000
			totalEventsCount = 2500
			batchInsertSize  = 100
		)

		t.Logf("BatchSize in code = %d, check on %d events", batchSize, totalEventsCount)

		db := helperNewDatabase(t)
		defer db.Close()

		baseTimestamp := time.Now().Unix()

		t.Run("create_events", func(t *testing.T) {
			t.Logf("Create %d events for pagination test", totalEventsCount)

			for i := 0; i < totalEventsCount; i += batchInsertSize {
				endIndex := helperMinInt(t, i+batchInsertSize, totalEventsCount)
				count := endIndex - i

				helperCreateBatchEvents(t, db, i, count, baseTimestamp)
				t.Logf("Created %d events from %d", endIndex, totalEventsCount)
			}
			t.Logf("All %d events created", totalEventsCount)
		})

		t.Run("verify_events_in_db", func(t *testing.T) {
			count := helperCountEvents(t, db, model.CustomIONKindDeviceRegistration)
			require.Equal(t, totalEventsCount, count, "Events count in DB must be equal to expected")
		})

		t.Run("collect_and_sort", func(t *testing.T) {
			it := db.collectDeviceRegistrationEvents(t.Context())
			var events []*model.Event
			var lastCreatedAt time.Time
			var lastID string

			for event, err := range it {
				require.NoError(t, err)
				events = append(events, event)

				currentCreatedAt := time.Unix(int64(event.CreatedAt), 0)
				if !lastCreatedAt.IsZero() {
					if currentCreatedAt.Equal(lastCreatedAt) {
						require.True(t, event.ID > lastID, "Events with the same time must be sorted by ID")
					} else {
						require.True(t, currentCreatedAt.After(lastCreatedAt), "Events must be sorted by creation time")
					}
				}

				lastCreatedAt = currentCreatedAt
				lastID = event.ID
			}

			require.Len(t, events, totalEventsCount, "Have all created events")
		})

		t.Run("batch_loading", func(t *testing.T) {
			it := db.collectDeviceRegistrationEvents(t.Context())
			batchSizes, events := helperCollectBatchSizes(t, it, batchSize)

			require.Greater(t, len(batchSizes), 1, "Have several batches")
			t.Logf("Events collected in %d batches: %v", len(batchSizes), batchSizes)
			require.Len(t, events, totalEventsCount, "Have all created events")
		})

		t.Run("unique_and_complete", func(t *testing.T) {
			it := db.collectDeviceRegistrationEvents(t.Context())
			events := helperCollectAllEvents(t, it)

			eventIDs := make(map[string]bool)
			for _, event := range events {
				require.False(t, eventIDs[event.ID], "Event ID must be unique")
				eventIDs[event.ID] = true

				require.Equal(t, model.CustomIONKindDeviceRegistration, event.Kind,
					"Event must have DeviceRegistration type")
			}
			require.Equal(t, totalEventsCount, len(eventIDs), "Have all created events without duplicates")
			t.Logf("Found %d unique events from expected %d", len(eventIDs), totalEventsCount)
		})

		t.Logf("Pagination test successfully completed!")
	})
}

func TestMarkTokenAsInvalidInEventTags(t *testing.T) {
	t.Parallel()

	t.Run("mark_single_token_as_invalid", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()

		event := helperCreateDeviceRegistrationEvent(t, "pubkey1", "device1", "token_value1")
		require.NoError(t, db.AcceptEvents(t.Context(), event))

		helperAddTokenTag(t, db, event.ID, "token_value1")
		helperCheckTokenStatus(t, db, event.ID, "token_value1")

		err := db.markTokenAsInvalidInEventTags(t.Context(), []*model.Event{event})
		require.NoError(t, err)

		helperCheckTokenStatus(t, db, event.ID, "invalid")

		it := db.collectDeviceRegistrationEvents(t.Context())
		events := helperCollectAllEvents(t, it)
		require.Empty(t, events, "Events with invalid tokens must not be returned")
	})

	t.Run("mark_multiple_tokens_as_invalid", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()

		const eventsCount = 5
		baseTimestamp := time.Now().Unix()
		events := helperCreateBatchEvents(t, db, 0, eventsCount, baseTimestamp)

		it1 := db.collectDeviceRegistrationEvents(t.Context())
		initialEvents := helperCollectAllEvents(t, it1)
		require.Len(t, initialEvents, eventsCount, "Have all created events")

		var eventsToMark []*model.Event
		for i, event := range events {
			if i%2 == 1 {
				eventsToMark = append(eventsToMark, event)
			}
		}
		err := db.markTokenAsInvalidInEventTags(t.Context(), eventsToMark)
		require.NoError(t, err)

		for i, event := range events {
			expectedValue := "token_value" + strconv.Itoa(i)
			if i%2 == 1 {
				expectedValue = "invalid"
			}
			helperCheckTokenStatus(t, db, event.ID, expectedValue)
		}

		it2 := db.collectDeviceRegistrationEvents(t.Context())
		filteredEvents := helperCollectAllEvents(t, it2)
		require.Len(t, filteredEvents, (eventsCount+1)/2, "Have only events with valid tokens")
	})

	t.Run("empty_events_list", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()

		err := db.markTokenAsInvalidInEventTags(t.Context(), []*model.Event{})
		require.NoError(t, err, "Empty list must be processed without errors")
	})

	t.Run("events_without_token_tag", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()

		event := &model.Event{
			Event: nostr.Event{
				ID:        "event" + uuid.NewString(),
				PubKey:    "pubkey1",
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.CustomIONKindDeviceRegistration,
				Tags: model.Tags{
					{"d", "device1"},
					{"t", "android"},
				},
				Content: `{"kinds":[1]}`,
			},
		}
		require.NoError(t, db.AcceptEvents(t.Context(), event))

		count := helperCountEvents(t, db, model.CustomIONKindDeviceRegistration)
		require.Equal(t, 1, count, "Event must be saved")

		err := db.markTokenAsInvalidInEventTags(t.Context(), []*model.Event{event})
		require.Error(t, err, "There must be an error when updating non-existent tokens")
	})

	t.Run("interaction_with_collectDeviceRegistrationEvents", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()

		validEvent := helperCreateDeviceRegistrationEvent(t, "pubkey1", "valid_device", "valid_token")
		invalidEvent := helperCreateDeviceRegistrationEvent(t, "pubkey2", "invalid_device", "invalid_token")
		require.NoError(t, db.AcceptEvents(t.Context(), validEvent, invalidEvent))

		helperAddTokenTag(t, db, validEvent.ID, "valid_token")
		helperAddTokenTag(t, db, invalidEvent.ID, "invalid_token")

		it1 := db.collectDeviceRegistrationEvents(t.Context())
		initialEvents := helperCollectAllEvents(t, it1)
		require.Len(t, initialEvents, 2, "Have both events")

		err := db.markTokenAsInvalidInEventTags(t.Context(), []*model.Event{invalidEvent})
		require.NoError(t, err)

		it2 := db.collectDeviceRegistrationEvents(t.Context())
		filteredEvents := helperCollectAllEvents(t, it2)
		require.Len(t, filteredEvents, 1, "Have only valid event")
		require.Equal(t, validEvent.ID, filteredEvents[0].ID, "Have valid event")
	})
}

func helperCreateDeviceRegistrationEvent(t *testing.T, pubKey, deviceID, tokenValue string) *model.Event {
	t.Helper()
	return &model.Event{
		Event: nostr.Event{
			ID:        "devreg-" + deviceID + "-" + uuid.NewString(),
			PubKey:    pubKey,
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.CustomIONKindDeviceRegistration,
			Tags: model.Tags{
				{"d", deviceID},
				{"token", tokenValue},
				{"t", "android"},
			},
			Content: `{"kinds":[1]}`,
		},
	}
}

func helperAddTokenTag(t *testing.T, db *dbClient, eventID, tokenValue string) {
	t.Helper()
	_, err := db.DB.ExecContext(t.Context(), `
		INSERT INTO event_tags (
			event_id, 
			event_tag_key, 
			event_tag_value1, 
			event_tag_value2
		) VALUES ($1, $2, $3, $4)
		ON CONFLICT (event_id, event_tag_key, event_tag_value1) DO NOTHING
	`, eventID, "token", tokenValue, tokenValue)
	require.NoError(t, err)
}

func helperCheckTokenStatus(t *testing.T, db *dbClient, eventID, expectedValue string) {
	t.Helper()
	var tokenValue string
	err := db.DB.QueryRowContext(t.Context(), `
		SELECT event_tag_value2
			FROM event_tags
		WHERE event_id = $1 AND event_tag_key = 'token'
	`, eventID).Scan(&tokenValue)
	require.NoError(t, err)
	require.Equal(t, expectedValue, tokenValue, "Token must have value '%s'", expectedValue)
}

func helperCreateBatchEvents(t *testing.T, db *dbClient, startIdx, count int, baseTimestamp int64) []*model.Event {
	events := make([]*model.Event, 0, count)

	for i := startIdx; i < startIdx+count; i++ {
		pubKey := "pubkey" + strconv.Itoa(i%10)
		deviceID := "device" + strconv.Itoa(i)
		tokenValue := "token_value" + strconv.Itoa(i)

		event := &model.Event{
			Event: nostr.Event{
				ID:        "devreg" + strconv.Itoa(i) + uuid.NewString(),
				PubKey:    pubKey,
				CreatedAt: nostr.Timestamp(baseTimestamp + int64(i)),
				Kind:      model.CustomIONKindDeviceRegistration,
				Tags: model.Tags{
					{"d", deviceID},
					{"token", tokenValue},
					{"t", "android"},
				},
				Content: `{"kinds":[1]}`,
			},
		}
		events = append(events, event)
	}

	require.NoError(t, db.AcceptEvents(t.Context(), events...))

	for _, event := range events {
		helperAddTokenTag(t, db, event.ID, event.Tags.GetFirst([]string{"token"}).Value())
	}

	return events
}

func helperCountEvents(t *testing.T, db *dbClient, kind int) int {
	t.Helper()
	var count int
	err := db.DB.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM events WHERE kind = $1`, kind).Scan(&count)
	require.NoError(t, err)

	return count
}

func helperCollectAllEvents(t *testing.T, it EventIterator) []*model.Event {
	t.Helper()
	var events []*model.Event
	for event, err := range it {
		require.NoError(t, err)
		events = append(events, event)
	}

	return events
}

func helperCollectBatchSizes(t *testing.T, it EventIterator, batchSize int) ([]int, []*model.Event) {
	t.Helper()
	var events []*model.Event
	var batchSizes []int
	var currentBatchSize int
	var collectedCount int

	for event, err := range it {
		require.NoError(t, err)
		events = append(events, event)
		currentBatchSize++
		collectedCount++

		if collectedCount%100 == 0 {
			t.Logf("Collected %d events", collectedCount)
		}

		if currentBatchSize >= batchSize {
			batchSizes = append(batchSizes, currentBatchSize)
			currentBatchSize = 0
		}
	}
	if currentBatchSize > 0 {
		batchSizes = append(batchSizes, currentBatchSize)
	}

	return batchSizes, events
}

func helperMinInt(t *testing.T, a, b int) int {
	t.Helper()
	if a < b {
		return a
	}

	return b
}
