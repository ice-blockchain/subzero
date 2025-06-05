// SPDX-License-Identifier: ice License 1.0

package query

import (
	"cmp"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func helperSignAndSaveEvent(t *testing.T, db *dbClient, key string, events ...*model.Event) {
	t.Helper()
	for _, ev := range events {
		pk := cmp.Or(key, model.GeneratePrivateKey())
		require.NoError(t, ev.SignWithAlg(pk, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	}
	require.NoError(t, db.AcceptEvents(t.Context(), events...))
}

func TestLookupTagsT(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	var ev1, ev2, ev3 model.Event

	t.Run("Create events with t tags", func(t *testing.T) {
		ev1.Kind, ev2.Kind, ev3.Kind = nostr.KindTextNote, nostr.KindArticle, model.CustomIONKindEditableTextNote
		ev1.CreatedAt, ev2.CreatedAt, ev3.CreatedAt = nostr.Now().Add(time.Second), nostr.Now().Add(-time.Second), nostr.Now().Add(-time.Hour)
		ev1.Content, ev2.Content, ev3.Content = "text note", "article content", "editable text note"

		ev1.Tags = model.Tags{
			{"t", "cars"},
			{"t", "show"},
			{"t", "acura"},
			{"t", "music"},
		}

		ev2.Tags = model.Tags{
			{"t", "music"},
			{"t", "dance"},
			{"t", "festival"},
		}

		ev3.Tags = model.Tags{
			{"t", "dance"},
			{"t", "party"},
			{"t", "newyork"},
		}

		helperSignAndSaveEvent(t, db, "", &ev1, &ev2, &ev3)
	})

	t.Run("Common tag", func(t *testing.T) {
		events := helperSelectEvents(t, db, model.Filter{
			Tags: model.TagMap{}.SetLiterals("t", "music"),
		})
		require.ElementsMatch(t, []*model.Event{&ev1, &ev2}, events)
	})
	t.Run("Common tags", func(t *testing.T) {
		events := helperSelectEvents(t, db, model.Filter{
			Tags: model.TagMap{}.SetLiterals("t", "music", "cars"),
		})
		require.ElementsMatch(t, []*model.Event{&ev1, &ev2}, events)
	})
	t.Run("Common tag with negative", func(t *testing.T) {
		events := helperSelectEvents(t, db, model.Filter{
			Tags: model.TagMap{}.
				SetLiterals("t", "dance").
				SetLiterals("!t", "newyork"),
		})
		require.ElementsMatch(t, []*model.Event{&ev2}, events)
	})
}
