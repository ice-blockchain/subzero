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
	t.Run("create events", func(t *testing.T) {
		var tags1 model.Tags
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
				Content:   "Hello world! This is my first message. Looking forward to connecting with everyone.",
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
				Content:   "Just posted a new update about my project progress!",
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
				Content:   "Exploring blockchain technology and decentralized networks. Very exciting developments!",
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
	t.Run("search by content - blockchain", func(t *testing.T) {
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindTextNote},
			Search: `"blockchain"`,
		})
		require.Len(t, stored, 1)
		require.EqualValues(t, expectedEvents[2], stored[0])
	})
	t.Run("search by content - blockchain", func(t *testing.T) {
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindTextNote},
			Search: `"bl"`,
		})
		require.Len(t, stored, 1)
		require.EqualValues(t, expectedEvents[2], stored[0])
	})
	t.Run("search kind text note by content and pubkey", func(t *testing.T) {
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindTextNote},
			Search: `"blockchain"`,
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
				Search: `"blockchain"`,
			},
			model.Filter{
				Kinds:  []int{nostr.KindTextNote},
				Search: `"Hello"`,
			},
		}
		stored := helperSelectEvents(t, db, filters...)
		require.Len(t, stored, 2)
	})
	t.Run("contains search - partial word in middle", func(t *testing.T) {
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindTextNote},
			Search: `"ckcha"`,
		})
		require.Len(t, stored, 1)
		require.EqualValues(t, expectedEvents[2], stored[0])
	})
	t.Run("contains search - partial content", func(t *testing.T) {
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindTextNote},
			Search: `"project"`,
		})
		require.Len(t, stored, 1)
		require.EqualValues(t, expectedEvents[1], stored[0])
	})
	t.Run("contains search - case insensitive", func(t *testing.T) {
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindTextNote},
			Search: `"HELLO"`,
		})
		require.Len(t, stored, 1)
		require.EqualValues(t, expectedEvents[0], stored[0])
	})
	t.Run("contains search - beginning of word", func(t *testing.T) {
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindTextNote},
			Search: `"decen"`,
		})
		require.Len(t, stored, 1)
		require.EqualValues(t, expectedEvents[2], stored[0])
	})
	t.Run("contains search - end of word", func(t *testing.T) {
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindTextNote},
			Search: `"world"`,
		})
		require.Len(t, stored, 1)
		require.EqualValues(t, expectedEvents[0], stored[0])
	})
	t.Run("contains search - no results", func(t *testing.T) {
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindTextNote},
			Search: `"nonexistent"`,
		})
		require.Len(t, stored, 0)
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
			Search: `"blockchain"`,
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
				Content:   "Test pos 12345\n",
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
				Content:   "lalalala hey",
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
		helperEventsMatch(t, []*model.Event{expectedEvents[1], expectedEvents[0]}, stored)
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
		helperEventsMatch(t, expectedEvents, stored)
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
		helperEventsMatch(t, expectedEvents, stored)
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
	require.EqualValues(t, updatedEvent.Event, storedUpdated[0].Event)

	t.Run("search updated event", func(t *testing.T) {
		searchResult := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindArticle},
			Search: `"updated"`,
		})
		require.Len(t, searchResult, 1)
		require.EqualValues(t, updatedEvent.Event, searchResult[0].Event)
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

func TestPrepareSearchContent_WithRichText(t *testing.T) {
	t.Parallel()
	t.Run("TextNote with rich text", func(t *testing.T) {
		deltaJSON := `[{"insert":"Rich text content "},{"insert":"with bold","attributes":{"bold":true}},{"insert":"\n"}]`
		var ev model.Event
		ev.Kind = nostr.KindTextNote
		ev.Content = "Plain text content"
		ev.Tags = model.Tags{
			{model.CustomIONTagRichText, model.QuillDeltaProtocol, deltaJSON},
		}
		require.Equal(t, "Plain text content", prepareSearchContent(&ev))
	})
	t.Run("TextNote with empty content and rich text", func(t *testing.T) {
		deltaJSON := `[{"insert":"Rich text content "},{"insert":"with bold","attributes":{"bold":true}},{"insert":"\n"}]`
		var ev model.Event
		ev.Kind = nostr.KindTextNote
		ev.Content = ""
		ev.Tags = model.Tags{
			{model.CustomIONTagRichText, model.QuillDeltaProtocol, deltaJSON},
		}
		require.Equal(t, "Rich text content with bold", prepareSearchContent(&ev))
	})
	t.Run("EditableTextNote with rich text", func(t *testing.T) {
		deltaJSON := `[{"insert":"Editable rich content\n"}]`
		var ev model.Event
		ev.Kind = model.CustomIONKindEditableTextNote
		ev.Content = "Editable plain content"
		ev.Tags = model.Tags{
			{model.CustomIONTagRichText, model.QuillDeltaProtocol, deltaJSON},
		}
		require.Equal(t, "Editable plain content", prepareSearchContent(&ev))
	})
	t.Run("EditableTextNote with empty content and rich text", func(t *testing.T) {
		deltaJSON := `[{"insert":"Editable rich content\n"}]`
		var ev model.Event
		ev.Kind = model.CustomIONKindEditableTextNote
		ev.Content = ""
		ev.Tags = model.Tags{
			{model.CustomIONTagRichText, model.QuillDeltaProtocol, deltaJSON},
		}
		require.Equal(t, "Editable rich content", prepareSearchContent(&ev))
	})
	t.Run("Article with rich text", func(t *testing.T) {
		deltaJSON := `[{"insert":"Article rich content\n"}]`
		var ev model.Event
		ev.Kind = nostr.KindArticle
		ev.Content = "Article plain content"
		ev.Tags = model.Tags{
			{model.CustomIONTagRichText, model.QuillDeltaProtocol, deltaJSON},
		}
		require.Equal(t, "Article plain content", prepareSearchContent(&ev))
	})
	t.Run("Article with empty content and rich text", func(t *testing.T) {
		deltaJSON := `[{"insert":"Article rich content\n"}]`
		var ev model.Event
		ev.Kind = nostr.KindArticle
		ev.Content = ""
		ev.Tags = model.Tags{
			{model.CustomIONTagRichText, model.QuillDeltaProtocol, deltaJSON},
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
			{model.CustomIONTagRichText, model.QuillDeltaProtocol, deltaJSON},
		}
		require.Equal(t, "testuser Test User", prepareSearchContent(&ev))
	})
	t.Run("Complex Quill Delta with various formats", func(t *testing.T) {
		deltaJSON := `[{"insert":"Main Title"},{"insert":"\n","attributes":{"header":1}},{"insert":"Some "},{"insert":"bold","attributes":{"bold":true}},{"insert":" text and "},{"insert":"italic","attributes":{"italic":true}},{"insert":" text.\n"},{"insert":"List item 1"},{"insert":"\n","attributes":{"list":"bullet"}},{"insert":"List item 2 with "},{"insert":"link","attributes":{"link":"https://example.com"}},{"insert":"\n","attributes":{"list":"bullet"}},{"insert":"Image: "},{"insert":{"image":"https://example.com/img.jpg"}},{"insert":"\n"},{"insert":"Quote text"},{"insert":"\n","attributes":{"blockquote":true}}]`
		var ev model.Event
		ev.Kind = nostr.KindArticle
		ev.Content = "Markdown version of content"
		ev.Tags = model.Tags{
			{model.CustomIONTagRichText, model.QuillDeltaProtocol, deltaJSON},
		}
		require.Equal(t, "Markdown version of content", prepareSearchContent(&ev))
	})
	t.Run("Complex Quill Delta with empty content", func(t *testing.T) {
		deltaJSON := `[{"insert":"Main Title"},{"insert":"\n","attributes":{"header":1}},{"insert":"Some "},{"insert":"bold","attributes":{"bold":true}},{"insert":" text and "},{"insert":"italic","attributes":{"italic":true}},{"insert":" text.\n"},{"insert":"List item 1"},{"insert":"\n","attributes":{"list":"bullet"}},{"insert":"List item 2"},{"insert":"\n","attributes":{"list":"bullet"}}]`
		var ev model.Event
		ev.Kind = nostr.KindArticle
		ev.Content = ""
		ev.Tags = model.Tags{
			{model.CustomIONTagRichText, model.QuillDeltaProtocol, deltaJSON},
		}
		require.Equal(t, "Main Title Some bold text and italic text. List item 1 List item 2", prepareSearchContent(&ev))
	})
}

func TestSearchEvents_ComprehensiveMultilingualSearch(t *testing.T) {
	t.Parallel()

	privKey, _ := model.GenerateKeyPair()

	db := helperNewDatabase(t)
	defer db.Close()

	testCases := []struct {
		name           string
		content        string
		searchTerms    []string
		substringTerms []string
		failTerms      []string
		language       string
		description    string
	}{
		{
			name:           "Japanese_Hiragana_Katakana_Kanji",
			content:        "こんにちは世界！カタカナでテストします。日本語の検索機能をテストしています。",
			searchTerms:    []string{"こんにちは", "世界", "カタカナ", "日本語"},
			substringTerms: []string{"こんに", "世", "カタ", "日本"},
			failTerms:      []string{"英語", "中国", "韓国", "ドイツ"},
			language:       "ja",
			description:    "Japanese mixed scripts (Hiragana, Katakana, Kanji)",
		},
		{
			name:           "Chinese_Simplified",
			content:        "你好世界！这是中文简体字测试。搜索功能正常工作。",
			searchTerms:    []string{"你好", "世界", "中文", "简体字", "搜索"},
			substringTerms: []string{"你", "世", "中", "简体", "搜"},
			failTerms:      []string{"英文", "日语", "韩语", "德语"},
			language:       "zh-CN",
			description:    "Chinese Simplified characters",
		},
		{
			name:           "Chinese_Traditional",
			content:        "你好世界！這是中文繁體字測試。搜索功能正常工作。",
			searchTerms:    []string{"你好", "世界", "中文", "繁體字", "搜索"},
			substringTerms: []string{"你", "世", "中", "繁體", "搜"},
			failTerms:      []string{"英文", "日語", "韓語", "德語"},
			language:       "zh-TW",
			description:    "Chinese Traditional characters",
		},
		{
			name:           "Arabic_With_Diacritics",
			content:        "مرحباً بالعالم! هذا اختبار للنص العربي. البحث يعمل بشكل صحيح.",
			searchTerms:    []string{"مرحباً", "بالعالم", "العربي", "البحث"},
			substringTerms: []string{"مرح", "بال", "العرب", "البح"},
			failTerms:      []string{"الإنجليزية", "الفرنسية", "الألمانية", "الروسية"},
			language:       "ar",
			description:    "Arabic with diacritics and complex forms",
		},
		{
			name:           "Hebrew_With_Vowels",
			content:        "שלום עולם! זהו מבחן לטקסט עברי. החיפוש עובד כראוי.",
			searchTerms:    []string{"שלום", "עולם", "עברי", "החיפוש"},
			substringTerms: []string{"של", "עול", "עבר", "החיפ"},
			failTerms:      []string{"אנגלית", "צרפתית", "גרמנית", "רוסית"},
			language:       "he",
			description:    "Hebrew with vowel marks",
		},
		{
			name:           "Russian_Cyrillic",
			content:        "Привет мир! Это тест русского текста. Поиск работает правильно.",
			searchTerms:    []string{"Привет", "русского", "текста", "работает"},
			substringTerms: []string{"Прив", "русск", "текс", "работ"},
			failTerms:      []string{"английский", "французский", "немецкий", "японский"},
			language:       "ru",
			description:    "Russian Cyrillic script",
		},
		{
			name:           "Hindi_Devanagari_Complex",
			content:        "नमस्ते दुनिया! यह हिंदी पाठ का परीक्षण है। खोज कार्यक्षमता सही तरीके से काम करती है।",
			searchTerms:    []string{"नमस्ते", "दुनिया", "हिंदी", "परीक्षण", "कार्यक्षमता"},
			substringTerms: []string{"नमस", "दुनि", "हिंद", "परीक्ष", "कार्यक्षम"},
			failTerms:      []string{"अंग्रेजी", "फ्रेंच", "जर्मन", "रूसी"},
			language:       "hi",
			description:    "Hindi Devanagari with complex conjuncts and matras",
		},
		{
			name:           "French_Diacritics_Complete",
			content:        "Bonjour le monde! Voici un test avec des caractères spéciaux: café, naïve, être, où, ça marche très bien.",
			searchTerms:    []string{"café", "naïve", "être", "où", "très"},
			substringTerms: []string{},
			failTerms:      []string{"cafe", "naive", "etre", "xyz", "tres"},
			language:       "fr",
			description:    "French with all major diacritics",
		},
	}

	var events []*model.Event
	for i, tc := range testCases {
		event := &model.Event{
			Event: nostr.Event{
				ID:        fmt.Sprintf("test-%d-%s", i, uuid.New().String()[:8]),
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Tags:      model.Tags{},
				Content:   tc.content,
			},
		}
		event.SignWithAlg(privKey, model.SignAlgEDDSA, model.KeyAlgCurve25519)
		events = append(events, event)
		require.NoError(t, db.AcceptEvents(t.Context(), event))
	}
	for i, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("Testing: %s", tc.description)
			t.Logf("Content: %s", tc.content)

			foundCount := 0
			for _, term := range tc.searchTerms {
				stored := helperSelectEvents(t, db, model.Filter{
					Kinds:  []int{nostr.KindTextNote},
					Search: fmt.Sprintf(`"%s"`, term),
					IDs:    []string{events[i].ID},
				})
				if len(stored) > 0 {
					t.Logf("✅ FOUND (expected full term): '%s'", term)
					foundCount++
				} else {
					t.Errorf("❌ NOT FOUND (should be found): '%s' in %s", term, tc.language)
				}
			}
			substringFoundCount := 0
			for _, term := range tc.substringTerms {
				stored := helperSelectEvents(t, db, model.Filter{
					Kinds:  []int{nostr.KindTextNote},
					Search: fmt.Sprintf(`"%s"`, term),
					IDs:    []string{events[i].ID},
				})

				if len(stored) > 0 {
					t.Logf("✅ FOUND (expected substring): '%s'", term)
					substringFoundCount++
				} else {
					t.Errorf("❌ NOT FOUND (substring should be found): '%s' in %s", term, tc.language)
				}
			}

			notFoundCount := 0
			for _, term := range tc.failTerms {
				stored := helperSelectEvents(t, db, model.Filter{
					Kinds:  []int{nostr.KindTextNote},
					Search: fmt.Sprintf(`"%s"`, term),
					IDs:    []string{events[i].ID},
				})

				if len(stored) == 0 {
					t.Logf("✅ NOT FOUND (expected, term not in text): '%s'", term)
					notFoundCount++
				} else {
					t.Errorf("❌ FOUND (should not be found, term not in text): '%s' in %s", term, tc.language)
				}
			}

			expectedFound := len(tc.searchTerms)
			expectedSubstringFound := len(tc.substringTerms)
			expectedNotFound := len(tc.failTerms)

			if foundCount != expectedFound {
				t.Errorf("Full term search failed for %s: found %d/%d expected terms",
					tc.language, foundCount, expectedFound)
			}
			if substringFoundCount != expectedSubstringFound {
				t.Errorf("Substring search failed for %s: found %d/%d expected substrings",
					tc.language, substringFoundCount, expectedSubstringFound)
			}
			if notFoundCount != expectedNotFound {
				t.Errorf("Negative search failed for %s: correctly not found %d/%d expected absent terms",
					tc.language, notFoundCount, expectedNotFound)
			}
		})
	}

	t.Run("Edge_Cases", func(t *testing.T) {
		edgeCases := []struct {
			name        string
			content     string
			searchTerm  string
			shouldFind  bool
			description string
		}{
			{
				name:        "Hindi_Halant_Preservation",
				content:     "नमस्ते दोस्त",
				searchTerm:  "नमस्ते",
				shouldFind:  true,
				description: "Hindi halant (्) must be preserved",
			},
			{
				name:        "Arabic_Shadda_Preservation",
				content:     "مرحبّا بكم",
				searchTerm:  "مرحبّا",
				shouldFind:  true,
				description: "Arabic shadda (ّ) must be preserved",
			},
			{
				name:        "French_Cedilla_Exact",
				content:     "français garçon",
				searchTerm:  "français",
				shouldFind:  true,
				description: "French cedilla (ç) must be exact",
			},
			{
				name:        "French_Without_Cedilla_Should_Not_Match",
				content:     "français garçon",
				searchTerm:  "francais",
				shouldFind:  false,
				description: "Without cedilla should not match",
			},
		}

		for _, ec := range edgeCases {
			t.Run(ec.name, func(t *testing.T) {
				event := &model.Event{
					Event: nostr.Event{
						ID:        "edge-" + uuid.New().String()[:8],
						CreatedAt: nostr.Now(),
						Kind:      nostr.KindTextNote,
						Tags:      model.Tags{},
						Content:   ec.content,
					},
				}
				event.SignWithAlg(privKey, model.SignAlgEDDSA, model.KeyAlgCurve25519)
				require.NoError(t, db.AcceptEvents(t.Context(), event))

				stored := helperSelectEvents(t, db, model.Filter{
					Kinds:  []int{nostr.KindTextNote},
					Search: fmt.Sprintf(`"%s"`, ec.searchTerm),
					IDs:    []string{event.ID},
				})

				found := len(stored) > 0
				if found == ec.shouldFind {
					if found {
						t.Logf("✅ FOUND (expected): '%s' - %s", ec.searchTerm, ec.description)
					} else {
						t.Logf("✅ NOT FOUND (expected): '%s' - %s", ec.searchTerm, ec.description)
					}
				} else {
					if ec.shouldFind {
						t.Errorf("❌ Should find but didn't: '%s' - %s", ec.searchTerm, ec.description)
					} else {
						t.Errorf("❌ Should not find but did: '%s' - %s", ec.searchTerm, ec.description)
					}
				}
			})
		}
	})
}
