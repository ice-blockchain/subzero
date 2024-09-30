// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
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

		expectedEvents := []*model.Event{}
		require.NoError(t, db.AcceptEvent(ctx, &model.Event{
			Event: nostr.Event{
				ID:        "replaceable " + uuid.NewString(),
				PubKey:    "bogus",
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindFollowList,
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
				Kind:      nostr.KindFollowList,
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
				Kind:      nostr.KindFollowList,
				Tags: nostr.Tags{
					[]string{"p", uuid.NewString(), "wss://localhost:9999/"},
				},
				Content: "bogus" + uuid.NewString(),
				Sig:     "bogus" + uuid.NewString(),
			},
		})
		require.NoError(t, db.AcceptEvent(ctx, expectedEvents[1]))
		stored, err := helperGetStoredEventsAll(t, db, ctx, helperNewFilterSubscription(func(apply *model.Filter) {
			apply.Kinds = []int{nostr.KindFollowList}
		}))
		require.NoError(t, err)
		require.Len(t, stored, 2)
		require.Contains(t, stored, expectedEvents[0])
		require.Contains(t, stored, expectedEvents[1])
	})
}

func TestParametrizedReplaceableEvents(t *testing.T) {
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

		tags := model.Tags{{"imeta", "m video"}}
		event.Kind = nostr.KindTextNote
		event.ID = generateHexString()
		event.PubKey = "1"
		event.Tags = slices.Clone(tags)
		event.CreatedAt = 1

		err := db.SaveEvent(context.TODO(), &event)
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
			require.Equal(t, "m video", mime)
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
	})
}
