package query

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"pgregory.net/rand"

	"github.com/ice-blockchain/subzero/model"
)

func TestQueryEventsCount(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	const totalEvents = int64(4242)
	authors := make(map[string]int64)
	ids := make(map[string]model.Event, totalEvents)
	events := make([]model.Event, 0, totalEvents)

	t.Run("Generate", func(t *testing.T) {
		for range totalEvents {
			event := helperGenerateEvent(t, db, true)
			authors[event.PubKey]++
			ids[event.ID] = event
			events = append(events, event)
		}
	})
	t.Run("CountAll", func(t *testing.T) {
		count, err := db.CountEvents(context.TODO(), nil)
		require.NoError(t, err)
		require.Equal(t, totalEvents, count)
	})
	t.Run("CountByAuthor", func(t *testing.T) {
		for author, expectedCount := range authors {
			count, err := db.CountEvents(context.TODO(),
				helperNewFilterSubscription(func(apply *model.Filter) {
					apply.Authors = []string{author}
				}))
			require.NoError(t, err)
			require.Equal(t, expectedCount, count)
		}
	})
	t.Run("RandomTag", func(t *testing.T) {
		for range 10 {
			ev := events[rand.Int31n(int32(len(events)))]
			count, err := db.CountEvents(context.TODO(),
				helperNewFilterSubscription(func(apply *model.Filter) {
					apply.Tags = model.TagMap{ev.Tags[0][0]: ev.Tags[0][1:]}
				}))
			require.NoError(t, err)
			require.Equal(t, int64(1), count)
		}
	})
	t.Run("EventsOR", func(t *testing.T) {
		ev1 := events[rand.Int31n(int32(len(events)))]
		ev2 := events[rand.Int31n(int32(len(events)))]
		ev3 := events[rand.Int31n(int32(len(events)))]
		count, err := db.CountEvents(context.TODO(),
			&model.Subscription{
				Filters: model.Filters{
					helperNewFilter(func(apply *model.Filter) {
						apply.IDs = []string{ev1.ID}
					}),
					helperNewFilter(func(apply *model.Filter) {
						apply.Authors = []string{ev2.PubKey}
					}),
					helperNewFilter(func(apply *model.Filter) {
						apply.Tags = model.TagMap{ev3.Tags[0][0]: ev3.Tags[0][1:]}
					}),
				},
			},
		)
		require.NoError(t, err)
		require.Equal(t, int64(3), count)
	})

	require.NoError(t, db.Close())
}
