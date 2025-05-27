// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"fmt"
	"math/rand/v2"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"
	"github.com/schollz/progressbar/v3"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/database/query/internal/connector"
	"github.com/ice-blockchain/subzero/model"
)

type testEvents struct {
	Events []*model.Event
}

func (te *testEvents) Random(h interface{ Helper() }) *model.Event {
	h.Helper()

	idx := int(rand.IntN(len(te.Events)))
	ev := te.Events[idx]
	te.Events = slices.Delete(te.Events, idx, idx+1)

	return ev
}

func helperEnsureDatabaseWithData(t *testing.T, count ...int) (*dbClient, *testEvents) {
	t.Helper()

	var eventCount int

	if len(count) > 0 {
		eventCount = count[0]
	} else {
		eventCount = 100
	}

	db := helperNewDatabase(t)
	helperFillDatabase(t, db, eventCount)

	return db, &testEvents{Events: helperPreloadDataForFilter(t, db)}
}

func helperPreloadDataForFilter(
	t interface {
		Helper()
		Context() context.Context
		require.TestingT
	},
	db *dbClient,
) (events []*model.Event) {
	const stmt = `select
	e.kind,
	e.created_at,
	e.id,
	e.pubkey,
	e.sig,
	e.content,
	e.tags
from
	events e
order by
	random()
limit 1000`

	it := db.newReadEventIterator(t.Context(), stmt, map[string]any{})
	for ev, err := range it {
		require.NoError(t, err)
		events = append(events, ev)
	}

	rand.Shuffle(len(events), func(i, j int) { events[i], events[j] = events[j], events[i] })

	return events
}

func generateHexString() string {
	// The ids, authors, #e and #p filter lists MUST contain exact 64-character lowercase hex values.
	var buf [64]byte

	if _, err := crand.Read(buf[:]); err != nil {
		panic(err)
	}

	return hex.EncodeToString(buf[:])
}

func generateKind() int {
	kinds := []int{
		nostr.KindProfileMetadata,
		nostr.KindTextNote,
		nostr.KindRecommendServer,
		nostr.KindFollowList,
		nostr.KindEncryptedDirectMessage,
		nostr.KindRepost,
		nostr.KindReaction,
		nostr.KindSimpleGroupChatMessage,
		nostr.KindSimpleGroupThread,
		nostr.KindSimpleGroupReply,
		nostr.KindChannelCreation,
		nostr.KindChannelMetadata,
		nostr.KindChannelMessage,
		nostr.KindChannelHideMessage,
		nostr.KindChannelMuteUser,
		nostr.KindPatch,
		nostr.KindFileMetadata,
		nostr.KindSimpleGroupRemoveUser,
		nostr.KindSimpleGroupEditMetadata,
		nostr.KindSimpleGroupDeleteEvent,
		nostr.KindSimpleGroupCreateGroup,
		nostr.KindSimpleGroupJoinRequest,
		nostr.KindZapRequest,
		nostr.KindZap,
		nostr.KindMuteList,
		nostr.KindPinList,
		nostr.KindRelayListMetadata,
		nostr.KindNWCWalletInfo,
		nostr.KindClientAuthentication,
		nostr.KindNWCWalletRequest,
		nostr.KindNWCWalletResponse,
		nostr.KindNostrConnect,
		nostr.KindCategorizedPeopleList,
		nostr.KindCategorizedBookmarksList,
		nostr.KindProfileBadges,
		nostr.KindBadgeDefinition,
		nostr.KindStallDefinition,
		nostr.KindProductDefinition,
		nostr.KindArticle,
		nostr.KindApplicationSpecificData,
		nostr.KindRepositoryAnnouncement,
		nostr.KindSimpleGroupMetadata,
		nostr.KindSimpleGroupAdmins,
		nostr.KindSimpleGroupMembers,

		model.CustomIONKindRepostOfArticle,
		model.CustomIONKindRepostOfEditableTextNote,
	}

	return kinds[rand.IntN(len(kinds))]
}

func generateRandomString(n int) string {
	var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

	if n < 0 {
		panic("invalid length")
	}

	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.IntN(len(letters))]
	}

	return string(b)
}

func generateCreatedAt() int64 {
	const (
		start = 1645680655
		end   = 1740375055
	)

	return rand.Int64N(end-start) + start
}

func helperGenerateEvent(
	t interface {
		require.TestingT
		Helper()
		Context() context.Context
	},
	db *dbClient,
	withTags bool,
) model.Event {
	t.Helper()

	var ev model.Event

	ev.ID = generateHexString()
	ev.PubKey = generateHexString()
	ev.CreatedAt = model.Timestamp(generateCreatedAt())
	ev.Kind = generateKind()
	ev.Content = generateRandomString(rand.IntN(1024))

	if withTags {
		ev.Tags = []model.Tag{
			{"o", generateHexString(), generateRandomString(rand.IntN(20)), generateRandomString(rand.IntN(30))},
			{"p", generateHexString()},
		}
	}

	var req databaseBatchRequest
	require.NoError(t, req.Save(&ev))
	require.NoError(t, db.executeBatch(t.Context(), &req))

	return ev
}

func helperFillDatabase(t *testing.T, client *dbClient, size int) {
	t.Helper()

	eventsCount, err := connector.Get[int](t.Context(), client.db, "select count(*) from events")
	require.NoError(t, err)
	require.NotNil(t, eventsCount)

	if *eventsCount >= size {
		return
	}
	t.Logf("found %d event(s)", *eventsCount)

	need := size - *eventsCount
	t.Logf("generating %d event(s)", need)

	bar := progressbar.Default(int64(need), "generating events")
	for range need {
		bar.Add(1) //nolint:errcheck
		helperGenerateEvent(t, client, true)
	}
}

func TestWhereBuilderByAuthor(t *testing.T) {
	t.Parallel()

	db, ev := helperEnsureDatabaseWithData(t)
	defer db.Close()
	events := helperSelectEvents(t, db,
		model.Filter{
			Authors: []string{ev.Random(t).PubKey, ev.Random(t).PubKey},
		},
		model.Filter{
			Authors: []string{ev.Random(t).PubKey},
		},
	)
	require.Len(t, events, 3)
}

func TestWhereBuilderByID(t *testing.T) {
	t.Parallel()

	db, ev := helperEnsureDatabaseWithData(t)
	defer db.Close()
	events := helperSelectEvents(t, db,
		model.Filter{
			IDs: []string{ev.Random(t).ID},
		},
		model.Filter{
			IDs: []string{ev.Random(t).ID},
		},
	)
	require.Len(t, events, 2)
}

func TestWhereBuilderByMany(t *testing.T) {
	t.Parallel()

	db, ev := helperEnsureDatabaseWithData(t)
	defer db.Close()
	ev1 := ev.Random(t)
	ev2 := ev.Random(t)
	events := helperSelectEvents(t, db,
		model.Filter{
			IDs:     []string{ev1.ID, "bar"},
			Authors: []string{ev1.PubKey, "fooo"},
			Kinds:   []int{ev1.Kind},
		},
		model.Filter{
			IDs:     []string{ev2.ID, "123"},
			Authors: []string{ev2.PubKey},
			Kinds:   []int{ev2.Kind, 1, 2, 3},
			Since:   &ev2.CreatedAt,
			Until:   &ev2.CreatedAt,
		},
	)
	require.Len(t, events, 2)
}

func TestWhereBuilderByTagsNoValuesSingle(t *testing.T) {
	t.Parallel()

	db, ev := helperEnsureDatabaseWithData(t)
	defer db.Close()
	event := ev.Random(t)
	filter := model.Filter{
		IDs:     []string{event.ID},
		Authors: []string{event.PubKey},
	}

	t.Run("Something", func(t *testing.T) {
		events := helperSelectEvents(t, db, filter)
		require.Len(t, events, 1)
	})

	t.Run("Nothing", func(t *testing.T) {
		// Add additional tag to the filter, so query will return no results because all 4 tags MUST be present.
		filter.Tags = model.TagMap{}.Set("x")
		events := helperSelectEvents(t, db, filter)
		require.Empty(t, events)
	})
}

func TestWhereBuilderByTagsSingle(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	t.Run("Fill", func(t *testing.T) {
		helperFillDatabase(t, db, 10)

		var event model.Event
		event.Kind = nostr.KindTextNote
		event.ID = "1"
		event.PubKey = "1"
		event.Tags = model.Tags{{"e", "etag"}, {"p", "ptag"}, {"d", "dtag"}, {"imeta", "m video/mpeg4"}}
		event.CreatedAt = 1

		err := db.AcceptEvents(t.Context(), &event)
		require.NoError(t, err)
	})

	filter := model.Filter{
		IDs: []string{"1"},
		Tags: model.TagMap{}.
			SetLiterals("e", "etag").
			SetLiterals("p", "ptag").
			SetLiterals("d", "dtag"),
	}

	t.Run("Match", func(t *testing.T) {
		events := helperSelectEvents(t, db, filter)
		require.Len(t, events, 1)
	})
	t.Run("Empty", func(t *testing.T) {
		filter.Tags.SetLiterals("x") // Add 4th tag, so query will return no results.
		events := helperSelectEvents(t, db, filter)
		require.Empty(t, events)
	})
}

func TestWhereBuilderByTagsOnlySingle(t *testing.T) {
	t.Parallel()

	db, ev := helperEnsureDatabaseWithData(t)
	defer db.Close()
	event := ev.Random(t)

	filter := model.Filter{
		Tags: model.TagMap{}.SetLiterals(event.Tags[0][0], event.Tags[0][1:]...),
	}

	t.Run("Match", func(t *testing.T) {
		events := helperSelectEvents(t, db, filter)
		require.Len(t, events, 1)
	})
	t.Run("Empty", func(t *testing.T) {
		filter.Tags.SetLiterals("x") // Add 3rd value to the tag list, so query will return no results.
		events := helperSelectEvents(t, db, filter)
		require.Empty(t, events)
	})
}

func TestWhereBuilderByTagsOnlyMulti(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	t.Run("Fill", func(t *testing.T) {
		helperFillDatabase(t, db, 10)

		var event model.Event
		event.Kind = nostr.KindTextNote
		event.ID = "1"
		event.PubKey = "1"
		event.Tags = model.Tags{{"e", "etag"}}
		event.CreatedAt = 1

		err := db.AcceptEvents(t.Context(), &event)
		require.NoError(t, err)

		event.Kind = nostr.KindTextNote
		event.ID = "2"
		event.PubKey = "2"
		event.Tags = model.Tags{{"p", "ptag"}}
		event.CreatedAt = 2

		err = db.AcceptEvents(t.Context(), &event)
		require.NoError(t, err)
	})

	events := helperSelectEvents(t, db,
		model.Filter{
			Tags: model.TagMap{}.SetLiterals("e", "etag"),
		},
		model.Filter{
			Tags: model.TagMap{}.SetLiterals("p", "ptag"),
		},
	)
	require.Len(t, events, 2)
}

func TestSelectEventNoTags(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	id := helperGenerateEvent(t, db, false).ID
	require.NotEmpty(t, id)
	for ev, err := range db.SelectEvents(t.Context(), model.Filter{
		IDs: []string{id},
	}) {
		require.NoError(t, err)
		require.NotNil(t, ev)
		t.Logf("event: %+v", ev)
		require.Equal(t, id, ev.ID)
		require.Empty(t, ev.Tags)
	}

	require.NoError(t, db.Close())
}

func TestSelectByMimeType(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	t.Run("Fill", func(t *testing.T) {
		helperFillDatabase(t, db, 100)

		var event model.Event
		event.Kind = nostr.KindTextNote
		event.ID = "1"
		event.PubKey = "1"
		event.Tags = model.Tags{{"imeta", "m video/mpeg4"}}
		event.CreatedAt = 1

		err := db.AcceptEvents(t.Context(), &event)
		require.NoError(t, err)

		event.Kind = nostr.KindTextNote
		event.ID = "2"
		event.PubKey = "2"
		event.Tags = model.Tags{{"imeta", "m image/png"}}
		event.CreatedAt = 2

		err = db.AcceptEvents(t.Context(), &event)
		require.NoError(t, err)
	})
	t.Run("QueryNoImeta", func(t *testing.T) {
		count, err := db.CountEvents(t.Context(), model.Filter{
			Search: "videos:false images:false",
		})
		require.NoError(t, err)
		require.Equal(t, int64(100), count)
	})
	t.Run("Image", func(t *testing.T) {
		events := helperSelectEvents(t, db, model.Filter{
			Search: "images:true",
		})
		require.Len(t, events, 1)
		require.Equal(t, "2", events[0].ID)
	})
	t.Run("Video", func(t *testing.T) {
		events := helperSelectEvents(t, db, model.Filter{
			Search: "videos:true",
		})
		require.Lenf(t, events, 1, "expected 1 video event, got %d", len(events))
		require.Equal(t, "1", events[0].ID)
	})
	t.Run("Media", func(t *testing.T) {
		events := helperSelectEvents(t, db, model.Filter{
			Search: "media:true",
		})
		require.Len(t, events, 2, "expected 2 events with media tags, got %d", len(events))
	})
	t.Run("No Media", func(t *testing.T) {
		events := helperSelectEvents(t, db, model.Filter{
			Search: "media:false",
		})
		require.Len(t, events, 100)
	})
	t.Run("VideoByID", func(t *testing.T) {
		count, err := db.CountEvents(t.Context(), model.Filter{
			Search: "videos:true",
			IDs:    []string{"1"},
		})
		require.NoError(t, err)
		require.Equal(t, int64(1), count)
	})
	t.Run("NoVideo", func(t *testing.T) {
		count, err := db.CountEvents(t.Context(), model.Filter{
			Search: "videos:false",
		})
		require.NoError(t, err)
		require.Equal(t, int64(101), count)
	})
}

func TestSelectQuotesReferences(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	t.Run("Fill", func(t *testing.T) {
		helperFillDatabase(t, db, 100)

		var event model.Event
		event.Kind = nostr.KindTextNote
		event.ID = "1"
		event.PubKey = "1"
		event.Tags = model.Tags{{"q", "fooo"}, {"bar", "foo"}}
		event.CreatedAt = 1

		err := db.AcceptEvents(t.Context(), &event)
		require.NoError(t, err)

		event.Kind = nostr.KindTextNote
		event.ID = "2"
		event.PubKey = "2"
		event.Tags = model.Tags{{"e", "fooo"}, {"foo", "bar"}}
		event.CreatedAt = 1

		err = db.AcceptEvents(t.Context(), &event)
		require.NoError(t, err)
	})
	t.Run("SelectQuotes", func(t *testing.T) {
		count, err := db.CountEvents(t.Context(), model.Filter{
			Search: "quotes:true",
		})
		require.NoError(t, err)
		require.Equal(t, int64(1), count)
	})
	t.Run("SelectReferences", func(t *testing.T) {
		count, err := db.CountEvents(t.Context(), model.Filter{
			Search: "references:true",
		})
		require.NoError(t, err)
		require.Equal(t, int64(1), count)
	})
	t.Run("SelectReferencesAndQuotes", func(t *testing.T) {
		count, err := db.CountEvents(t.Context(), model.Filter{
			Search: "references:true quotes:true",
			IDs:    []string{"1", "2"},
		})
		require.NoError(t, err)
		require.Equal(t, int64(0), count)
	})
	t.Run("SelectReferencesAndQuotesUnknownID", func(t *testing.T) {
		count, err := db.CountEvents(t.Context(), model.Filter{
			Search: "references:true quotes:true",
			IDs:    []string{"5", "6"},
		})
		require.NoError(t, err)
		require.Zero(t, count)
	})
	t.Run("SelectQuotesByID", func(t *testing.T) {
		count, err := db.CountEvents(t.Context(), model.Filter{
			Search: "quotes:true",
			IDs:    []string{"1", "2"},
		})
		require.NoError(t, err)
		require.Equal(t, int64(1), count)
	})
	t.Run("SelectNonQuotes", func(t *testing.T) {
		count, err := db.CountEvents(t.Context(), model.Filter{
			Search: "quotes:false",
		})
		require.NoError(t, err)
		require.Equal(t, int64(101), count)
	})
	t.Run("SelectNonQuoteByID", func(t *testing.T) {
		count, err := db.CountEvents(t.Context(), model.Filter{
			Search: "quotes:false",
			IDs:    []string{"1"},
		})
		require.NoError(t, err)
		require.Zero(t, count)
	})
	t.Run("SelectAll", func(t *testing.T) {
		count, err := db.CountEvents(t.Context())
		require.NoError(t, err)
		require.Equal(t, int64(102), count)
	})
}

func helperCountExpiredEvents(t *testing.T, client *dbClient) int {
	t.Helper()

	count, err := connector.Get[int](t.Context(), client.db,
		`select count(*) from events WHERE expiration <= get_current_timestamp_nano()`,
	)
	require.NoError(t, err)
	require.NotNil(t, count)

	return *count
}

func TestSelectEventsExpiration(t *testing.T) {
	t.Parallel()

	db, events := helperEnsureDatabaseWithData(t)
	defer db.Close()

	t.Run("Fill", func(t *testing.T) {
		var event model.Event
		event.Kind = nostr.KindTextNote
		event.ID = "expired"
		event.PubKey = "1"
		event.Tags = model.Tags{{"expiration", strconv.FormatInt(time.Now().Unix()-0xff, 10)}, {"q", "fooo"}}
		event.CreatedAt = 1

		err := db.AcceptEvents(t.Context(), &event)
		require.NoError(t, err)

		event.Kind = nostr.KindTextNote
		event.ID = "alive"
		event.PubKey = "2"
		event.Tags = model.Tags{{"expiration", strconv.FormatInt(time.Now().Unix()+0xff, 10)}, {"e", "bar"}}
		event.CreatedAt = 1

		err = db.AcceptEvents(t.Context(), &event)
		require.NoError(t, err)
	})
	t.Run("All", func(t *testing.T) {
		count, err := db.CountEvents(t.Context())
		require.NoError(t, err)
		require.Equal(t, int64(102), count)
	})
	t.Run("WithoutExpiration", func(t *testing.T) {
		count, err := db.CountEvents(t.Context(), model.Filter{
			Search: "expiration:false",
		})
		require.NoError(t, err)
		require.Equal(t, int64(100), count)
	})
	t.Run("WithoutExpirationByID", func(t *testing.T) {
		ev := events.Random(t)
		count, err := db.CountEvents(t.Context(), model.Filter{
			Search:  "expiration:false",
			IDs:     []string{ev.ID},
			Authors: []string{ev.PubKey},
		})
		require.NoError(t, err)
		require.Equal(t, int64(1), count)
	})
	t.Run("Expired", func(t *testing.T) {
		for ev, er := range db.SelectEvents(t.Context(), model.Filter{
			Search: "expiration:false",
			IDs:    []string{"expired"},
		}) {
			require.NoError(t, er)
			t.Logf("expired event: %+v", ev)
		}

		count, err := db.CountEvents(t.Context(), model.Filter{
			Search: "expiration:false",
			IDs:    []string{"expired"},
		})
		require.NoError(t, err)
		require.Equal(t, int64(0), count)
	})
	t.Run("NotExpired", func(t *testing.T) {
		count, err := db.CountEvents(t.Context(), model.Filter{
			Search: "expiration:true",
		})
		require.NoError(t, err)
		require.Equal(t, int64(1), count)
	})
	t.Run("NotExpiredByID", func(t *testing.T) {
		count, err := db.CountEvents(t.Context(), model.Filter{
			Kinds:  []int{nostr.KindTextNote},
			Search: "expiration:true",
			IDs:    []string{"alive"},
		})
		require.NoError(t, err)
		require.Equal(t, int64(1), count)
	})
	t.Run("Fill by expired events", func(t *testing.T) {
		for i := range 100 {
			var event model.Event
			event.Kind = nostr.KindTextNote
			event.ID = fmt.Sprintf("expired:%v", i)
			event.PubKey = "1"
			event.Tags = model.Tags{{"expiration", strconv.FormatInt(time.Now().Unix()-int64(i), 10)}, {"q", "fooo"}}
			event.CreatedAt = 1

			err := db.AcceptEvents(t.Context(), &event)
			require.NoError(t, err)
		}
	})
	t.Run("Delete expired events", func(t *testing.T) {
		require.Equal(t, 101, helperCountExpiredEvents(t, db))
		err := db.deleteExpiredEvents(t.Context())
		require.NoError(t, err)
		require.Zero(t, helperCountExpiredEvents(t, db))
	})
}

func TestSelectWithExtensions(t *testing.T) {
	t.Parallel()

	db, events := helperEnsureDatabaseWithData(t)
	defer db.Close()

	t.Run("Fill", func(t *testing.T) {
		var event model.Event
		event.Kind = nostr.KindTextNote
		event.ID = "expired"
		event.PubKey = "1"
		event.Tags = model.Tags{{"expiration", "1"}, {"q", "fooo"}}
		event.CreatedAt = 1

		err := db.AcceptEvents(t.Context(), &event)
		require.NoError(t, err)

		event.Kind = nostr.KindTextNote
		event.ID = "alive"
		event.PubKey = "2"
		event.Tags = model.Tags{{"expiration", "2177366400"}, {"e", "bar"}}
		event.CreatedAt = 1

		err = db.AcceptEvents(t.Context(), &event)
		require.NoError(t, err)
	})
	t.Run("AliveAndE", func(t *testing.T) {
		count, err := db.CountEvents(t.Context(), model.Filter{
			IDs:    []string{"alive"},
			Search: "expiration:true references:true",
		})
		require.NoError(t, err)
		require.Equal(t, int64(1), count)
	})
	t.Run("ExpiredAndQ", func(t *testing.T) {
		count, err := db.CountEvents(t.Context(), model.Filter{
			Search: "expiration:off quotes:on",
		})
		require.NoError(t, err)
		require.Zero(t, count)
	})
	t.Run("NoEAndNoQ", func(t *testing.T) {
		count, err := db.CountEvents(t.Context(), model.Filter{
			Search: "quotes:false references:false",
		})
		require.NoError(t, err)
		require.Equal(t, int64(100), count)
	})
	t.Run("IdNoTags", func(t *testing.T) {
		ev := events.Random(t)
		count, err := db.CountEvents(t.Context(), model.Filter{
			IDs:    []string{ev.ID},
			Search: "quotes:true",
		})
		require.NoError(t, err)
		require.Zero(t, count)
	})
}

func TestSelectRepostWithReference(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	t.Run("Fill", func(t *testing.T) {
		var event model.Event
		event.Kind = nostr.KindRepost
		event.ID = "1"
		event.PubKey = "1"
		event.Tags = model.Tags{{"e", "fooo"}, {"bar", "foo"}}
		event.Content = `{"id":"3","pubkey":"4","created_at":1712594952,"kind":1,"tags":[["imeta","url https://example.com/foo.jpg","ox f63ccef25fcd9b9a181ad465ae40d282eeadd8a4f5c752434423cb0539f73e69 https://nostr.build","x f9c8b660532a6e8236779283950d875fbfbdc6f4dbc7c675bc589a7180299c30","m image/jpeg","dim 1066x1600","bh L78C~=$%0%ERjENbWX$g0jNI}:-S","blurhash L78C~=$%0%ERjENbWX$g0jNI}:-S"]],"content":"foo","sig":"sig"}`
		event.CreatedAt = 1

		err := db.AcceptEvents(t.Context(), &event)
		require.NoError(t, err)
	})
	t.Run("Reference extension must be ignored for reposts", func(t *testing.T) {
		count, err := db.CountEvents(t.Context(), model.Filter{
			Search: "references:false",
		})
		require.NoError(t, err)
		require.Equal(t, int64(1), count)
	})
}

func TestSelectFilterKind6AsKind1(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	t.Run("Fill", func(t *testing.T) {
		helperFillDatabase(t, db, 10)

		var event model.Event
		event.Kind = nostr.KindRepost
		event.ID = "2"
		event.PubKey = "2"
		event.Tags = model.Tags{{"e", "2"}}
		event.CreatedAt = 2
		event.Content = `{"id":"3","pubkey":"4","created_at":1712594952,"kind":1,"tags":[["imeta","url https://example.com/foo.jpg","ox f63ccef25fcd9b9a181ad465ae40d282eeadd8a4f5c752434423cb0539f73e69 https://nostr.build","x f9c8b660532a6e8236779283950d875fbfbdc6f4dbc7c675bc589a7180299c30","m image/jpeg","dim 1066x1600","bh L78C~=$%0%ERjENbWX$g0jNI}:-S","blurhash L78C~=$%0%ERjENbWX$g0jNI}:-S"]],"content":"foo","sig":"sig"}`

		err := db.AcceptEvents(t.Context(), &event)
		require.NoError(t, err)
	})
	t.Run("SelectRepost", func(t *testing.T) {
		filter := model.Filter{
			Search: "images:yes",
		}
		t.Run("Count", func(t *testing.T) {
			count, err := db.CountEvents(t.Context(), filter)
			require.NoError(t, err)
			require.Equal(t, int64(1), count)
		})
		t.Run("Select", func(t *testing.T) {
			for ev, err := range db.SelectEvents(t.Context(), filter) {
				require.NoError(t, err)
				require.NotNil(t, ev)
				t.Logf("event: %+v", ev)
				require.Equal(t, "2", ev.ID)
			}
		})
	})
}

func helperMustGetPrecalculatedCounters(t *testing.T, db *dbClient, filters ...model.Filter) int64 {
	t.Helper()

	where, params, err := newQueryBuilder().BuildForPrecalculatedCounters(filters...)
	require.NoError(t, err, filters)

	counter, err := connector.GetNamed[int64](t.Context(), db.db,
		`select coalesce(sum(value), 0) from event_counters where `+where, params,
	)
	if errors.Is(err, connector.ErrNotFound) {
		err = nil
		counter = model.PointerOf[int64](0)
	}
	require.NoErrorf(t, err, "failed to prepare statement where: %v", where)
	require.NotNil(t, counter)

	t.Logf("Precalculated count result:\n\tWhere: %v\n\tParams: %+v\n\tCounter: %v", where, params, *counter)

	return *counter
}

func TestWhereBuilderSyntaxForCounter(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	t.Run("Reply", func(t *testing.T) {
		t.Run("Reply", func(t *testing.T) {
			helperMustGetPrecalculatedCounters(t, db, model.Filter{Kinds: []int{nostr.KindTextNote}, Tags: model.TagMap{}.SetLiterals("e", "1")})
		})
		t.Run("Quote", func(t *testing.T) {
			helperMustGetPrecalculatedCounters(t, db, model.Filter{Kinds: []int{nostr.KindTextNote}, Tags: model.TagMap{}.SetLiterals("q", "1")})
		})
	})
	t.Run("Repost", func(t *testing.T) {
		helperMustGetPrecalculatedCounters(t, db, model.Filter{Kinds: []int{nostr.KindRepost}, Tags: model.TagMap{}.SetLiterals("e", "1")})
	})
	t.Run("Reaction", func(t *testing.T) {
		helperMustGetPrecalculatedCounters(t, db, model.Filter{Kinds: []int{nostr.KindReaction}, Tags: model.TagMap{}.SetLiterals("e", "1", "2")})
	})
	t.Run("Followers", func(t *testing.T) {
		helperMustGetPrecalculatedCounters(t, db, model.Filter{Kinds: []int{nostr.KindFollowList}, Tags: model.TagMap{}.SetLiterals("e", "1")})
	})
	t.Run("Multiple", func(t *testing.T) {
		helperMustGetPrecalculatedCounters(t, db,
			model.Filter{Kinds: []int{nostr.KindFollowList}, Tags: model.TagMap{}.SetLiterals("p", "1")},
			model.Filter{Kinds: []int{nostr.KindRepost}, Tags: model.TagMap{}.SetLiterals("e", "1")},
		)
	})
}

func TestTagMarkerWithRepost(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	t.Run("Insert", func(t *testing.T) {
		var event model.Event
		event.Kind = nostr.KindGenericRepost
		event.ID = "1"
		event.PubKey = "1"
		event.Tags = model.Tags{{"e", "1"}, {"q", "2"}}
		event.Content = `{"id":"3","pubkey":"4","created_at":1712594952,"kind":1,"tags":[["imeta","url https://example.com/foo.jpg","ox f63ccef25fcd9b9a181ad465ae40d282eeadd8a4f5c752434423cb0539f73e69 https://nostr.build"], ["e", "foo", "", "root"]],"content":"foo","sig":"sig"}`
		event.CreatedAt = 1

		err := db.AcceptEvents(t.Context(), &event)
		require.NoError(t, err)
	})
	t.Run("Select", func(t *testing.T) {
		events := helperSelectEvents(t, db, model.Filter{Search: "emarker:root"})
		require.Len(t, events, 1)
		require.Equal(t, "1", events[0].ID)
	})
}

func TestFilterTagsNegative(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	t.Run("Insert", func(t *testing.T) {
		err := db.AcceptEvents(t.Context(),
			&model.Event{
				Event: nostr.Event{
					ID:        "1id",
					Kind:      nostr.KindTextNote,
					CreatedAt: 1,
					Content:   "foo",
					PubKey:    "fookey",
					Tags: model.Tags{
						{"e", "2", "", "root"},
						{"q", "2"},
					},
				},
			},
			&model.Event{
				Event: nostr.Event{
					ID:        "2id",
					Kind:      nostr.KindTextNote,
					CreatedAt: 2,
					Content:   "bar",
					PubKey:    "barkey",
					Tags: model.Tags{
						{"e", "2", "", "reply"},
						{"q", "2"},
					},
				},
			},
		)
		require.NoError(t, err)
		count, err := db.CountEvents(t.Context())
		require.NoError(t, err)
		require.Equal(t, int64(2), count)
	})
	t.Run("TagMarker", func(t *testing.T) {
		events := helperSelectEvents(t, db, model.Filter{Search: "!emarker:root"})
		require.Len(t, events, 1)
		require.Equal(t, "2id", events[0].ID)
	})
	t.Run("Tags", func(t *testing.T) {
		events := helperSelectEvents(t, db, model.Filter{
			Tags: model.TagMap{}.Set("!e", nil, nil, model.PointerOf("root")),
		})
		require.Len(t, events, 1)
		require.Equal(t, "2id", events[0].ID)
	})
}

func TestGetReplyTypeFromValues(t *testing.T) {
	t.Parallel()

	require.Empty(t, getReplyTypeFromValues(nil))
	require.Empty(t, getReplyTypeFromValues([]model.TagValues{}))
	require.Empty(t, getReplyTypeFromValues([]model.TagValues{{}}))
	require.Empty(t, getReplyTypeFromValues([]model.TagValues{{nil, nil}}))
	require.Equal(t, "root", getReplyTypeFromValues([]model.TagValues{{nil, nil, model.PointerOf("root")}}))
	require.Equal(t, "reply", getReplyTypeFromValues([]model.TagValues{{nil, nil, model.PointerOf("reply")}}))
}

func TestCommunityEventsLookup(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	var event, event2 model.Event
	event.Kind = nostr.KindTextNote
	event.ID = "1"
	event.PubKey = "1"

	event2.Kind = nostr.KindTextNote
	event2.ID = "2"
	event2.PubKey = "2"
	event2.Tags = model.Tags{{model.CustomIONTagCommunity, "foo"}}

	require.NoError(t, db.AcceptEvents(t.Context(), &event, &event2))
	eventsNoFilter := helperSelectEvents(t, db)
	require.Len(t, eventsNoFilter, 1)

	events := helperSelectEvents(t, db, model.Filter{IDs: []string{"1", "2"}, Limit: 3})
	require.Equal(t, 1, len(events)) // Only one event with ID 1.
	require.Equal(t, "1", events[0].ID)

	eventsWithCommunity := helperSelectEvents(t, db, model.Filter{Tags: model.TagMap{}.Set(model.CustomIONTagCommunity), Limit: 3})
	require.Equal(t, 1, len(eventsWithCommunity)) // Only one event with tag "h".
	require.Equal(t, "2", eventsWithCommunity[0].ID)
}

func TestBuilderLookupByAddress(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	var eventRegular, eventAddressable model.Event
	eventRegular.Kind = nostr.KindTextNote
	eventRegular.ID = "1"
	eventRegular.PubKey = "1"
	eventRegular.CreatedAt = 1
	eventRegular.Content = "foo"
	eventRegular.Tags = model.Tags{}

	eventAddressable.Kind = model.CustomIONKindEditableTextNote
	eventAddressable.ID = "2"
	eventAddressable.PubKey = "2"
	eventAddressable.CreatedAt = 2
	eventAddressable.Content = "bar"
	eventAddressable.Tags = model.Tags{{"d", "test-addressable"}}

	err := db.AcceptEvents(t.Context(), &eventRegular, &eventAddressable)
	require.NoError(t, err)

	events := helperSelectEvents(t, db, model.Filter{
		Addresses: []string{eventRegular.Address(), eventAddressable.Address()},
	})
	require.Equal(t, []*model.Event{&eventAddressable, &eventRegular}, events)
}
