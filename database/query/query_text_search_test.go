// SPDX-License-Identifier: ice License 1.0

package query

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	combinations "github.com/mxschmitt/golang-combinations"
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
		expectedEvents = append(expectedEvents, &model.Event{
			Event: nostr.Event{
				ID:        "normal" + uuid.NewString(),
				PubKey:    "end" + uuid.NewString(),
				CreatedAt: nostr.Now(),
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
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Tags:      model.Tags{},
				Content:   "post",
				Sig:       "bogus" + uuid.NewString(),
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
				CreatedAt: nostr.Now(),
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
		require.EqualValues(t, expectedEvents[0], stored[1])
		require.EqualValues(t, expectedEvents[2], stored[0])
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
		require.Len(t, stored, 0)
	})
}

func TestSearchEvents_EditableTextNote(t *testing.T) {
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
		expectedEvents = append(expectedEvents, &model.Event{
			Event: nostr.Event{
				ID:        "normal" + uuid.NewString(),
				PubKey:    "end" + uuid.NewString(),
				CreatedAt: nostr.Now(),
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
				CreatedAt: nostr.Now(),
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
				CreatedAt: nostr.Now(),
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
				Kinds:  []int{nostr.KindGenericRepost},
				Tags:   model.TagMap{}.SetLiterals("k", strconv.Itoa(model.CustomIONKindEditableTextNote)),
				Search: `"lala"`,
				Limit:  20,
			},
		}
		stored := helperSelectEvents(t, db, filters...)
		require.Len(t, stored, 2)
		require.EqualValues(t, expectedEvents[1], stored[0])
		require.EqualValues(t, expectedEvents[0], stored[1])
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

func TestSearchEvents_KindProfileMetadata_SpecialChars(t *testing.T) {
	t.Parallel()
	db := helperNewDatabase(t)
	defer db.Close()

	expectedEvents := []*model.Event{}

	t.Run("Events with profile metadata kind", func(t *testing.T) {
		for range 3 {
			randomName := helperGenerateRandomStringWithSpecialChars(t, 10)
			randomDisplayName := helperGenerateRandomStringWithSpecialChars(t, 10)
			t.Logf("randomName: %s, randomDisplayName: %s", randomName, randomDisplayName)

			event := &model.Event{
				Event: nostr.Event{
					ID:        uuid.NewString(),
					PubKey:    uuid.NewString(),
					CreatedAt: nostr.Now(),
					Kind:      nostr.KindProfileMetadata,
					Tags:      model.Tags{},
					Content:   `{"name":"` + randomName + `","display_name":"` + randomDisplayName + `"}`,
					Sig:       uuid.NewString(),
				},
			}
			expectedEvents = append(expectedEvents, event)
			require.NoError(t, db.AcceptEvents(t.Context(), event))
		}

		stored := helperSelectEvents(t, db, model.Filter{
			Kinds: []int{nostr.KindProfileMetadata},
		})
		require.Len(t, stored, 3)
		for i := 0; i < len(expectedEvents); i++ {
			require.EqualValues(t, expectedEvents[len(expectedEvents)-i-1], stored[i])
		}
	})

	t.Run("search profile by name with special characters", func(t *testing.T) {
		name, _ := helperExtractProfileMetadataFields(t, expectedEvents[0].Event.Content)
		if len(name) > 1 {
			name = name[0 : len(name)/2]
		}
		t.Logf("searchTerm: %s", name)

		stored := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindProfileMetadata},
			Search: `"` + name + `"`,
		})
		require.Len(t, stored, 1)
		require.EqualValues(t, expectedEvents[0], stored[0])
	})

	t.Run("search profile by display_name with special characters", func(t *testing.T) {
		_, displayName := helperExtractProfileMetadataFields(t, expectedEvents[0].Event.Content)
		if len(displayName) > 1 {
			displayName = displayName[0 : len(displayName)/2]
		}

		stored := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindProfileMetadata},
			Search: `"` + displayName + `"`,
		})
		require.Len(t, stored, 1)
		require.EqualValues(t, expectedEvents[0], stored[0])
	})
}

func helperExtractProfileMetadataFields(t *testing.T, content string) (string, string) {
	t.Helper()

	var parsedContent model.ProfileMetadataContent
	require.NoError(t, json.Unmarshal([]byte(content), &parsedContent))

	return parsedContent.Name, parsedContent.DisplayName
}

func helperGenerateRandomStringWithSpecialChars(t *testing.T, length int) string {
	t.Helper()
	specialChars := "!@#$%^&*()-_=+[]{}|;:,.<>?/~`"
	allChars := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 " + specialChars
	var result strings.Builder
	for i := 0; i < length; i++ {
		idx := int(rand.IntN(len(allChars)))
		result.WriteByte(allChars[idx])
	}

	return result.String()
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

		require.EqualValues(t, expectedEvents[2], stored[2])
		require.EqualValues(t, expectedEvents[1], stored[1])
		require.EqualValues(t, expectedEvents[0], stored[0])
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

		require.EqualValues(t, expectedEvents[2], stored[2])
		require.EqualValues(t, expectedEvents[1], stored[1])
		require.EqualValues(t, expectedEvents[0], stored[0])
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
		require.Equal(t, "id2", stored[0].ID)
		require.Equal(t, "id1", stored[1].ID)

		stored = helperSelectEvents(t, db, model.Filter{
			IDs:    []string{"id2"},
			Search: `"not" include:dependencies:kind1>kind0`,
			Limit:  100,
		})
		require.Len(t, stored, 2)
		require.Equal(t, "id2", stored[0].ID)
		require.Equal(t, "id1", stored[1].ID)

		stored = helperSelectEvents(t, db, model.Filter{
			IDs:    []string{"id2"},
			Search: `"not include:dependencies:kind1>kind0`,
			Limit:  100,
		})
		require.Len(t, stored, 2)
		require.Equal(t, "id1", stored[1].ID)
		require.Equal(t, "id2", stored[0].ID)

		stored = helperSelectEvents(t, db, model.Filter{
			IDs:    []string{"id2"},
			Search: `"not" include:dependencies:kind1>kind0`,
			Limit:  100,
		})
		require.Len(t, stored, 2)
		require.Equal(t, "id1", stored[1].ID)
		require.Equal(t, "id2", stored[0].ID)

		stored = helperSelectEvents(t, db, model.Filter{
			IDs:    []string{"id2"},
			Search: `""not include:dependencies:kind1>kind0`,
			Limit:  100,
		})
		require.Len(t, stored, 2)
		require.Equal(t, "id1", stored[1].ID)
		require.Equal(t, "id2", stored[0].ID)

		stored = helperSelectEvents(t, db, model.Filter{
			IDs:    []string{"id2"},
			Search: `not" include:dependencies:kind1>kind0`,
			Limit:  100,
		})
		require.Len(t, stored, 2)
		require.Equal(t, "id1", stored[1].ID)
		require.Equal(t, "id2", stored[0].ID)
	})
}

func TestSearchEvents_Replace_Update(t *testing.T) {
	t.Parallel()
	db := helperNewDatabase(t)
	defer db.Close()

	initialEvent := &model.Event{
		Event: nostr.Event{
			ID:        "initial" + uuid.NewString(),
			PubKey:    "pubkey123",
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: nostr.Tags{
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
			Sig:     "sig123",
		},
	}
	require.NoError(t, db.AcceptEvents(t.Context(), initialEvent))

	t.Run("search initial event", func(t *testing.T) {
		searchResult := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindTextNote},
			Search: `"initial"`,
		})
		require.Len(t, searchResult, 1)
		require.EqualValues(t, initialEvent, searchResult[0])
	})

	updatedEvent := &model.Event{
		Event: nostr.Event{
			ID:        initialEvent.ID,
			PubKey:    "pubkey123",
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: nostr.Tags{
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
			Sig:     "sig123_updated",
		},
	}
	require.NoError(t, db.AcceptEvents(t.Context(), updatedEvent))

	storedUpdated := helperSelectEvents(t, db, model.Filter{
		Kinds: []int{nostr.KindTextNote},
	})
	require.Len(t, storedUpdated, 1)
	require.EqualValues(t, updatedEvent, storedUpdated[0])

	t.Run("search updated event", func(t *testing.T) {
		searchResult := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindTextNote},
			Search: `"updated"`,
		})
		require.Len(t, searchResult, 1)
		require.EqualValues(t, updatedEvent, searchResult[0])
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
			filter.Search = fmt.Sprintf("%q", generateRandomString(3)) + filter.Search
			sql, params, err := db.generateSelectEventsSQL(t.Context(), model.Filters{filter}, 0, 100)
			require.NoErrorf(t, err, "failed to generate select events sql for set #%d (%#v)", i+1, set)
			sql = "EXPLAIN QUERY PLAN " + sql
			stmt, err := db.prepare(t.Context(), sql, hashSQL(sql))
			require.NoError(t, err)

			rows, err := stmt.QueryContext(t.Context(), params)
			require.NoError(t, err)
			var hasIndex bool
			for rows.Next() {
				var s1, s2, s3, s4 string
				err := rows.Scan(&s1, &s2, &s3, &s4)
				require.NoError(t, err)
				op[s4]++
				if strings.Contains(s4, "SEARCH e USING INDEX") {
					hasIndex = true
				}
				if s4 == "USE TEMP B-TREE FOR ORDER BY" || (strings.HasPrefix(s4, "SCAN ") && !strings.Contains(s4, "INDEX")) {
					if strings.Contains(filter.Search, "Expiration:true") {
						// It uses SCAN over CTE, which is expected.
						continue
					} else if (hasIndex || len(filter.Authors) > 0) && s4 == "USE TEMP B-TREE FOR ORDER BY" {
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

func TestFts5DeleteNestedEvents(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	rootPriv := model.GeneratePrivateKey()
	var root model.Event

	t.Run("Add events", func(t *testing.T) {
		// Root event.
		root.CreatedAt = 1
		root.Kind = nostr.KindTextNote
		root.Content = "root event"
		require.NoError(t, root.SignWithAlg(rootPriv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, db.AcceptEvents(t.Context(), &root))

		// First level events.
		var ev1 model.Event
		ev1.CreatedAt = 2
		ev1.Kind = nostr.KindTextNote
		ev1.Content = "regular event"
		ev1.Tags = model.Tags{{"e", root.ID}}
		require.NoError(t, ev1.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, db.AcceptEvents(t.Context(), &ev1))

		var ev2 model.Event
		ev2.CreatedAt = 3
		ev2.Kind = nostr.KindArticle
		ev2.Content = "addressable event"
		ev2.Tags = model.Tags{
			{"d", "article1"},
			{"e", root.ID},
		}
		require.NoError(t, ev2.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, db.AcceptEvents(t.Context(), &ev2))

		var ev3 model.Event
		ev3.CreatedAt = 4
		ev3.Kind = nostr.KindProfileMetadata
		ev3.Content = "replaceable event"
		ev3.Tags = model.Tags{{"e", root.ID}}
		require.NoError(t, ev3.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, db.AcceptEvents(t.Context(), &ev3))

		// Second level events.
		var ev4 model.Event
		ev4.CreatedAt = 5
		ev4.Kind = nostr.KindTextNote
		ev4.Content = "regular child event"
		ev4.Tags = model.Tags{{"e", ev1.ID}}
		require.NoError(t, ev4.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, db.AcceptEvents(t.Context(), &ev4))

		var ev5 model.Event
		ev5.CreatedAt = 6
		ev5.Kind = nostr.KindArticle
		ev5.Content = "addressable child event"
		ev5.Tags = model.Tags{
			{"d", "article2"},
			{"a", ev2.Address()},
		}
		require.NoError(t, ev5.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, db.AcceptEvents(t.Context(), &ev5))

		var ev6 model.Event
		ev6.CreatedAt = 7
		ev6.Kind = nostr.KindProfileMetadata
		ev6.Content = `{"name":"replaceable child event"}`
		ev6.Tags = model.Tags{
			{"a", ev3.Address()},
		}
		require.NoError(t, ev6.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, db.AcceptEvents(t.Context(), &ev6))
	})

	events := helperSelectEvents(t, db)
	require.Len(t, events, 7)
	stored := helperSelectEvents(t, db, model.Filter{
		Kinds:  []int{nostr.KindTextNote, nostr.KindProfileMetadata},
		Search: `"child"`,
		Limit:  100,
	})
	require.Len(t, stored, 2)

	// Delete root event.
	var rootDelete model.Event
	rootDelete.CreatedAt = 8
	rootDelete.Kind = nostr.KindDeletion
	rootDelete.Content = "delete root event"
	rootDelete.Tags = model.Tags{{"e", root.ID}}
	require.NoError(t, rootDelete.SignWithAlg(rootPriv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, db.AcceptEvents(t.Context(), &rootDelete))

	require.Zero(t, len(helperSelectEvents(t, db)))
	stored = helperSelectEvents(t, db, model.Filter{
		Kinds:  []int{nostr.KindTextNote, nostr.KindProfileMetadata},
		Search: `"child"`,
		Limit:  100,
	})
	require.Zero(t, stored, 0)
}
