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

func helperSelectEvents(t *testing.T, db *dbClient, filters ...model.Filter) (events []*model.Event) {
	t.Helper()

	for ev, err := range db.SelectEvents(context.Background(), &model.Subscription{Filters: filters}) {
		require.NoError(t, err)
		require.NotNil(t, ev)
		events = append(events, ev)
	}

	return events
}

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
	key := model.GeneratePrivateKey()

	t.Run("Save", func(t *testing.T) {
		var ev model.Event

		ev.Kind = nostr.KindTextNote
		ev.CreatedAt = 1
		ev.Content = "content"
		ev.Tags = tags
		require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, db.AcceptEvents(context.Background(), &ev))
	})
	t.Run("Select", func(t *testing.T) {
		events := helperSelectEvents(t, db, model.Filter{Kinds: []int{nostr.KindTextNote}})
		require.Len(t, events, 1)

		ev := events[0]
		require.NotNil(t, ev)
		require.Equal(t, tags, ev.Tags)

		ok, err := ev.CheckSignature()
		require.NoError(t, err)
		require.True(t, ok)
	})
}
