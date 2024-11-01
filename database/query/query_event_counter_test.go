// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"os"
	"strconv"
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func helperMustEventCount(t *testing.T, db *dbClient, expectedCount int64, f ...model.Filter) {
	t.Helper()

	count := helperMustGetPrecalculatedCounters(t, db, f...)
	require.Equal(t, expectedCount, count)
}

func TestEventCounters(t *testing.T) {
	t.Parallel()

	os.Remove("foo3.sqlite")
	db := openDatabase("foo3.sqlite", true)
	defer db.Close()

	helperFillDatabase(t, db, 10)

	t.Run("Quote", func(t *testing.T) {
		t.Run("Post", func(t *testing.T) {
			var ev model.Event
			ev.ID = "1"
			ev.Kind = nostr.KindTextNote
			ev.PubKey = "pubkey1"
			ev.CreatedAt = 1
			ev.Content = "content"
			require.NoError(t, db.AcceptEvents(context.Background(), &ev))
		})
		t.Run("Do", func(t *testing.T) {
			for i := range 3 {
				var q model.Event

				q.Kind = nostr.KindTextNote
				q.ID = "q" + strconv.Itoa(i)
				q.PubKey = "pubkey" + strconv.Itoa(i)
				q.CreatedAt = model.Timestamp(i)
				q.Tags = model.Tags{{"q", "1"}}

				require.NoError(t, db.AcceptEvents(context.Background(), &q))
			}
			helperMustEventCount(t, db, 3, model.Filter{Kinds: []int{nostr.KindTextNote}, Tags: model.TagMap{"q": nil}, IDs: []string{"1"}})
		})
		t.Run("Delete", func(t *testing.T) {
			var ev model.Event

			ev.PubKey = "pubkey2"
			ev.ID = "delete"
			ev.Kind = nostr.KindDeletion
			ev.Tags = model.Tags{{"e", "q2"}}
			require.NoError(t, db.AcceptEvents(context.Background(), &ev))

			c, err := db.CountEvents(context.Background(), nil)
			require.NoError(t, err)
			require.Equal(t, int64(13), c) // 11 posts, 2 quotes.
			helperMustEventCount(t, db, 2, model.Filter{Kinds: []int{nostr.KindTextNote}, Tags: model.TagMap{"q": nil}, IDs: []string{"1"}})
		})
		t.Run("Non existent", func(t *testing.T) {
			var q model.Event
			q.Kind = nostr.KindTextNote
			q.ID = "qnonexistent"
			q.PubKey = "pubkey"
			q.CreatedAt = 1
			q.Tags = model.Tags{{"q", "foo"}}
			require.NoError(t, db.AcceptEvents(context.Background(), &q))
			helperMustEventCount(t, db, 0, model.Filter{Kinds: []int{nostr.KindTextNote}, Tags: model.TagMap{"q": nil}, IDs: []string{"foo"}})
		})
	})
	t.Run("Folowers", func(t *testing.T) {
		t.Run("Init", func(t *testing.T) {
			var ev model.Event
			ev.ID = "1f"
			ev.Kind = nostr.KindFollowList
			ev.PubKey = "pubkeyf3"
			ev.CreatedAt = 1
			ev.Tags = model.Tags{
				{"p", "alicekey", "wss://alicerelay.com/", "alice"},
				{"p", "bobkey", "wss://bobrelay.com/nostr", "bob"},
				{"p", "carolkey", "ws://carolrelay.com/ws", "carol"},
			}
			require.NoError(t, db.AcceptEvents(context.Background(), &ev))
			for _, key := range []string{"alicekey", "bobkey", "carolkey"} {
				helperMustEventCount(t, db, 1, model.Filter{Kinds: []int{nostr.KindFollowList}, Authors: []string{key}})
			}
		})
		t.Run("RemoveCarol", func(t *testing.T) {
			var ev model.Event
			ev.ID = "2f"
			ev.Kind = nostr.KindFollowList
			ev.PubKey = "pubkeyf3"
			ev.CreatedAt = 1
			ev.Tags = model.Tags{
				{"p", "alicekey", "wss://alicerelay.com/", "alice"},
				{"p", "bobkey", "wss://bobrelay.com/nostr", "bob"},
			}
			require.NoError(t, db.AcceptEvents(context.Background(), &ev))
			for _, key := range []string{"alicekey", "bobkey"} {
				helperMustEventCount(t, db, 1, model.Filter{Kinds: []int{nostr.KindFollowList}, Authors: []string{key}})
			}
			helperMustEventCount(t, db, 0, model.Filter{Kinds: []int{nostr.KindFollowList}, Authors: []string{"carolkey"}})
		})
		t.Run("AddMegan", func(t *testing.T) {
			var ev model.Event
			ev.ID = "3f"
			ev.Kind = nostr.KindFollowList
			ev.PubKey = "pubkeyf3"
			ev.CreatedAt = 1
			ev.Tags = model.Tags{
				{"p", "alicekey", "wss://alicerelay.com/", "alice"},
				{"p", "bobkey", "wss://bobrelay.com/nostr", "bob"},
				{"p", "megankey", "wss://meganrelay.com/", "megan"},
			}
			require.NoError(t, db.AcceptEvents(context.Background(), &ev))
			helperMustEventCount(t, db, 3, model.Filter{Kinds: []int{nostr.KindFollowList}, Authors: []string{"alicekey", "bobkey", "megankey"}})
		})
		t.Run("AddMeganToJoe", func(t *testing.T) {
			var ev model.Event
			ev.ID = "5f"
			ev.Kind = nostr.KindFollowList
			ev.PubKey = "joef5"
			ev.CreatedAt = 1
			ev.Tags = model.Tags{
				{"p", "megankey", "wss://meganrelay.com/", "megan"},
			}
			require.NoError(t, db.AcceptEvents(context.Background(), &ev))
			helperMustEventCount(t, db, 2, model.Filter{Kinds: []int{nostr.KindFollowList}, Authors: []string{"megankey"}})
		})
		t.Run("RemoveOriginalList", func(t *testing.T) {
			var ev model.Event
			ev.ID = "4f"
			ev.Kind = nostr.KindDeletion
			ev.PubKey = "pubkeyf3"
			ev.Tags = model.Tags{{"e", "3f"}}
			require.NoError(t, db.AcceptEvents(context.Background(), &ev))
			for _, key := range []string{"alicekey", "bobkey"} {
				helperMustEventCount(t, db, 0, model.Filter{Kinds: []int{nostr.KindFollowList}, Authors: []string{key}})
			}
			helperMustEventCount(t, db, 1, model.Filter{Kinds: []int{nostr.KindFollowList}, Authors: []string{"megankey"}})
		})
	})
	t.Run("Reactions", func(t *testing.T) {
		t.Run("Post", func(t *testing.T) {
			var ev model.Event
			ev.ID = "1r"
			ev.Kind = nostr.KindTextNote
			ev.PubKey = "pubkeyr1"
			ev.CreatedAt = 1
			ev.Content = "content"
			require.NoError(t, db.AcceptEvents(context.Background(), &ev))
		})
		t.Run("Reaction simple", func(t *testing.T) {
			var ev model.Event
			ev.ID = "2r"
			ev.Kind = nostr.KindReaction
			ev.PubKey = "pubkeyr2"
			ev.CreatedAt = 2
			ev.Tags = model.Tags{{"e", "1r"}, {"p", "pubkeyr1"}, {"k", "1"}}
			require.NoError(t, db.AcceptEvents(context.Background(), &ev))
			helperMustEventCount(t, db, 1, model.Filter{Kinds: []int{nostr.KindReaction}, IDs: []string{"1r"}})
		})
		t.Run("Reaction with multiple E tags", func(t *testing.T) {
			var ev model.Event
			ev.ID = "3r"
			ev.Kind = nostr.KindReaction
			ev.PubKey = "pubkeyr3"
			ev.CreatedAt = 2
			ev.Tags = model.Tags{
				{"e", "ev1", "wss://relay1", "reply"},
				{"e", "ev2", "wss://relay2", "root"},
				{"e", "ev3", "wss://relay2", "reply"},
				{"e", "1r"},
				{"p", "pkey1"},
				{"p", "pkey2"},
				{"p", "pkey3"},
				{"p", "pubkeyr2"},
			}
			require.NoError(t, db.AcceptEvents(context.Background(), &ev))
			helperMustEventCount(t, db, 2, model.Filter{Kinds: []int{nostr.KindReaction}, IDs: []string{"1r"}})
		})
		t.Run("Delete", func(t *testing.T) {
			var ev model.Event
			ev.Kind = nostr.KindDeletion
			ev.PubKey = "pubkeyr2"
			ev.Tags = model.Tags{{"e", "2r"}}
			require.NoError(t, db.AcceptEvents(context.Background(), &ev))
			helperMustEventCount(t, db, 1, model.Filter{Kinds: []int{nostr.KindReaction}, IDs: []string{"1r"}})

			ev.PubKey = "pubkeyr3"
			ev.Tags = model.Tags{{"e", "3r"}}
			require.NoError(t, db.AcceptEvents(context.Background(), &ev))
			helperMustEventCount(t, db, 0, model.Filter{Kinds: []int{nostr.KindReaction}, IDs: []string{"1r"}})
		})
	})
	t.Run("Reply", func(t *testing.T) {
		t.Run("Post", func(t *testing.T) {
			var ev model.Event
			ev.ID = "1rp"
			ev.Kind = nostr.KindTextNote
			ev.PubKey = "pubkeyrp1"
			ev.CreatedAt = 1
			ev.Content = "content"
			require.NoError(t, db.AcceptEvents(context.Background(), &ev))
		})
		t.Run("Reply", func(t *testing.T) {
			var ev model.Event
			ev.ID = "2rp"
			ev.Kind = nostr.KindTextNote
			ev.PubKey = "pubkeyrp2"
			ev.CreatedAt = 2
			ev.Tags = model.Tags{{"e", "1rp", "", "reply", "pubkeyrp2"}, {"p", "pubkeyrp1"}}
			require.NoError(t, db.AcceptEvents(context.Background(), &ev))
			helperMustEventCount(t, db, 1, model.Filter{Kinds: []int{nostr.KindTextNote}, IDs: []string{"1rp"}})
		})
		t.Run("Repost with multiple E tags", func(t *testing.T) {
			var ev model.Event
			ev.ID = "14r"
			ev.Kind = nostr.KindRepost
			ev.PubKey = "pubkeyr3"
			ev.CreatedAt = 2
			ev.Tags = model.Tags{
				{"e", "1r", "wss://relay1", "reply"},
				{"e", "2rp"},
				{"p", "pkey1"},
			}
			require.NoError(t, db.AcceptEvents(context.Background(), &ev))
			helperMustEventCount(t, db, 1, model.Filter{Kinds: []int{nostr.KindRepost}, IDs: []string{"1r"}})
			helperMustEventCount(t, db, 0, model.Filter{Kinds: []int{nostr.KindRepost}, IDs: []string{"2rp"}})
		})
		t.Run("Repost with qoute", func(t *testing.T) {
			var ev model.Event
			ev.ID = "15r"
			ev.Kind = nostr.KindRepost
			ev.PubKey = "pubkeyr3"
			ev.CreatedAt = 2
			ev.Tags = model.Tags{
				{"e", "1r", "wss://relay1", "reply"},
				{"q", "2rp"},
				{"p", "pkey1"},
			}
			require.NoError(t, db.AcceptEvents(context.Background(), &ev))
			f1 := model.Filter{Kinds: []int{nostr.KindRepost}, IDs: []string{"1r"}}
			f2 := model.Filter{Kinds: []int{nostr.KindRepost}, IDs: []string{"2rp"}, Tags: model.TagMap{"q": nil}}
			helperMustEventCount(t, db, 2, f1)
			helperMustEventCount(t, db, 1, f2)
			helperMustEventCount(t, db, 3, f1, f2)
		})
	})
}
