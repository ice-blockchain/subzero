// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"strconv"
	"testing"

	"github.com/nbd-wtf/go-nostr"
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

func TestIteratorScanTagsWithGaps(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	tags := model.Tags{{"a", "", "b", "c"}, {"1", "2", "3", "", "", "4"}}
	key := nostr.GeneratePrivateKey()

	t.Run("Save", func(t *testing.T) {
		var ev model.Event

		ev.Kind = nostr.KindTextNote
		ev.CreatedAt = 1
		ev.Content = "content"
		ev.Tags = tags
		require.NoError(t, ev.Sign(key))
		require.NoError(t, db.AcceptEvent(context.Background(), &ev))
	})
	t.Run("Select", func(t *testing.T) {
		events, err := helperGetStoredEventsAll(t, db, context.TODO(), helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Kinds = []int{nostr.KindTextNote}
		}))
		require.NoError(t, err)
		require.Len(t, events, 1)

		ev := events[0]
		require.NotNil(t, ev)
		require.Equal(t, tags, ev.Tags)

		ok, err := ev.CheckSignature()
		require.NoError(t, err)
		require.True(t, ok)
	})
}
