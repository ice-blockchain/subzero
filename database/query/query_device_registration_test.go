// SPDX-License-Identifier: ice License 1.0

package query

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func TestCollectDeviceRegistrationEvents(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	t.Run("Prepare different types of events", func(t *testing.T) {
		nonDeviceEvents := []*model.Event{
			{
				Event: nostr.Event{
					ID:        "regular-event-1-" + uuid.NewString(),
					PubKey:    "pubkey-1",
					CreatedAt: nostr.Timestamp(time.Now().Unix()),
					Kind:      nostr.KindTextNote,
					Tags:      model.Tags{},
					Content:   "test content",
				},
			},
			{
				Event: nostr.Event{
					ID:        "regular-event-2-" + uuid.NewString(),
					PubKey:    "pubkey-2",
					CreatedAt: nostr.Timestamp(time.Now().Unix() + 1),
					Kind:      nostr.KindProfileMetadata,
					Tags:      model.Tags{},
					Content:   "test metadata",
				},
			},
		}
		for _, event := range nonDeviceEvents {
			require.NoError(t, db.AcceptEvents(t.Context(), event))
		}
	})

	const batchSize = 1000
	const regularEvents = batchSize - 50
	const extraEvents = 50
	deviceEvents := make([]*model.Event, 0, regularEvents+extraEvents)

	baseTimestamp := time.Now().Unix()
	var boundaryTimestamp nostr.Timestamp
	var sameTimestamp nostr.Timestamp
	var boundaryEvents []*model.Event
	var sameTimestampEvents []*model.Event

	t.Run("Create main device registration events", func(t *testing.T) {
		for i := 0; i < regularEvents; i++ {
			event := &model.Event{
				Event: nostr.Event{
					ID:        fmt.Sprintf("device-event-%04d-%s", i, uuid.NewString()),
					PubKey:    fmt.Sprintf("device-pubkey-%04d", i%10),
					CreatedAt: nostr.Timestamp(baseTimestamp + int64(i)),
					Kind:      model.CustomIONKindDeviceRegistration,
					Tags: model.Tags{
						{"d", fmt.Sprintf("device-id-%04d", i)},
						{"t", "android"},
						{"relay", "wss://relay.example.com"},
						{"token", fmt.Sprintf("device-token-%04d", i)},
					},
					Content: `[{"kinds":[1]}]`,
				},
			}
			deviceEvents = append(deviceEvents, event)
		}
	})

	t.Run("Create events with the same time on the boundary of the batch", func(t *testing.T) {
		const boundaryTimestampCount = 60
		boundaryTimestamp = nostr.Timestamp(baseTimestamp + int64(regularEvents))
		boundaryEvents = make([]*model.Event, 0, boundaryTimestampCount)

		for i := 0; i < boundaryTimestampCount; i++ {
			event := &model.Event{
				Event: nostr.Event{
					ID:        fmt.Sprintf("boundary-event-%03d-%s", i, uuid.NewString()),
					PubKey:    fmt.Sprintf("boundary-pubkey-%02d", i%10),
					CreatedAt: boundaryTimestamp,
					Kind:      model.CustomIONKindDeviceRegistration,
					Tags: model.Tags{
						{"d", fmt.Sprintf("boundary-device-%03d", i)},
						{"t", "android"},
						{"relay", "wss://relay.example.com"},
						{"token", fmt.Sprintf("boundary-token-%03d", i)},
					},
					Content: `[{"kinds":[1]}]`,
				},
			}
			boundaryEvents = append(boundaryEvents, event)
		}

		for _, event := range boundaryEvents {
			require.NoError(t, db.AcceptEvents(t.Context(), event))
		}
	})

	t.Run("Create a group of events with the same time", func(t *testing.T) {
		const sameTimestampCount = 30
		sameTimestamp = nostr.Timestamp(time.Now().Add(2 * time.Hour).Unix())
		sameTimestampEvents = make([]*model.Event, 0, sameTimestampCount)

		for i := 0; i < sameTimestampCount; i++ {
			event := &model.Event{
				Event: nostr.Event{
					ID:        fmt.Sprintf("same-time-event-%02d-%s", i, uuid.NewString()),
					PubKey:    fmt.Sprintf("same-time-pubkey-%02d", i%5),
					CreatedAt: sameTimestamp,
					Kind:      model.CustomIONKindDeviceRegistration,
					Tags: model.Tags{
						{"d", fmt.Sprintf("same-time-device-%02d", i)},
						{"t", "android"},
						{"relay", "wss://relay.example.com"},
						{"token", fmt.Sprintf("same-time-token-%02d", i)},
					},
					Content: `[{"kinds":[1]}]`,
				},
			}
			sameTimestampEvents = append(sameTimestampEvents, event)
		}

		for _, event := range sameTimestampEvents {
			require.NoError(t, db.AcceptEvents(t.Context(), event))
		}
	})

	t.Run("Create events after the boundary of the batch", func(t *testing.T) {
		for i := 0; i < extraEvents; i++ {
			idx := regularEvents + i
			event := &model.Event{
				Event: nostr.Event{
					ID:        fmt.Sprintf("device-event-%04d-%s", idx, uuid.NewString()),
					PubKey:    fmt.Sprintf("device-pubkey-%04d", idx%10),
					CreatedAt: nostr.Timestamp(baseTimestamp + int64(regularEvents) + 100 + int64(i)),
					Kind:      model.CustomIONKindDeviceRegistration,
					Tags: model.Tags{
						{"d", fmt.Sprintf("device-id-%04d", idx)},
						{"t", "android"},
						{"relay", "wss://relay.example.com"},
						{"token", fmt.Sprintf("device-token-%04d", idx)},
					},
					Content: `[{"kinds":[1]}]`,
				},
			}
			deviceEvents = append(deviceEvents, event)
		}

		for _, event := range deviceEvents {
			require.NoError(t, db.AcceptEvents(t.Context(), event))
		}
	})

	var collected []*model.Event

	t.Run("Collect all device registration events", func(t *testing.T) {
		var err error
		collected, err = db.collectDeviceRegistrationEvents(t.Context())
		require.NoError(t, err)

		expectedTotal := regularEvents + extraEvents + len(boundaryEvents) + len(sameTimestampEvents)
		require.Equal(t, expectedTotal, len(collected), "Should receive all device registration events")

		for _, event := range collected {
			require.Equal(t, model.CustomIONKindDeviceRegistration, event.Kind,
				"All received events should have kind = CustomIONKindDeviceRegistration")
		}
	})

	t.Run("Check sorting of events", func(t *testing.T) {
		for i := 1; i < len(collected); i++ {
			prevEvent := collected[i-1]
			currentEvent := collected[i]

			if prevEvent.CreatedAt == currentEvent.CreatedAt {
				require.True(t, prevEvent.ID <= currentEvent.ID,
					"Events with the same time should be sorted by ID")
			} else {
				require.True(t, prevEvent.CreatedAt <= currentEvent.CreatedAt,
					"Events should be sorted by creation time")
			}
		}
	})

	t.Run("Check for duplicates", func(t *testing.T) {
		eventIDsSet := make(map[string]bool)
		for _, event := range collected {
			require.False(t, eventIDsSet[event.ID], "Duplicate event found with ID: %s", event.ID)
			eventIDsSet[event.ID] = true
		}
	})

	t.Run("Check completeness of event collection", func(t *testing.T) {
		eventsMap := make(map[string]*model.Event, len(deviceEvents)+len(sameTimestampEvents)+len(boundaryEvents))
		for _, event := range deviceEvents {
			eventsMap[event.ID] = event
		}
		for _, event := range sameTimestampEvents {
			eventsMap[event.ID] = event
		}
		for _, event := range boundaryEvents {
			eventsMap[event.ID] = event
		}

		for _, event := range collected {
			originalEvent, exists := eventsMap[event.ID]
			require.True(t, exists, "Received event with ID %s not found in original data", event.ID)
			require.Equal(t, originalEvent.ID, event.ID, "Event ID does not match")
			require.Equal(t, originalEvent.PubKey, event.PubKey, "PubKey event does not match")
			require.Equal(t, originalEvent.CreatedAt, event.CreatedAt, "CreatedAt event does not match")
			require.Equal(t, originalEvent.Kind, event.Kind, "Kind event does not match")
		}
	})

	t.Run("Check sorting of events with the same time", func(t *testing.T) {
		var sameTimestampCollected []*model.Event
		for _, event := range collected {
			if event.CreatedAt == sameTimestamp {
				sameTimestampCollected = append(sameTimestampCollected, event)
			}
		}

		require.Equal(t, len(sameTimestampEvents), len(sameTimestampCollected),
			"Should receive all events with the same time")

		for i := 1; i < len(sameTimestampCollected); i++ {
			prevEvent := sameTimestampCollected[i-1]
			currentEvent := sameTimestampCollected[i]

			require.True(t, prevEvent.ID <= currentEvent.ID,
				"Events with the same time should be sorted by ID")
		}
	})

	t.Run("Check correctness of event collection on the boundary of the batch", func(t *testing.T) {
		var boundaryCollected []*model.Event
		boundaryEventIDs := make(map[string]bool)

		for _, event := range collected {
			if event.CreatedAt == boundaryTimestamp {
				boundaryCollected = append(boundaryCollected, event)
				boundaryEventIDs[event.ID] = true
			}
		}

		require.Equal(t, len(boundaryEvents), len(boundaryCollected),
			"All boundary events should be collected without duplicates")

		for _, event := range boundaryEvents {
			require.True(t, boundaryEventIDs[event.ID],
				"Boundary event with ID %s should be in the collected events", event.ID)
		}

		for i := 1; i < len(boundaryCollected); i++ {
			prevEvent := boundaryCollected[i-1]
			currentEvent := boundaryCollected[i]

			require.True(t, prevEvent.ID <= currentEvent.ID,
				"Events on the boundary of the batch should be sorted by ID")
		}
	})
}
