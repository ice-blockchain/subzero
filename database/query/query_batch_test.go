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

func TestQueryBatchProcessor(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	pk := nostr.GeneratePrivateKey()
	require.NotEmpty(t, pk)

	var req databaseBatchRequest
	t.Run("Insert", func(t *testing.T) {
		const num = int64(10)
		for i := range num {
			var ev model.Event

			ev.CreatedAt = model.Timestamp(i)
			ev.Content = "content" + strconv.FormatInt(i, 10)
			ev.Kind = nostr.KindTextNote
			require.NoError(t, ev.Sign(pk))
			require.NoError(t, req.Save(&ev))
		}

		require.NoError(t, db.executeBatch(context.Background(), &req))
		count, err := db.CountEvents(context.Background(), nil)
		require.NoError(t, err)
		require.Equal(t, num, count)
	})
	t.Run("Delete", func(t *testing.T) {
		var ev model.Event

		ev.Kind = nostr.KindDeletion
		ev.CreatedAt = 1
		for i := range req.InsertOrReplace {
			ev.Tags = append(ev.Tags, model.Tag{"e", req.InsertOrReplace[i].ID})
		}
		require.NoError(t, ev.Sign(pk))
		require.NoError(t, req.Remove(&ev))

		req.InsertOrReplace = nil
		require.NoError(t, db.executeBatch(context.Background(), &req))

		count, err := db.CountEvents(context.Background(), nil)
		require.NoError(t, err)
		require.Zero(t, count)
	})
	t.Run("Combine", func(t *testing.T) {
		req.InsertOrReplace = nil
		req.Delete = nil

		const num = int64(10)
		for i := range num {
			var ev model.Event

			ev.CreatedAt = model.Timestamp(i)
			ev.Content = "content" + strconv.FormatInt(i, 10)
			ev.Kind = nostr.KindTextNote
			require.NoError(t, ev.Sign(pk))
			require.NoError(t, req.Save(&ev))
		}

		var del model.Event
		del.Kind = nostr.KindDeletion
		del.CreatedAt = 1
		for i := range 3 {
			del.Tags = append(del.Tags, model.Tag{"e", req.InsertOrReplace[i].ID})
		}

		require.NoError(t, del.Sign(pk))
		require.NoError(t, req.Remove(&del))

		require.NoError(t, db.executeBatch(context.Background(), &req))

		count, err := db.CountEvents(context.Background(), nil)
		require.NoError(t, err)
		require.Equal(t, num-3, count)
	})
}
