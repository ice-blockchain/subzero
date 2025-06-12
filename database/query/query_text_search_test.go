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

func TestExtractRichTextContent(t *testing.T) {
	t.Parallel()
	t.Run("Valid Quill Delta with text content", func(t *testing.T) {
		deltaJSON := `[{"insert":"Header 1"},{"insert":"\n","attributes":{"header":1}},{"insert":"Regular text "},{"insert":"Bold","attributes":{"bold":true}},{"insert":" "},{"insert":"Italic","attributes":{"italic":true}},{"insert":"\n"}]`
		var ev model.Event
		ev.Tags = model.Tags{
			{model.CustomIONTagRichText, "quill_delta", deltaJSON},
		}
		result := extractRichTextContent(&ev)
		require.Equal(t, "Header 1 Regular text Bold Italic", result)
	})
	t.Run("Valid Quill Delta with simple text", func(t *testing.T) {
		deltaJSON := `[{"insert":"Hello World!\n"}]`
		var ev model.Event
		ev.Tags = model.Tags{
			{model.CustomIONTagRichText, "quill_delta", deltaJSON},
		}
		result := extractRichTextContent(&ev)
		require.Equal(t, "Hello World!", result)
	})
	t.Run("Valid Quill Delta with embeds", func(t *testing.T) {
		deltaJSON := `[{"insert":"Text before image "},{"insert":{"image":"https://example.com/img.jpg"}},{"insert":" text after image\n"}]`
		var ev model.Event
		ev.Tags = model.Tags{
			{model.CustomIONTagRichText, "quill_delta", deltaJSON},
		}
		result := extractRichTextContent(&ev)
		require.Equal(t, "Text before image text after image", result)
	})
	t.Run("Invalid JSON", func(t *testing.T) {
		invalidJSON := `[{"insert":"Header 1"}`
		var ev model.Event
		ev.Tags = model.Tags{
			{model.CustomIONTagRichText, "quill_delta", invalidJSON},
		}
		result := extractRichTextContent(&ev)
		require.Empty(t, result)
	})
	t.Run("Unsupported protocol", func(t *testing.T) {
		var ev model.Event
		ev.Tags = model.Tags{
			{model.CustomIONTagRichText, "unsupported_protocol", "some content"},
		}
		result := extractRichTextContent(&ev)
		require.Empty(t, result)
	})
	t.Run("No rich_text tag", func(t *testing.T) {
		var ev model.Event
		ev.Tags = model.Tags{}
		result := extractRichTextContent(&ev)
		require.Empty(t, result)
	})
	t.Run("Malformed rich_text tag", func(t *testing.T) {
		var ev model.Event
		ev.Tags = model.Tags{
			{model.CustomIONTagRichText, "quill_delta"},
		}
		result := extractRichTextContent(&ev)
		require.Empty(t, result)
	})
}

func TestParseQuillDeltaToPlainText(t *testing.T) {
	t.Parallel()
	t.Run("Simple text operations", func(t *testing.T) {
		deltaJSON := `[{"insert":"Hello "},{"insert":"World","attributes":{"bold":true}},{"insert":"!\n"}]`
		require.Equal(t, "Hello World !", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Text with headers", func(t *testing.T) {
		deltaJSON := `[{"insert":"Main Title"},{"insert":"\n","attributes":{"header":1}},{"insert":"Some content\n"}]`
		require.Equal(t, "Main Title Some content", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Text with formatting", func(t *testing.T) {
		deltaJSON := `[{"insert":"Start "},{"insert":"bold text","attributes":{"bold":true}},{"insert":" and "},{"insert":"italic text","attributes":{"italic":true}},{"insert":" end\n"}]`
		require.Equal(t, "Start bold text and italic text end", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Empty operations", func(t *testing.T) {
		deltaJSON := `[]`
		require.Empty(t, parseQuillDeltaToPlainText(deltaJSON))
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		invalidJSON := `[{"insert":"test"`
		require.Empty(t, parseQuillDeltaToPlainText(invalidJSON))
	})
}

func TestHtmlToPlainText(t *testing.T) {
	t.Parallel()
	t.Run("Simple HTML", func(t *testing.T) {
		html := `<p>Hello <strong>World</strong>!</p>`
		require.Equal(t, "Hello World !", htmlToPlainText(html))
	})
	t.Run("HTML with multiple tags", func(t *testing.T) {
		html := `<h1>Title</h1><p>Paragraph with <em>italic</em> and <strong>bold</strong> text.</p>`
		require.Equal(t, "Title Paragraph with italic and bold text.", htmlToPlainText(html))
	})
	t.Run("HTML with entities", func(t *testing.T) {
		html := `<p>&lt;script&gt;alert(&quot;test&quot;)&lt;/script&gt;</p>`
		require.Equal(t, `<script>alert("test")</script>`, htmlToPlainText(html))
	})
	t.Run("HTML with newlines", func(t *testing.T) {
		html := "<p>Line 1</p>\n<p>Line 2</p>"
		require.Equal(t, "Line 1 Line 2", htmlToPlainText(html))
	})
	t.Run("Empty HTML", func(t *testing.T) {
		html := ``
		require.Empty(t, htmlToPlainText(html))
	})
}

func TestPrepareSearchContent_WithRichText(t *testing.T) {
	t.Parallel()
	t.Run("TextNote with rich text", func(t *testing.T) {
		deltaJSON := `[{"insert":"Rich text content "},{"insert":"with bold","attributes":{"bold":true}},{"insert":"\n"}]`
		var ev model.Event
		ev.Kind = nostr.KindTextNote
		ev.Content = "Plain text content"
		ev.Tags = model.Tags{
			{model.CustomIONTagRichText, "quill_delta", deltaJSON},
		}
		require.Equal(t, "Plain text content", prepareSearchContent(&ev))
	})
	t.Run("TextNote with empty content and rich text", func(t *testing.T) {
		deltaJSON := `[{"insert":"Rich text content "},{"insert":"with bold","attributes":{"bold":true}},{"insert":"\n"}]`
		var ev model.Event
		ev.Kind = nostr.KindTextNote
		ev.Content = ""
		ev.Tags = model.Tags{
			{model.CustomIONTagRichText, "quill_delta", deltaJSON},
		}
		require.Equal(t, "Rich text content with bold", prepareSearchContent(&ev))
	})
	t.Run("EditableTextNote with rich text", func(t *testing.T) {
		deltaJSON := `[{"insert":"Editable rich content\n"}]`
		var ev model.Event
		ev.Kind = model.CustomIONKindEditableTextNote
		ev.Content = "Editable plain content"
		ev.Tags = model.Tags{
			{model.CustomIONTagRichText, "quill_delta", deltaJSON},
		}
		require.Equal(t, "Editable plain content", prepareSearchContent(&ev))
	})
	t.Run("EditableTextNote with empty content and rich text", func(t *testing.T) {
		deltaJSON := `[{"insert":"Editable rich content\n"}]`
		var ev model.Event
		ev.Kind = model.CustomIONKindEditableTextNote
		ev.Content = ""
		ev.Tags = model.Tags{
			{model.CustomIONTagRichText, "quill_delta", deltaJSON},
		}
		require.Equal(t, "Editable rich content", prepareSearchContent(&ev))
	})
	t.Run("Article with rich text", func(t *testing.T) {
		deltaJSON := `[{"insert":"Article rich content\n"}]`
		var ev model.Event
		ev.Kind = nostr.KindArticle
		ev.Content = "Article plain content"
		ev.Tags = model.Tags{
			{model.CustomIONTagRichText, "quill_delta", deltaJSON},
		}
		require.Equal(t, "Article plain content", prepareSearchContent(&ev))
	})
	t.Run("Article with empty content and rich text", func(t *testing.T) {
		deltaJSON := `[{"insert":"Article rich content\n"}]`
		var ev model.Event
		ev.Kind = nostr.KindArticle
		ev.Content = ""
		ev.Tags = model.Tags{
			{model.CustomIONTagRichText, "quill_delta", deltaJSON},
		}
		require.Equal(t, "Article rich content", prepareSearchContent(&ev))
	})
	t.Run("Event without rich text tag", func(t *testing.T) {
		var ev model.Event
		ev.Kind = nostr.KindTextNote
		ev.Content = "Only plain text"
		ev.Tags = model.Tags{}
		require.Equal(t, "Only plain text", prepareSearchContent(&ev))
	})
	t.Run("Profile metadata ignores rich text", func(t *testing.T) {
		deltaJSON := `[{"insert":"Profile rich content\n"}]`
		var ev model.Event
		ev.Kind = nostr.KindProfileMetadata
		ev.Content = `{"name":"testuser","display_name":"Test User"}`
		ev.Tags = model.Tags{
			{model.CustomIONTagRichText, "quill_delta", deltaJSON},
		}
		require.Equal(t, "testuser Test User", prepareSearchContent(&ev))
	})
	t.Run("Complex Quill Delta with various formats", func(t *testing.T) {
		deltaJSON := `[{"insert":"Main Title"},{"insert":"\n","attributes":{"header":1}},{"insert":"Some "},{"insert":"bold","attributes":{"bold":true}},{"insert":" text and "},{"insert":"italic","attributes":{"italic":true}},{"insert":" text.\n"},{"insert":"List item 1"},{"insert":"\n","attributes":{"list":"bullet"}},{"insert":"List item 2 with "},{"insert":"link","attributes":{"link":"https://example.com"}},{"insert":"\n","attributes":{"list":"bullet"}},{"insert":"Image: "},{"insert":{"image":"https://example.com/img.jpg"}},{"insert":"\n"},{"insert":"Quote text"},{"insert":"\n","attributes":{"blockquote":true}}]`
		var ev model.Event
		ev.Kind = nostr.KindArticle
		ev.Content = "Markdown version of content"
		ev.Tags = model.Tags{
			{model.CustomIONTagRichText, "quill_delta", deltaJSON},
		}
		require.Equal(t, "Markdown version of content", prepareSearchContent(&ev))
	})
	t.Run("Complex Quill Delta with empty content", func(t *testing.T) {
		deltaJSON := `[{"insert":"Main Title"},{"insert":"\n","attributes":{"header":1}},{"insert":"Some "},{"insert":"bold","attributes":{"bold":true}},{"insert":" text and "},{"insert":"italic","attributes":{"italic":true}},{"insert":" text.\n"},{"insert":"List item 1"},{"insert":"\n","attributes":{"list":"bullet"}},{"insert":"List item 2"},{"insert":"\n","attributes":{"list":"bullet"}}]`
		var ev model.Event
		ev.Kind = nostr.KindArticle
		ev.Content = ""
		ev.Tags = model.Tags{
			{model.CustomIONTagRichText, "quill_delta", deltaJSON},
		}
		require.Equal(t, "Main Title Some bold text and italic text. List item 1 List item 2", prepareSearchContent(&ev))
	})
}

func TestQuillDeltaFormatVariants(t *testing.T) {
	t.Parallel()
	t.Run("Document - Basic text with formatting", func(t *testing.T) {
		deltaJSON := `[{"insert":"Gandalf","attributes":{"bold":true}},{"insert":" the "},{"insert":"Grey","attributes":{"color":"#cccccc"}},{"insert":"\n"}]`
		require.Equal(t, "Gandalf the Grey", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Document - Complex text with multiple formats", func(t *testing.T) {
		deltaJSON := `[{"insert":"Bold text","attributes":{"bold":true}},{"insert":" and "},{"insert":"italic text","attributes":{"italic":true}},{"insert":" and "},{"insert":"underlined","attributes":{"underline":true}},{"insert":" text.\n"}]`
		require.Equal(t, "Bold text and italic text and underlined text.", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Embeds - Image embed", func(t *testing.T) {
		deltaJSON := `[{"insert":{"image":"https://quilljs.com/assets/images/icon.png"},"attributes":{"link":"https://quilljs.com"}}]`
		require.Equal(t, "", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Embeds - Text with image embed", func(t *testing.T) {
		deltaJSON := `[{"insert":"Check out this image: "},{"insert":{"image":"https://example.com/image.png"}},{"insert":" Amazing!\n"}]`
		require.Equal(t, "Check out this image: Amazing!", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Embeds - Multiple embed types", func(t *testing.T) {
		deltaJSON := `[{"insert":"Video: unsupported embed"},{"insert":" and formula: "},{"insert":"e=mc^2"},{"insert":" end.\n"}]`
		require.Equal(t, "Video: unsupported embed and formula: e=mc^2 end.", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Line Formatting - Headers", func(t *testing.T) {
		deltaJSON := `[{"insert":"The Two Towers"},{"insert":"\n","attributes":{"header":1}},{"insert":"Aragorn sped on up the hill.\n"}]`
		require.Equal(t, "The Two Towers Aragorn sped on up the hill.", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Line Formatting - Multiple header levels", func(t *testing.T) {
		deltaJSON := `[{"insert":"Main Title"},{"insert":"\n","attributes":{"header":1}},{"insert":"Subtitle"},{"insert":"\n","attributes":{"header":2}},{"insert":"Sub-subtitle"},{"insert":"\n","attributes":{"header":3}},{"insert":"Regular text\n"}]`
		require.Equal(t, "Main Title Subtitle Sub-subtitle Regular text", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Line Formatting - Bullet lists", func(t *testing.T) {
		deltaJSON := `[{"insert":"First item"},{"insert":"\n","attributes":{"list":"bullet"}},{"insert":"Second item"},{"insert":"\n","attributes":{"list":"bullet"}},{"insert":"Third item"},{"insert":"\n","attributes":{"list":"bullet"}}]`
		require.Equal(t, "First item Second item Third item", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Line Formatting - Ordered lists", func(t *testing.T) {
		deltaJSON := `[{"insert":"First numbered item"},{"insert":"\n","attributes":{"list":"ordered"}},{"insert":"Second numbered item"},{"insert":"\n","attributes":{"list":"ordered"}},{"insert":"Third numbered item"},{"insert":"\n","attributes":{"list":"ordered"}}]`
		require.Equal(t, "First numbered item Second numbered item Third numbered item", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Line Formatting - Blockquotes", func(t *testing.T) {
		deltaJSON := `[{"insert":"This is a quote"},{"insert":"\n","attributes":{"blockquote":true}},{"insert":"This is another quote"},{"insert":"\n","attributes":{"blockquote":true}},{"insert":"Regular text\n"}]`
		require.Equal(t, "This is a quote This is another quote Regular text", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Line Formatting - Code blocks", func(t *testing.T) {
		deltaJSON := `[{"insert":"function hello() {"},{"insert":"\n","attributes":{"code-block":true}},{"insert":"  console.log('Hello');"},{"insert":"\n","attributes":{"code-block":true}},{"insert":"}"},{"insert":"\n","attributes":{"code-block":true}}]`
		require.Equal(t, "function hello() { console.log('Hello'); }", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Line Formatting - Text alignment", func(t *testing.T) {
		deltaJSON := `[{"insert":"Left aligned text"},{"insert":"\n"},{"insert":"Center aligned text"},{"insert":"\n","attributes":{"align":"center"}},{"insert":"Right aligned text"},{"insert":"\n","attributes":{"align":"right"}},{"insert":"Justify aligned text"},{"insert":"\n","attributes":{"align":"justify"}}]`
		require.Equal(t, "Left aligned text Center aligned text Right aligned text Justify aligned text", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Mixed formatting - Complex document", func(t *testing.T) {
		deltaJSON := `[{"insert":"Document Title"},{"insert":"\n","attributes":{"header":1}},{"insert":"This is "},{"insert":"bold","attributes":{"bold":true}},{"insert":" and "},{"insert":"italic","attributes":{"italic":true}},{"insert":" text.\n"},{"insert":"List item 1"},{"insert":"\n","attributes":{"list":"bullet"}},{"insert":"List item 2 with "},{"insert":"link","attributes":{"link":"https://example.com"}},{"insert":"\n","attributes":{"list":"bullet"}},{"insert":"Image: "},{"insert":{"image":"https://example.com/img.jpg"}},{"insert":"\n"},{"insert":"Quote text"},{"insert":"\n","attributes":{"blockquote":true}}]`
		require.Equal(t, "Document Title This is bold and italic text. List item 1 List item 2 with link Image: Quote text", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Special characters and entities", func(t *testing.T) {
		deltaJSON := `[{"insert":"Special chars: <>&\"'"},{"insert":"\n","attributes":{"header":2}},{"insert":"Math: α + β = γ"},{"insert":"\n"},{"insert":"Code: "},{"insert":"console.log(\"Hello\");","attributes":{"code":true}},{"insert":"\n"}]`
		require.Equal(t, "Special chars: <>&\"' Math: α + β = γ Code: console.log(\"Hello\");", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Links and formatting", func(t *testing.T) {
		deltaJSON := `[{"insert":"Visit "},{"insert":"our website","attributes":{"link":"https://example.com","bold":true}},{"insert":" for more info. Also check "},{"insert":"this link","attributes":{"link":"https://other.com","italic":true}},{"insert":".\n"}]`
		require.Equal(t, "Visit our website for more info. Also check this link .", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Color and background formatting", func(t *testing.T) {
		deltaJSON := `[{"insert":"Red text","attributes":{"color":"#ff0000"}},{"insert":" and "},{"insert":"blue background","attributes":{"background":"#0000ff"}},{"insert":" and "},{"insert":"both","attributes":{"color":"#00ff00","background":"#ffff00"}},{"insert":".\n"}]`
		require.Equal(t, "Red text and blue background and both .", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Font styling", func(t *testing.T) {
		deltaJSON := `[{"insert":"Arial text","attributes":{"font":"arial"}},{"insert":" and "},{"insert":"serif text","attributes":{"font":"serif"}},{"insert":" and "},{"insert":"monospace","attributes":{"font":"monospace"}},{"insert":".\n"}]`
		require.Equal(t, "Arial text and serif text and monospace.", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Size formatting", func(t *testing.T) {
		deltaJSON := `[{"insert":"Small","attributes":{"size":"small"}},{"insert":" "},{"insert":"Large","attributes":{"size":"large"}},{"insert":" "},{"insert":"Huge","attributes":{"size":"huge"}},{"insert":" text.\n"}]`
		require.Equal(t, "Small Large Huge text.", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Superscript and subscript", func(t *testing.T) {
		deltaJSON := `[{"insert":"E=mc"},{"insert":"2","attributes":{"script":"super"}},{"insert":" and H"},{"insert":"2","attributes":{"script":"sub"}},{"insert":"O.\n"}]`
		require.Equal(t, "E=mc 2 and H 2 O.", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Empty operations", func(t *testing.T) {
		deltaJSON := `[]`
		require.Equal(t, "", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Only newlines", func(t *testing.T) {
		deltaJSON := `[{"insert":"\n"},{"insert":"\n"},{"insert":"\n"}]`
		require.Equal(t, "", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Whitespace handling", func(t *testing.T) {
		deltaJSON := `[{"insert":"   Multiple   "},{"insert":"   spaces   "},{"insert":"   here   "},{"insert":"\n"}]`
		require.Equal(t, "Multiple spaces here", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Nested formatting", func(t *testing.T) {
		deltaJSON := `[{"insert":"This is "},{"insert":"bold and italic","attributes":{"bold":true,"italic":true}},{"insert":" and "},{"insert":"underlined bold","attributes":{"bold":true,"underline":true}},{"insert":" text.\n"}]`
		require.Equal(t, "This is bold and italic and underlined bold text.", parseQuillDeltaToPlainText(deltaJSON))
	})
}

func TestParseQuillDeltaToPlainText_WithCustomElements(t *testing.T) {
	t.Parallel()
	t.Run("Text with custom elements", func(t *testing.T) {
		deltaJSON := `[
			{"insert":"Header"},
			{"insert":"\n","attributes":{"header":1}},
			{"insert":"Text before image "},
			{"insert":{"text-editor-single-image":"img123"}},
			{"insert":" text after image.\n"},
			{"insert":"Separator below:\n"},
			{"insert":{"text-editor-separator":"---"}},
			{"insert":"Code block:\n"},
			{"insert":{"text-editor-code":"console.log('hello world')"}},
			{"insert":"Profile mention: "},
			{"insert":{"text-editor-profile":"npub1alice123"}},
			{"insert":"\n"}
		]`
		require.Equal(t, "Header Text before image text after image. Separator below: Code block: Profile mention: console.log('hello world') npub1alice123", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Only custom elements with useful content", func(t *testing.T) {
		deltaJSON := `[
			{"insert":{"text-editor-single-image":"img1"}},
			{"insert":{"text-editor-code":"function test() { return 42; }"}},
			{"insert":{"text-editor-profile":"user789"}}
		]`
		require.Equal(t, "function test() { return 42; } user789", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Only useless custom elements", func(t *testing.T) {
		deltaJSON := `[
			{"insert":{"text-editor-single-image":"img1"}},
			{"insert":{"text-editor-separator":"---"}}
		]`
		require.Equal(t, "", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Invalid JSON", func(t *testing.T) {
		deltaJSON := `[{"insert":{"text-editor-single-image"`
		require.Equal(t, "", parseQuillDeltaToPlainText(deltaJSON))
	})
	t.Run("Example from ICIP-7000 spec", func(t *testing.T) {
		deltaJSON := `[
			{"insert":"Header 1"},
			{"insert":"\n","attributes":{"header":1}},
			{"insert":"Header 2"},
			{"insert":"\n","attributes":{"header":2}},
			{"insert":"Header 3"},
			{"insert":"\n","attributes":{"header":3}},
			{"insert":"Regular "},
			{"insert":"Bold","attributes":{"bold":true}},
			{"insert":" "},
			{"insert":"Italic","attributes":{"italic":true}},
			{"insert":" "},
			{"insert":"Underline","attributes":{"underline":true}},
			{"insert":" "},
			{"insert":"Link wrapped","attributes":{"link":"http://ice.io"}},
			{"insert":" "},
			{"insert":"https://ice.io","attributes":{"link":"https://ice.io"}},
			{"insert":" Image "},
			{"insert":{"text-editor-single-image":"64489600-DB30-4725-A178-A9DDE09061E4/L0/001"}},
			{"insert":" List One"},
			{"insert":"\n","attributes":{"list":"bullet"}},
			{"insert":"Two"},
			{"insert":"\n","attributes":{"list":"bullet"}},
			{"insert":" Quote Some quote"},
			{"insert":"\n","attributes":{"blockquote":true}},
			{"insert":" Mentions: "},
			{"insert":"@ckreioosss","attributes":{"mention":"@ckreioosss"}},
			{"insert":" Hashtags: "},
			{"insert":"#Habits","attributes":{"hashtag":"#Habits"}},
			{"insert":" Separator: "},
			{"insert":{"text-editor-separator":"---"}},
			{"insert":" Code block "},
			{"insert":{"text-editor-code":"8361e203-09ba-4eff-aab8-9c9f06df92d3"}},
			{"insert":"\n"}
		]`
		require.Equal(t, "Header 1 Header 2 Header 3 Regular Bold Italic Underline Link wrapped https://ice.io Image List One Two Quote Some quote Mentions: @ckreioosss Hashtags: #Habits Separator: Code block 8361e203-09ba-4eff-aab8-9c9f06df92d3", parseQuillDeltaToPlainText(deltaJSON))
	})
}
