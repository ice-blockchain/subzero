package query

import (
	"context"
	"encoding/hex"
	"os"
	"sync"
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/schollz/progressbar/v3"
	"github.com/stretchr/testify/require"
	"pgregory.net/rand"

	"github.com/ice-blockchain/subzero/model"
)

const (
	testDbPath1K = `.testdata/testdb_1K.sqlite3`
)

var (
	testDbClient *dbClient
	testDbOnce   sync.Once
)

func helperEnsureDatabase(t *testing.T) {
	t.Helper()

	if _, err := os.Stat(testDbPath1K); err != nil {
		t.Skipf("no test database found at %q", testDbPath1K)
	}

	testDbOnce.Do(func() {
		t.Logf("opening test database at %q", testDbPath1K)
		testDbClient = openDatabase(testDbPath1K+"?mode=ro&_foreign_keys=on", true)
		require.NotNil(t, testDbClient)
	})
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

func helperGenerateEventWithTags(t *testing.T, db *dbClient) {
	t.Helper()

	var ev model.Event

	ev.ID = generateHexString()
	ev.PubKey = generateHexString()
	ev.CreatedAt = model.Timestamp(generateCreatedAt())
	ev.Kind = generateKind()
	ev.Content = generateRandomString(rand.Intn(1024))

	ev.Tags = []model.Tag{
		{"#e", generateHexString(), generateRandomString(rand.Intn(20)), generateRandomString(rand.Intn(30))},
		{"#p", generateHexString()},
		{"#d", generateHexString(), generateRandomString(rand.Intn(10))},
	}

	err := db.SaveEvent(context.Background(), &ev)
	require.NoError(t, err)
}

func helperFillDatabase(t *testing.T, db *dbClient, size int) {
	t.Helper()

	var eventsCount []int
	err := db.Select(&eventsCount, "select count(*) from events")
	require.NoError(t, err)

	t.Logf("found %d event(s)", eventsCount[0])
	if eventsCount[0] >= size {
		return
	}

	need := size - eventsCount[0]
	t.Logf("generating %d event(s)", need)

	bar := progressbar.Default(int64(need), "generating events")
	for range need {
		bar.Add(1) //nolint:errcheck
		helperGenerateEventWithTags(t, db)
	}
}

func TestWhereBuilderByAuthor(t *testing.T) {
	t.Parallel()

	helperEnsureDatabase(t)
	events, err := helperGetStoredEventsAll(t, testDbClient, context.Background(), &model.Subscription{
		Filters: model.Filters{
			model.Filter{
				Authors: []string{
					"9b1d14e385b2de2098c11862c4276407afd99b321e86a0125ec64c78a1309947bc01a401ad30acd4a21a3dc16e36178625a36c4dce8479a05c1946e31f53c42e",
					"4e2371fe2d7ee7b37d6bdb79ec31c36a6a41236b4601839e91636b066119dc9f5014c918b17b91841aa99f06dc169611357c14ac2edf551a83dbbcfa7628384e",
				},
			},
			model.Filter{
				Authors: []string{"5e1ac9750abda30bde2a30a1ad2b4af2fbce5d258a6a8dfdd1cc9f5df4e08c94d3e3425c1bc36325789efd49984f9c2d0cc1c0067e983fe93be0935d98b99a5c"},
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, events, 3)
}

func TestWhereBuilderByID(t *testing.T) {
	t.Parallel()

	helperEnsureDatabase(t)
	events, err := helperGetStoredEventsAll(t, testDbClient, context.Background(), &model.Subscription{
		Filters: model.Filters{
			model.Filter{
				IDs: []string{
					"0aa556e4b7fea020af4eb40eabae958a0f04337077e6fef09fac969edf71eee0e8f83ec6dcf233098ae6f56936c851daddaf2fca869790990a40d7ca773e3142",
				},
			},
			model.Filter{
				IDs: []string{
					"0bd34db276ccacaeaaa45760742787523c6670258aac4f2fd959c53459c3840061d54d9a5c70045af6909598c4774baf33eee83a697304410e9c809a72d74196",
				},
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, events, 2)
}

func TestWhereBuilderByMany(t *testing.T) {
	t.Parallel()

	helperEnsureDatabase(t)
	ts2 := model.Timestamp(1702743869)
	events, err := helperGetStoredEventsAll(t, testDbClient, context.Background(), &model.Subscription{
		Filters: model.Filters{
			model.Filter{
				IDs: []string{
					"231f3b0bc2bbe1669430cdc8c9343db0c69e1fe054625ae47dc3740d6571fd0ef195d507813619cb37c604e70f065c519baac2bb7af6ad00ed61805ddaedd1f2",
					"bar",
				},
				Authors: []string{
					"cef80189700a0c012d243666fac85fdad551f55d230d781837b43e5bbead052cca826d2f246c177c1fe1ef8f1845bba162fd4b6a9d38d01f77b282ac799199eb",
					"fooo",
				},
				Kinds: []int{35326},
			},
			model.Filter{
				IDs: []string{
					"2588da1670c3281337f31eedf399a990f49c2a7c901777d1d8c3309d59440a2bb113719b036f39b6d95af34bae0e3913adf0f4826068c683dbf4a1afe6ad1774",
					"123",
				},
				Authors: []string{
					"02771903bc5c0f937f81b2215fc8b8284d7e60cab2ae3478cd113cbff8282e7932b2abab32c0b605f8f623023c9123662f18880923a62ce60f3d6cf1959a7ca2",
				},
				Kinds: []int{28022, 1, 2, 3},
				Since: &ts2,
				Until: &ts2,
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, events, 2)
}

func TestWhereBuilderByTagsNoValuesSingle(t *testing.T) {
	t.Parallel()

	helperEnsureDatabase(t)
	filter := model.Filter{
		IDs: []string{
			"000c579336863d330b659b0a4d11ad820e4c48ef22cd34d9ab62314be625759d787f9d703c238fc2d84c8799e5d7d0f7399beb5414a3d5f6b0b4582160c136ff",
			"bar",
		},
		Authors: []string{
			"5d26995088d55615f31fa87af4a80d012bf16b8df39dfc1d876342c9af0cb661282eae025da01c974d2b4ffaa9422cbad4e7a0ee828e4d890c0d84dac8f01e00",
		},
		Tags: map[string][]string{
			"#e": nil,
			"#p": nil,
			"#d": nil,
		},
	}

	t.Run("Something", func(t *testing.T) {
		events, err := helperGetStoredEventsAll(t, testDbClient, context.Background(), &model.Subscription{
			Filters: model.Filters{filter},
		})
		require.NoError(t, err)
		require.Len(t, events, 1)
	})

	t.Run("Nothing", func(t *testing.T) {
		x := filter

		// Add additional tag to the filter, so query will return no results because all 4 tags MUST be present.
		x.Tags["#x"] = nil

		events, err := helperGetStoredEventsAll(t, testDbClient, context.Background(), &model.Subscription{
			Filters: model.Filters{filter},
		})
		require.NoError(t, err)
		require.Empty(t, events)
	})
}

func TestWhereBuilderByTagsSingle(t *testing.T) {
	t.Parallel()

	helperEnsureDatabase(t)
	filter := model.Filter{
		IDs: []string{
			"2176eaf2a4b75accde2fe1547bea34824722d4ef68845ead6d0131568afbda7fda91835703541b157c113bd51e26bf1e0de288e3a5b2c21fa6cfd3140e0caf57",
		},
		Tags: map[string][]string{
			"#e": {
				"fc18970a9ea4d37bbeb4499af32f9813c9993140b1a1a0671d1ffbd1a4784f3b0a93e3975dc853da7309a1061db85864446e82c9dcd95ea84cffc82da61ed853",
				"kqWNdM",
				"dbcIlZURIOEYYGbDVMk",
			},
			"#p": {
				"4f7a706d9b78562e06f06a2e1cae1cd0ec1105f93fc62a353d935019ff79e9ac5d513496fa394e0b312ff3bf06da2a889fe57f75e18887d5b7e07ee3e7611d5e",
			},
			"#d": {
				"c752d8212d645abf032a3c303fd0a3e746362fba66abd76f35a5aeb939df90cb2901c4848a52ea95c9f443778b2ff68010ae7476edd3c4cecf2a1d9e6368221a",
				"PsoA",
				"",
			},
		},
	}

	t.Run("Match", func(t *testing.T) {
		events, err := helperGetStoredEventsAll(t, testDbClient, context.Background(), &model.Subscription{
			Filters: model.Filters{filter},
		})
		require.NoError(t, err)
		require.Len(t, events, 1)
	})
	t.Run("Empty", func(t *testing.T) {
		filter.Tags["#e"] = append(filter.Tags["#e"], "fooo") // Add 4th value to the tag list, so query will return no results.

		events, err := helperGetStoredEventsAll(t, testDbClient, context.Background(), &model.Subscription{
			Filters: model.Filters{filter},
		})
		require.NoError(t, err)
		require.Empty(t, events)
	})
}

func TestWhereBuilderByTagsOnlySingle(t *testing.T) {
	t.Parallel()

	helperEnsureDatabase(t)
	filter := model.Filter{
		Tags: map[string][]string{
			"#d": {
				"da312bf00ee7e41467d32c4bc32c65c4841608647ff275cb1bb2571f1faee0783e8851482201b6b9a577a1fd148f416c0ea633c574de8ef0e70121e89b237a50",
				"WDHCvha",
			},
		},
	}

	t.Run("Match", func(t *testing.T) {
		events, err := helperGetStoredEventsAll(t, testDbClient, context.Background(), &model.Subscription{
			Filters: model.Filters{filter},
		})
		require.NoError(t, err)
		require.Len(t, events, 1)
	})
	t.Run("Empty", func(t *testing.T) {
		filter.Tags["#d"] = append(filter.Tags["#d"], "fooo") // Add 3rd value to the tag list, so query will return no results.

		events, err := helperGetStoredEventsAll(t, testDbClient, context.Background(), &model.Subscription{
			Filters: model.Filters{filter},
		})
		require.NoError(t, err)
		require.Empty(t, events)
	})
}

func TestWhereBuilderByTagsOnlyMulti(t *testing.T) {
	t.Parallel()

	helperEnsureDatabase(t)
	events, err := helperGetStoredEventsAll(t, testDbClient, context.Background(), &model.Subscription{
		Filters: model.Filters{
			{
				Tags: map[string][]string{
					"#e": {
						"6de2c9b0820e236466b9db18d06cb5e54843377fff5d62b9c54a050c4966440908f6238ea15514ab10a5fee438b64b7a64e0d6e3a3070c4fd63189ec1b5a5f19",
					},
				},
			},
			{
				Tags: map[string][]string{
					"#d": {
						"452f4c903f1730370538c07f1b4ecfdade820b01646049aeb1e61a7c3d2479e662d91e0d6e05d55218ec6e8050ab494a44964ee5d8b7515c327c3f1b9b489857",
					},
				},
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, events, 2)
}

func TestGenerateDataInMemory(t *testing.T) {
	t.Parallel()

	db := openDatabase(":memory:", true)
	require.NotNil(t, db)
	defer db.Close()

	helperFillDatabase(t, db, 100)
}

func TestGenerateDataForFile(t *testing.T) {
	t.Parallel()

	if _, err := os.Stat(testDbPath1K); err == nil {
		t.Skipf("test database already exists at %q", testDbPath1K)
	}

	db := openDatabase(testDbPath1K, true)
	require.NotNil(t, db)
	defer db.Close()

	helperFillDatabase(t, db, 1000)
}

func TestSelectEventsIterator(t *testing.T) {
	t.Parallel()

	helperEnsureDatabase(t)
	t.Run("PartialFetch", func(t *testing.T) {
		for ev, err := range testDbClient.SelectEvents(context.Background(),
			&model.Subscription{Filters: model.Filters{model.Filter{Limit: 5}}}) {
			require.NoError(t, err)
			require.NotNil(t, ev)
			break
		}
	})
	t.Run("FullFetch", func(t *testing.T) {
		var count int
		for ev, err := range testDbClient.SelectEvents(context.Background(),
			&model.Subscription{Filters: model.Filters{model.Filter{Limit: 10}}}) {
			require.NoError(t, err)
			require.NotNil(t, ev)
			count++
		}
		require.Equal(t, 10, count)
	})
}

func TestGenerateDataForFile3M(t *testing.T) {
	const amount = 3_000_000

	t.Skip("this test is too slow")

	const dbPath = `.testdata/testdb_3M.sqlite3`

	if _, err := os.Stat(dbPath); err == nil {
		t.Skipf("test database already exists at %q", dbPath)
	}

	db := openDatabase(dbPath+"?_foreign_keys=on&_journal_mode=off&_synchronous=off", true)
	require.NotNil(t, db)
	defer db.Close()

	helperFillDatabase(t, db, amount)
}
