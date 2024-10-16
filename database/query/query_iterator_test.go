// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func helperSelectEventsN(t *testing.T, db *dbClient, limit int) (events map[string]*model.Event) {
	t.Helper()

	ctx := context.Background()
	iter := db.SelectEvents(ctx, helperNewFilterSubscription(func(apply *model.Filter) {
		apply.Limit = limit
	}))

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
	helperFillDatabase(t, db, 300)

	t.Run("Limit", func(t *testing.T) {
		for _, limit := range []int{1, 10, 15, 100, 125, selectDefaultBatchLimit + 1, 200, 222, 300} {
			t.Run(strconv.Itoa(limit), func(t *testing.T) {
				events := helperSelectEventsN(t, db, limit)
				t.Logf("fetched %d event(s)", len(events))
				require.Len(t, events, limit)
			})
		}
	})
	t.Run("All", func(t *testing.T) {
		events := helperSelectEventsN(t, db, 0)
		t.Logf("fetched %d event(s)", len(events))
		require.Len(t, events, 300)
	})

	require.NoError(t, db.Close())
}
