// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/jmoiron/sqlx"
	"github.com/nbd-wtf/go-nostr"
	"github.com/schollz/progressbar/v3"
	"github.com/stretchr/testify/require"
	"pgregory.net/rand"

	"github.com/ice-blockchain/subzero/model"
)

type testEvents struct {
	Events []*model.Event
}

func (te *testEvents) Random(h interface{ Helper() }) *model.Event {
	h.Helper()

	idx := int(rand.Int31n(int32(len(te.Events))))
	ev := te.Events[idx]
	te.Events = slices.Delete(te.Events, idx, idx+1)

	return ev
}

func helperEnsureDatabase(t *testing.T) (*dbClient, *testEvents) {
	t.Helper()

	const eventCount = 100

	db := helperNewDatabase(t)
	helperFillDatabase(t, db, eventCount)

	return db, &testEvents{Events: helperPreloadDataForFilter(t, db)}
}

func helperPreloadDataForFilter(
	t interface {
		Helper()
		require.TestingT
	},
	db *dbClient,
) (events []*model.Event) {
	const stmt = `select
	e.kind,
	e.created_at,
	e.system_created_at,
	e.id,
	e.pubkey,
	e.sig,
	e.content,
	'[]' as tags,
	(select json_group_array(json_array(event_tag_key, event_tag_value1,event_tag_value2,event_tag_value3,event_tag_value4)) from event_tags where event_id = e.id) as jtags
from
	events e
order by
	random()
limit 1000`

	it := &eventIterator{
		oneShot: true,
		fetch: func(int64) (*sqlx.Rows, error) {
			stmt, err := db.prepare(context.TODO(), stmt, hashSQL(stmt))
			if err != nil {
				return nil, errors.Wrapf(err, "failed to prepare query sql: %v", stmt)
			}

			return stmt.QueryxContext(context.TODO(), map[string]any{})
		}}

	err := it.Each(context.TODO(), func(ev *model.Event) error {
		events = append(events, ev)

		return nil
	})
	require.NoError(t, err)
	rand.ShuffleSlice(nil, events)

	return events
}

func generateHexString() string {
	// The ids, authors, #e and #p filter lists MUST contain exact 64-character lowercase hex values.
	var buf [64]byte

	if _, err := rand.Read(buf[:]); err != nil {
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
		nostr.KindDeletion,
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
		nostr.KindSimpleGroupAddUser,
		nostr.KindSimpleGroupRemoveUser,
		nostr.KindSimpleGroupEditMetadata,
		nostr.KindSimpleGroupAddPermission,
		nostr.KindSimpleGroupRemovePermission,
		nostr.KindSimpleGroupDeleteEvent,
		nostr.KindSimpleGroupEditGroupStatus,
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
	}

	return kinds[rand.Intn(len(kinds))]
}

func generateRandomString(n int) string {
	var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

	if n < 0 {
		panic("invalid length")
	}

	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}

	return string(b)
}

func generateCreatedAt() int64 {
	const (
		start = 1645680655
		end   = 1740375055
	)

	return rand.Int63n(end-start) + start
}

func helperGenerateEvent(
	t interface {
		require.TestingT
		Helper()
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
	ev.Content = generateRandomString(rand.Intn(1024))

	if withTags {
		ev.Tags = []model.Tag{
			{"#e", generateHexString(), generateRandomString(rand.Intn(20)), generateRandomString(rand.Intn(30))},
			{"#p", generateHexString()},
			{"#d", generateHexString(), generateRandomString(rand.Intn(10))},
		}
	}

	err := db.saveEvent(context.Background(), &ev)
	require.NoError(t, err)

	return ev
}

func helperFillDatabase(t *testing.T, db *dbClient, size int) {
	t.Helper()

	var eventsCount []int
	err := db.Select(&eventsCount, "select count(*) from events")
	require.NoError(t, err)

	if eventsCount[0] >= size {
		return
	}
	t.Logf("found %d event(s)", eventsCount[0])

	need := size - eventsCount[0]
	t.Logf("generating %d event(s)", need)

	bar := progressbar.Default(int64(need), "generating events")
	for range need {
		bar.Add(1) //nolint:errcheck
		helperGenerateEvent(t, db, true)
	}
}

func TestWhereBuilderByAuthor(t *testing.T) {
	t.Parallel()

	db, ev := helperEnsureDatabase(t)
	defer db.Close()
	events, err := helperGetStoredEventsAll(t, db, context.Background(), &model.Subscription{
		Filters: model.Filters{
			helperNewFilter(func(apply *model.Filter) {
				apply.Authors = []string{
					ev.Random(t).PubKey,
					ev.Random(t).PubKey,
				}
			}),
			helperNewFilter(func(apply *model.Filter) {
				apply.Authors = []string{ev.Random(t).PubKey}
			}),
		},
	})
	require.NoError(t, err)
	require.Len(t, events, 3)
}

func TestWhereBuilderByID(t *testing.T) {
	t.Parallel()

	db, ev := helperEnsureDatabase(t)
	defer db.Close()
	events, err := helperGetStoredEventsAll(t, db, context.Background(), &model.Subscription{
		Filters: model.Filters{
			helperNewFilter(func(apply *model.Filter) {
				apply.IDs = []string{ev.Random(t).ID}
			}),
			helperNewFilter(func(apply *model.Filter) {
				apply.IDs = []string{ev.Random(t).ID}
			}),
		},
	})
	require.NoError(t, err)
	require.Len(t, events, 2)
}

func TestWhereBuilderByMany(t *testing.T) {
	t.Parallel()

	db, ev := helperEnsureDatabase(t)
	defer db.Close()
	ev1 := ev.Random(t)
	ev2 := ev.Random(t)
	events, err := helperGetStoredEventsAll(t, db, context.Background(), &model.Subscription{
		Filters: model.Filters{
			helperNewFilter(func(apply *model.Filter) {
				apply.IDs = []string{ev1.ID, "bar"}
				apply.Authors = []string{ev1.PubKey, "fooo"}
				apply.Kinds = []int{ev1.Kind}
			}),
			helperNewFilter(func(apply *model.Filter) {
				apply.IDs = []string{ev2.ID, "123"}
				apply.Authors = []string{ev2.PubKey}
				apply.Kinds = []int{ev2.Kind, 1, 2, 3}
				apply.Since = &ev2.CreatedAt
				apply.Until = &ev2.CreatedAt
			}),
		},
	})
	require.NoError(t, err)
	require.Len(t, events, 2)
}

func TestWhereBuilderByTagsNoValuesSingle(t *testing.T) {
	t.Parallel()

	db, ev := helperEnsureDatabase(t)
	defer db.Close()
	event := ev.Random(t)
	filter := helperNewFilter(func(apply *model.Filter) {
		apply.IDs = []string{event.ID}
		apply.Authors = []string{event.PubKey}
		apply.Tags = model.TagMap{
			"#e": nil,
			"#p": nil,
			"#d": nil,
		}
	})

	t.Run("Something", func(t *testing.T) {
		events, err := helperGetStoredEventsAll(t, db, context.Background(), &model.Subscription{
			Filters: model.Filters{filter},
		})
		require.NoError(t, err)
		require.Len(t, events, 1)
	})

	t.Run("Nothing", func(t *testing.T) {
		x := filter

		// Add additional tag to the filter, so query will return no results because all 4 tags MUST be present.
		x.Tags["#x"] = nil

		events, err := helperGetStoredEventsAll(t, db, context.Background(), &model.Subscription{
			Filters: model.Filters{filter},
		})
		require.NoError(t, err)
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

		err := db.AcceptEvent(context.TODO(), &event)
		require.NoError(t, err)
	})

	filter := helperNewFilter(func(apply *model.Filter) {
		apply.IDs = []string{"1"}
		apply.Tags = model.TagMap{
			"e": {"etag"},
			"p": {"ptag"},
			"d": {"dtag"},
		}
	})

	t.Run("Match", func(t *testing.T) {
		events, err := helperGetStoredEventsAll(t, db, context.Background(), &model.Subscription{
			Filters: model.Filters{filter},
		})
		require.NoError(t, err)
		require.Len(t, events, 1)
	})
	t.Run("Empty", func(t *testing.T) {
		filter.Tags["e"] = append(filter.Tags["e"], "fooo") // Add 4th value to the tag list, so query will return no results.

		events, err := helperGetStoredEventsAll(t, db, context.Background(), &model.Subscription{
			Filters: model.Filters{filter},
		})
		require.NoError(t, err)
		require.Empty(t, events)
	})
}

func TestWhereBuilderByTagsOnlySingle(t *testing.T) {
	t.Parallel()

	db, ev := helperEnsureDatabase(t)
	defer db.Close()
	event := ev.Random(t)

	filter := helperNewFilter(func(apply *model.Filter) {
		apply.Tags = model.TagMap{
			event.Tags[0][0]: event.Tags[0][1:],
		}
	})

	t.Run("Match", func(t *testing.T) {
		events, err := helperGetStoredEventsAll(t, db, context.Background(), &model.Subscription{
			Filters: model.Filters{filter},
		})
		require.NoError(t, err)
		require.Len(t, events, 1)
	})
	t.Run("Empty", func(t *testing.T) {
		filter.Tags["#d"] = append(filter.Tags["#d"], "fooo") // Add 3rd value to the tag list, so query will return no results.

		events, err := helperGetStoredEventsAll(t, db, context.Background(), &model.Subscription{
			Filters: model.Filters{filter},
		})
		require.NoError(t, err)
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

		err := db.AcceptEvent(context.TODO(), &event)
		require.NoError(t, err)

		event.Kind = nostr.KindTextNote
		event.ID = "2"
		event.PubKey = "2"
		event.Tags = model.Tags{{"p", "ptag"}}
		event.CreatedAt = 2

		err = db.AcceptEvent(context.TODO(), &event)
		require.NoError(t, err)
	})

	events, err := helperGetStoredEventsAll(t, db, context.Background(), &model.Subscription{
		Filters: model.Filters{
			helperNewFilter(func(apply *model.Filter) {
				apply.Tags = model.TagMap{
					"e": {"etag"},
				}
			}),
			helperNewFilter(func(apply *model.Filter) {
				apply.Tags = model.TagMap{
					"p": {"ptag"},
				}
			}),
		},
	})
	require.NoError(t, err)
	require.Len(t, events, 2)
}

func TestSelectEventNoTags(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	id := helperGenerateEvent(t, db, false).ID
	require.NotEmpty(t, id)

	filter := helperNewFilter(func(apply *model.Filter) {
		apply.IDs = []string{id}
	})
	for ev, err := range db.SelectEvents(context.Background(), &model.Subscription{Filters: model.Filters{filter}}) {
		require.NoError(t, err)
		require.NotNil(t, ev)
		t.Logf("event: %+v", ev)
		require.Equal(t, id, ev.ID)
		require.Empty(t, ev.Tags)
	}

	require.NoError(t, db.Close())
}

func TestGenerateDataForFile3M(t *testing.T) {
	const amount = 3_000_000

	if os.Getenv("GENDB") != "yes" {
		t.Skip("skipping test; to enable, set GENDB=yes")
	}

	dbPath := `.testdata/testdb_3M.sqlite3`
	if n := os.Getenv("TESTDB"); n != "" {
		t.Logf("using custom database path %q from env (TESTDB)", n)
		dbPath = n
	}

	t.Logf("generating test database at %q with %d event(s)", dbPath, amount)
	db := openDatabase(dbPath+"?_foreign_keys=on&_journal_mode=off&_synchronous=off", true)
	require.NotNil(t, db)
	defer db.Close()

	helperFillDatabase(t, db, amount)
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

		err := db.AcceptEvent(context.TODO(), &event)
		require.NoError(t, err)

		event.Kind = nostr.KindTextNote
		event.ID = "2"
		event.PubKey = "2"
		event.Tags = model.Tags{{"imeta", "m image/png"}}
		event.CreatedAt = 2

		err = db.AcceptEvent(context.TODO(), &event)
		require.NoError(t, err)
	})
	t.Run("QueryNoImeta", func(t *testing.T) {
		count, err := db.CountEvents(context.TODO(), helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Search = "videos:false images:false"
		}))
		require.NoError(t, err)
		require.Equal(t, int64(100), count)
	})
	t.Run("Image", func(t *testing.T) {
		for ev, err := range db.SelectEvents(context.TODO(), helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Search = "images:true"
		})) {
			require.NoError(t, err)
			t.Logf("event: %+v", ev)
			require.NotNil(t, ev)
			require.Equal(t, "2", ev.ID)
		}
	})
	t.Run("Video", func(t *testing.T) {
		for ev, err := range db.SelectEvents(context.TODO(), helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Search = "videos:true"
		})) {
			require.NoError(t, err)
			t.Logf("event: %+v", ev)
			require.NotNil(t, ev)
			require.Equal(t, "1", ev.ID)
		}
	})
	t.Run("VideoByID", func(t *testing.T) {
		count, err := db.CountEvents(context.TODO(), helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Search = "videos:true"
			apply.IDs = []string{"1"}
		}))
		require.NoError(t, err)
		require.Equal(t, int64(1), count)
	})
	t.Run("NoVideo", func(t *testing.T) {
		count, err := db.CountEvents(context.TODO(), helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Search = "videos:false"
		}))
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

		err := db.AcceptEvent(context.TODO(), &event)
		require.NoError(t, err)

		event.Kind = nostr.KindTextNote
		event.ID = "2"
		event.PubKey = "2"
		event.Tags = model.Tags{{"e", "fooo"}, {"foo", "bar"}}
		event.CreatedAt = 1

		err = db.AcceptEvent(context.TODO(), &event)
		require.NoError(t, err)
	})
	t.Run("SelectQuotes", func(t *testing.T) {
		count, err := db.CountEvents(context.TODO(), helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Search = "quotes:true"
		}))
		require.NoError(t, err)
		require.Equal(t, int64(1), count)
	})
	t.Run("SelectReferences", func(t *testing.T) {
		count, err := db.CountEvents(context.TODO(), helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Search = "references:true"
		}))
		require.NoError(t, err)
		require.Equal(t, int64(1), count)
	})
	t.Run("SelectReferencesAndQuotes", func(t *testing.T) {
		count, err := db.CountEvents(context.TODO(), helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Search = "references:true quotes:true"
			apply.IDs = []string{"1", "2"}
		}))
		require.NoError(t, err)
		require.Equal(t, int64(2), count)
	})
	t.Run("SelectReferencesAndQuotesUnknownID", func(t *testing.T) {
		count, err := db.CountEvents(context.TODO(), helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Search = "references:true quotes:true"
			apply.IDs = []string{"5", "6"}
		}))
		require.NoError(t, err)
		require.Zero(t, count)
	})
	t.Run("SelectQuotesByID", func(t *testing.T) {
		count, err := db.CountEvents(context.TODO(), helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Search = "quotes:true"
			apply.IDs = []string{"1", "2"}
		}))
		require.NoError(t, err)
		require.Equal(t, int64(1), count)
	})
	t.Run("SelectNonQuotes", func(t *testing.T) {
		count, err := db.CountEvents(context.TODO(), helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Search = "quotes:false"
		}))
		require.NoError(t, err)
		require.Equal(t, int64(101), count)
	})
	t.Run("SelectNonQuoteByID", func(t *testing.T) {
		count, err := db.CountEvents(context.TODO(), helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Search = "quotes:false"
			apply.IDs = []string{"1"}
		}))
		require.NoError(t, err)
		require.Zero(t, count)
	})
	t.Run("SelectAll", func(t *testing.T) {
		count, err := db.CountEvents(context.TODO(), helperNewFilterSubscription(func(apply *model.Filter) {}))
		require.NoError(t, err)
		require.Equal(t, int64(102), count)
	})
}

func TestSelectEventsExpiration(t *testing.T) {
	t.Parallel()

	db, events := helperEnsureDatabase(t)
	defer db.Close()

	t.Run("Fill", func(t *testing.T) {
		var event model.Event
		event.Kind = nostr.KindTextNote
		event.ID = "expired"
		event.PubKey = "1"
		event.Tags = model.Tags{{"expiration", strconv.FormatInt(time.Now().Unix()-0xff, 10)}, {"q", "fooo"}}
		event.CreatedAt = 1

		err := db.AcceptEvent(context.TODO(), &event)
		require.NoError(t, err)

		event.Kind = nostr.KindTextNote
		event.ID = "alive"
		event.PubKey = "2"
		event.Tags = model.Tags{{"expiration", strconv.FormatInt(time.Now().Unix()+0xff, 10)}, {"e", "bar"}}
		event.CreatedAt = 1

		err = db.AcceptEvent(context.TODO(), &event)
		require.NoError(t, err)
	})
	t.Run("All", func(t *testing.T) {
		count, err := db.CountEvents(context.TODO(), helperNewFilterSubscription(func(apply *model.Filter) {}))
		require.NoError(t, err)
		require.Equal(t, int64(102), count)
	})
	t.Run("WithoutExpiration", func(t *testing.T) {
		count, err := db.CountEvents(context.TODO(), helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Search = "expiration:false"
		}))
		require.NoError(t, err)
		require.Equal(t, int64(100), count)
	})
	t.Run("WithoutExpirationByID", func(t *testing.T) {
		ev := events.Random(t)
		count, err := db.CountEvents(context.TODO(), helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Search = "expiration:false"
			apply.IDs = []string{ev.ID}
			apply.Authors = []string{ev.PubKey}
		}))
		require.NoError(t, err)
		require.Equal(t, int64(1), count)
	})
	t.Run("Expired", func(t *testing.T) {
		for ev, er := range db.SelectEvents(context.TODO(), helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Search = "expiration:false"
			apply.IDs = []string{"expired"}
		})) {
			require.NoError(t, er)
			t.Logf("expired event: %+v", ev)
		}

		count, err := db.CountEvents(context.TODO(), helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Search = "expiration:false"
			apply.IDs = []string{"expired"}
		}))
		require.NoError(t, err)
		require.Equal(t, int64(0), count)
	})
	t.Run("NotExpired", func(t *testing.T) {
		count, err := db.CountEvents(context.TODO(), helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Search = "expiration:true"
		}))
		require.NoError(t, err)
		require.Equal(t, int64(1), count)
	})
	t.Run("NotExpiredByID", func(t *testing.T) {
		count, err := db.CountEvents(context.TODO(), helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Kinds = []int{nostr.KindTextNote}
			apply.Search = "expiration:true"
			apply.IDs = []string{"alive"}
		}))
		require.NoError(t, err)
		require.Equal(t, int64(1), count)
	})
	t.Run("Fill by expired events", func(t *testing.T) {
		for i := range 100 {
			var event model.Event
			event.Kind = nostr.KindTextNote
			event.ID = fmt.Sprintf("expired:%v", i)
			event.PubKey = "1"
			event.Tags = model.Tags{{"expiration", strconv.FormatInt(time.Now().Unix()-0xff, 10)}, {"q", "fooo"}}
			event.CreatedAt = 1

			err := db.SaveEvent(context.TODO(), &event)
			require.NoError(t, err)
		}
	})
	t.Run("All expired events", func(t *testing.T) {
		count, err := db.CountEvents(context.TODO(), helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Search = "expiration:expired"
		}))
		require.NoError(t, err)
		require.Equal(t, int64(101), count)
	})
	t.Run("Delete expired events", func(t *testing.T) {
		count, err := db.CountEvents(context.TODO(), helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Search = "expiration:expired"
		}))
		require.NoError(t, err)
		require.Equal(t, int64(101), count)
		err = db.deleteExpiredEvents(context.TODO(), &model.Subscription{
			Filters: model.Filters{
				model.Filter{Search: "expiration:expired"},
			},
		})
		require.NoError(t, err)
		count, err = db.CountEvents(context.TODO(), helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Search = "expiration:expired"
		}))
		require.NoError(t, err)
		require.Equal(t, int64(0), count)
	})
}

func TestSelectWithExtensions(t *testing.T) {
	t.Parallel()

	db, events := helperEnsureDatabase(t)
	defer db.Close()

	t.Run("Fill", func(t *testing.T) {
		var event model.Event
		event.Kind = nostr.KindTextNote
		event.ID = "expired"
		event.PubKey = "1"
		event.Tags = model.Tags{{"expiration", strconv.FormatInt(time.Now().Unix()-0xff, 10)}, {"q", "fooo"}}
		event.CreatedAt = 1

		err := db.AcceptEvent(context.TODO(), &event)
		require.NoError(t, err)

		event.Kind = nostr.KindTextNote
		event.ID = "alive"
		event.PubKey = "2"
		event.Tags = model.Tags{{"expiration", strconv.FormatInt(time.Now().Unix()+0xff, 10)}, {"e", "bar"}}
		event.CreatedAt = 1

		err = db.AcceptEvent(context.TODO(), &event)
		require.NoError(t, err)
	})
	t.Run("AliveAndE", func(t *testing.T) {
		count, err := db.CountEvents(context.TODO(), helperNewFilterSubscription(func(apply *model.Filter) {
			apply.IDs = []string{"alive"}
			apply.Search = "expiration:true references:true"

		}))
		require.NoError(t, err)
		require.Equal(t, int64(1), count)
	})
	t.Run("ExpiredAndQ", func(t *testing.T) {
		count, err := db.CountEvents(context.TODO(), helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Search = "expiration:off quotes:on"
		}))
		require.NoError(t, err)
		require.Zero(t, count)
	})
	t.Run("NoEAndNoQ", func(t *testing.T) {
		count, err := db.CountEvents(context.TODO(), helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Search = "quotes:false references:false"
		}))
		require.NoError(t, err)
		require.Equal(t, int64(100), count)
	})
	t.Run("IdNoTags", func(t *testing.T) {
		ev := events.Random(t)
		count, err := db.CountEvents(context.TODO(), helperNewFilterSubscription(func(apply *model.Filter) {
			apply.IDs = []string{ev.ID}
			apply.Search = "quotes:true"
		}))
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
		event.CreatedAt = 1

		err := db.AcceptEvent(context.TODO(), &event)
		require.NoError(t, err)
	})
	t.Run("SelectRepost", func(t *testing.T) {
		count, err := db.CountEvents(context.TODO(), helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Search = "references:false"
			apply.Kinds = []int{nostr.KindRepost}
		}))
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

		err := db.AcceptEvent(context.TODO(), &event)
		require.NoError(t, err)
	})
	t.Run("SelectRepost", func(t *testing.T) {
		filter := helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Search = "images:yes"
			apply.Kinds = []int{nostr.KindRepost}
		})
		t.Run("Count", func(t *testing.T) {
			count, err := db.CountEvents(context.TODO(), filter)
			require.NoError(t, err)
			require.Equal(t, int64(1), count)
		})
		t.Run("Select", func(t *testing.T) {
			for ev, err := range db.SelectEvents(context.TODO(), filter) {
				require.NoError(t, err)
				require.NotNil(t, ev)
				t.Logf("event: %+v", ev)
				require.Equal(t, "2", ev.ID)
			}
		})
	})
}
