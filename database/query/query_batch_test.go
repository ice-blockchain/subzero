// SPDX-License-Identifier: ice License 1.0

package query

import (
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

	pk := model.GeneratePrivateKey()
	require.NotEmpty(t, pk)

	var req databaseBatchRequest
	mockHash := "hash"
	t.Run("Insert", func(t *testing.T) {
		const num = int64(10)
		for i := range num {
			var ev model.Event

			ev.CreatedAt = model.Timestamp(i)
			ev.Content = "content" + strconv.FormatInt(i, 10)
			ev.Kind = nostr.KindTextNote
			require.NoError(t, ev.SignWithAlg(pk, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.NoError(t, req.Save(&ev))
			req.EventsHash = &mockHash
		}

		require.NoError(t, db.executeBatch(t.Context(), &req))
		count, err := db.CountEvents(t.Context())
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
		require.NoError(t, ev.SignWithAlg(pk, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, req.Remove(&ev))

		req.InsertOrReplace = nil
		req.EventsHash = &mockHash
		require.NoError(t, db.executeBatch(t.Context(), &req))

		count, err := db.CountEvents(t.Context())
		require.NoError(t, err)
		require.Zero(t, count)
	})
	var hashableEvents []*model.Event
	t.Run("Combine", func(t *testing.T) {
		req.InsertOrReplace = nil
		req.Delete = nil
		var events []*model.Event
		const num = int64(10)
		for i := range num {
			var ev model.Event

			ev.CreatedAt = model.Timestamp(i)
			ev.Content = "content" + strconv.FormatInt(i, 10)
			ev.Kind = nostr.KindTextNote
			require.NoError(t, ev.SignWithAlg(pk, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.NoError(t, req.Save(&ev))
			events = append(events, &ev)
		}

		var del model.Event
		del.Kind = nostr.KindDeletion
		del.CreatedAt = 1
		for i := range 3 {
			del.Tags = append(del.Tags, model.Tag{"e", req.InsertOrReplace[i].ID})
		}
		require.NoError(t, del.SignWithAlg(pk, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, req.Remove(&del))
		hashableEvents = append(events[3:], &del)
		hash := hashEvents(hashableEvents...)
		req.EventsHash = &hash
		require.NoError(t, db.executeBatch(t.Context(), &req))

		count, err := db.CountEvents(t.Context())
		require.NoError(t, err)
		require.Equal(t, num-3, count)
	})
	t.Run("Rollback", func(t *testing.T) {
		require.NoError(t, db.RollbackEvents(t.Context(), hashableEvents...))
		count, err := db.CountEvents(t.Context())
		require.NoError(t, err)
		require.Equal(t, int64(3), count)
	})
}
