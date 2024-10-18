// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strconv"
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
	goleak.VerifyTestMain(m)
}

func TestReplaceableEvents(t *testing.T) {
	t.Parallel()

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
				Tags:      model.Tags{},
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
				Tags:      model.Tags{},
				Content:   "bogus" + uuid.NewString(),
				Sig:       "bogus" + uuid.NewString(),
			},
		})
		require.NoError(t, db.AcceptEvent(ctx, expectedEvents[1]))
		stored, err := helperGetStoredEventsAll(t, db, ctx, helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Kinds = []int{nostr.KindTextNote}
		}))
		require.NoError(t, err)
		require.Len(t, stored, 2)
		require.EqualValues(t, expectedEvents[1], stored[0])
		require.EqualValues(t, expectedEvents[0], stored[1])
	})
	t.Run("ephemeral event", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()

		require.NoError(t, db.AcceptEvent(ctx, &model.Event{
			Event: nostr.Event{
				ID:        "normal, 1st event" + uuid.NewString(),
				PubKey:    "bogus" + uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindProfileMetadata,
				Tags:      model.Tags{},
				Content:   `{"name":"username","about":"bogus","picture":"https://localhost:9999/bogus.jpg"}`,
				Sig:       "bogus" + uuid.NewString(),
			},
		}))
		stored, err := helperGetStoredEventsAll(t, db, ctx, helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Kinds = []int{nostr.KindTextNote}
		}))
		require.NoError(t, err)
		require.Empty(t, stored)
	})
	t.Run("normal, replaceable event with user metadata", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()

		expectedEvents := []*model.Event{}
		expectedEvents = append(expectedEvents, &model.Event{Event: nostr.Event{Tags: model.Tags{}}})
		require.NoError(t, db.AcceptEvent(ctx, expectedEvents[0]))
		expectedEvents = append(expectedEvents, &model.Event{
			Event: nostr.Event{
				ID:        "normal, 2nd event" + uuid.NewString(),
				PubKey:    "bogus" + uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindProfileMetadata,
				Tags:      model.Tags{},
				Content:   `{"name":"username","about":"bogus","picture":"https://localhost:9999/bogus.jpg"}`,
				Sig:       "bogus" + uuid.NewString(),
			},
		})
		require.NoError(t, db.AcceptEvent(ctx, expectedEvents[1]))
		stored, err := helperGetStoredEventsAll(t, db, ctx, helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Kinds = []int{nostr.KindProfileMetadata}
		}))
		require.NoError(t, err)
		require.Len(t, stored, 2)
		require.EqualValues(t, stored[0], expectedEvents[1])
		require.EqualValues(t, stored[1], expectedEvents[0])
	})

	t.Run("replaceable event", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()

		ev1 := &model.Event{
			Event: nostr.Event{
				ID:        "replaceable event 1 that must be replaced",
				PubKey:    "bogus",
				CreatedAt: 1,
				Kind:      nostr.KindFollowList,
				Tags:      model.Tags{{"p", "event1", "wss://localhost:9999/"}},
			},
		}
		require.NoError(t, db.AcceptEvent(ctx, ev1))

		// Overwrite.
		ev2 := &model.Event{
			Event: nostr.Event{
				ID:        "replaceable event 2",
				PubKey:    "bogus",
				CreatedAt: 2,
				Kind:      nostr.KindFollowList,
				Tags:      nostr.Tags{{"p", "event2", "wss://localhost:9999/"}},
			},
		}
		require.NoError(t, db.AcceptEvent(ctx, ev2))

		// Add another event.
		ev3 := &model.Event{
			Event: nostr.Event{
				ID:        "replaceable event 3",
				PubKey:    "another bogus",
				CreatedAt: 3,
				Kind:      nostr.KindFollowList,
				Tags:      nostr.Tags{{"p", "event3", "wss://localhost:9999/"}},
			},
		}
		require.NoError(t, db.AcceptEvent(ctx, ev3))

		stored, err := helperGetStoredEventsAll(t, db, ctx, helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Kinds = []int{nostr.KindFollowList}
		}))
		require.NoError(t, err)
		require.Len(t, stored, 2)
		require.Equal(t, ev3, stored[0], "event 3")
		require.Equal(t, ev2, stored[1], "event 2")
	})
}

func TestParametrizedReplaceableEvents(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
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
				Tags: model.Tags{
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
				Tags: model.Tags{
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
				Tags: model.Tags{
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
				Tags: model.Tags{
					[]string{"d", "bogus" + uuid.NewString()},
				},
				Content: "bogus" + uuid.NewString(),
				Sig:     "bogus" + uuid.NewString(),
			},
		})
		require.NoError(t, db.AcceptEvent(ctx, expectedEvents[2]))
		stored, err := helperGetStoredEventsAll(t, db, ctx, helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Kinds = []int{nostr.KindRepositoryAnnouncement}
		}))
		require.NoError(t, err)
		require.Len(t, stored, 3)
		require.Contains(t, stored, expectedEvents[0])
		require.Contains(t, stored, expectedEvents[1])
		require.Contains(t, stored, expectedEvents[2])
	})
}

func TestEphemeralEvents(t *testing.T) {
	t.Parallel()

	t.Run("ephemeral event", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), testDeadline)
		defer cancel()
		db := helperNewDatabase(t)
		defer db.Close()

		require.NoError(t, db.AcceptEvent(ctx, &model.Event{
			Event: nostr.Event{
				ID:        "ephemeral" + uuid.NewString(),
				PubKey:    "bogus" + uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindClientAuthentication,
				Tags:      model.Tags{},
				Content:   "bogus" + uuid.NewString(),
				Sig:       "bogus" + uuid.NewString(),
			},
		}))
		stored, err := helperGetStoredEventsAll(t, db, ctx, helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Kinds = []int{nostr.KindTextNote}
		}))
		require.NoError(t, err)
		require.Empty(t, stored)
	})
}

func TestNIP09DeleteEvents(t *testing.T) {
	t.Parallel()

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
				Tags:      model.Tags{},
				Content:   "bogus" + uuid.NewString(),
				Sig:       "bogus" + uuid.NewString(),
			},
		}
		require.NoError(t, db.AcceptEvent(ctx, publishedEvent))
		stored, err := helperGetStoredEventsAll(t, db, ctx, helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Kinds = []int{nostr.KindTextNote}
		}))
		require.NoError(t, err)
		require.Len(t, stored, 1)
		require.Contains(t, stored, publishedEvent)
		require.NoError(t, db.AcceptEvent(ctx, &model.Event{
			Event: nostr.Event{
				ID:        "deletion event" + uuid.NewString(),
				PubKey:    publishedEvent.PubKey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindDeletion,
				Tags:      model.Tags{}.AppendUnique(nostr.Tag{"e", publishedEvent.ID}),
				Content:   "bogus" + uuid.NewString(),
				Sig:       "bogus" + uuid.NewString(),
			},
		}))
		stored, err = helperGetStoredEventsAll(t, db, ctx, helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Kinds = []int{nostr.KindTextNote}
		}))
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
				Tags:      model.Tags{},
				Content:   "{\"name\": \"bogus\", \"about\": \"bogus\", \"picture\": \"bogus\"}",
				Sig:       "bogus" + uuid.NewString(),
			},
		}
		require.NoError(t, db.AcceptEvent(ctx, publishedEvent))
		stored, err := helperGetStoredEventsAll(t, db, ctx, helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Kinds = []int{nostr.KindProfileMetadata}
		}))
		require.NoError(t, err)
		require.Len(t, stored, 1)
		require.Contains(t, stored, publishedEvent)
		require.NoError(t, db.AcceptEvent(ctx, &model.Event{
			Event: nostr.Event{
				ID:        "deletion event" + uuid.NewString(),
				PubKey:    publishedEvent.PubKey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindDeletion,
				Tags:      model.Tags{}.AppendUnique(nostr.Tag{"a", fmt.Sprintf("%v:%v:", nostr.KindProfileMetadata, publishedEvent.PubKey)}),
				Content:   "bogus" + uuid.NewString(),
				Sig:       "bogus" + uuid.NewString(),
			},
		}))
		stored, err = helperGetStoredEventsAll(t, db, ctx, helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Kinds = []int{nostr.KindProfileMetadata}
		}))
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
				Tags:      model.Tags{}.AppendUnique(nostr.Tag{"d", "bogus"}),
				Content:   "{\"name\": \"bogus\", \"about\": \"bogus\", \"picture\": \"bogus\"}",
				Sig:       "bogus" + uuid.NewString(),
			},
		}
		require.NoError(t, db.AcceptEvent(ctx, publishedEvent))
		stored, err := helperGetStoredEventsAll(t, db, ctx, helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Kinds = []int{nostr.KindArticle}
		}))
		require.NoError(t, err)
		require.Len(t, stored, 1)
		require.Contains(t, stored, publishedEvent)
		require.NoError(t, db.AcceptEvent(ctx, &model.Event{
			Event: nostr.Event{
				ID:        "deletion event" + uuid.NewString(),
				PubKey:    publishedEvent.PubKey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindDeletion,
				Tags:      model.Tags{}.AppendUnique(nostr.Tag{"a", fmt.Sprintf("%v:%v:bogus", nostr.KindArticle, publishedEvent.PubKey)}),
				Content:   "bogus" + uuid.NewString(),
				Sig:       "bogus" + uuid.NewString(),
			},
		}))
		stored, err = helperGetStoredEventsAll(t, db, ctx, helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Kinds = []int{nostr.KindProfileMetadata}
		}))
		require.NoError(t, err)
		require.Empty(t, stored)
	})
}

func TestNIP25ReactionEvents(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
	t.Run("reaction to non-replaceable event", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()
		publishedEvent := &model.Event{
			Event: nostr.Event{
				ID:        "normal" + uuid.NewString(),
				PubKey:    "bogus" + uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindTextNote,
				Tags:      model.Tags{},
				Content:   "bogus" + uuid.NewString(),
				Sig:       "bogus" + uuid.NewString(),
			},
		}
		require.NoError(t, db.AcceptEvent(ctx, publishedEvent))
		stored, err := helperGetStoredEventsAll(t, db, ctx, helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Kinds = []int{nostr.KindTextNote}
		}))
		require.NoError(t, err)
		require.Len(t, stored, 1)
		require.Contains(t, stored, publishedEvent)
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "first"}, nostr.Tag{"e", "second"}, nostr.Tag{"e", publishedEvent.ID})
		tags = append(tags, nostr.Tag{"p", "first"}, nostr.Tag{"p", "second"}, nostr.Tag{"p", publishedEvent.PubKey})
		require.NoError(t, db.AcceptEvent(ctx, &model.Event{
			Event: nostr.Event{
				ID:        "reaction event" + uuid.NewString(),
				PubKey:    publishedEvent.PubKey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindReaction,
				Tags:      tags,
				Content:   "+",
				Sig:       "bogus" + uuid.NewString(),
			},
		}))
		stored, err = helperGetStoredEventsAll(t, db, ctx, helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Kinds = []int{nostr.KindReaction}
		}))
		require.NoError(t, err)
		require.Len(t, stored, 1)
	})
	t.Run("reaction to non-replaceable event with k tag", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()
		publishedEvent := &model.Event{
			Event: nostr.Event{
				ID:        "normal" + uuid.NewString(),
				PubKey:    "bogus" + uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindTextNote,
				Tags:      model.Tags{},
				Content:   "bogus" + uuid.NewString(),
				Sig:       "bogus" + uuid.NewString(),
			},
		}
		require.NoError(t, db.AcceptEvent(ctx, publishedEvent))
		stored, err := helperGetStoredEventsAll(t, db, ctx, helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Kinds = []int{nostr.KindTextNote}
		}))
		require.NoError(t, err)
		require.Len(t, stored, 1)
		require.Contains(t, stored, publishedEvent)
		require.NoError(t, db.AcceptEvent(ctx, &model.Event{
			Event: nostr.Event{
				ID:        "reaction event" + uuid.NewString(),
				PubKey:    publishedEvent.PubKey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindReaction,
				Tags:      model.Tags{}.AppendUnique(nostr.Tag{"e", publishedEvent.ID}).AppendUnique(nostr.Tag{"p", publishedEvent.PubKey}).AppendUnique(nostr.Tag{"k", fmt.Sprint(nostr.KindTextNote)}),
				Content:   "+",
				Sig:       "bogus" + uuid.NewString(),
			},
		}))
		stored, err = helperGetStoredEventsAll(t, db, ctx, helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Kinds = []int{nostr.KindReaction}
		}))
		require.NoError(t, err)
		require.Len(t, stored, 1)
	})
	t.Run("reaction to replaceable event with a tag", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()
		dTag := "dummy"
		publishedEvent := &model.Event{
			Event: nostr.Event{
				ID:        "normal" + uuid.NewString(),
				PubKey:    "bogus" + uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindProfileMetadata,
				Tags:      model.Tags{}.AppendUnique(nostr.Tag{"d", dTag}),
				Content:   "bogus" + uuid.NewString(),
				Sig:       "bogus" + uuid.NewString(),
			},
		}
		require.NoError(t, db.AcceptEvent(ctx, publishedEvent))
		stored, err := helperGetStoredEventsAll(t, db, ctx, helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Kinds = []int{nostr.KindProfileMetadata}
		}))
		require.NoError(t, err)
		require.Len(t, stored, 1)
		require.Contains(t, stored, publishedEvent)
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "first"}, nostr.Tag{"e", "second"}, nostr.Tag{"e", publishedEvent.ID})
		tags = append(tags, nostr.Tag{"p", "first"}, nostr.Tag{"p", "second"}, nostr.Tag{"p", publishedEvent.PubKey})
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:%v:%v", nostr.KindProfileMetadata, publishedEvent.PubKey, dTag)})
		require.NoError(t, db.AcceptEvent(ctx, &model.Event{
			Event: nostr.Event{
				ID:        "reaction event" + uuid.NewString(),
				PubKey:    publishedEvent.PubKey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindReaction,
				Tags:      tags,
				Content:   "+",
				Sig:       "bogus" + uuid.NewString(),
			},
		}))
		stored, err = helperGetStoredEventsAll(t, db, ctx, helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Kinds = []int{nostr.KindReaction}
		}))
		require.NoError(t, err)
		require.Len(t, stored, 1)
	})
	t.Run("invalid: reaction to normal event e tag", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()
		publishedEvent := &model.Event{
			Event: nostr.Event{
				ID:        "normal" + uuid.NewString(),
				PubKey:    "bogus" + uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindTextNote,
				Tags:      model.Tags{},
				Content:   "bogus" + uuid.NewString(),
				Sig:       "bogus" + uuid.NewString(),
			},
		}
		require.NoError(t, db.AcceptEvent(ctx, publishedEvent))
		stored, err := helperGetStoredEventsAll(t, db, ctx, helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Kinds = []int{nostr.KindTextNote}
		}))
		require.NoError(t, err)
		require.Len(t, stored, 1)
		require.Contains(t, stored, publishedEvent)
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", publishedEvent.ID}, nostr.Tag{"e", "first"}, nostr.Tag{"e", "second"})
		tags = append(tags, nostr.Tag{"p", "first"}, nostr.Tag{"p", "second"}, nostr.Tag{"p", publishedEvent.PubKey})
		require.Error(t, db.AcceptEvent(ctx, &model.Event{
			Event: nostr.Event{
				ID:        "reaction event" + uuid.NewString(),
				PubKey:    publishedEvent.PubKey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindReaction,
				Tags:      tags,
				Content:   "+",
				Sig:       "bogus" + uuid.NewString(),
			},
		}))
		stored, err = helperGetStoredEventsAll(t, db, ctx, helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Kinds = []int{nostr.KindReaction}
		}))
		require.NoError(t, err)
		require.Empty(t, stored)
	})
	t.Run("invalid: reaction to normal event p tag", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()
		publishedEvent := &model.Event{
			Event: nostr.Event{
				ID:        "normal" + uuid.NewString(),
				PubKey:    "bogus" + uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindTextNote,
				Tags:      model.Tags{},
				Content:   "bogus" + uuid.NewString(),
				Sig:       "bogus" + uuid.NewString(),
			},
		}
		require.NoError(t, db.AcceptEvent(ctx, publishedEvent))
		stored, err := helperGetStoredEventsAll(t, db, ctx, helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Kinds = []int{nostr.KindTextNote}
		}))
		require.NoError(t, err)
		require.Len(t, stored, 1)
		require.Contains(t, stored, publishedEvent)
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "first"}, nostr.Tag{"e", "second"}, nostr.Tag{"e", publishedEvent.ID})
		tags = append(tags, nostr.Tag{"p", publishedEvent.PubKey}, nostr.Tag{"p", "first"}, nostr.Tag{"p", "second"})
		require.Error(t, db.AcceptEvent(ctx, &model.Event{
			Event: nostr.Event{
				ID:        "reaction event" + uuid.NewString(),
				PubKey:    publishedEvent.PubKey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindReaction,
				Tags:      tags,
				Content:   "+",
				Sig:       "bogus" + uuid.NewString(),
			},
		}))
		stored, err = helperGetStoredEventsAll(t, db, ctx, helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Kinds = []int{nostr.KindReaction}
		}))
		require.NoError(t, err)
		require.Empty(t, stored)
	})
	t.Run("invalid: reaction to replaceable event with wrong a tag", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()
		dTag := "dummy"
		publishedEvent := &model.Event{
			Event: nostr.Event{
				ID:        "normal" + uuid.NewString(),
				PubKey:    "bogus" + uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindProfileMetadata,
				Tags:      model.Tags{}.AppendUnique(nostr.Tag{"d", dTag}),
				Content:   "bogus" + uuid.NewString(),
				Sig:       "bogus" + uuid.NewString(),
			},
		}
		require.NoError(t, db.AcceptEvent(ctx, publishedEvent))
		stored, err := helperGetStoredEventsAll(t, db, ctx, helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Kinds = []int{nostr.KindProfileMetadata}
		}))
		require.NoError(t, err)
		require.Len(t, stored, 1)
		require.Contains(t, stored, publishedEvent)
		var tags nostr.Tags
		tags = append(tags, nostr.Tag{"e", "first"}, nostr.Tag{"e", "second"}, nostr.Tag{"e", publishedEvent.ID})
		tags = append(tags, nostr.Tag{"p", "first"}, nostr.Tag{"p", "second"}, nostr.Tag{"p", publishedEvent.PubKey})
		tags = append(tags, nostr.Tag{"a", fmt.Sprintf("%v:%v:%v", nostr.KindProfileMetadata, publishedEvent.PubKey, "wrong d tag")})
		require.Error(t, db.AcceptEvent(ctx, &model.Event{
			Event: nostr.Event{
				ID:        "reaction event" + uuid.NewString(),
				PubKey:    publishedEvent.PubKey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindReaction,
				Tags:      tags,
				Content:   "+",
				Sig:       "bogus" + uuid.NewString(),
			},
		}))
		stored, err = helperGetStoredEventsAll(t, db, ctx, helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Kinds = []int{nostr.KindReaction}
		}))
		require.NoError(t, err)
		require.Empty(t, stored)
	})
}

func helperNewFilter(f func(apply *model.Filter)) model.Filter {
	var filter model.Filter

	f(&filter)

	return filter
}

func helperNewSingleFilter(f func(apply *model.Filter)) model.Filters {
	return model.Filters{helperNewFilter(f)}
}

func helperNewFilterSubscription(f func(apply *model.Filter)) *model.Subscription {
	return &model.Subscription{Filters: helperNewSingleFilter(f)}
}

func TestSaveEventWithRepost(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	t.Run("Regular", func(t *testing.T) {
		var event model.Event

		tags := model.Tags{{"imeta", "m video/mp4"}}
		event.Kind = nostr.KindTextNote
		event.ID = generateHexString()
		event.PubKey = "1"
		event.Tags = slices.Clone(tags)
		event.CreatedAt = 1

		err := db.AcceptEvent(context.TODO(), &event)
		require.NoError(t, err)

		t.Run("CheckSelect", func(t *testing.T) {
			var event2 model.Event
			for ev, err := range db.SelectEvents(context.TODO(), helperNewFilterSubscription(func(apply *model.Filter) {
				apply.IDs = []string{event.ID}
			})) {
				require.NoError(t, err)
				event2 = *ev

				break
			}

			event.Tags = slices.Clone(tags)
			require.Equal(t, event, event2)
		})

		t.Run("CheckTags", func(t *testing.T) {
			var mime string

			err := db.QueryRow("SELECT "+tagValueMimeType+" FROM event_tags WHERE event_id = $1", event.ID).Scan(&mime)
			require.NoError(t, err)
			require.Equal(t, "m video/mp4", mime)
		})
	})

	t.Run("Repost", func(t *testing.T) {
		var event model.Event

		event.Kind = nostr.KindRepost
		event.ID = generateHexString()
		event.PubKey = "2"
		event.Tags = model.Tags{{"e", "2"}}
		event.CreatedAt = 2
		event.Content = `{"id":"3","pubkey":"4","created_at":1712594952,"kind":1,"tags":[["imeta","url https://example.com/foo.jpg","ox f63ccef25fcd9b9a181ad465ae40d282eeadd8a4f5c752434423cb0539f73e69 https://nostr.build","x f9c8b660532a6e8236779283950d875fbfbdc6f4dbc7c675bc589a7180299c30","m image/jpeg","dim 1066x1600","bh L78C~=$%0%ERjENbWX$g0jNI}:-S","blurhash L78C~=$%0%ERjENbWX$g0jNI}:-S"]],"content":"foo","sig":"sig"}`

		err := db.AcceptEvent(context.TODO(), &event)
		require.NoError(t, err)

		t.Run("CheckTags", func(t *testing.T) {
			var (
				mime string
				url  string
			)

			err := db.QueryRow("SELECT "+tagValueURL+", "+tagValueMimeType+" FROM event_tags WHERE event_id = $1", "3").Scan(&url, &mime)
			require.NoError(t, err)
			require.Equal(t, "m image/jpeg", mime)
			require.Equal(t, "url https://example.com/foo.jpg", url)
		})

		t.Run("CheckEventLink", func(t *testing.T) {
			var id sql.NullString
			err := db.QueryRow("SELECT reference_id FROM events WHERE id = $1", event.ID).Scan(&id)
			require.NoError(t, err)
			require.True(t, id.Valid)
			require.Equal(t, "3", id.String)
		})

		t.Run("NoOverrideRepost", func(t *testing.T) {
			var event model.Event

			event.Kind = nostr.KindRepost
			event.ID = generateHexString()
			event.PubKey = "2"
			event.Tags = model.Tags{{"e", "2"}}
			event.CreatedAt = 2
			event.Content = `{"id":"3","pubkey":"4","created_at":1712594952,"kind":1,"tags":[["imeta","url https://example.com/foo.jpg","ox f63ccef25fcd9b9a181ad465ae40d282eeadd8a4f5c752434423cb0539f73e69 https://nostr.build","x f9c8b660532a6e8236779283950d875fbfbdc6f4dbc7c675bc589a7180299c30","m image/jpeg","dim 1066x1600","bh L78C~=$%0%ERjENbWX$g0jNI}:-S","blurhash L78C~=$%0%ERjENbWX$g0jNI}:-S"]],"content":"foo","sig":"sig"}`

			err := db.AcceptEvent(context.TODO(), &event)
			require.NoError(t, err)
		})
	})
}

func TestQueryEventWithTagsReorderAndSignature(t *testing.T) {
	t.Parallel()

	pk := nostr.GeneratePrivateKey()
	require.NotEmpty(t, pk)

	var ev model.Event
	ev.Tags = model.Tags{{"a", "b"}, {"imeta", "foo", "bar", "m image/png"}, {"c", "d"}}
	ev.Content = "some tags and content here"
	ev.CreatedAt = 1
	ev.Kind = nostr.KindTextNote

	require.NoError(t, ev.Sign(pk))
	t.Logf("event id: %s (sign %v)", ev.ID, ev.Sig)

	ok, err := ev.CheckSignature()
	require.NoError(t, err)
	require.True(t, ok)

	t.Run("SingleEvent", func(t *testing.T) {
		db := helperNewDatabase(t)
		defer db.Close()
		t.Run("Save", func(t *testing.T) {
			err := db.AcceptEvent(context.Background(), &ev)
			require.NoError(t, err)
		})
		t.Run("ByID", func(t *testing.T) {
			events, err := helperGetStoredEventsAll(t, db, context.TODO(), helperNewFilterSubscription(func(apply *model.Filter) {
				apply.IDs = []string{ev.ID}
			}))
			require.NoError(t, err)
			require.Len(t, events, 1)
			t.Logf("event = %+v", events[0])
			ok, err := events[0].CheckSignature()
			require.NoError(t, err)
			require.True(t, ok)
		})
		t.Run("ByMimeType", func(t *testing.T) {
			events, err := helperGetStoredEventsAll(t, db, context.TODO(), helperNewFilterSubscription(func(apply *model.Filter) {
				apply.Search = "images:true"
			}))
			require.NoError(t, err)
			require.Len(t, events, 1)
			t.Logf("event = %+v", events[0])
			ok, err := events[0].CheckSignature()
			require.NoError(t, err)
			require.True(t, ok)
		})
	})
	t.Run("RepostEvent", func(t *testing.T) {
		var repostEvent model.Event

		pk2 := nostr.GeneratePrivateKey()
		require.NotEmpty(t, pk2)

		data, err := ev.MarshalJSON()
		require.NoError(t, err)

		repostEvent.Content = string(data)
		repostEvent.CreatedAt = 2
		repostEvent.Kind = nostr.KindRepost

		require.NoError(t, repostEvent.Sign(pk2))
		t.Logf("event id: %s (sign %v)", repostEvent.ID, repostEvent.Sig)

		db := helperNewDatabase(t)
		defer db.Close()

		t.Run("Save", func(t *testing.T) {
			err := db.AcceptEvent(context.Background(), &repostEvent)
			require.NoError(t, err)
		})
		t.Run("ByID", func(t *testing.T) {
			events, err := helperGetStoredEventsAll(t, db, context.TODO(), helperNewFilterSubscription(func(apply *model.Filter) {
				apply.IDs = []string{repostEvent.ID}
			}))
			require.NoError(t, err)
			require.Len(t, events, 1)
			t.Logf("event = %+v", events[0])
			ok, err := events[0].CheckSignature()
			require.NoError(t, err)
			require.True(t, ok)
		})
		t.Run("ByMimeType", func(t *testing.T) {
			events, err := helperGetStoredEventsAll(t, db, context.TODO(), helperNewFilterSubscription(func(apply *model.Filter) {
				apply.Search = "images:true"
				apply.Kinds = []int{nostr.KindRepost}
			}))
			require.NoError(t, err)
			require.Len(t, events, 1)
			t.Logf("event = %+v", events[0])
			require.Equal(t, repostEvent.ID, events[0].ID) // Should be the reposted event.
			ok, err := events[0].CheckSignature()
			require.NoError(t, err)
			require.True(t, ok)
		})
		t.Run("Count", func(t *testing.T) {
			count, err := db.CountEvents(context.TODO(), helperNewFilterSubscription(func(apply *model.Filter) {}))
			require.NoError(t, err)
			require.Equal(t, int64(1), count) // Only the reposted event should be counted.
		})
	})
}

func TestQueryEventAttestation(t *testing.T) {
	t.Parallel()

	const (
		master = "c24f7ab5b42254d6558e565ec1c170b266a7cd2be1edf9f42bfb375640f7f559"
		active = "3c00c01e6556c4b603b4c49d12059e02c42161d055b658e5635fa6206f594306"
	)

	masterPk, err := nostr.GetPublicKey(master)
	require.NoError(t, err)

	activePk, err := nostr.GetPublicKey(active)
	require.NoError(t, err)

	t.Logf("master   public key: %s", masterPk)
	t.Logf("onbehalf public key: %s", activePk)

	db := helperNewDatabase(t)
	defer db.Close()

	now := time.Now().Unix()

	t.Run("AddAttestation", func(t *testing.T) {
		var ev model.Event

		t.Log("add first attestation")
		ev.Kind = model.CustomIONKindAttestation
		ev.CreatedAt = 1
		ev.Tags = model.Tags{{model.TagAttestationName, activePk, "", model.CustomIONAttestationKindActive + ":" + strconv.FormatInt(now, 10)}}
		require.NoError(t, ev.Sign(master))
		t.Logf("event %+v", ev)
		require.NoError(t, db.AcceptEvent(context.TODO(), &ev))

		count, err := db.CountEvents(context.TODO(), helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Kinds = []int{model.CustomIONKindAttestation}
			apply.Authors = []string{masterPk}
		}))
		require.NoError(t, err)
		require.Equal(t, int64(1), count)
		t.Run("TryOverride", func(t *testing.T) {
			ev.Kind = model.CustomIONKindAttestation
			ev.CreatedAt = 2
			ev.Tags = model.Tags{
				{model.TagAttestationName, activePk, "", model.CustomIONAttestationKindActive + ":" + strconv.FormatInt(now-1, 10)},
			}
			require.NoError(t, ev.Sign(master))
			t.Logf("event %+v", ev)
			require.ErrorIs(t, db.AcceptEvent(context.TODO(), &ev), ErrAttestationUpdateRejected)
		})

		t.Log("add second attestation")
		ev.Kind = model.CustomIONKindAttestation
		ev.CreatedAt = 3
		ev.Tags = model.Tags{
			{model.TagAttestationName, activePk, "", model.CustomIONAttestationKindActive + ":" + strconv.FormatInt(now, 10)},
			{model.TagAttestationName, activePk, "", model.CustomIONAttestationKindActive + ":" + strconv.FormatInt(now-20, 10)},
			{model.TagAttestationName, activePk, "", model.CustomIONAttestationKindInactive + ":" + strconv.FormatInt(now-10, 10)},
			{model.TagAttestationName, activePk, "", model.CustomIONAttestationKindActive + ":" + strconv.FormatInt(now-5, 10)},
		}
		require.NoError(t, ev.Sign(master))
		t.Logf("event %+v", ev)
		require.NoError(t, db.AcceptEvent(context.TODO(), &ev))

		count, err = db.CountEvents(context.TODO(), helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Kinds = []int{model.CustomIONKindAttestation}
			apply.Authors = []string{masterPk}
		}))
		require.NoError(t, err)
		require.Equal(t, int64(1), count)
	})
	t.Run("Publish", func(t *testing.T) {
		t.Run("AsMaster", func(t *testing.T) {
			var ev model.Event
			ev.Kind = nostr.KindTextNote
			ev.CreatedAt = 1
			ev.Content = "hello world"
			require.NoError(t, ev.Sign(master))
			require.NoError(t, db.AcceptEvent(context.TODO(), &ev))
		})
		t.Run("OnBehalf", func(t *testing.T) {
			var ev model.Event
			ev.Kind = nostr.KindTextNote
			ev.CreatedAt = 2
			ev.Content = "hello world from active"
			ev.Tags = model.Tags{{model.CustomIONTagOnBehalfOf, masterPk}}
			require.NoError(t, ev.Sign(active))
			t.Logf("event %+v", ev)
			require.NoError(t, db.AcceptEvent(context.TODO(), &ev))
		})
		t.Run("OnBehalfOfUnknownUser", func(t *testing.T) {
			var ev model.Event
			ev.Kind = nostr.KindTextNote
			ev.CreatedAt = 3
			ev.Content = "hello world from non-existing user"
			ev.Tags = model.Tags{{model.CustomIONTagOnBehalfOf, nostr.GeneratePrivateKey()}}
			require.NoError(t, ev.Sign(active))
			t.Logf("event %+v", ev)
			require.ErrorIs(t, db.AcceptEvent(context.TODO(), &ev), ErrOnBehalfAccessDenied)
		})
		t.Run("Count", func(t *testing.T) {
			count, err := db.CountEvents(context.TODO(), helperNewFilterSubscription(func(apply *model.Filter) {
				apply.Kinds = []int{nostr.KindTextNote}
				apply.Authors = []string{masterPk}
			}))
			require.NoError(t, err)
			require.Equal(t, int64(2), count) // Both events should be counted, master + on behalf.
		})
	})
}

func TestEventDeleteWithAttestation(t *testing.T) {
	t.Parallel()

	const (
		masterPrivate = "c24f7ab5b42254d6558e565ec1c170b266a7cd2be1edf9f42bfb375640f7f559"
		masterPublic  = "e08ab1786373d6bde8ce1d790a08730536bab1b5dbc6fb603def1d6110a707f9"

		user1Private = "3c00c01e6556c4b603b4c49d12059e02c42161d055b658e5635fa6206f594306"
		user1Public  = "9c7e93ead06f045703bdcbbab442b158a2093b3b0e1e389cc5b0d4884849c6a9"

		user2Private = "cea41ff6c6e9eb0cde6740a1fbe8c134bda650ce819e43b68bf61add2c68f8d9"
		user2Public  = "32d45e035d10fd630bd315215370cc2c694f2eb79487bb84abc30c855503a98c"

		hackerPrivate = "e8eb18e16a3bf3ad88c448551d026e586fa996a27aaeda908fe714779cda4017"
		hackerPublic  = "48cc7f09058b32f2bfe31bee016098a2b6b302dc02724986b38d7763e849ca84"
	)

	db := helperNewDatabase(t)
	defer db.Close()
	now := time.Now().Unix()

	baseAttestation := model.Tags{
		{model.TagAttestationName, user1Public, "", model.CustomIONAttestationKindActive + ":" + strconv.Itoa(int(now-10))},
		{model.TagAttestationName, user2Public, "", model.CustomIONAttestationKindActive + ":" + strconv.Itoa(int(now-5))},
	}
	masterMessageIds := []string{}
	user1MessageIds := []string{}
	user2MessageIds := []string{}

	counter := func(t *testing.T, kinds []int, ids, authors []string) int64 {
		count, err := db.CountEvents(context.TODO(), helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Authors = authors
			apply.Kinds = kinds
			apply.IDs = ids
		}))
		require.NoError(t, err)
		return count
	}
	mustBeZero := func(t *testing.T, id string) {
		require.Zero(t, counter(t, nil, []string{id}, nil))
	}
	mustBeOne := func(t *testing.T, id string) {
		require.Equal(t, int64(1), counter(t, nil, []string{id}, nil))
	}

	t.Run("AddAttestation", func(t *testing.T) {
		var ev model.Event
		ev.Kind = model.CustomIONKindAttestation
		ev.CreatedAt = 1
		ev.Tags = baseAttestation
		require.NoError(t, ev.Sign(masterPrivate))
		require.NoError(t, db.AcceptEvent(context.TODO(), &ev))
	})
	t.Run("AddEvents", func(t *testing.T) {
		t.Run("Master", func(t *testing.T) {
			for n := range 2 {
				var ev model.Event
				ev.Kind = nostr.KindTextNote
				ev.CreatedAt = model.Timestamp(3 + n)
				ev.Content = "hello world" + strconv.Itoa(n)
				require.NoError(t, ev.Sign(masterPrivate))
				masterMessageIds = append(masterMessageIds, ev.ID)
				require.NoError(t, db.AcceptEvent(context.TODO(), &ev))
			}
		})
		t.Run("Master of behalf of user1", func(t *testing.T) {
			for n := range 2 {
				var ev model.Event
				ev.Kind = nostr.KindTextNote
				ev.CreatedAt = model.Timestamp(5 + n)
				ev.Content = "hello world from user1 number" + strconv.Itoa(n)
				ev.Tags = model.Tags{{model.CustomIONTagOnBehalfOf, masterPublic}}
				require.NoError(t, ev.Sign(user1Private))
				user1MessageIds = append(user1MessageIds, ev.ID)
				require.NoError(t, db.AcceptEvent(context.TODO(), &ev))
			}
			t.Logf("user 1 messages = %v", user1MessageIds)
		})
		t.Run("Master of behalf of user2", func(t *testing.T) {
			for n := range 2 {
				var ev model.Event
				ev.Kind = nostr.KindTextNote
				ev.CreatedAt = model.Timestamp(7 + n)
				ev.Content = "hello world from user2 number" + strconv.Itoa(n)
				ev.Tags = model.Tags{{model.CustomIONTagOnBehalfOf, masterPublic}}
				require.NoError(t, ev.Sign(user2Private))
				user2MessageIds = append(user2MessageIds, ev.ID)
				require.NoError(t, db.AcceptEvent(context.TODO(), &ev))
			}
			t.Logf("user 2 messages = %v", user2MessageIds)
		})
		t.Run("Count", func(t *testing.T) {
			require.Equal(t, int64(6), counter(t, []int{nostr.KindTextNote}, nil, []string{masterPublic}))
		})
		t.Run("User2 could not add master attestation", func(t *testing.T) {
			var ev model.Event
			ev.Kind = model.CustomIONKindAttestation
			ev.CreatedAt = 11
			ev.Tags = append(ev.Tags, baseAttestation...)
			ev.Tags = append(ev.Tags, model.Tag{model.TagAttestationName, hackerPublic, "", model.CustomIONAttestationKindActive + ":" + strconv.Itoa(int(now-1))})
			ev.Tags = append(ev.Tags, model.Tag{model.CustomIONTagOnBehalfOf, masterPublic})
			require.NoError(t, ev.Sign(user2Private))
			require.ErrorIs(t, db.AcceptEvent(context.TODO(), &ev), ErrOnBehalfAccessDenied)
		})
	})
	t.Run("DeleteEvents", func(t *testing.T) {
		t.Run("Master could remove events of user1", func(t *testing.T) {
			var ev model.Event
			ev.Kind = nostr.KindDeletion
			ev.CreatedAt = 10
			ev.Tags = model.Tags{{"e", user1MessageIds[0]}}
			require.NoError(t, ev.Sign(masterPrivate))
			require.NoError(t, db.AcceptEvent(context.TODO(), &ev))

			mustBeZero(t, user1MessageIds[0])
			require.Equal(t, int64(5), counter(t, []int{nostr.KindTextNote}, nil, []string{masterPublic}))
		})
		t.Run("User2 could remove events of user1", func(t *testing.T) {
			var ev model.Event
			ev.Kind = nostr.KindDeletion
			ev.CreatedAt = 11
			ev.Tags = model.Tags{{"e", user1MessageIds[1]}}
			require.NoError(t, ev.Sign(user2Private))
			require.NoError(t, db.AcceptEvent(context.TODO(), &ev))

			mustBeZero(t, user1MessageIds[1])
			require.Equal(t, int64(4), counter(t, []int{nostr.KindTextNote}, nil, []string{masterPublic}))
		})
		t.Run("Hacker could not remove events of user2 nor master", func(t *testing.T) {
			var ev model.Event
			ev.Kind = nostr.KindDeletion
			ev.CreatedAt = 11
			ev.Tags = model.Tags{{"e", user2MessageIds[0]}}
			require.NoError(t, ev.Sign(hackerPrivate))
			require.Error(t, db.AcceptEvent(context.TODO(), &ev))

			ev.Tags = model.Tags{{"e", masterMessageIds[0]}}
			require.NoError(t, ev.Sign(hackerPrivate))
			require.Error(t, db.AcceptEvent(context.TODO(), &ev))
		})
		t.Run("User1 could not remove master events", func(t *testing.T) {
			var ev model.Event
			ev.Kind = nostr.KindDeletion
			ev.CreatedAt = 11
			ev.Tags = model.Tags{{"e", masterMessageIds[1]}}
			require.NoError(t, ev.Sign(user2Private))
			require.Error(t, db.AcceptEvent(context.TODO(), &ev))
			mustBeOne(t, masterMessageIds[1])
			require.Equal(t, int64(4), counter(t, []int{nostr.KindTextNote}, nil, []string{masterPublic}))
		})
	})
	t.Run("Rewoke", func(t *testing.T) {
		t.Run("Revoke attestation of user1", func(t *testing.T) {
			var ev model.Event
			ev.Kind = model.CustomIONKindAttestation
			ev.CreatedAt = 12
			ev.Tags = append(ev.Tags, baseAttestation...)
			ev.Tags = append(ev.Tags, model.Tag{model.TagAttestationName, user1Public, "", model.CustomIONAttestationKindRevoked + ":" + strconv.Itoa(int(now-3))})
			require.NoError(t, ev.Sign(masterPrivate))
			require.NoError(t, db.AcceptEvent(context.TODO(), &ev))
		})
		t.Run("User1 could not remove events of user2", func(t *testing.T) {
			var ev model.Event
			ev.Kind = nostr.KindDeletion
			ev.CreatedAt = 11
			ev.Tags = model.Tags{{"e", user2MessageIds[0]}}
			require.NoError(t, ev.Sign(user1Private))
			require.Error(t, db.AcceptEvent(context.TODO(), &ev))
			mustBeOne(t, user2MessageIds[0])
		})
	})
}
