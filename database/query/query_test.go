package query

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/ice-blockchain/subzero/model"
)

const testDeadline = 30 * time.Second

func helperGetStoredEventsAll(t *testing.T, client *dbClient, ctx context.Context, subscription *model.Subscription) (events []*model.Event, err error) {
	t.Helper()

	for ev, evErr := range client.SelectEvents(ctx, subscription) {
		require.NoError(t, evErr)
		events = append(events, ev)
	}

	return events, err
}

func helperNewDatabase(t *testing.T) *dbClient {
	t.Helper()

	return openDatabase(":memory:", true)
}

func TestMain(m *testing.M) {
	exitCode := m.Run()

	if testDB.Ready {
		testDB.Client.Close()
	}

	if exitCode == 0 {
		if err := goleak.Find(); err != nil {
			fmt.Fprintf(os.Stderr, "goleak: errors on successful test run: %v\n", err)
			exitCode = 1
		}
	}

	os.Exit(exitCode)
}

func TestReplaceEvents(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
	t.Run("normal, non-replaceable event", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()

		expectedEvents := []*model.Event{}
		expectedEvents = append(expectedEvents, &model.Event{
			Event: nostr.Event{
				ID:        "normal" + uuid.NewString(),
				PubKey:    "bogus" + uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindTextNote,
				Tags:      nostr.Tags{},
				Content:   "bogus" + uuid.NewString(),
				Sig:       "bogus" + uuid.NewString(),
			},
		})
		require.NoError(t, db.AcceptEvent(ctx, expectedEvents[0]))
		expectedEvents = append(expectedEvents, &model.Event{
			Event: nostr.Event{
				ID:        "normal, 2nd event" + uuid.NewString(),
				PubKey:    "bogus" + uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()) + 1,
				Kind:      nostr.KindTextNote,
				Tags:      nostr.Tags{},
				Content:   "bogus" + uuid.NewString(),
				Sig:       "bogus" + uuid.NewString(),
			},
		})
		require.NoError(t, db.AcceptEvent(ctx, expectedEvents[1]))
		stored, err := helperGetStoredEventsAll(t, db, ctx, &model.Subscription{Filters: []nostr.Filter{{Kinds: []int{nostr.KindTextNote}}}})
		require.NoError(t, err)
		require.Len(t, stored, 2)
		require.Equal(t, expectedEvents[0], stored[1])
		require.Equal(t, expectedEvents[1], stored[0])
	})
	t.Run("ephemeral event", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()

		require.NoError(t, db.AcceptEvent(ctx, &model.Event{
			Event: nostr.Event{
				ID:        "normal" + uuid.NewString(),
				PubKey:    "bogus" + uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindClientAuthentication,
				Tags:      nostr.Tags{},
				Content:   "bogus" + uuid.NewString(),
				Sig:       "bogus" + uuid.NewString(),
			},
		}))
		stored, err := helperGetStoredEventsAll(t, db, ctx, &model.Subscription{Filters: []nostr.Filter{{Kinds: []int{nostr.KindTextNote}}}})
		require.NoError(t, err)
		require.Empty(t, stored)
	})
	t.Run("replaceable event", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()

		expectedEvents := []*model.Event{}
		require.NoError(t, db.AcceptEvent(ctx, &model.Event{
			Event: nostr.Event{
				ID:        "replaceable " + uuid.NewString(),
				PubKey:    "bogus",
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindContactList,
				Tags: nostr.Tags{
					[]string{"p", uuid.NewString(), "wss://localhost:9999/"},
				},
				Content: "bogus" + uuid.NewString(),
				Sig:     "bogus" + uuid.NewString(),
			},
		}))
		expectedEvents = append(expectedEvents, &model.Event{
			Event: nostr.Event{
				ID:        "replaceable " + uuid.NewString(),
				PubKey:    "bogus",
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindContactList,
				Tags: nostr.Tags{
					[]string{"p", uuid.NewString(), "wss://localhost:9999/"},
				},
				Content: "bogus" + uuid.NewString(),
				Sig:     "bogus" + uuid.NewString(),
			},
		})
		require.NoError(t, db.AcceptEvent(ctx, expectedEvents[0]))
		expectedEvents = append(expectedEvents, &model.Event{
			Event: nostr.Event{
				ID:        "replaceable " + uuid.NewString(),
				PubKey:    "another bogus",
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindContactList,
				Tags: nostr.Tags{
					[]string{"p", uuid.NewString(), "wss://localhost:9999/"},
				},
				Content: "bogus" + uuid.NewString(),
				Sig:     "bogus" + uuid.NewString(),
			},
		})
		require.NoError(t, db.AcceptEvent(ctx, expectedEvents[1]))
		stored, err := helperGetStoredEventsAll(t, db, ctx, &model.Subscription{Filters: []nostr.Filter{{Kinds: []int{nostr.KindContactList}}}})
		require.NoError(t, err)
		require.Len(t, stored, 2)
		require.Contains(t, stored, expectedEvents[0])
		require.Contains(t, stored, expectedEvents[1])
	})
	t.Run("param replaceable event", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()
		expectedEvents := []*model.Event{}
		require.NoError(t, db.AcceptEvent(ctx, &model.Event{
			Event: nostr.Event{
				ID:        "item to be replaced" + uuid.NewString(),
				PubKey:    "bogus",
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindRepositoryAnnouncement,
				Tags: nostr.Tags{
					[]string{"d", "bogus"},
				},
				Content: "bogus" + uuid.NewString(),
				Sig:     "bogus" + uuid.NewString(),
			},
		}))
		// Overwrite
		expectedEvents = append(expectedEvents, &model.Event{
			Event: nostr.Event{
				ID:        "param replaceable 1 " + uuid.NewString(),
				PubKey:    "bogus",
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindRepositoryAnnouncement,
				Tags: nostr.Tags{
					[]string{"d", "bogus"},
				},
				Content: "bogus" + uuid.NewString(),
				Sig:     "bogus" + uuid.NewString(),
			},
		})
		require.NoError(t, db.AcceptEvent(ctx, expectedEvents[0]))
		// Another D value
		expectedEvents = append(expectedEvents, &model.Event{
			Event: nostr.Event{
				ID:        "param replaceable 2 " + uuid.NewString(),
				PubKey:    "bogus",
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindRepositoryAnnouncement,
				Tags: nostr.Tags{
					[]string{"d", "another bogus" + uuid.NewString()},
				},
				Content: "bogus" + uuid.NewString(),
				Sig:     "bogus" + uuid.NewString(),
			},
		})
		require.NoError(t, db.AcceptEvent(ctx, expectedEvents[1]))
		// Another pubkey
		expectedEvents = append(expectedEvents, &model.Event{
			Event: nostr.Event{
				ID:        "param replaceable 3 " + uuid.NewString(),
				PubKey:    "another bogus",
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindRepositoryAnnouncement,
				Tags: nostr.Tags{
					[]string{"d", "bogus" + uuid.NewString()},
				},
				Content: "bogus" + uuid.NewString(),
				Sig:     "bogus" + uuid.NewString(),
			},
		})
		require.NoError(t, db.AcceptEvent(ctx, expectedEvents[2]))
		stored, err := helperGetStoredEventsAll(t, db, ctx, &model.Subscription{Filters: []nostr.Filter{{Kinds: []int{nostr.KindRepositoryAnnouncement}}}})
		require.NoError(t, err)
		require.Len(t, stored, 3)
		require.Contains(t, stored, expectedEvents[0])
		require.Contains(t, stored, expectedEvents[1])
		require.Contains(t, stored, expectedEvents[2])
	})
}

func TestNIP09DeleteEvents(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
	t.Run("normal, non-replaceable event", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()

		publishedEvent := &model.Event{
			Event: nostr.Event{
				ID:        "normal" + uuid.NewString(),
				PubKey:    "bogus" + uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindTextNote,
				Tags:      nostr.Tags{},
				Content:   "bogus" + uuid.NewString(),
				Sig:       "bogus" + uuid.NewString(),
			},
		}
		require.NoError(t, db.AcceptEvent(ctx, publishedEvent))
		stored, err := helperGetStoredEventsAll(t, db, ctx, &model.Subscription{Filters: []nostr.Filter{{Kinds: []int{nostr.KindTextNote}}}})
		require.NoError(t, err)
		require.Len(t, stored, 1)
		require.Contains(t, stored, publishedEvent)
		require.NoError(t, db.AcceptEvent(ctx, &model.Event{
			Event: nostr.Event{
				ID:        "deletion event" + uuid.NewString(),
				PubKey:    publishedEvent.PubKey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindDeletion,
				Tags:      nostr.Tags{}.AppendUnique(nostr.Tag{"e", publishedEvent.ID}),
				Content:   "bogus" + uuid.NewString(),
				Sig:       "bogus" + uuid.NewString(),
			},
		}))
		stored, err = helperGetStoredEventsAll(t, db, ctx, &model.Subscription{Filters: []nostr.Filter{{Kinds: []int{nostr.KindTextNote}}}})
		require.NoError(t, err)
		require.Empty(t, stored)
	})
	t.Run("replaceable event without d-tag", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()

		publishedEvent := &model.Event{
			Event: nostr.Event{
				ID:        "replaceable" + uuid.NewString(),
				PubKey:    "bogus" + uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindProfileMetadata,
				Tags:      nostr.Tags{},
				Content:   "{\"name\": \"bogus\", \"about\": \"bogus\", \"picture\": \"bogus\"}",
				Sig:       "bogus" + uuid.NewString(),
			},
		}
		require.NoError(t, db.AcceptEvent(ctx, publishedEvent))
		stored, err := helperGetStoredEventsAll(t, db, ctx, &model.Subscription{Filters: []nostr.Filter{{Kinds: []int{nostr.KindProfileMetadata}}}})
		require.NoError(t, err)
		require.Len(t, stored, 1)
		require.Contains(t, stored, publishedEvent)
		require.NoError(t, db.AcceptEvent(ctx, &model.Event{
			Event: nostr.Event{
				ID:        "deletion event" + uuid.NewString(),
				PubKey:    publishedEvent.PubKey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindDeletion,
				Tags:      nostr.Tags{}.AppendUnique(nostr.Tag{"a", fmt.Sprintf("%v:%v:", nostr.KindProfileMetadata, publishedEvent.PubKey)}),
				Content:   "bogus" + uuid.NewString(),
				Sig:       "bogus" + uuid.NewString(),
			},
		}))
		stored, err = helperGetStoredEventsAll(t, db, ctx, &model.Subscription{Filters: []nostr.Filter{{Kinds: []int{nostr.KindProfileMetadata}}}})
		require.NoError(t, err)
		require.Empty(t, stored)
	})
	t.Run("replaceable event with d tag", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()

		publishedEvent := &model.Event{
			Event: nostr.Event{
				ID:        "param replaceable" + uuid.NewString(),
				PubKey:    "bogus" + uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindArticle,
				Tags:      nostr.Tags{}.AppendUnique(nostr.Tag{"d", "bogus"}),
				Content:   "{\"name\": \"bogus\", \"about\": \"bogus\", \"picture\": \"bogus\"}",
				Sig:       "bogus" + uuid.NewString(),
			},
		}
		require.NoError(t, db.AcceptEvent(ctx, publishedEvent))
		stored, err := helperGetStoredEventsAll(t, db, ctx, &model.Subscription{Filters: []nostr.Filter{{Kinds: []int{nostr.KindArticle}}}})
		require.NoError(t, err)
		require.Len(t, stored, 1)
		require.Contains(t, stored, publishedEvent)
		require.NoError(t, db.AcceptEvent(ctx, &model.Event{
			Event: nostr.Event{
				ID:        "deletion event" + uuid.NewString(),
				PubKey:    publishedEvent.PubKey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindDeletion,
				Tags:      nostr.Tags{}.AppendUnique(nostr.Tag{"a", fmt.Sprintf("%v:%v:bogus", nostr.KindArticle, publishedEvent.PubKey)}),
				Content:   "bogus" + uuid.NewString(),
				Sig:       "bogus" + uuid.NewString(),
			},
		}))
		stored, err = helperGetStoredEventsAll(t, db, ctx, &model.Subscription{Filters: []nostr.Filter{{Kinds: []int{nostr.KindProfileMetadata}}}})
		require.NoError(t, err)
		require.Empty(t, stored)
	})
}
