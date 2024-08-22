package query

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func helperSelectEventsN(t *testing.T, db *dbClient, limit int) (events map[string]*model.Event) {
	t.Helper()

	ctx := context.Background()
	iter := db.SelectEvents(ctx, &model.Subscription{Filters: []model.Filter{{Limit: limit}}})

	events = make(map[string]*model.Event, limit)
	for ev, err := range iter {
		require.NoError(t, err)
		events[ev.ID] = ev
	}

	return events
}

func TestIteratorSelectEvents(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	helperFillDatabase(t, db, 3000)

	t.Run("Fetch_100", func(t *testing.T) {
		events := helperSelectEventsN(t, db, 100)
		t.Logf("fetched %d event(s)", len(events))
		require.Len(t, events, 100)
	})
	t.Run("Fetch_1001", func(t *testing.T) {
		events := helperSelectEventsN(t, db, 1001)
		t.Logf("fetched %d event(s)", len(events))
		require.Len(t, events, 1001)
	})
	t.Run("Fetch_2000", func(t *testing.T) {
		events := helperSelectEventsN(t, db, 2000)
		t.Logf("fetched %d event(s)", len(events))
		require.Len(t, events, 2000)
	})
	t.Run("Fetch_All", func(t *testing.T) {
		events := helperSelectEventsN(t, db, 0)
		t.Logf("fetched %d event(s)", len(events))
		require.Len(t, events, 3000)
	})

	require.NoError(t, db.Close())
}
