// SPDX-License-Identifier: ice License 1.0

package query

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func TestSearchEvents_KindTextNote(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	expectedEvents := []*model.Event{}
	searchPubkey := "bogusssss" + uuid.NewString()
	searchID := "normal, 3nd event" + uuid.NewString()
	t.Run("Create events with text note kind with imeta alt and summary tags", func(t *testing.T) {
		var tags1 nostr.Tags
		tags1 = append(tags1, nostr.Tag{
			"imeta",
			"url https://alicerelay.example.com",
			"m image/jpg",
			"dim 3024x4032",
			"i foobar",
			"alt alt1 text",
			"summary dummy summary1 content",
			fmt.Sprintf("x %x", []byte("https://alicerelay.example.com")),
			fmt.Sprintf("ox %x", []byte("https://alicerelay.example.com")),
		})
		now := nostr.Now()
		expectedEvents = append(expectedEvents, &model.Event{
			Event: nostr.Event{
				ID:        "normal" + uuid.NewString(),
				PubKey:    "end" + uuid.NewString(),
				CreatedAt: now,
				Kind:      nostr.KindTextNote,
				Tags:      tags1,
				Content:   "end, and, ond",
				Sig:       "end" + uuid.NewString(),
			},
		})
		require.NoError(t, db.AcceptEvents(t.Context(), expectedEvents[0]))

		expectedEvents = append(expectedEvents, &model.Event{
			Event: nostr.Event{
				ID:        "normal, 2nd event" + uuid.NewString(),
				PubKey:    "bogus" + uuid.NewString(),
				CreatedAt: now + 1,
				Kind:      nostr.KindTextNote,
				Tags:      model.Tags{},
				Content:   "post",
				Sig:       "bogus" + uuid.NewString(),
			},
		})
		require.NoError(t, db.AcceptEvents(t.Context(), expectedEvents[1]))

		var tags2 model.Tags
		tags2 = append(tags2, nostr.Tag{
			"imeta",
			"url https://alicerelay.example.com",
			"m image/jpg",
			"dim 3024x4032",
			"i foobar",
			"alt alt2 text",
			"summary dummy summary2 content",
			fmt.Sprintf("x %x", []byte("https://alicerelay.example.com")),
			fmt.Sprintf("ox %x", []byte("https://alicerelay.example.com")),
		})
		expectedEvents = append(expectedEvents, &model.Event{
			Event: nostr.Event{
				ID:        searchID,
				PubKey:    searchPubkey,
				CreatedAt: now + 2,
				Kind:      nostr.KindTextNote,
				Tags:      tags2,
				Content:   "bogusssss",
				Sig:       "bogusssss" + uuid.NewString(),
			},
		})
		require.NoError(t, db.AcceptEvents(t.Context(), expectedEvents[2]))

		stored := helperSelectEvents(t, db, model.Filter{
			Kinds: []int{nostr.KindTextNote},
		})
		require.Len(t, stored, 3)
		require.EqualValues(t, expectedEvents[2], stored[0])
		require.EqualValues(t, expectedEvents[1], stored[1])
		require.EqualValues(t, expectedEvents[0], stored[2])
	})
	t.Run("search event bogus", func(t *testing.T) {
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindTextNote},
			Search: `"bogu"`,
		})
		require.Len(t, stored, 1)
		require.EqualValues(t, expectedEvents[2], stored[0])
	})
	t.Run("search by imeta alt tag alt1 value", func(t *testing.T) {
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindTextNote},
			Search: `"alt1"`,
		})
		require.Len(t, stored, 1)

		require.EqualValues(t, expectedEvents[0], stored[0])
	})
	t.Run("search by imeta alt tag - alt2 value", func(t *testing.T) {
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindTextNote},
			Search: `"alt2"`,
		})
		require.Len(t, stored, 1)
		require.EqualValues(t, expectedEvents[2], stored[0])
	})
	t.Run("search by imeta alt tag", func(t *testing.T) {
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindTextNote},
			Search: `"alt"`,
		})
		require.Len(t, stored, 2)
		require.EqualValues(t, expectedEvents[0], stored[1])
		require.EqualValues(t, expectedEvents[2], stored[0])
	})
	t.Run("search by imeta alt tag summary1 value", func(t *testing.T) {
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindTextNote},
			Search: `"summary1"`,
		})
		require.Len(t, stored, 1)

		require.EqualValues(t, expectedEvents[0], stored[0])
	})
	t.Run("search by imeta summary tag - summary2 value", func(t *testing.T) {
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindTextNote},
			Search: `"summary2"`,
		})
		require.Len(t, stored, 1)
		require.EqualValues(t, expectedEvents[2], stored[0])
	})
	t.Run("search kind text note by imeta summary tag", func(t *testing.T) {
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindTextNote},
			Search: `"summ"`,
		})
		require.Len(t, stored, 2)
		require.EqualValues(t, expectedEvents[0], stored[1])
		require.EqualValues(t, expectedEvents[2], stored[0])
	})
	t.Run("search kind text note by imeta summary tag", func(t *testing.T) {
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindTextNote},
			Search: `"summ"`,
		})
		require.Len(t, stored, 2)
		require.EqualValues(t, expectedEvents[0], stored[1])
		require.EqualValues(t, expectedEvents[2], stored[0])
	})
	t.Run("search kind text note by content and pubkey", func(t *testing.T) {
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindTextNote},
			Search: `"summ"`,
			Authors: []string{
				searchPubkey,
			},
		})
		require.Len(t, stored, 1)
		require.EqualValues(t, expectedEvents[2], stored[0])
	})
	t.Run("search by 2 filters with search field", func(t *testing.T) {
		filters := model.Filters{
			model.Filter{
				Kinds:  []int{nostr.KindTextNote},
				Search: `"summ"`,
			},
			model.Filter{
				Kinds:  []int{nostr.KindTextNote},
				Search: `"end"`,
			},
		}
		stored := helperSelectEvents(t, db, filters...)
		require.Len(t, stored, 2)
		require.ElementsMatch(t, []*model.Event{expectedEvents[0], expectedEvents[2]}, stored)
	})
	t.Run("delete events", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        "normal" + uuid.NewString(),
				PubKey:    searchPubkey,
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindDeletion,
				Tags:      nostr.Tags{{"e", searchID}},
				Sig:       "end" + uuid.NewString(),
			},
		}
		require.NoError(t, db.AcceptEvents(t.Context(), ev))
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindTextNote},
			Search: `"summ"`,
			Authors: []string{
				searchPubkey,
			},
		})
		require.Len(t, stored, 0)
		require.NoError(t, db.AcceptEvents(t.Context(), ev))
	})
}

func TestSearchEvents_EditableTextNote(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()
	expectedEvents := []*model.Event{}
	searchPubkey := "bogusssss" + uuid.NewString()
	searchID := "normal, 3nd event" + uuid.NewString()
	now := nostr.Now()
	t.Run("Create events with text note kind with imeta alt and summary tags", func(t *testing.T) {
		var tags1 nostr.Tags
		tags1 = append(tags1, nostr.Tag{
			"imeta",
			"url https://alicerelay.example.com",
			"m image/jpg",
			"dim 3024x4032",
			"i foobar",
			"alt alt1 text",
			"summary dummy summary1 content",
			fmt.Sprintf("x %x", []byte("https://alicerelay.example.com")),
			fmt.Sprintf("ox %x", []byte("https://alicerelay.example.com")),
		})
		expectedEvents = append(expectedEvents, &model.Event{
			Event: nostr.Event{
				ID:        "normal" + uuid.NewString(),
				PubKey:    "end" + uuid.NewString(),
				CreatedAt: now,
				Kind:      model.CustomIONKindEditableTextNote,
				Tags:      tags1,
				Content:   "Test post 12345\n",
				Sig:       "1" + uuid.NewString(),
			},
		})
		require.NoError(t, db.AcceptEvents(t.Context(), expectedEvents[0]))

		expectedEvents = append(expectedEvents, &model.Event{
			Event: nostr.Event{
				ID:        "normal, 2nd event" + uuid.NewString(),
				PubKey:    "bogus" + uuid.NewString(),
				CreatedAt: now + 1,
				Kind:      model.CustomIONKindEditableTextNote,
				Tags:      model.Tags{},
				Content:   "lalala hey",
				Sig:       "2" + uuid.NewString(),
			},
		})
		require.NoError(t, db.AcceptEvents(t.Context(), expectedEvents[1]))

		var tags2 nostr.Tags
		tags2 = append(tags2, nostr.Tag{
			"imeta",
			"url https://alicerelay.example.com",
			"m image/jpg",
			"dim 3024x4032",
			"i foobar",
			"alt alt2 text",
			"summary dummy summary2 content",
			fmt.Sprintf("x %x", []byte("https://alicerelay.example.com")),
			fmt.Sprintf("ox %x", []byte("https://alicerelay.example.com")),
		})
		expectedEvents = append(expectedEvents, &model.Event{
			Event: nostr.Event{
				ID:        searchID,
				PubKey:    searchPubkey,
				CreatedAt: now + 3,
				Kind:      model.CustomIONKindEditableTextNote,
				Tags:      tags2,
				Content:   "abcd",
				Sig:       "3" + uuid.NewString(),
			},
		})
		require.NoError(t, db.AcceptEvents(t.Context(), expectedEvents[2]))

		stored := helperSelectEvents(t, db, model.Filter{
			Kinds: []int{model.CustomIONKindEditableTextNote},
		})
		require.Len(t, stored, 3)
		require.EqualValues(t, expectedEvents[2], stored[0])
		require.EqualValues(t, expectedEvents[1], stored[1])
		require.EqualValues(t, expectedEvents[0], stored[2])
	})
	t.Run("search by 2 filters with search field", func(t *testing.T) {
		filters := model.Filters{
			model.Filter{
				Kinds:  []int{nostr.KindTextNote, model.CustomIONKindEditableTextNote, nostr.KindRepost},
				Search: `"pos"`,
				Limit:  20,
			},
			model.Filter{
				Kinds:  []int{model.CustomIONKindEditableTextNote},
				Search: `"lala"`,
				Limit:  20,
			},
		}
		stored := helperSelectEvents(t, db, filters...)
		require.Len(t, stored, 2)
		require.ElementsMatch(t, []*model.Event{expectedEvents[1], expectedEvents[0]}, stored)
	})
}

func TestSearchEvents_KindProfileMetadata(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()
	expectedEvents := []*model.Event{}
	t.Run("Events with profile metadata kind", func(t *testing.T) {
		expectedEvents = append(expectedEvents, &model.Event{
			Event: nostr.Event{
				ID:        "normal" + uuid.NewString(),
				PubKey:    "ev1" + uuid.NewString(),
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindProfileMetadata,
				Tags:      model.Tags{},
				Content:   `{"name":"abcd","display_name":"efgh"}`,
				Sig:       "ev1" + uuid.NewString(),
			},
		})
		require.NoError(t, db.AcceptEvents(t.Context(), expectedEvents[0]))

		expectedEvents = append(expectedEvents, &model.Event{
			Event: nostr.Event{
				ID:        "normal, 2nd event" + uuid.NewString(),
				PubKey:    "ev2" + uuid.NewString(),
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindProfileMetadata,
				Tags:      model.Tags{},
				Content:   `{"name":"xyuz","display_name":"sprtq"}`,
				Sig:       "ev2" + uuid.NewString(),
			},
		})
		require.NoError(t, db.AcceptEvents(t.Context(), expectedEvents[1]))

		expectedEvents = append(expectedEvents, &model.Event{
			Event: nostr.Event{
				ID:        "normal, 3nd event" + uuid.NewString(),
				PubKey:    "ev3" + uuid.NewString(),
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindProfileMetadata,
				Tags:      model.Tags{},
				Content:   ``,
				Sig:       "ev3" + uuid.NewString(),
			},
		})
		require.NoError(t, db.AcceptEvents(t.Context(), expectedEvents[2]))

		stored := helperSelectEvents(t, db, model.Filter{
			Kinds: []int{nostr.KindProfileMetadata},
		})
		require.Len(t, stored, 3)
		require.ElementsMatch(t, expectedEvents, stored)
	})
	t.Run("search profile by name xyu", func(t *testing.T) {
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindProfileMetadata},
			Search: `"xyu"`,
		})
		require.Len(t, stored, 1)
		require.EqualValues(t, expectedEvents[1], stored[0])
	})
	t.Run("search profile by name abcd", func(t *testing.T) {
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindProfileMetadata},
			Search: `"abcd"`,
		})
		require.Len(t, stored, 1)
		require.EqualValues(t, expectedEvents[0], stored[0])
	})
	t.Run("search profile by name empty value", func(t *testing.T) {
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindProfileMetadata},
			Search: "",
		})
		require.Len(t, stored, 3)
		require.ElementsMatch(t, expectedEvents, stored)
	})
}

func TestSearchEvents_KindFileMetadata(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()
	expectedEvents := []*model.Event{}
	t.Run("Events with KindFileMetadata", func(t *testing.T) {
		expectedEvents = append(expectedEvents, &model.Event{
			Event: nostr.Event{
				ID:        "ev1" + uuid.NewString(),
				PubKey:    "ev1" + uuid.NewString(),
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindFileMetadata,
				Tags:      nostr.Tags{{"alt", "alt1 text"}, {"summary", "dummy summary1 content"}},
				Sig:       "ev1" + uuid.NewString(),
			},
		})
		require.NoError(t, db.AcceptEvents(t.Context(), expectedEvents[0]))
		expectedEvents = append(expectedEvents, &model.Event{
			Event: nostr.Event{
				ID:        "ev2" + uuid.NewString(),
				PubKey:    "ev2" + uuid.NewString(),
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindFileMetadata,
				Tags:      nostr.Tags{{"alt", "alt2 text"}, {"summary", "dummy summary2 content"}},
				Sig:       "ev2" + uuid.NewString(),
			},
		})
		require.NoError(t, db.AcceptEvents(t.Context(), expectedEvents[1]))
		expectedEvents = append(expectedEvents, &model.Event{
			Event: nostr.Event{
				ID:        "ev3" + uuid.NewString(),
				PubKey:    "ev3" + uuid.NewString(),
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindFileMetadata,
				Tags:      nostr.Tags{{"alt", "alt3 text"}, {"summary", "dummy summary3 content"}},
				Sig:       "ev3" + uuid.NewString(),
			},
		})
		require.NoError(t, db.AcceptEvents(t.Context(), expectedEvents[2]))

		stored := helperSelectEvents(t, db, model.Filter{
			Kinds: []int{nostr.KindFileMetadata},
		})
		require.Len(t, stored, 3)
		require.ElementsMatch(t, expectedEvents, stored)
	})
	t.Run("search by imeta alt tag alt1 value", func(t *testing.T) {
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindFileMetadata},
			Search: `"alt1"`,
		})
		require.Len(t, stored, 1)

		require.EqualValues(t, expectedEvents[0], stored[0])
	})
	t.Run("search by imeta alt tag alt2 value", func(t *testing.T) {
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindFileMetadata},
			Search: `"alt2"`,
		})
		require.Len(t, stored, 1)

		require.EqualValues(t, expectedEvents[1], stored[0])
	})
	t.Run("search by imeta alt tag alt value", func(t *testing.T) {
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindFileMetadata},
			Search: `"al"`,
		})
		require.Len(t, stored, 3)
		require.ElementsMatch(t, expectedEvents, stored)
	})
	t.Run("search by imeta alt tag summary1 value", func(t *testing.T) {
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindFileMetadata},
			Search: `"summary1"`,
		})
		require.Len(t, stored, 1)

		require.EqualValues(t, expectedEvents[0], stored[0])
	})
	t.Run("search by imeta alt tag summary2 value", func(t *testing.T) {
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindFileMetadata},
			Search: `"summary2"`,
		})
		require.Len(t, stored, 1)

		require.EqualValues(t, expectedEvents[1], stored[0])
	})
	t.Run("search by imeta summary tag", func(t *testing.T) {
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindFileMetadata},
			Search: `"summ"`,
		})
		require.Len(t, stored, 3)
		require.ElementsMatch(t, expectedEvents, stored)
	})
}

func TestSearchEvents_KindGenericRepost(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()
	expectedEvents := []*model.Event{}
	t.Run("Events", func(t *testing.T) {
		var tags1 nostr.Tags
		tags1 = append(tags1, nostr.Tag{
			"imeta",
			"url https://alicerelay.example.com",
			"m image/jpg",
			"dim 3024x4032",
			"i foobar",
			"alt alt1 text",
			"summary dummy summary1 content",
			fmt.Sprintf("x %x", []byte("https://alicerelay.example.com")),
			fmt.Sprintf("ox %x", []byte("https://alicerelay.example.com")),
		})
		expectedEvents = append(expectedEvents, &model.Event{
			Event: nostr.Event{
				ID:        "ev1" + uuid.NewString(),
				PubKey:    "ev1" + uuid.NewString(),
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindArticle,
				Tags:      tags1,
				Content:   "end, and, ond",
				Sig:       "ev1" + uuid.NewString(),
			},
		})
		require.NoError(t, db.AcceptEvents(t.Context(), expectedEvents[0]))

		expectedEvents = append(expectedEvents, &model.Event{
			Event: nostr.Event{
				ID:        "ev2" + uuid.NewString(),
				PubKey:    "ev2" + uuid.NewString(),
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindGenericRepost,
				Tags:      model.Tags{},
				Content:   expectedEvents[0].String(),
				Sig:       "ev2" + uuid.NewString(),
			},
		})
		require.NoError(t, db.AcceptEvents(t.Context(), expectedEvents[1]))

		stored := helperSelectEvents(t, db, model.Filter{
			Kinds: []int{nostr.KindArticle},
			Limit: 100,
		})
		require.Len(t, stored, 1)
		require.EqualValues(t, expectedEvents[0], stored[0])

		stored = helperSelectEvents(t, db, model.Filter{
			Kinds: []int{nostr.KindGenericRepost},
		})
		require.Len(t, stored, 1)
		require.EqualValues(t, expectedEvents[1], stored[0])
	})
	t.Run("search repost by repost value", func(t *testing.T) {
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindGenericRepost},
			Search: `"end"`,
			Limit:  100,
		})
		require.Len(t, stored, 1)
		require.EqualValues(t, expectedEvents[1], stored[0])
	})
	t.Run("search repost by reposted article tag alt", func(t *testing.T) {
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindGenericRepost},
			Search: `"alt1"`,
			Limit:  100,
		})
		require.Len(t, stored, 1)
		require.EqualValues(t, expectedEvents[1], stored[0])
	})
	t.Run("search repost by reposted article tag summary", func(t *testing.T) {
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindGenericRepost},
			Search: `"summary1"`,
			Limit:  100,
		})
		require.Len(t, stored, 1)
		require.EqualValues(t, expectedEvents[1], stored[0])
	})
}

func TestSearchEvents_KindTextNoteWithDependencies(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	t.Run("Create events with text note kind with imeta alt and summary tags", func(t *testing.T) {
		var ev model.Event

		ev.ID = "id1"
		ev.Kind = nostr.KindProfileMetadata
		ev.PubKey = "pk1"
		ev.CreatedAt = 1
		ev.Content = `{"name":"notcoin"}`
		err := db.AcceptEvents(t.Context(), &ev)
		require.NoError(t, err)

		ev.ID = "id2"
		ev.Kind = nostr.KindTextNote
		ev.PubKey = "pk1"
		ev.CreatedAt = 2
		ev.Content = "content of the note"
		err = db.AcceptEvents(t.Context(), &ev)
		require.NoError(t, err)

		stored := helperSelectEvents(t, db, model.Filter{
			IDs:    []string{"id2"},
			Search: `include:dependencies:kind1>kind0`,
			Limit:  100,
		})
		require.Len(t, stored, 2)
		require.ElementsMatch(t, []string{"id2", "id1"}, []string{stored[0].ID, stored[1].ID})

		stored = helperSelectEvents(t, db, model.Filter{
			IDs:    []string{"id2"},
			Search: `"not include:dependencies:kind1>kind0`,
			Limit:  100,
		})
		require.Len(t, stored, 2)
		require.ElementsMatch(t, []string{"id2", "id1"}, []string{stored[0].ID, stored[1].ID})
	})
}

func TestSearchEvents_Replace_Update(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	pk := model.GeneratePrivateKey()

	initialEvent := &model.Event{
		Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindArticle,
			Tags: model.Tags{
				{"d", "article"},
				{
					"imeta",
					"url https://alicerelay.example.com",
					"m image/jpg",
					"dim 3024x4032",
					"alt initial alt text",
					"summary initial summary content",
					fmt.Sprintf("x %x", []byte("https://alicerelay.example.com")),
					fmt.Sprintf("ox %x", []byte("https://alicerelay.example.com")),
				},
			},
			Content: "initial",
		},
	}
	require.NoError(t, initialEvent.SignWithAlg(pk, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, db.AcceptEvents(t.Context(), initialEvent))

	t.Run("search initial event", func(t *testing.T) {
		searchResult := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindArticle},
			Search: `"initial"`,
		})
		require.Len(t, searchResult, 1)
		require.EqualValues(t, initialEvent, searchResult[0])
	})

	updatedEvent := &model.Event{
		Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindArticle,
			Tags: model.Tags{
				{"d", "article"},
				{
					"imeta",
					"url https://alicerelay.example.com",
					"m image/jpg",
					"dim 3024x4032",
					"alt updated alt text",
					"summary updated summary content",
					fmt.Sprintf("x %x", []byte("https://alicerelay.example.com")),
					fmt.Sprintf("ox %x", []byte("https://alicerelay.example.com")),
				},
			},
			Content: "updated, content",
		},
	}
	require.NoError(t, updatedEvent.SignWithAlg(pk, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, db.AcceptEvents(t.Context(), updatedEvent))

	storedUpdated := helperSelectEvents(t, db, model.Filter{
		Kinds: []int{nostr.KindArticle},
	})
	require.Len(t, storedUpdated, 1)
	require.EqualValues(t, updatedEvent, storedUpdated[0])

	t.Run("search updated event", func(t *testing.T) {
		searchResult := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindArticle},
			Search: `"updated"`,
		})
		require.Len(t, searchResult, 1)
		require.EqualValues(t, updatedEvent, searchResult[0])
	})
}

func TestSearchEvents_ScoreWithSearch(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	ts := time.Now().Add(time.Hour).Unix()

	var evNote, evArticle model.Event
	evNote.ID = "note"
	evNote.Kind = nostr.KindTextNote
	evNote.PubKey = "note_pub"
	evNote.CreatedAt = model.Timestamp(ts)
	evNote.Content = "content 1"

	evArticle.ID = "article"
	evArticle.Kind = nostr.KindTextNote
	evArticle.PubKey = "article_pub"
	evArticle.CreatedAt = model.Timestamp(ts) + 1
	evArticle.Content = "content 2"
	evArticle.Tags = model.Tags{
		{"d", "my article"},
	}
	require.NoError(t, db.AcceptEvents(t.Context(), &evNote, &evArticle))

	type target struct {
		Tag     string
		ID      string
		Address string
		JSON    string
	}

	targetEventArticle := target{"a", evArticle.ID, evArticle.Address(), evArticle.String()}
	targetEventNote := target{"e", evNote.ID, evNote.Address(), evNote.String()}

	t.Run("Like article", func(t *testing.T) {
		var ev model.Event
		ev.Content = "+"
		ev.ID = "like_" + targetEventArticle.ID
		ev.PubKey = "like_pub_" + targetEventArticle.ID
		ev.Kind = nostr.KindReaction
		ev.CreatedAt = nostr.Now()
		ev.Tags = model.Tags{
			{targetEventArticle.Tag, targetEventArticle.Address},
		}
		require.NoError(t, db.AcceptEvents(t.Context(), &ev))
		helperPointsScoreEqual(t, db, targetEventArticle.ID, 1, 1e4)
	})
	t.Run("search ranked events", func(t *testing.T) {
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindTextNote},
			Search: `"content" top`,
		})
		require.Len(t, stored, 1)
		require.EqualValues(t, evArticle, *stored[0])
	})
	t.Run("Repost note", func(t *testing.T) {
		var ev model.Event
		ev.Content = targetEventNote.JSON
		ev.ID = "repost_" + targetEventNote.ID
		ev.PubKey = "repost_pub_" + targetEventNote.ID
		ev.Kind = nostr.KindRepost
		ev.CreatedAt = nostr.Now()
		ev.Tags = model.Tags{
			{targetEventNote.Tag, targetEventNote.Address},
		}
		require.NoError(t, db.AcceptEvents(t.Context(), &ev))
		helperPointsScoreEqual(t, db, targetEventNote.ID, 3, 3e4) // repost (3).
	})
	t.Run("search ranked events", func(t *testing.T) {
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindTextNote},
			Search: `"content" top`,
		})
		require.Len(t, stored, 2)
		require.ElementsMatch(t, []string{evNote.ID, evArticle.ID}, []string{stored[0].ID, stored[1].ID})
	})

	var quotes []string
	t.Run("Now quote article", func(t *testing.T) {
		var ev model.Event
		ev.Content = "quote"
		ev.ID = "quote" + targetEventArticle.ID
		ev.PubKey = "quote_pub"
		ev.Kind = nostr.KindTextNote
		ev.Kind = model.CustomIONKindEditableTextNote
		ev.Tags = model.Tags{
			{model.CustomIONTagAddressableQ, targetEventArticle.Address},
			{"d", "quote"},
		}
		ev.CreatedAt = nostr.Now()
		quotes = append(quotes, ev.ID)
		require.NoError(t, db.AcceptEvents(t.Context(), &ev))
		helperPointsScoreEqual(t, db, targetEventArticle.ID, 5, 5e4) // like (1) + quote (4).
	})

	t.Run("search ranked events", func(t *testing.T) {
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindTextNote},
			Search: `"content" top`,
		})
		require.Len(t, stored, 2)
		require.ElementsMatch(t, []string{evNote.ID, evArticle.ID}, []string{stored[0].ID, stored[1].ID})
	})
	t.Run("search ranked events with dependencies", func(t *testing.T) {
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindTextNote},
			Search: `"content" top include:dependencies:kind1>kind0`,
		})
		require.Len(t, stored, 2)
		require.ElementsMatch(t, []string{evNote.ID, evArticle.ID}, []string{stored[0].ID, stored[1].ID})
	})
}

func TestExtractIMetaTagValues(t *testing.T) {
	t.Parallel()

	var ev model.Event
	ev.Tags = model.Tags{
		{"imeta", "url https://alicerelay.example.com", "m image/jpg", "dim 3024x4032", "i foobar", "alt alt1 text", "summary dummy summary1 content"},
	}

	data := extractIMetaTagValues(&ev)
	require.NotEmpty(t, data)
	require.Equal(t, []string{"alt1", "text", "dummy", "summary1", "content"}, data)
}

func TestSearchEvents_WithNestedDependencyKind3Kind0(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	profileEvents := make([]*model.Event, 5)
	followListEvents := make([]*model.Event, 5)
	profileNames := []string{"bob", "john", "anna", "alice", "martin"}
	profilePubKeys := make([]string, len(profileNames))
	profilePrivKeys := make([]string, len(profileNames))
	now := nostr.Now()

	for i, name := range profileNames {
		profilePrivKeys[i], profilePubKeys[i] = model.GenerateKeyPair()

		content, err := json.Marshal(model.ProfileMetadataContent{
			Name:        name,
			DisplayName: strings.ToUpper(name),
			About:       "I am " + strings.ToTitle(name),
		})
		require.NoError(t, err)

		profileEvents[i] = &model.Event{
			Event: nostr.Event{
				Kind:      nostr.KindProfileMetadata,
				CreatedAt: now.Add(-time.Duration(i+1) * time.Second),
				Content:   string(content),
				Tags:      model.Tags{},
			},
		}
		require.NoError(t, profileEvents[i].SignWithAlg(profilePrivKeys[i], model.SignAlgEDDSA, model.KeyAlgCurve25519))

		followListEvents[i] = &model.Event{
			Event: nostr.Event{
				Kind:      nostr.KindFollowList,
				CreatedAt: profileEvents[i].CreatedAt + 1,
				Tags:      model.Tags{},
			},
		}
		require.NoError(t, followListEvents[i].SignWithAlg(profilePrivKeys[i], model.SignAlgEDDSA, model.KeyAlgCurve25519))
	}
	require.NoError(t, db.AcceptEvents(t.Context(), profileEvents...))
	require.NoError(t, db.AcceptEvents(t.Context(), followListEvents...))

	t.Run("kind3>kind0", func(t *testing.T) {
		events := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindFollowList},
			Search: `include:dependencies:kind3>kind0`,
		})
		require.Len(t, events, len(profileEvents)+len(followListEvents))
	})
	t.Run("kind3>kind0 plus profile name", func(t *testing.T) {
		events := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindFollowList},
			Search: `include:dependencies:kind3>kind0 "anna"`,
		})
		require.Len(t, events, len(followListEvents)+1)
		require.Contains(t, events, profileEvents[2])
		for i := range followListEvents {
			require.Contains(t, events, followListEvents[i])
		}
	})
}
