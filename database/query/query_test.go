package query

import (
	"context"
	"github.com/google/uuid"
	"github.com/ice-blockchain/subzero/model"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

const testDeadline = 30 * time.Second

func TestReplaceEvents(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()
	t.Run("normal, non-replaceable event", func(t *testing.T) {
		MustInit()
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
		require.NoError(t, AcceptEvent(ctx, expectedEvents[0]))
		expectedEvents = append(expectedEvents, &model.Event{
			Event: nostr.Event{
				ID:        "normal, 2nd event" + uuid.NewString(),
				PubKey:    "bogus" + uuid.NewString(),
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindTextNote,
				Tags:      nostr.Tags{},
				Content:   "bogus" + uuid.NewString(),
				Sig:       "bogus" + uuid.NewString(),
			},
		})
		require.NoError(t, AcceptEvent(ctx, expectedEvents[1]))
		stored, err := GetStoredEvents(ctx, &model.Subscription{Filters: []nostr.Filter{{Kinds: []int{nostr.KindTextNote}}}})
		require.NoError(t, err)
		require.Len(t, stored, 2)
		require.Contains(t, stored, expectedEvents[0])
		require.Contains(t, stored, expectedEvents[1])
	})
	t.Run("ephemeral event", func(t *testing.T) {
		MustInit()
		require.NoError(t, AcceptEvent(ctx, &model.Event{
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
		stored, err := GetStoredEvents(ctx, &model.Subscription{Filters: []nostr.Filter{{Kinds: []int{nostr.KindTextNote}}}})
		require.NoError(t, err)
		require.Empty(t, stored)
	})
	t.Run("replaceable event", func(t *testing.T) {
		MustInit()
		expectedEvents := []*model.Event{}
		require.NoError(t, AcceptEvent(ctx, &model.Event{
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
		require.NoError(t, AcceptEvent(ctx, expectedEvents[0]))
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
		require.NoError(t, AcceptEvent(ctx, expectedEvents[1]))
		stored, err := GetStoredEvents(ctx, &model.Subscription{Filters: []nostr.Filter{{Kinds: []int{nostr.KindContactList}}}})
		require.NoError(t, err)
		require.Len(t, stored, 2)
		expectedEvents[0].Tags = nostr.Tags{} // Tags fetching not implemented yet.
		expectedEvents[1].Tags = nostr.Tags{}
		require.Contains(t, stored, expectedEvents[0])
		require.Contains(t, stored, expectedEvents[1])
	})
	t.Run("param replaceable event", func(t *testing.T) {
		MustInit()
		expectedEvents := []*model.Event{}
		require.NoError(t, AcceptEvent(ctx, &model.Event{
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
		require.NoError(t, AcceptEvent(ctx, expectedEvents[0]))
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
		require.NoError(t, AcceptEvent(ctx, expectedEvents[1]))
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
		require.NoError(t, AcceptEvent(ctx, expectedEvents[2]))
		stored, err := GetStoredEvents(ctx, &model.Subscription{Filters: []nostr.Filter{{Kinds: []int{nostr.KindContactList}}}})
		require.NoError(t, err)
		require.Len(t, stored, 3)
		expectedEvents[0].Tags = nostr.Tags{} // Tags fetching not implemented yet.
		expectedEvents[1].Tags = nostr.Tags{}
		expectedEvents[2].Tags = nostr.Tags{}
		require.Contains(t, stored, expectedEvents[0])
		require.Contains(t, stored, expectedEvents[1])
		require.Contains(t, stored, expectedEvents[2])
	})
}
