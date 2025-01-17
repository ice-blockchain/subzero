// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	combinations "github.com/mxschmitt/golang-combinations"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func TestSearchEvents_KindTextNote(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()

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
			"alt a,lt1 text",
			"summary dummy summa;:,ry1 content",
			fmt.Sprintf("x %x", []byte("https://alicerelay.example.com")),
			fmt.Sprintf("ox %x", []byte("https://alicerelay.example.com")),
		})
		expectedEvents = append(expectedEvents, &model.Event{
			Event: nostr.Event{
				ID:        "normal" + uuid.NewString(),
				PubKey:    "end" + uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindTextNote,
				Tags:      tags1,
				Content:   "end, and, ond",
				Sig:       "end" + uuid.NewString(),
			},
		})
		require.NoError(t, db.AcceptEvents(ctx, expectedEvents[0]))

		expectedEvents = append(expectedEvents, &model.Event{
			Event: nostr.Event{
				ID:        "normal, 2nd event" + uuid.NewString(),
				PubKey:    "bogus" + uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindTextNote,
				Tags:      model.Tags{},
				Content:   "bogus",
				Sig:       "bogus" + uuid.NewString(),
			},
		})
		require.NoError(t, db.AcceptEvents(ctx, expectedEvents[1]))

		var tags2 nostr.Tags
		tags2 = append(tags2, nostr.Tag{
			"imeta",
			"url https://alicerelay.example.com",
			"m image/jpg",
			"dim 3024x4032",
			"i foobar",
			"alt al.t2 text",
			"summary dummy sum,&*mary2 content",
			fmt.Sprintf("x %x", []byte("https://alicerelay.example.com")),
			fmt.Sprintf("ox %x", []byte("https://alicerelay.example.com")),
		})
		expectedEvents = append(expectedEvents, &model.Event{
			Event: nostr.Event{
				ID:        searchID,
				PubKey:    searchPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindTextNote,
				Tags:      tags2,
				Content:   "bogusssss",
				Sig:       "bogusssss" + uuid.NewString(),
			},
		})
		require.NoError(t, db.AcceptEvents(ctx, expectedEvents[2]))

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
		require.Len(t, stored, 2)
		require.EqualValues(t, expectedEvents[2], stored[1])
		require.EqualValues(t, expectedEvents[1], stored[0])
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
	t.Run("Create events with text note kind with imeta alt and summary tags", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				ID:        "normal" + uuid.NewString(),
				PubKey:    searchPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindDeletion,
				Tags:      nostr.Tags{{"e", searchID}},
				Sig:       "end" + uuid.NewString(),
			},
		}
		require.NoError(t, db.AcceptEvents(ctx, ev))
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindTextNote},
			Search: `"summ"`,
			Authors: []string{
				searchPubkey,
			},
		})
		require.Len(t, stored, 0)
		require.NoError(t, db.AcceptEvents(ctx, ev))
		require.Len(t, stored, 0)
	})
}

func TestSearchEvents_KindProfileMetadata(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()

	db := helperNewDatabase(t)
	defer db.Close()
	expectedEvents := []*model.Event{}
	t.Run("Events with profile metadata kind", func(t *testing.T) {
		expectedEvents = append(expectedEvents, &model.Event{
			Event: nostr.Event{
				ID:        "normal" + uuid.NewString(),
				PubKey:    "ev1" + uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindProfileMetadata,
				Tags:      model.Tags{},
				Content:   `{"name":"abcd","display_name":"efgh"}`,
				Sig:       "ev1" + uuid.NewString(),
			},
		})
		require.NoError(t, db.AcceptEvents(ctx, expectedEvents[0]))

		expectedEvents = append(expectedEvents, &model.Event{
			Event: nostr.Event{
				ID:        "normal, 2nd event" + uuid.NewString(),
				PubKey:    "ev2" + uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindProfileMetadata,
				Tags:      model.Tags{},
				Content:   `{"name":"xyuz","display_name":"sprtq"}`,
				Sig:       "ev2" + uuid.NewString(),
			},
		})
		require.NoError(t, db.AcceptEvents(ctx, expectedEvents[1]))

		expectedEvents = append(expectedEvents, &model.Event{
			Event: nostr.Event{
				ID:        "normal, 3nd event" + uuid.NewString(),
				PubKey:    "ev3" + uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindProfileMetadata,
				Tags:      model.Tags{},
				Content:   ``,
				Sig:       "ev3" + uuid.NewString(),
			},
		})
		require.NoError(t, db.AcceptEvents(ctx, expectedEvents[2]))

		stored := helperSelectEvents(t, db, model.Filter{
			Kinds: []int{nostr.KindProfileMetadata},
		})
		require.Len(t, stored, 3)
		require.EqualValues(t, expectedEvents[2], stored[0])
		require.EqualValues(t, expectedEvents[1], stored[1])
		require.EqualValues(t, expectedEvents[0], stored[2])
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
		require.EqualValues(t, expectedEvents[2], stored[0])
		require.EqualValues(t, expectedEvents[1], stored[1])
		require.EqualValues(t, expectedEvents[0], stored[2])
	})
}

func TestSearchEvents_KindFileMetadata(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()

	db := helperNewDatabase(t)
	defer db.Close()
	expectedEvents := []*model.Event{}
	t.Run("Events with KindFileMetadata", func(t *testing.T) {
		var tags1 model.Tags
		tags1 = append(tags1, nostr.Tag{
			"imeta",
			"url https://alicerelay.example.com",
			"m image/jpg",
			"dim 3024x4032",
			"i foobar",
			"alt a,lt1 text",
			"summary dummy summa;:,ry1 content",
			fmt.Sprintf("x %x", []byte("https://alicerelay.example.com")),
			fmt.Sprintf("ox %x", []byte("https://alicerelay.example.com")),
		})
		expectedEvents = append(expectedEvents, &model.Event{
			Event: nostr.Event{
				ID:        "ev1" + uuid.NewString(),
				PubKey:    "ev1" + uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindFileMetadata,
				Tags:      tags1,
				Sig:       "ev1" + uuid.NewString(),
			},
		})
		require.NoError(t, db.AcceptEvents(ctx, expectedEvents[0]))

		var tags2 model.Tags
		tags2 = append(tags2, nostr.Tag{
			"imeta",
			"url https://alicerelay.example.com",
			"m image/jpg",
			"dim 3024x4032",
			"i foobar",
			"alt a,lt2 text",
			"summary dummy summa;:,ry2 content",
			fmt.Sprintf("x %x", []byte("https://alicerelay.example.com")),
			fmt.Sprintf("ox %x", []byte("https://alicerelay.example.com")),
		})
		expectedEvents = append(expectedEvents, &model.Event{
			Event: nostr.Event{
				ID:        "ev2" + uuid.NewString(),
				PubKey:    "ev2" + uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindFileMetadata,
				Tags:      tags2,
				Sig:       "ev2" + uuid.NewString(),
			},
		})
		require.NoError(t, db.AcceptEvents(ctx, expectedEvents[1]))

		var tags3 model.Tags
		tags3 = append(tags3, nostr.Tag{
			"imeta",
			"url https://alicerelay.example.com",
			"m image/jpg",
			"dim 3024x4032",
			"i foobar",
			"alt a,lt3 text",
			"summary dummy summa;:,ry3 content",
			fmt.Sprintf("x %x", []byte("https://alicerelay.example.com")),
			fmt.Sprintf("ox %x", []byte("https://alicerelay.example.com")),
		})
		expectedEvents = append(expectedEvents, &model.Event{
			Event: nostr.Event{
				ID:        "ev3" + uuid.NewString(),
				PubKey:    "ev3" + uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindFileMetadata,
				Tags:      tags3,
				Sig:       "ev3" + uuid.NewString(),
			},
		})
		require.NoError(t, db.AcceptEvents(ctx, expectedEvents[2]))

		stored := helperSelectEvents(t, db, model.Filter{
			Kinds: []int{nostr.KindFileMetadata},
		})
		require.Len(t, stored, 3)
		require.EqualValues(t, expectedEvents[2], stored[0])
		require.EqualValues(t, expectedEvents[1], stored[1])
		require.EqualValues(t, expectedEvents[0], stored[2])
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

		require.EqualValues(t, expectedEvents[2], stored[0])
		require.EqualValues(t, expectedEvents[1], stored[1])
		require.EqualValues(t, expectedEvents[0], stored[2])
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

		require.EqualValues(t, expectedEvents[2], stored[0])
		require.EqualValues(t, expectedEvents[1], stored[1])
		require.EqualValues(t, expectedEvents[0], stored[2])
	})
}

func TestSearchEvents_KindRepost(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()

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
			"alt a,lt1 text",
			"summary dummy summa;:,ry1 content",
			fmt.Sprintf("x %x", []byte("https://alicerelay.example.com")),
			fmt.Sprintf("ox %x", []byte("https://alicerelay.example.com")),
		})
		expectedEvents = append(expectedEvents, &model.Event{
			Event: nostr.Event{
				ID:        "normal" + uuid.NewString(),
				PubKey:    "end" + uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindTextNote,
				Tags:      tags1,
				Content:   "end, and, ond",
				Sig:       "end" + uuid.NewString(),
			},
		})
		require.NoError(t, db.AcceptEvents(ctx, expectedEvents[0]))

		expectedEvents = append(expectedEvents, &model.Event{
			Event: nostr.Event{
				ID:        "normal" + uuid.NewString(),
				PubKey:    "ev1" + uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindRepost,
				Tags:      model.Tags{},
				Content:   expectedEvents[0].String(),
				Sig:       "ev1" + uuid.NewString(),
			},
		})
		require.NoError(t, db.AcceptEvents(ctx, expectedEvents[1]))

		stored := helperSelectEvents(t, db, model.Filter{
			Kinds: []int{nostr.KindTextNote},
		})
		require.Len(t, stored, 1)
		require.EqualValues(t, expectedEvents[0], stored[0])

		stored = helperSelectEvents(t, db, model.Filter{
			Kinds: []int{nostr.KindRepost},
		})
		require.Len(t, stored, 1)
		require.EqualValues(t, expectedEvents[1], stored[0])
	})
	t.Run("search repost by repost value", func(t *testing.T) {
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindRepost},
			Search: `"end"`,
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
		err := db.AcceptEvents(context.Background(), &ev)
		require.NoError(t, err)

		ev.ID = "id2"
		ev.Kind = nostr.KindTextNote
		ev.PubKey = "pk1"
		ev.CreatedAt = 2
		ev.Content = "content of the note"
		err = db.AcceptEvents(context.Background(), &ev)
		require.NoError(t, err)

		stored := helperSelectEvents(t, db, model.Filter{
			IDs:    []string{"id2"},
			Search: `include:dependencies:kind1>kind0`,
		})
		require.Len(t, stored, 2)
		require.Equal(t, "id2", stored[0].ID)
		require.Equal(t, "id1", stored[1].ID)

		stored = helperSelectEvents(t, db, model.Filter{
			IDs:    []string{"id2"},
			Search: `"not" include:dependencies:kind1>kind0`,
		})
		require.Len(t, stored, 2)
		require.Equal(t, "id1", stored[0].ID)
		require.Equal(t, "id2", stored[1].ID)
	})
}

func TestSearchEvents_Replace(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()

	db := helperNewDatabase(t)
	defer db.Close()
	expectedEvents := []*model.Event{}
	expectedEvents = append(expectedEvents, &model.Event{
		Event: nostr.Event{
			ID:        "normal" + uuid.NewString(),
			PubKey:    "end" + uuid.NewString(),
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindTextNote,
			Tags: nostr.Tags{
				{
					"imeta",
					"url https://alicerelay.example.com",
					"m image/jpg",
					"dim 3024x4032",
					"i foobar",
					"alt a,lt1 text",
					"summary dummy summa;:,ry1 content",
					fmt.Sprintf("x %x", []byte("https://alicerelay.example.com")),
					fmt.Sprintf("ox %x", []byte("https://alicerelay.example.com")),
				},
			},
			Content: "end, and, ond",
			Sig:     "end" + uuid.NewString(),
		},
	})
	t.Run("Create events with text note kind with imeta alt and summary tags", func(t *testing.T) {
		require.NoError(t, db.AcceptEvents(ctx, expectedEvents[0]))
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds: []int{nostr.KindTextNote},
		})
		require.Len(t, stored, 1)
		require.EqualValues(t, expectedEvents[0], stored[0])
	})
	t.Run("search event end", func(t *testing.T) {
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindTextNote},
			Search: `"end"`,
		})
		require.Len(t, stored, 1)
		require.EqualValues(t, expectedEvents[0], stored[0])
	})
	require.NoError(t, db.AcceptEvents(ctx, expectedEvents[0]))
	t.Run("search event end", func(t *testing.T) {
		stored := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindTextNote},
			Search: `"end"`,
		})
		require.Len(t, stored, 1)
		require.EqualValues(t, expectedEvents[0], stored[0])
	})
}

func TestQuerySearchFuzzNoUseTempBTREEOrScan(t *testing.T) {
	t.Parallel()

	var sets [][]*structElement
	t.Run("PrepareSets", func(t *testing.T) {
		var filter model.Filter

		fields := helperParseFilterStruct(t, reflect.TypeOf(filter), nil)
		sets = combinations.All(fields)
		t.Logf("found %d total combination(s)", len(sets))
		slices.SortStableFunc(sets, func(i, j []*structElement) int {
			if len(i) < len(j) {
				return -1
			}
			if len(i) > len(j) {
				return 1
			}
			return 0
		})
	})

	db := helperNewDatabase(t)
	defer db.Close()
	helperFillDatabase(t, db, 100)

	op := make(map[string]int)

	t.Run("Fuzz", func(t *testing.T) {
		for i, set := range sets {
			filter := helperNewFilterFromElements(t, set)
			filter.Search = fmt.Sprintf(`"%v"`, generateRandomString(3)) + filter.Search
			sql, params, err := db.generateSelectEventsSQL(context.TODO(), model.Filters{filter}, 0, 100)
			require.NoErrorf(t, err, "failed to generate select events sql for set #%d (%#v)", i+1, set)
			sql = "EXPLAIN QUERY PLAN " + sql
			stmt, err := db.prepare(context.Background(), sql, hashSQL(sql))
			require.NoError(t, err)

			rows, err := stmt.QueryContext(context.Background(), params)
			require.NoError(t, err)
			var hasPK bool
			for rows.Next() {
				var s1, s2, s3, s4 string
				err := rows.Scan(&s1, &s2, &s3, &s4)
				require.NoError(t, err)
				op[s4]++
				if strings.Contains(s4, "SEARCH e USING PRIMARY KEY") {
					hasPK = true
				}
				if s4 == "USE TEMP B-TREE FOR ORDER BY" || (strings.HasPrefix(s4, "SCAN ") && !strings.Contains(s4, "INDEX")) {
					if strings.Contains(filter.Search, "Expiration:true") {
						// It uses SCAN over CTE, which is expected.
						continue
					} else if (hasPK || len(filter.Authors) > 0) && s4 == "USE TEMP B-TREE FOR ORDER BY" {
						// Allow B-TREE for ORDER BY if there are multiple authors or PK is used.
						continue
					}
					t.Logf("filter: %#v", filter)
					t.Logf("set #%d: %s (%+v)", i+1, sql, params)
					t.Log(s1, s2, s3, s4)
					t.FailNow()
				}
			}
			rows.Close()
		}
	})

	t.Run("OpSummary", func(t *testing.T) {
		keys := make([]string, 0, len(op))
		for k := range op {
			keys = append(keys, k)
		}
		slices.SortStableFunc(keys, func(i, j string) int {
			if op[i] > op[j] {
				return -1
			}
			if op[i] < op[j] {
				return 1
			}
			return 0
		})
		t.Log("Operations Summary:")
		for _, k := range keys {
			t.Logf("%s: %d", k, op[k])
		}
	})
}

func TestFts5CleanupText(t *testing.T) {
	t.Parallel()

	t.Run("Usual", func(t *testing.T) {
		require.Equal(t, "", fts5CleanupText(""))
		require.Equal(t, "a", fts5CleanupText("a"))
		require.Equal(t, "a b", fts5CleanupText("a b"))
	})
	t.Run("Multiple spacs", func(t *testing.T) {
		require.Equal(t, "a b", fts5CleanupText("a        b   "))
	})
	t.Run("With punctuation", func(t *testing.T) {
		require.Equal(t, "a b", fts5CleanupText("a, b"))
		require.Equal(t, "a b", fts5CleanupText("a; b"))
		require.Equal(t, "a b", fts5CleanupText("a. b"))
		require.Equal(t, "a b", fts5CleanupText("a* b"))
		require.Equal(t, "a b", fts5CleanupText("a& b"))
		require.Equal(t, "a b", fts5CleanupText("a% b"))
		require.Equal(t, "a b", fts5CleanupText("a# b"))
		require.Equal(t, "a b", fts5CleanupText("a? b"))
		require.Equal(t, "a b", fts5CleanupText("a? b $"))
	})
	t.Run("nostr", func(t *testing.T) {
		require.Equal(t, "a b", fts5CleanupText("npub112412951925 a npub112412951925 b npub112412951925"))
		require.Equal(t, "a b", fts5CleanupText("nsec35235236622 a nsec35235236622 b nsec35235236622"))
		require.Equal(t, "a b", fts5CleanupText("nprofile35235236622 a nprofile35235236622 b nprofile35235236622"))
		require.Equal(t, "a b", fts5CleanupText("nostr:nprofile35235236622 a nostr:nprofile35235236622 b nostr:nprofile35235236622"))
	})
	t.Run("hashes", func(t *testing.T) {
		require.Equal(t, "a b", fts5CleanupText("a b #some"))
		require.Equal(t, "a b", fts5CleanupText("#hash a #testhash b #some #some2 #some_hash"))
	})
}
