// SPDX-License-Identifier: ice License 1.0

package query

import (
	"crypto/rand"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/database/query/internal/connector"
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

	t.Run("relay url with port and without port", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()

		const eventsCount = 3
		var relayURLs = []string{
			db.relayURL + ":443",
			db.relayURL + ":4443",
			db.relayURL,
		}

		var events []*model.Event
		for range eventsCount {
			for _, relayURL := range relayURLs {
				deviceID := "device_" + rand.Text()
				tokenValue := "token_value_of_" + deviceID
				event := helperCreateDeviceRegistrationEventWithRelay(t, "", rand.Text(), tokenValue, relayURL)
				require.NoError(t, event.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

				t.Logf("Inserting event with relay URL: %s: %v", relayURL, event.Address())
				require.NoError(t, db.AcceptEvents(t.Context(), event))
				events = append(events, event)
			}
		}
		require.Len(t, events, eventsCount*len(relayURLs))

		collectedEvents := helperCollectAllEvents(t, db.collectDeviceRegistrationEvents(t.Context()))
		require.Len(t, collectedEvents, len(events))

		require.ElementsMatch(t, events, collectedEvents, "All events must be collected regardless of relay URL format")
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

		t.Run("verify_token_tags_in_db", func(t *testing.T) {
			t.Logf("Verify that token tags are present in event_tags table")

			tokenCount, err := connector.Get[int](t.Context(), db.db, `
				SELECT COUNT(*) FROM event_tags 
				WHERE event_tag_key = 'token'
			`)
			require.NoError(t, err)
			require.NotNil(t, tokenCount)
			require.Equal(t, totalEventsCount, *tokenCount, "All events must have token tags")

			t.Logf("Found %d token tags for %d events", *tokenCount, totalEventsCount)
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

		helperCheckTokenStatus(t, db, event.ID, "")

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
			expectedValue := ""
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

		it1 := db.collectDeviceRegistrationEvents(t.Context())
		initialEvents := helperCollectAllEvents(t, it1)
		require.Len(t, initialEvents, 2, "Have both events")

		err := db.markTokenAsInvalidInEventTags(t.Context(), []*model.Event{invalidEvent})
		require.NoError(t, err)

		helperCheckTokenStatus(t, db, validEvent.ID, "")
		helperCheckTokenStatus(t, db, invalidEvent.ID, "invalid")

		it2 := db.collectDeviceRegistrationEvents(t.Context())
		filteredEvents := helperCollectAllEvents(t, it2)
		require.Len(t, filteredEvents, 1, "Have only valid event")
		require.Equal(t, validEvent.ID, filteredEvents[0].ID, "Have valid event")
	})
}

func helperCreateDeviceRegistrationEventWithRelay(t *testing.T, pubKey, deviceID, tokenValue, relayURL string) *model.Event {
	t.Helper()

	event := &model.Event{}
	event.ID = uuid.New().String()
	event.PubKey = pubKey
	event.CreatedAt = nostr.Now()
	event.Kind = model.CustomIONKindDeviceRegistration
	event.Tags = model.Tags{
		{"d", deviceID},
		{"t", "android"},
		{"relay", relayURL},
	}
	if tokenValue != "" {
		event.Tags = append(event.Tags, model.Tag{"token", tokenValue})
	}

	return event
}

func helperCreateDeviceRegistrationEvent(t *testing.T, pubKey, deviceID, tokenValue string) *model.Event {
	t.Helper()
	return helperCreateDeviceRegistrationEventWithRelay(t, pubKey, deviceID, tokenValue, "wss://localhost")
}

func helperCheckTokenStatus(t *testing.T, db *dbClient, eventID, expectedValue string) {
	t.Helper()

	value, err := connector.Get[string](t.Context(), db.db, `
		SELECT event_tag_value2 FROM event_tags
		WHERE event_id = $1 AND event_tag_key = 'token'
	`, eventID)
	require.NoError(t, err)
	require.NotNil(t, value)
	require.Equal(t, expectedValue, *value)
}

func helperCreateBatchEvents(t *testing.T, db *dbClient, startIdx, count int, baseTimestamp int64) []*model.Event {
	t.Helper()

	events := make([]*model.Event, count)
	for i := 0; i < count; i++ {
		idx := startIdx + i
		pubKey := "pubkey" + strconv.Itoa(idx)
		deviceID := "device" + strconv.Itoa(idx)
		tokenValue := "token_value" + strconv.Itoa(idx)

		event := helperCreateDeviceRegistrationEvent(t, pubKey, deviceID, tokenValue)
		event.CreatedAt = nostr.Timestamp(baseTimestamp + int64(idx))

		require.NoError(t, db.AcceptEvents(t.Context(), event))

		events[i] = event
	}

	return events
}

func helperCountEvents(t *testing.T, client *dbClient, kind int) int {
	t.Helper()

	count, err := connector.Get[int](t.Context(), client.db, `SELECT COUNT(*) FROM events WHERE kind = $1`, kind)
	require.NoError(t, err)
	require.NotNil(t, count)

	return *count
}

func helperCollectAllEvents(t *testing.T, it EventIterator) []*model.Event {
	t.Helper()

	var events []*model.Event
	it(func(event *model.Event, err error) bool {
		require.NoError(t, err)
		if event != nil {
			events = append(events, event)
		}
		return true
	})
	return events
}

func helperCollectBatchSizes(t *testing.T, it EventIterator, batchSize int) ([]int, []*model.Event) {
	t.Helper()

	var events []*model.Event
	var batchSizes []int
	var currentBatchSize int

	it(func(event *model.Event, err error) bool {
		require.NoError(t, err)
		if event != nil {
			events = append(events, event)
			currentBatchSize++

			if currentBatchSize == batchSize {
				batchSizes = append(batchSizes, currentBatchSize)
				currentBatchSize = 0
			}
		}
		return true
	})

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
