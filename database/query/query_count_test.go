// SPDX-License-Identifier: ice License 1.0

package query

import (
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func TestQueryEventsCount(t *testing.T) {
	t.Parallel()

	db, data := helperEnsureDatabaseWithData(t)
	defer db.Close()

	authors := make(map[string]int64)
	ids := make(map[string]*model.Event, len(data.Events))

	t.Run("Generate", func(t *testing.T) {
		for _, event := range data.Events {
			authors[event.PubKey]++
			ids[event.ID] = event
		}
	})
	t.Run("CountAll", func(t *testing.T) {
		count, err := db.CountEvents(t.Context())
		require.NoError(t, err)
		require.EqualValues(t, len(data.Events), count)
	})
	t.Run("CountByAuthor", func(t *testing.T) {
		for author, expectedCount := range authors {
			count, err := db.CountEvents(t.Context(), model.Filter{
				Authors: []string{author},
				Search:  "countbyauthor",
			})
			require.NoError(t, err)
			require.Equal(t, expectedCount, count)
		}
	})
	t.Run("EventsOR", func(t *testing.T) {
		ev1 := data.Random(t)
		ev2 := data.Random(t)
		ev3 := data.Random(t)
		count, err := db.CountEvents(t.Context(),
			model.Filter{
				IDs: []string{ev1.ID},
			},
			model.Filter{
				Authors: []string{ev2.PubKey},
			},
			model.Filter{
				Tags: model.TagMap{}.SetLiterals(ev3.Tags[0][0], ev3.Tags[0][1:]...),
			},
		)
		require.NoError(t, err)
		require.Equal(t, int64(3), count)
	})

	require.NoError(t, db.Close())
}

func helperMustBePrecalculatedCount(t *testing.T, db *dbClient, expectedCount int64, f ...model.Filter) {
	t.Helper()

	count := helperMustGetPrecalculatedCounters(t, db, f...)
	require.Equal(t, expectedCount, count)
}

func TestEventCounters(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
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
			require.NoError(t, db.AcceptEvents(t.Context(), &ev))
		})
		quotes := []*model.Event{}
		t.Run("Do", func(t *testing.T) {
			for i := range 3 {
				var q model.Event

				q.Kind = nostr.KindTextNote
				q.ID = "q" + strconv.Itoa(i)
				q.PubKey = "pubkey" + strconv.Itoa(i)
				q.CreatedAt = model.Timestamp(i)
				q.Tags = model.Tags{{"q", "1"}}
				quotes = append(quotes, &q)
			}
			require.NoError(t, db.AcceptEvents(t.Context(), quotes...))
			helperMustBePrecalculatedCount(t, db, 3, model.Filter{Kinds: []int{nostr.KindTextNote}, Tags: model.TagMap{}.SetLiterals("q", "1")})
		})
		t.Run("Delete", func(t *testing.T) {
			var ev model.Event

			ev.PubKey = "pubkey2"
			ev.ID = "delete"
			ev.Kind = nostr.KindDeletion
			ev.Tags = model.Tags{{"e", "q2"}}
			require.NoError(t, db.AcceptEvents(t.Context(), &ev))

			c, err := db.CountEvents(t.Context())
			require.NoError(t, err)
			require.Equal(t, int64(13), c) // 11 posts, 2 quotes.
			helperMustBePrecalculatedCount(t, db, 2, model.Filter{Kinds: []int{nostr.KindTextNote}, Tags: model.TagMap{}.SetLiterals("q", "1")})
		})
		t.Run("Rollback", func(t *testing.T) {
			require.NoError(t, db.RollbackEvents(t.Context(), quotes...))

			c, err := db.CountEvents(t.Context())
			require.NoError(t, err)
			require.Equal(t, int64(11), c) // 11 posts, 2 quotes.
			helperMustBePrecalculatedCount(t, db, 0, model.Filter{Kinds: []int{nostr.KindTextNote}, Tags: model.TagMap{}.SetLiterals("q", "1")})
		})
		t.Run("Non existent", func(t *testing.T) {
			var q model.Event
			q.Kind = nostr.KindTextNote
			q.ID = "qnonexistent"
			q.PubKey = "pubkey"
			q.CreatedAt = 1
			q.Tags = model.Tags{{"q", "foo"}}
			require.NoError(t, db.AcceptEvents(t.Context(), &q))
			helperMustBePrecalculatedCount(t, db, 0, model.Filter{Kinds: []int{nostr.KindTextNote}, Tags: model.TagMap{}.SetLiterals("q", "foo")})
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
			require.NoError(t, db.AcceptEvents(t.Context(), &ev))
			for _, key := range []string{"alicekey", "bobkey", "carolkey"} {
				helperMustBePrecalculatedCount(t, db, 1, model.Filter{Kinds: []int{nostr.KindFollowList}, Tags: model.TagMap{}.SetLiterals("p", key)})
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
			require.NoError(t, db.AcceptEvents(t.Context(), &ev))
			for _, key := range []string{"alicekey", "bobkey"} {
				helperMustBePrecalculatedCount(t, db, 1, model.Filter{Kinds: []int{nostr.KindFollowList}, Tags: model.TagMap{}.SetLiterals("p", key)})
			}
			helperMustBePrecalculatedCount(t, db, 0, model.Filter{Kinds: []int{nostr.KindFollowList}, Tags: model.TagMap{}.SetLiterals("p", "carolkey")})
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
			require.NoError(t, db.AcceptEvents(t.Context(), &ev))
			helperMustBePrecalculatedCount(t, db, 3, model.Filter{Kinds: []int{nostr.KindFollowList}, Tags: model.TagMap{}.SetLiterals("p", "alicekey", "bobkey", "megankey")})
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
			require.NoError(t, db.AcceptEvents(t.Context(), &ev))
			helperMustBePrecalculatedCount(t, db, 2, model.Filter{Kinds: []int{nostr.KindFollowList}, Tags: model.TagMap{}.SetLiterals("p", "megankey")})
		})
		t.Run("RemoveOriginalList", func(t *testing.T) {
			var ev model.Event
			ev.ID = "4f"
			ev.Kind = nostr.KindDeletion
			ev.PubKey = "pubkeyf3"
			ev.Tags = model.Tags{{"e", "3f"}}
			require.NoError(t, db.AcceptEvents(t.Context(), &ev))
			for _, key := range []string{"alicekey", "bobkey"} {
				helperMustBePrecalculatedCount(t, db, 0, model.Filter{Kinds: []int{nostr.KindFollowList}, Tags: model.TagMap{}.SetLiterals("p", key)})
			}
			helperMustBePrecalculatedCount(t, db, 1, model.Filter{Kinds: []int{nostr.KindFollowList}, Tags: model.TagMap{}.SetLiterals("p", "megankey")})
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
			require.NoError(t, db.AcceptEvents(t.Context(), &ev))
		})
		t.Run("Reaction simple", func(t *testing.T) {
			var ev model.Event
			ev.ID = "2r"
			ev.Kind = nostr.KindReaction
			ev.PubKey = "pubkeyr2"
			ev.CreatedAt = 2
			ev.Tags = model.Tags{{"e", "1r"}, {"p", "pubkeyr1"}, {"k", "1"}}
			require.NoError(t, db.AcceptEvents(t.Context(), &ev))
			helperMustBePrecalculatedCount(t, db, 1, model.Filter{Kinds: []int{nostr.KindReaction}, Tags: model.TagMap{}.SetLiterals("e", "1r")})
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
			require.NoError(t, db.AcceptEvents(t.Context(), &ev))
			helperMustBePrecalculatedCount(t, db, 2, model.Filter{Kinds: []int{nostr.KindReaction}, Tags: model.TagMap{}.SetLiterals("e", "1r")})
		})
		var delEv1, delEv2 model.Event
		t.Run("Delete", func(t *testing.T) {
			delEv1.Kind = nostr.KindDeletion
			delEv1.PubKey = "pubkeyr2"
			delEv1.Tags = model.Tags{{"e", "2r"}}
			require.NoError(t, db.AcceptEvents(t.Context(), &delEv1))
			helperMustBePrecalculatedCount(t, db, 1, model.Filter{Kinds: []int{nostr.KindReaction}, Tags: model.TagMap{}.SetLiterals("e", "1r")})
			delEv2 = delEv1
			delEv2.PubKey = "pubkeyr3"
			delEv2.Tags = model.Tags{{"e", "3r"}}
			require.NoError(t, db.AcceptEvents(t.Context(), &delEv2))
			helperMustBePrecalculatedCount(t, db, 0, model.Filter{Kinds: []int{nostr.KindReaction}, Tags: model.TagMap{}.SetLiterals("e", "1r")})
		})
		t.Run("RollbackDelete", func(t *testing.T) {
			require.NoError(t, db.RollbackEvents(t.Context(), &delEv1))
			require.NoError(t, db.RollbackEvents(t.Context(), &delEv2))
			helperMustBePrecalculatedCount(t, db, 2, model.Filter{Kinds: []int{nostr.KindReaction}, Tags: model.TagMap{}.SetLiterals("e", "1r")})
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
			require.NoError(t, db.AcceptEvents(t.Context(), &ev))
		})
		t.Run("Reply", func(t *testing.T) {
			var ev model.Event
			ev.ID = "2rp"
			ev.Kind = nostr.KindTextNote
			ev.PubKey = "pubkeyrp2"
			ev.CreatedAt = 2
			ev.Tags = model.Tags{{"e", "1rp", "", "reply", "pubkeyrp2"}, {"p", "pubkeyrp1"}}
			require.NoError(t, db.AcceptEvents(t.Context(), &ev))
			helperMustBePrecalculatedCount(t, db, 1, model.Filter{Kinds: []int{nostr.KindTextNote}, Tags: model.TagMap{}.SetLiterals("e", "1rp")})
		})
		t.Run("Root", func(t *testing.T) {
			var ev model.Event
			ev.ID = "3rp"
			ev.Kind = nostr.KindTextNote
			ev.PubKey = "pubkeyrp3"
			ev.CreatedAt = 2
			ev.Tags = model.Tags{
				{"e", "1rp", "", "root", "pubkeyrp2"},
				{"e", "1rp", "", "reply", "pubkeyrp2"},
				{"p", "pubkeyrp1"},
			}
			require.NoError(t, db.AcceptEvents(t.Context(), &ev))
			// Total: root + reply.
			helperMustBePrecalculatedCount(t, db, 2, model.Filter{Kinds: []int{nostr.KindTextNote}, Tags: model.TagMap{}.SetLiterals("e", "1rp")})
			helperMustBePrecalculatedCount(t, db, 2, model.Filter{Kinds: []int{nostr.KindTextNote}, Tags: model.TagMap{}.Set("e", new("1rp"), nil, new("reply"))})
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
			require.NoError(t, db.AcceptEvents(t.Context(), &ev))
			helperMustBePrecalculatedCount(t, db, 1, model.Filter{Kinds: []int{nostr.KindRepost}, Tags: model.TagMap{}.SetLiterals("e", "1r")})
			helperMustBePrecalculatedCount(t, db, 1, model.Filter{Tags: model.TagMap{}.SetLiterals("e", "2rp")})
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
			require.NoError(t, db.AcceptEvents(t.Context(), &ev))
			f1 := model.Filter{Kinds: []int{nostr.KindRepost}, Tags: model.TagMap{}.SetLiterals("e", "1r")}
			f2 := model.Filter{Kinds: []int{nostr.KindRepost}, Tags: model.TagMap{}.SetLiterals("q", "2rp")}
			helperMustBePrecalculatedCount(t, db, 2, f1)
			helperMustBePrecalculatedCount(t, db, 1, f2)
			helperMustBePrecalculatedCount(t, db, 3, f1, f2)
		})
	})
	t.Run("Reactions on TC", func(t *testing.T) {
		var tcDef model.Event
		tcDef.ID = "1tc"
		tcDef.Kind = model.CustomIONKindTokenizedCommunityDefinition
		tcDef.PubKey = "pub1tc"
		tcDef.CreatedAt = 1
		require.NoError(t, db.AcceptEvents(t.Context(), &tcDef))
		t.Run("React", func(t *testing.T) {
			var ev model.Event
			ev.ID = "2tc"
			ev.Kind = nostr.KindReaction
			ev.PubKey = "pub2tc"
			ev.CreatedAt = 2
			ev.Tags = model.Tags{{"e", tcDef.ID}, {"p", tcDef.PubKey}, {"k", strconv.Itoa(tcDef.Kind)}}
			require.NoError(t, db.AcceptEvents(t.Context(), &ev))
			helperMustBePrecalculatedCount(t, db, 1, model.Filter{Kinds: []int{nostr.KindReaction}, Tags: model.TagMap{}.SetLiterals("e", tcDef.ID)})
		})
		t.Run("Delete", func(t *testing.T) {
			var delEv1 model.Event
			delEv1.Kind = nostr.KindDeletion
			delEv1.PubKey = "pub2tc"
			delEv1.Tags = model.Tags{{"e", "2tc"}}
			require.NoError(t, db.AcceptEvents(t.Context(), &delEv1))
			helperMustBePrecalculatedCount(t, db, 0, model.Filter{Kinds: []int{nostr.KindReaction}, Tags: model.TagMap{}.SetLiterals("e", tcDef.ID)})
		})
	})
}

func TestEventMultiReactions(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	var ev model.Event
	ev.ID = "t1id1"
	ev.Content = "hello world"
	ev.CreatedAt = 1
	ev.PubKey = "t1pub1"
	require.NoError(t, db.AcceptEvents(t.Context(), &ev))
	helperMustBePrecalculatedCount(t, db, 0, model.Filter{Tags: model.TagMap{}.Set("e", &ev.ID), Kinds: []int{nostr.KindReaction}})

	for _, r := range []string{"+", "-", "*"} {
		t.Run(r, func(t *testing.T) {
			const num = 2
			for i := range num {
				var reaction model.Event
				reaction.ID = r + "reaction" + strconv.Itoa(i)
				reaction.Kind = nostr.KindReaction
				reaction.PubKey = r + "pubkeyr" + strconv.Itoa(i)
				reaction.CreatedAt = model.Timestamp(i)
				reaction.Content = r
				reaction.Tags = model.Tags{{"e", "t1id1"}, {"p", "t1pub1"}}
				require.NoError(t, db.AcceptEvents(t.Context(), &reaction))
			}
		})
	}
	helperMustBePrecalculatedCount(t, db, 6, model.Filter{Tags: model.TagMap{}.Set("e", &ev.ID), Kinds: []int{nostr.KindReaction}})

	// Remove one `-` reaction.
	var deleteEv model.Event
	deleteEv.Kind = nostr.KindDeletion
	deleteEv.PubKey = "-pubkeyr1"
	deleteEv.Tags = model.Tags{{"e", "-reaction1"}}
	require.NoError(t, db.AcceptEvents(t.Context(), &deleteEv))
	helperMustBePrecalculatedCount(t, db, 5, model.Filter{Tags: model.TagMap{}.Set("e", &ev.ID), Kinds: []int{nostr.KindReaction}})

	// Remove one `*` reaction.
	deleteEv.Kind = nostr.KindDeletion
	deleteEv.PubKey = "*pubkeyr0"
	deleteEv.Tags = model.Tags{{"e", "*reaction0"}}
	require.NoError(t, db.AcceptEvents(t.Context(), &deleteEv))
	helperMustBePrecalculatedCount(t, db, 4, model.Filter{Tags: model.TagMap{}.Set("e", &ev.ID), Kinds: []int{nostr.KindReaction}})

	// Remove original event.
	deleteEv.Kind = nostr.KindDeletion
	deleteEv.PubKey = "t1pub1"
	deleteEv.Tags = model.Tags{{"e", "t1id1"}}
	require.NoError(t, db.AcceptEvents(t.Context(), &deleteEv))
	helperMustBePrecalculatedCount(t, db, 0, model.Filter{Tags: model.TagMap{}.Set("e", &ev.ID), Kinds: []int{nostr.KindReaction}})
}

func TestCounterRootReply(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	var root model.Event
	root.ID = "rootid"
	root.Kind = nostr.KindTextNote
	root.Content = "root"
	root.PubKey = "rootpub"
	root.CreatedAt = 1
	require.NoError(t, db.AcceptEvents(t.Context(), &root))

	var replyToRoot model.Event
	replyToRoot.ID = "replytorootid"
	replyToRoot.Kind = nostr.KindTextNote
	replyToRoot.Content = "reply"
	replyToRoot.PubKey = "replypub"
	replyToRoot.CreatedAt = 2
	replyToRoot.Tags = model.Tags{
		{"e", root.ID, "", "root"},
		{"e", root.ID, "", "reply"},
	}
	require.NoError(t, db.AcceptEvents(t.Context(), &replyToRoot))

	var replyToReply model.Event
	replyToReply.ID = "replytoreplyid"
	replyToReply.Kind = nostr.KindTextNote
	replyToReply.Content = "reply"
	replyToReply.PubKey = "replypub"
	replyToReply.CreatedAt = 3
	replyToReply.Tags = model.Tags{
		{"e", replyToRoot.ID, "", "reply"},
		{"e", root.ID, "", "root"},
	}
	require.NoError(t, db.AcceptEvents(t.Context(), &replyToReply))

	var replyToReplyToReply model.Event
	replyToReplyToReply.ID = "replytoreplytoreplyid"
	replyToReplyToReply.Kind = nostr.KindTextNote
	replyToReplyToReply.Content = "reply"
	replyToReplyToReply.PubKey = "replypub"
	replyToReplyToReply.CreatedAt = 4
	replyToReplyToReply.Tags = model.Tags{
		{"e", replyToReply.ID, "", "reply"},
		{"e", root.ID, "", "root"},
	}
	require.NoError(t, db.AcceptEvents(t.Context(), &replyToReplyToReply))

	helperMustBePrecalculatedCount(t, db, 1, model.Filter{Tags: model.TagMap{}.Set("e", &root.ID)})
	helperMustBePrecalculatedCount(t, db, 1, model.Filter{Tags: model.TagMap{}.Set("e", &replyToReply.ID)})
	helperMustBePrecalculatedCount(t, db, 1, model.Filter{Tags: model.TagMap{}.Set("e", &replyToRoot.ID)})

	events := helperSelectEvents(t, db, model.Filter{
		Kinds:  []int{nostr.KindTextNote, nostr.KindRepost},
		Limit:  10,
		Search: "references:false expiration:false include:dependencies:kind1>kind6400+kind1+group+reply",
	})
	require.Len(t, events, 2) // Root + DVM.
	require.Equal(t, root.ID, events[1].ID)
	require.Equal(t, model.KindDVMCountResponse, events[0].Kind)
	require.Equal(t, "1", events[0].Content)

	t.Run("Delete", func(t *testing.T) {
		var deleteEv model.Event

		deleteEv.Kind = nostr.KindDeletion
		deleteEv.PubKey = "replypub"
		deleteEv.Tags = model.Tags{{"e", replyToRoot.ID}}
		require.NoError(t, db.AcceptEvents(t.Context(), &deleteEv))
		helperMustBePrecalculatedCount(t, db, 0, model.Filter{Tags: model.TagMap{}.Set("e", &root.ID)})
	})
}

func TestCounterOpenCommunityMembers(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	hVal, err := uuid.NewV7()
	require.NoError(t, err)
	communityID := hVal.String()

	var openCommunity model.Event
	openCommunity.ID = "community"
	openCommunity.Kind = model.CustomIONKindCommunityDefinition
	openCommunity.PubKey = "owner"
	openCommunity.CreatedAt = 1
	openCommunity.Tags = nostr.Tags{
		{"h", communityID},
		{"d", "dtagvalue"},
		{"name", "some name"},
		{"description", "some description"},
		{"open"},
	}
	require.NoError(t, db.AcceptEvents(t.Context(), &openCommunity))

	events := helperSelectEvents(t, db, model.Filter{
		Kinds: []int{model.CustomIONKindCommunityDefinition},
		Limit: 10,
	})
	require.Len(t, events, 1)

	var memberEvent model.Event
	memberEvent.ID = "member1"
	memberEvent.Kind = model.CustomIONKindCommunityJoin
	memberEvent.PubKey = "member1"
	memberEvent.CreatedAt = 1
	memberEvent.Tags = nostr.Tags{
		{"p", "member1"},
		{"h", communityID},
		{"d", "member1"},
	}
	require.NoError(t, db.AcceptEvents(t.Context(), &memberEvent))

	helperMustBePrecalculatedCount(t, db, 1, model.Filter{
		Tags:  model.TagMap{}.Set("h", &communityID),
		Kinds: []int{model.CustomIONKindCommunityJoin},
	})
}

func TestCounterClosedCommunityMembers(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	hVal, err := uuid.NewV7()
	require.NoError(t, err)
	communityID := hVal.String()

	var closedCommunity model.Event
	t.Run("create closed community", func(t *testing.T) {
		closedCommunity.ID = "community"
		closedCommunity.Kind = model.CustomIONKindCommunityDefinition
		closedCommunity.PubKey = "owner"
		closedCommunity.CreatedAt = 1
		closedCommunity.Tags = nostr.Tags{
			{"h", communityID},
			{"d", "dtagvalue"},
			{"name", "some name"},
			{"description", "some description"},
			{"closed"},
		}
		require.NoError(t, db.AcceptEvents(t.Context(), &closedCommunity))

		events := helperSelectEvents(t, db, model.Filter{
			Kinds: []int{model.CustomIONKindCommunityDefinition},
			Limit: 10,
		})
		require.Len(t, events, 1)
	})

	var member1 model.Event
	t.Run("try to add member1 to the community without authorization tag. Not added", func(t *testing.T) {
		member1.ID = "member1"
		member1.Kind = model.CustomIONKindCommunityJoin
		member1.PubKey = "member1"
		member1.CreatedAt = 1
		member1.Tags = nostr.Tags{
			{"p", "member1"},
			{"h", communityID},
		}
		require.NoError(t, db.AcceptEvents(t.Context(), &member1))

		helperMustBePrecalculatedCount(t, db, 0, model.Filter{
			Tags:  model.TagMap{}.Set("h", &communityID),
			Kinds: []int{model.CustomIONKindCommunityJoin},
		})
	})

	var member2 model.Event
	t.Run("add member2 to the community with authorization tag", func(t *testing.T) {
		member2.ID = "member2"
		member2.Kind = model.CustomIONKindCommunityJoin
		member2.PubKey = "member2"
		member2.CreatedAt = 1
		member2.Tags = nostr.Tags{
			{"p", "member2"},
			{"h", communityID},
			{"authorization", "dummy"},
		}
		require.NoError(t, db.AcceptEvents(t.Context(), &member2))

		helperMustBePrecalculatedCount(t, db, 1, model.Filter{
			Tags:  model.TagMap{}.Set("h", &communityID),
			Kinds: []int{model.CustomIONKindCommunityJoin},
		})
	})
}

func TestCounterRootReplyAddressable(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	var root model.Event
	root.ID = "rootid"
	root.Kind = nostr.KindArticle
	root.Content = "root"
	root.PubKey = "rootpub"
	root.CreatedAt = 1
	root.Tags = model.Tags{
		{"d", "root"},
	}
	require.NoError(t, db.AcceptEvents(t.Context(), &root))

	var replyToRoot model.Event
	replyToRoot.ID = "replytorootid"
	replyToRoot.Kind = nostr.KindArticle
	replyToRoot.Content = "reply"
	replyToRoot.PubKey = "replytorootpub"
	replyToRoot.CreatedAt = 2
	replyToRoot.Tags = model.Tags{
		{"d", "reply_to_root"},
		{"a", root.Address(), "", "root"},
		{"a", root.Address(), "", "reply"},
	}
	require.NoError(t, db.AcceptEvents(t.Context(), &replyToRoot))

	var replyToReply model.Event
	replyToReply.ID = "replytoreplyid"
	replyToReply.Kind = nostr.KindArticle
	replyToReply.Content = "reply"
	replyToReply.PubKey = "replytoreplypub"
	replyToReply.CreatedAt = 3
	replyToReply.Tags = model.Tags{
		{"d", "reply_to_reply"},
		{"a", replyToRoot.Address(), "", "reply"},
		{"a", root.Address(), "", "root"},
	}
	require.NoError(t, db.AcceptEvents(t.Context(), &replyToReply))

	var replyToReplyToReply model.Event
	replyToReplyToReply.ID = "replytoreplytoreplyid"
	replyToReplyToReply.Kind = nostr.KindArticle
	replyToReplyToReply.Content = "reply"
	replyToReplyToReply.PubKey = "replytoreplytoreplypub"
	replyToReplyToReply.CreatedAt = 4
	replyToReplyToReply.Tags = model.Tags{
		{"d", "reply_to_reply_to_reply"},
		{"a", replyToReply.Address(), "", "reply"},
		{"a", root.Address(), "", "root"},
	}
	require.NoError(t, db.AcceptEvents(t.Context(), &replyToReplyToReply))

	helperMustBePrecalculatedCount(t, db, 1, model.Filter{Tags: model.TagMap{}.SetLiterals("a", root.Address())})
	helperMustBePrecalculatedCount(t, db, 1, model.Filter{Tags: model.TagMap{}.SetLiterals("a", replyToReply.Address())})
	helperMustBePrecalculatedCount(t, db, 1, model.Filter{Tags: model.TagMap{}.SetLiterals("a", replyToRoot.Address())})

	events := helperSelectEvents(t, db, model.Filter{
		Kinds:  []int{nostr.KindArticle, nostr.KindRepost},
		Limit:  10,
		Search: "references:false expiration:false include:dependencies:kind30023>kind6400+kind30023+group+reply",
	})
	require.Len(t, events, 2) // Root + DVM.
	require.Equal(t, "rootid", events[1].ID)
	require.Equal(t, model.KindDVMCountResponse, events[0].Kind)
	require.Equal(t, "1", events[0].Content)
}
