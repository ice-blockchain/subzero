package query

import (
	"context"
	"encoding/hex"
	"os"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/nbd-wtf/go-nostr"
	"github.com/pkg/errors"
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

	return helperRandomEvent(h, te.Events)
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

func helperRandomEvent(t interface{ Helper() }, events []*model.Event) *model.Event {
	t.Helper()

	return events[rand.Int31n(int32(len(events)))]
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
		nostr.KindContactList,
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

func helperGenerateEvent(t interface {
	require.TestingT
	Helper()
}, db *dbClient, withTags bool) string {
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

	err := db.SaveEvent(context.Background(), &ev)
	require.NoError(t, err)

	return ev.ID
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
			model.Filter{
				Authors: []string{
					ev.Random(t).PubKey,
					ev.Random(t).PubKey,
				},
			},
			model.Filter{
				Authors: []string{ev.Random(t).PubKey},
			},
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
			model.Filter{
				IDs: []string{
					ev.Random(t).ID,
				},
			},
			model.Filter{
				IDs: []string{
					ev.Random(t).ID,
				},
			},
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
			model.Filter{
				IDs: []string{
					ev1.ID,
					"bar",
				},
				Authors: []string{
					ev1.PubKey,
					"fooo",
				},
				Kinds: []int{ev1.Kind},
			},
			model.Filter{
				IDs: []string{
					ev2.ID,
					"123",
				},
				Authors: []string{
					ev2.PubKey,
				},
				Kinds: []int{ev2.Kind, 1, 2, 3},
				Since: &ev2.CreatedAt,
				Until: &ev2.CreatedAt,
			},
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
	filter := model.Filter{
		IDs:     []string{event.ID, "bar"},
		Authors: []string{event.PubKey},
		Tags: map[string][]string{
			"#e": nil,
			"#p": nil,
			"#d": nil,
		},
	}

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

	db, ev := helperEnsureDatabase(t)
	defer db.Close()
	event := ev.Random(t)
	filter := model.Filter{
		IDs:  []string{event.ID},
		Tags: map[string][]string{},
	}

	for _, tag := range event.Tags {
		filter.Tags[tag[0]] = tag[1:]
	}

	t.Run("Match", func(t *testing.T) {
		events, err := helperGetStoredEventsAll(t, db, context.Background(), &model.Subscription{
			Filters: model.Filters{filter},
		})
		require.NoError(t, err)
		require.Len(t, events, 1)
	})
	t.Run("Empty", func(t *testing.T) {
		filter.Tags["#e"] = append(filter.Tags["#e"], "fooo") // Add 4th value to the tag list, so query will return no results.

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
	filter := model.Filter{
		Tags: map[string][]string{
			event.Tags[0][0]: event.Tags[0][1:],
		},
	}

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

	db, ev := helperEnsureDatabase(t)
	defer db.Close()
	ev1 := ev.Random(t)
	ev2 := ev.Random(t)
	events, err := helperGetStoredEventsAll(t, db, context.Background(), &model.Subscription{
		Filters: model.Filters{
			{
				Tags: map[string][]string{
					ev1.Tags[1][0]: ev1.Tags[1][1:],
				},
			},
			{
				Tags: map[string][]string{
					ev2.Tags[1][0]: ev2.Tags[1][1:],
				},
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, events, 2)
}

func TestSelectEventNoTags(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	id := helperGenerateEvent(t, db, false)
	require.NotEmpty(t, id)

	filter := model.Filter{IDs: []string{id}}
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
