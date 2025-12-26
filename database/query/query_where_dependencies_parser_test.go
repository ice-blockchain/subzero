// SPDX-License-Identifier: ice License 1.0

package query

import (
	"encoding/json"
	"math/rand/v2"
	"strconv"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func TestParseDepRequest(t *testing.T) {
	t.Parallel()

	cases := []struct {
		Input    string
		Expected filterDependency
		Err      error
	}{
		{
			Input: "kind30175>kind30008+profile_badges>kind30009>kind8",
			Expected: filterDependency{
				Start: filterDependencyStart{
					Kind:          30175,
					ProfileBadges: true,
				},
				Reduce: filterDependencyReduce{
					Kinds: []int{30008, 30009, 8},
				},
			},
		},
		{
			Input: "kind1>kind30008+profile_badges>kind30009>kind8",
			Expected: filterDependency{
				Start: filterDependencyStart{
					Kind:          1,
					ProfileBadges: true,
				},
				Reduce: filterDependencyReduce{
					Kinds: []int{30008, 30009, 8},
				},
			},
		},
		{
			Input: "kind30008+profile_badges>kind30009>kind8",
			Expected: filterDependency{
				Start: filterDependencyStart{
					Kind:          30008,
					ProfileBadges: true,
				},
				Reduce: filterDependencyReduce{
					Kinds: []int{30009, 8},
				},
			},
		},
		{
			Input: "kind1175>kind31175",
			Expected: filterDependency{
				Start: filterDependencyStart{
					Kind: 1175,
				},
				Reduce: filterDependencyReduce{
					Kinds: []int{31175},
				},
			},
		},
		{
			Input: "kind6>kind10002",
			Expected: filterDependency{
				Start: filterDependencyStart{
					Kind: 6,
				},
				Reduce: filterDependencyReduce{
					Kinds: []int{10002},
				},
			},
		},
		{
			Input: "kind3>kind0",
			Expected: filterDependency{
				Start: filterDependencyStart{
					Kind: 3,
				},
				Reduce: filterDependencyReduce{
					Kinds: []int{0},
				},
			},
		},
		{
			Input: "kind0>kind3",
			Expected: filterDependency{
				Start: filterDependencyStart{
					Kind: 0,
				},
				Reduce: filterDependencyReduce{
					Kinds: []int{3},
				},
			},
		},
		{
			Input: "kind1+q>kind10002",
			Expected: filterDependency{
				Start: filterDependencyStart{
					Kind: 1,
					Tag:  "q",
				},
				Reduce: filterDependencyReduce{
					Kinds: []int{10002},
				},
			},
		},
		{
			Input: "kind1>e4f0cf865fd1b24845b694a9dbe5296f663b0c4d449308c3afbd9f319ecbbcd4@kind1+e+root",
			Expected: filterDependency{
				Start: filterDependencyStart{
					Kind: 1,
				},
				Reduce: filterDependencyReduce{
					Kinds:   []int{1},
					Author:  "e4f0cf865fd1b24845b694a9dbe5296f663b0c4d449308c3afbd9f319ecbbcd4",
					Tag:     "e",
					Context: "root",
				},
			},
		},
		{
			Input: "kind1>3cfb1533dd7534bc0bbd60ad40492a4f131c2cb05ca47994d12ea530d7c40183@kind1+e+root",
			Expected: filterDependency{
				Start: filterDependencyStart{
					Kind: 1,
				},
				Reduce: filterDependencyReduce{
					Kinds:   []int{1},
					Author:  "3cfb1533dd7534bc0bbd60ad40492a4f131c2cb05ca47994d12ea530d7c40183",
					Tag:     "e",
					Context: "root",
				},
			},
		},
		{
			Input: "kind1>3cfb1533dd7534bc0bbd60ad40492a4f131c2cb05ca47994d12ea530d7c40183@kind1+e+reply",
			Expected: filterDependency{
				Start: filterDependencyStart{
					Kind: 1,
				},
				Reduce: filterDependencyReduce{
					Kinds:   []int{1},
					Author:  "3cfb1533dd7534bc0bbd60ad40492a4f131c2cb05ca47994d12ea530d7c40183",
					Tag:     "e",
					Context: "reply",
				},
			},
		},
		{
			Input: "kind1>3cfb1533dd7534bc0bbd60ad40492a4f131c2cb05ca47994d12ea530d7c40183@kind1+q",
			Expected: filterDependency{
				Start: filterDependencyStart{
					Kind: 1,
				},
				Reduce: filterDependencyReduce{
					Kinds:  []int{1},
					Author: "3cfb1533dd7534bc0bbd60ad40492a4f131c2cb05ca47994d12ea530d7c40183",
					Tag:    "q",
				},
			},
		},
		{
			Input: "kind1>3cfb1533dd7534bc0bbd60ad40492a4f131c2cb05ca47994d12ea530d7c40183@kind6",
			Expected: filterDependency{
				Start: filterDependencyStart{
					Kind: 1,
				},
				Reduce: filterDependencyReduce{
					Kinds:  []int{6},
					Author: "3cfb1533dd7534bc0bbd60ad40492a4f131c2cb05ca47994d12ea530d7c40183",
				},
			},
		},
		{
			Input: "kind1234>3cfb1533dd7534bc0bbd60ad40492a4f131c2cb05ca47994d12ea530d7c40183@kind1754",
			Expected: filterDependency{
				Start: filterDependencyStart{
					Kind: 1234,
				},
				Reduce: filterDependencyReduce{
					Kinds:  []int{1754},
					Author: "3cfb1533dd7534bc0bbd60ad40492a4f131c2cb05ca47994d12ea530d7c40183",
				},
			},
		},
		{
			Input: "kind1>kind6400+kind1+group+root",
			Expected: filterDependency{
				Start: filterDependencyStart{
					Kind: 1,
				},
				Reduce: filterDependencyReduce{
					Kinds:   []int{6400, 1},
					Group:   true,
					Context: "root",
				},
			},
		},
		{
			Input: "kind1>kind6400+kind1+group+q",
			Expected: filterDependency{
				Start: filterDependencyStart{
					Kind: 1,
				},
				Reduce: filterDependencyReduce{
					Kinds: []int{6400, 1},
					Group: true,
					Tag:   "q",
				},
			},
		},
		{
			Input: "kind0>kind6400+kind3+group+p",
			Expected: filterDependency{
				Start: filterDependencyStart{
					Kind: 0,
				},
				Reduce: filterDependencyReduce{
					Kinds: []int{6400, 3},
					Group: true,
					Tag:   "p",
				},
			},
		},
		{
			Input: "kind1>kind6400+kind7+group+content",
			Expected: filterDependency{
				Start: filterDependencyStart{
					Kind: 1,
				},
				Reduce: filterDependencyReduce{
					Kinds:   []int{6400, 7},
					Group:   true,
					Context: "content",
				},
			},
		},
		{
			Input: "kind30023>kind6400+kind1754+group+content",
			Expected: filterDependency{
				Start: filterDependencyStart{
					Kind: 30023,
				},
				Reduce: filterDependencyReduce{
					Kinds:   []int{6400, 1754},
					Group:   true,
					Context: "content",
				},
			},
		},
		{
			Input: "kind123>kind6400+kind30175+expiration",
			Expected: filterDependency{
				Start: filterDependencyStart{
					Kind: 123,
				},
				Reduce: filterDependencyReduce{
					Kinds:      []int{6400, 30175},
					Expiration: true,
				},
			},
		},
		{
			Input: "kind3>kind0+p+|key1,key2,keyN|",
			Expected: filterDependency{
				Start: filterDependencyStart{
					Kind: 3,
				},
				Reduce: filterDependencyReduce{
					Kinds:  []int{0},
					Author: "key1,key2,keyN",
				},
			},
		},
		{
			Input: "kind3>kind0+p+|key1|",
			Expected: filterDependency{
				Start: filterDependencyStart{
					Kind: 3,
				},
				Reduce: filterDependencyReduce{
					Kinds:  []int{0},
					Author: "key1",
				},
			},
		},
		{
			Input: "kind1>kind6400+kind7+group+foo",
			Err:   errDepParserUnexpectedToken,
		},
		{
			Input: "",
			Err:   errDepParserUnexpectedToken,
		},
	}

	for _, c := range cases {
		t.Run(c.Input, func(t *testing.T) {
			filter, err := parseDepRequest(c.Input)
			if c.Err != nil {
				require.ErrorIs(t, err, c.Err)
			} else {
				c.Expected.Expr = c.Input
				require.Equal(t, &c.Expected, filter)
			}
		})
	}
}

func TestSelectWithDependencies(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	t.Run("kind1>kind0", func(t *testing.T) {
		var ev model.Event

		ev.ID = "id1"
		ev.Kind = nostr.KindProfileMetadata
		ev.PubKey = "pk1"
		ev.CreatedAt = 1
		err := db.AcceptEvents(t.Context(), &ev)
		require.NoError(t, err)

		ev.ID = "id2"
		ev.Kind = nostr.KindTextNote
		ev.PubKey = "pk1"
		ev.CreatedAt = 2
		ev.Content = "content of the note"
		err = db.AcceptEvents(t.Context(), &ev)
		require.NoError(t, err)

		events := helperSelectEvents(t, db, model.Filter{
			IDs:    []string{"id2"},
			Search: "include:dependencies:kind1>kind0",
		})
		require.Len(t, events, 2)
		require.ElementsMatch(t, []string{"id2", "id1"}, []string{events[0].ID, events[1].ID})

		t.Run("Select with ANY kind", func(t *testing.T) {
			events := helperSelectEvents(t, db, model.Filter{
				IDs:    []string{"id2"},
				Search: "include:dependencies:kind65535>kind0",
			})
			require.Len(t, events, 2)
			require.ElementsMatch(t, []string{"id2", "id1"}, []string{events[0].ID, events[1].ID})
		})
	})
	t.Run("kind0>kind3", func(t *testing.T) {
		var ev model.Event

		ev.ID = "id3f"
		ev.Kind = nostr.KindFollowList
		ev.PubKey = "pk1"
		ev.CreatedAt = 2
		err := db.AcceptEvents(t.Context(), &ev)
		require.NoError(t, err)

		events := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{nostr.KindProfileMetadata},
			Search: "include:dependencies:kind0>kind3",
		})
		require.Len(t, events, 2)
		require.ElementsMatch(t, []string{"id1", "id3f"}, []string{events[0].ID, events[1].ID})
	})
	t.Run("kind1>$logged_in_user_pubkey@kind1+e+root", func(t *testing.T) {
		var ev model.Event

		ev.ID = "t2id1"
		ev.Kind = nostr.KindTextNote
		ev.PubKey = "t2pk1"
		ev.CreatedAt = 1
		ev.Content = "text note 1"
		err := db.AcceptEvents(t.Context(), &ev)
		require.NoError(t, err)

		ev.ID = "t2id4"
		ev.Kind = nostr.KindTextNote
		ev.Content = "text note 4"
		ev.PubKey = "t2pk1"
		ev.CreatedAt = 1
		err = db.AcceptEvents(t.Context(), &ev)
		require.NoError(t, err)

		ev.ID = "t2id2"
		ev.Kind = nostr.KindTextNote
		ev.PubKey = "t2pk2"
		ev.Content = "text note 2"
		ev.CreatedAt = 2
		ev.Tags = model.Tags{
			{"e", "t2id1", "", "root"},
		}
		err = db.AcceptEvents(t.Context(), &ev)
		require.NoError(t, err)

		ev.ID = "t2id3"
		ev.Kind = nostr.KindTextNote
		ev.PubKey = "t2pk2"
		ev.Content = "text note 3"
		ev.CreatedAt = 3
		ev.Tags = model.Tags{
			{"e", "t2id1", "", "root"},
		}
		err = db.AcceptEvents(t.Context(), &ev)
		require.NoError(t, err)

		ev.ID = "t2id5"
		ev.Kind = nostr.KindTextNote
		ev.PubKey = "t2pk2"
		ev.Content = "text note 5"
		ev.CreatedAt = 4
		ev.Tags = model.Tags{
			{"e", "id2", "", "root"},
			{"e", "t2id3", "", "reply"},
		}
		err = db.AcceptEvents(t.Context(), &ev)
		require.NoError(t, err)

		ev.ID = "t2id6"
		ev.Kind = nostr.KindTextNote
		ev.PubKey = "t2pk2"
		ev.Content = "text note 6"
		ev.CreatedAt = 5
		ev.Tags = model.Tags{
			{"e", "id2", "", "root"},
		}
		err = db.AcceptEvents(t.Context(), &ev)
		require.NoError(t, err)

		events := helperSelectEvents(t, db, model.Filter{
			IDs:    []string{"t2id1", "id2"},
			Search: "include:dependencies:kind1>t2pk2@kind1+e+root",
		})
		require.Len(t, events, 4) // Two original notes, two replies (only one single reply per note). t2id3 must be excluded.
		require.ElementsMatch(t, []string{"t2id1", "id2", "t2id2", "t2id6"}, []string{events[0].ID, events[1].ID, events[2].ID, events[3].ID})
	})
	t.Run("kind1>kind6400+kind7+group+content", func(t *testing.T) {
		var ev model.Event

		ev.ID = "t3id1"
		ev.Kind = nostr.KindReaction
		ev.PubKey = "t3pk1"
		ev.CreatedAt = 13
		ev.Content = "+"
		ev.Tags = model.Tags{
			{"e", "t2id3"},
		}
		err := db.AcceptEvents(t.Context(), &ev)
		require.NoError(t, err)

		ev.ID = "t3id2"
		ev.Kind = nostr.KindReaction
		ev.PubKey = "t3pk2"
		ev.CreatedAt = 13
		ev.Content = "*"
		ev.Tags = model.Tags{
			{"e", "t2id3"},
		}
		err = db.AcceptEvents(t.Context(), &ev)
		require.NoError(t, err)
		helperMustBePrecalculatedCount(t, db, 2, model.Filter{Tags: model.TagMap{}.Set("e", model.PointerOf("t2id3")), Kinds: []int{nostr.KindReaction}})

		result, err := db.CountGroupedEventReactions(t.Context(), model.Filter{
			Kinds: []int{nostr.KindReaction},
			Tags: model.TagMap{}.
				Append("e", model.PointerOf("t2id2")).
				Append("e", model.PointerOf("t2id3")),
		})
		require.NoError(t, err)
		require.JSONEq(t, `{"*":1,"+":1}`, result)

		events := helperSelectEvents(t, db, model.Filter{
			IDs:    []string{"t2id2", "t2id3"},
			Search: "include:dependencies:kind1>kind6400+kind7+group+content",
		})
		require.Len(t, events, 3)
		require.Equal(t, "t2id3", events[1].ID)
		require.Equal(t, "t2id2", events[2].ID)
		for _, ev := range events[:1] {
			t.Logf("dvm event: %+v", ev)
			require.Equal(t, model.KindDVMCountResponse, ev.Kind)
			require.JSONEq(t, `{"*":1,"+":1}`, ev.Content)
			require.GreaterOrEqual(t, len(ev.Tags), 3)
			valid, err := ev.CheckSignature()
			require.NoError(t, err)
			require.Truef(t, valid, "signature is invalid: %+v", ev)
			req := ev.GetTag("request")
			require.NotNil(t, req)
			value := req.Value()
			require.NotEmpty(t, value)
			var reqEvent model.Event
			err = json.Unmarshal([]byte(value), &reqEvent)
			require.NoError(t, err)
			require.NotNil(t, reqEvent.GetTag("output"))
			require.Equal(t, "JSON", reqEvent.GetTag("output").Value())
		}
	})
	t.Run("kind30008+profile_badges>kind30009>kind8", func(t *testing.T) {
		var ev model.Event

		// badge definition of `testbadge`.
		ev.ID = "t4id1"
		ev.Kind = nostr.KindBadgeDefinition
		ev.PubKey = "t4pk1"
		ev.CreatedAt = 1
		ev.Tags = model.Tags{
			{"d", "testbadge"},
			{"name", "Test Badge"},
		}
		err := db.AcceptEvents(t.Context(), &ev)
		require.NoError(t, err)

		// t4pk2 wants to award t4pk3 the badge `testbadge`.
		ev.ID = "t4id2"
		ev.Kind = nostr.KindBadgeAward
		ev.PubKey = "t4pk2"
		ev.CreatedAt = 2
		ev.Tags = model.Tags{
			{"a", "30009:t4pk1:testbadge"},
		}
		err = db.AcceptEvents(t.Context(), &ev)
		require.NoError(t, err)

		// t4pk3 accepts the badge and updates their profile.
		ev.ID = "t4id3"
		ev.Kind = nostr.KindProfileBadges
		ev.PubKey = "t4pk3"
		ev.CreatedAt = 3
		ev.Tags = model.Tags{
			{"d", "profile_badges"},
			{"a", "30009:t4pk1:testbadge"},
			{"e", "t4id2"},
		}
		err = db.AcceptEvents(t.Context(), &ev)
		require.NoError(t, err)

		events := helperSelectEvents(t, db, model.Filter{
			Authors: []string{"t4pk3"},
			Search:  "include:dependencies:kind30008+profile_badges>kind30009>kind8",
		})
		require.Len(t, events, 3)
		require.ElementsMatch(t, []int{nostr.KindProfileBadges, nostr.KindBadgeDefinition, nostr.KindBadgeAward}, []int{events[0].Kind, events[1].Kind, events[2].Kind})
	})
	t.Run("Combined Dependency", func(t *testing.T) {
		events := helperSelectEvents(t, db, model.Filter{
			Authors: []string{"t4pk3", "pk1"},
			Search:  "include:dependencies:kind1>kind0 include:dependencies:kind30008+profile_badges>kind30009>kind8",
		})
		require.Len(t, events, 8) // 5 from the first search, 3 from the second.
	})
	t.Run("kind10002", func(t *testing.T) {
		t.Run("kind6>kind10002", func(t *testing.T) {
			var ev1, ev2 model.Event

			ev1.ID = "t6id1"
			ev1.Kind = nostr.KindRepost
			ev1.PubKey = "t6pk1"
			ev1.CreatedAt = 13
			ev1.Tags = model.Tags{
				{"e", "t2id3"},
			}
			ev2.ID = "t6id2"
			ev2.Kind = nostr.KindRepost
			ev2.PubKey = "t6pk2"
			ev2.CreatedAt = 14
			ev2.Tags = model.Tags{
				{"e", "t2id5"},
			}
			err := db.AcceptEvents(t.Context(), &ev1, &ev2)
			require.NoError(t, err)

			evRelayMetadata := model.Event{}
			evRelayMetadata.ID = "t6id3"
			evRelayMetadata.Kind = nostr.KindRelayListMetadata
			evRelayMetadata.PubKey = "t6pk2"
			evRelayMetadata.CreatedAt = 15
			evRelayMetadata.Tags = model.Tags{
				{"r", "wss://foo.bar"},
			}
			err = db.AcceptEvents(t.Context(), &evRelayMetadata)
			require.NoError(t, err)
			events := helperSelectEvents(t, db, model.Filter{
				IDs:    []string{"t6id1", "t6id2", "t2id5"},
				Search: "include:dependencies:kind6>kind10002",
			})
			require.Len(t, events, 5) // 2 reposts, 1 note, 2 relay metadata.
			for i, k := range []int{model.CustomIONKindRelayListMetadata, nostr.KindRelayListMetadata, nostr.KindRepost, nostr.KindRepost, nostr.KindTextNote} {
				require.Equalf(t, k, events[i].Kind, "event %d: %v", i, events[i])
				if events[i].Kind == model.CustomIONKindRelayListMetadata {
					ok, err := events[i].CheckSignature()
					require.NoError(t, err)
					require.True(t, ok)
				}
			}
		})
		t.Run("kind1+q>kind10002", func(t *testing.T) {
			var ev1, ev2 model.Event

			ev1.ID = "t7id1"
			ev1.Kind = nostr.KindTextNote
			ev1.PubKey = "t7pk2"
			ev1.CreatedAt = 15
			ev1.Tags = model.Tags{
				{"q", "t2id3"},
			}
			ev2.ID = "t7id2"
			ev2.Kind = nostr.KindRelayListMetadata
			ev2.PubKey = "t7pk2"
			ev2.CreatedAt = 15
			ev2.Tags = model.Tags{
				{"r", "wss://foo.bar2"},
			}

			err := db.AcceptEvents(t.Context(), &ev1, &ev2)
			require.NoError(t, err)

			ev1.ID = "t7id3"
			ev1.Kind = nostr.KindTextNote
			ev1.PubKey = "t7pk3"
			ev1.CreatedAt = 16
			ev1.Tags = model.Tags{}
			err = db.AcceptEvents(t.Context(), &ev1)
			require.NoError(t, err)

			events := helperSelectEvents(t, db, model.Filter{
				IDs:    []string{"t7id1", "t7id3", "t2id5"},
				Search: "include:dependencies:kind1+q>kind10002",
			})
			require.Len(t, events, 4) // 3 notes, 1 relay metadata.
			for i, k := range []int{nostr.KindTextNote, nostr.KindTextNote, nostr.KindRelayListMetadata, nostr.KindTextNote} {
				require.Equalf(t, k, events[i].Kind, "event %d: %v", i, events[i])
			}
		})
	})
	t.Run("kind30023>kind0", func(t *testing.T) {
		var ev model.Event

		ev.ID = "t8id1"
		ev.Kind = nostr.KindProfileMetadata
		ev.PubKey = "t8pk1"
		ev.CreatedAt = 1
		err := db.AcceptEvents(t.Context(), &ev)
		require.NoError(t, err)

		ev.ID = "t8id2"
		ev.Kind = nostr.KindArticle
		ev.PubKey = "t8pk1"
		ev.CreatedAt = 2
		ev.Content = "content of the article"
		err = db.AcceptEvents(t.Context(), &ev)
		require.NoError(t, err)

		events := helperSelectEvents(t, db, model.Filter{
			IDs:    []string{"t8id2"},
			Search: "include:dependencies:kind30023>kind0",
		})
		require.Len(t, events, 2)
		require.ElementsMatch(t, []string{"t8id2", "t8id1"}, []string{events[0].ID, events[1].ID})
	})
	t.Run("kind0>kind6400+kind3+group+p", func(t *testing.T) {
		// Celebrity 1, and two fans.
		require.NoError(t, db.AcceptEvents(t.Context(),
			&model.Event{
				Event: nostr.Event{
					ID:        "t9id1",
					Kind:      nostr.KindProfileMetadata,
					PubKey:    "t9pk1",
					CreatedAt: 1,
				},
			},
			&model.Event{
				Event: nostr.Event{
					ID:        "t9id2",
					Kind:      nostr.KindFollowList,
					PubKey:    "t9pk2",
					CreatedAt: 2,
					Tags: model.Tags{
						{"p", "t9pk1"},
					},
				},
			},
			&model.Event{
				Event: nostr.Event{
					ID:        "t9id3",
					Kind:      nostr.KindFollowList,
					PubKey:    "t9pk3",
					CreatedAt: 2,
					Tags: model.Tags{
						{"p", "t9pk1"},
					},
				},
			},
		))
		// Celebrity 2, and single fan.
		require.NoError(t, db.AcceptEvents(t.Context(),
			&model.Event{
				Event: nostr.Event{
					ID:        "t9id4",
					Kind:      nostr.KindProfileMetadata,
					PubKey:    "t9pk4",
					CreatedAt: 1,
				},
			},
			&model.Event{
				Event: nostr.Event{
					ID:        "t9id5",
					Kind:      nostr.KindFollowList,
					PubKey:    "t9pk5",
					CreatedAt: 2,
					Tags: model.Tags{
						{"p", "t9pk4"},
					},
				},
			},
		))
		helperMustBePrecalculatedCount(t, db, 2, model.Filter{Tags: model.TagMap{}.Set("p", model.PointerOf("t9pk1")), Kinds: []int{nostr.KindFollowList}})
		helperMustBePrecalculatedCount(t, db, 1, model.Filter{Tags: model.TagMap{}.Set("p", model.PointerOf("t9pk4")), Kinds: []int{nostr.KindFollowList}})
		events := helperSelectEvents(t, db, model.Filter{
			IDs:    []string{"t9id1", "t9id4"},
			Search: "include:dependencies:kind0>kind6400+kind3+group+p",
		})
		require.Len(t, events, 4)
		for _, ev := range events {
			t.Logf("dvm event: %+v", ev)
			switch ev.Kind {
			case model.KindDVMCountResponse:
				req := ev.GetTag("request")
				require.NotNil(t, req)
				value := req.Value()
				require.NotEmpty(t, value)

				var reqEvent model.Event
				err := json.Unmarshal([]byte(value), &reqEvent)
				require.NoError(t, err)
				t.Logf("request event: %+v", reqEvent)

				var filters model.Filters
				err = json.Unmarshal([]byte(reqEvent.Content), &filters)
				require.NoError(t, err)
				require.Len(t, filters, 1)
				t.Logf("filters: %+v", filters)
				require.Len(t, filters[0].Kinds, 1)
				require.Equal(t, nostr.KindFollowList, filters[0].Kinds[0])
				require.Len(t, filters[0].Tags, 1)

				keys, ok := filters[0].Tags["p"]
				require.True(t, ok)
				require.Len(t, keys, 1)
				require.Len(t, keys[0], 1)
				switch *keys[0][0] {
				case "t9pk1":
					require.Equal(t, "2", ev.Content)
				case "t9pk4":
					require.Equal(t, "1", ev.Content)
				default:
					t.Fatalf("unexpected author: %s", filters[0].Authors[0])
				}
			}
		}
		t.Run("Delegated", func(t *testing.T) {
			const (
				masterPrivate = `612be8342c593ba8a592f34c462e65a63c8233bad13aea022c5dcb3656a975d560f97174e3fc1c8decee03bfed97157a1a8db0d1140b8792958cd57f6de252e0`
				userPrivate   = `f66568ed325fac494593d5a191a591ca0e3cd4b04141350aa4703776f603e5769c1a22718581dc75961acc4b49f43935356bbecfc4cdb4229b12c31017a8a70c`
			)
			masterPublic, err := model.GetPublicKey(masterPrivate)
			require.NoError(t, err)
			userPublic, err := model.GetPublicKey(userPrivate)
			require.NoError(t, err)

			t.Logf("master public key: %s", masterPublic)
			t.Logf("user public key:   %s", userPublic)

			ev := &model.Event{
				Event: nostr.Event{
					Kind:      model.CustomIONKindAttestation,
					CreatedAt: 1,
					Tags:      model.Tags{{model.TagAttestationName, userPublic, "", model.CustomIONAttestationKindActive + ":1"}},
				},
			}
			require.NoError(t, ev.SignWithAlg(masterPrivate, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.NoError(t, db.AcceptEvents(t.Context(), ev))

			meta := &model.Event{
				Event: nostr.Event{
					Kind:      nostr.KindProfileMetadata,
					CreatedAt: 2,
					Tags:      model.Tags{{model.CustomIONTagOnBehalfOf, masterPublic}},
				},
			}
			require.NoError(t, meta.SignWithAlg(userPrivate, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.NoError(t, db.AcceptEvents(t.Context(), meta))

			require.NoError(t, db.AcceptEvents(t.Context(),
				&model.Event{
					Event: nostr.Event{
						ID:        "t10id1",
						Kind:      nostr.KindFollowList,
						PubKey:    "t10pk1",
						CreatedAt: 3,
						Tags: model.Tags{
							{"p", masterPublic},
						},
					},
				},
				&model.Event{
					Event: nostr.Event{
						ID:        "t10id2",
						Kind:      nostr.KindFollowList,
						PubKey:    "t10pk2",
						CreatedAt: 4,
						Tags: model.Tags{
							{"p", masterPublic},
						},
					},
				},
				&model.Event{
					Event: nostr.Event{
						ID:        "t10id3",
						Kind:      nostr.KindFollowList,
						PubKey:    "t10pk3",
						CreatedAt: 5,
						Tags: model.Tags{
							{"p", masterPublic},
						},
					},
				},
			))
			events := helperSelectEvents(t, db, model.Filter{
				Authors: []string{masterPublic},
				Search:  "include:dependencies:kind0>kind6400+kind3+group+p",
			})
			require.Len(t, events, 3) // Attestation, Profile metadata, follower count.
			for i, kind := range []int{model.KindDVMCountResponse, nostr.KindProfileMetadata, model.CustomIONKindAttestation} {
				require.Equalf(t, kind, events[i].Kind, "event %d: %v", i, events[i])
			}
			require.Equal(t, "3", events[0].Content)
		})
	})
}

func TestSelectDependencyQuote(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	user1Priv, user1Pub := model.GenerateKeyPair()
	user2Priv, _ := model.GenerateKeyPair()

	var event1 model.Event
	event1.Kind = nostr.KindTextNote
	event1.Content = "Hey"
	event1.CreatedAt = 1
	require.NoError(t, event1.SignWithAlg(user1Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	var event2 model.Event
	event2.Kind = nostr.KindTextNote
	event2.Content = "Repost"
	event2.CreatedAt = 2
	event2.Tags = model.Tags{
		{"q", event1.ID, "", user1Pub},
	}
	require.NoError(t, event2.SignWithAlg(user2Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, db.AcceptEvents(t.Context(), &event1, &event2))

	events := helperSelectEvents(t, db, model.Filter{
		Kinds:  []int{nostr.KindTextNote, nostr.KindRepost},
		Search: "include:dependencies:kind1>kind6400+kind1+group+q",
		Limit:  10,
	})
	require.Len(t, events, 3) // 2 notes, 1 dvm event.
	t.Logf("dvm event: %+v", events[0])
	require.Equal(t, model.KindDVMCountResponse, events[0].Kind)
	require.Equal(t, "1", events[0].Content)
}

func TestSelectDepsAuthorTags(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	// Original note.
	err := db.AcceptEvents(t.Context(), &model.Event{
		Event: nostr.Event{
			ID:        "id1",
			Kind:      nostr.KindTextNote,
			PubKey:    "pk1",
			CreatedAt: 1,
			Content:   "content of the note",
		},
	})
	require.NoError(t, err)

	// Two replies, different authors.
	err = db.AcceptEvents(t.Context(),
		&model.Event{
			Event: nostr.Event{
				ID:        "id2",
				Kind:      model.CustomIONKindEditableTextNote,
				PubKey:    "pk2",
				CreatedAt: 1,
				Content:   "content of the reply from pk2",
				Tags: model.Tags{
					{"e", "id1", "", "root"},
					{"published_at", "1"},
					{"d", "reply1"},
				},
			},
		},
		&model.Event{
			Event: nostr.Event{
				ID:        "id3",
				Kind:      model.CustomIONKindEditableTextNote,
				PubKey:    "pk3",
				CreatedAt: 3,
				Content:   "content of the reply from pk3",
				Tags: model.Tags{
					{"e", "id1", "", "root"},
					{"published_at", "3"},
					{"d", "reply1"},
				},
			},
		},
		&model.Event{
			Event: nostr.Event{
				ID:        "id4",
				Kind:      model.CustomIONKindEditableTextNote,
				PubKey:    "pk3",
				CreatedAt: 4,
				Content:   "content of the reply from pk3",
				Tags: model.Tags{
					{"e", "id1", "", "root"},
					{"published_at", "4"},
					{"d", "reply2"},
				},
			},
		},
	)
	require.NoError(t, err)

	t.Run("Select with dependencies", func(t *testing.T) {
		events := helperSelectEvents(t, db, model.Filter{
			IDs:    []string{"id1"},
			Search: "include:dependencies:kind1>pk3@kind30175+e+root",
		})
		require.Len(t, events, 2) // Original note, one reply.
		require.Equal(t, "id1", events[1].ID)
		// No pk2 (id2) reply.
		require.Equal(t, "id3", events[0].ID)
	})
	t.Run("Soft delete first reply", func(t *testing.T) {
		require.NoError(t, db.AcceptEvents(t.Context(), &model.Event{
			Event: nostr.Event{
				ID:        "id5",
				Kind:      model.CustomIONKindEditableTextNote,
				PubKey:    "pk3",
				CreatedAt: 5,
				Tags: model.Tags{
					{"e", "id1", "", "root"},
					{"published_at", "3"},
					{"d", "reply1"},
				},
			},
		}))
	})
	t.Run("Select with dependencies after delete", func(t *testing.T) {
		events := helperSelectEvents(t, db, model.Filter{
			IDs:    []string{"id1"},
			Search: "include:dependencies:kind1>pk3@kind30175+e+root",
		})
		require.Len(t, events, 2)
		require.Equal(t, "id1", events[1].ID)
		require.Equal(t, "id4", events[0].ID) // id3 was soft-deleted, so we get id4 instead.
	})
}

func helperEventsMatchFilter(t *testing.T, events []*model.Event, expectedCount int, filters ...model.Filter) {
	t.Helper()

	var matched int
	f := model.Filters(filters)
	for _, ev := range events {
		if f.Match(&ev.Event) {
			t.Logf("matched: %s with %s", ev.String(), f.String())
			matched++
		}
	}

	require.Equal(t, expectedCount, matched)
}

func TestDepMetadaAndMuteList(t *testing.T) {
	t.Parallel()

	db, _ := helperEnsureDatabaseWithData(t)
	defer db.Close()

	err := db.AcceptEvents(t.Context(),
		// Has no KindRelayListMetadata and no KindMuteList.
		&model.Event{
			Event: nostr.Event{
				Kind:      nostr.KindArticle,
				CreatedAt: 1,
				PubKey:    "pk1",
				ID:        "id1",
			},
		},

		// Has both KindRelayListMetadata and KindMuteList.
		&model.Event{
			Event: nostr.Event{
				Kind:      nostr.KindArticle,
				CreatedAt: 2,
				PubKey:    "pk2",
				ID:        "id2",
			},
		},
		&model.Event{
			Event: nostr.Event{
				Kind:      nostr.KindRelayListMetadata,
				CreatedAt: 22,
				PubKey:    "pk2",
				ID:        "id22",
			},
		},
		&model.Event{
			Event: nostr.Event{
				Kind:      nostr.KindMuteList,
				CreatedAt: 23,
				PubKey:    "pk2",
				ID:        "id23",
			},
		},

		// Has KindRelayListMetadata only.
		&model.Event{
			Event: nostr.Event{
				Kind:      nostr.KindArticle,
				CreatedAt: 3,
				PubKey:    "pk3",
				ID:        "id3",
			},
		},
		&model.Event{
			Event: nostr.Event{
				Kind:      nostr.KindRelayListMetadata,
				CreatedAt: 33,
				PubKey:    "pk3",
				ID:        "id33",
			},
		},

		// Has KindMuteList only.
		&model.Event{
			Event: nostr.Event{
				Kind:      nostr.KindArticle,
				CreatedAt: 4,
				PubKey:    "pk4",
				ID:        "id4",
			},
		},
		&model.Event{
			Event: nostr.Event{
				Kind:      nostr.KindMuteList,
				CreatedAt: 4,
				PubKey:    "pk4",
				ID:        "id42",
			},
		},

		// Has KindMuteList only.
		&model.Event{
			Event: nostr.Event{
				Kind:      nostr.KindArticle,
				CreatedAt: 5,
				PubKey:    "pk5",
				ID:        "id5",
			},
		},
		&model.Event{
			Event: nostr.Event{
				Kind:      nostr.KindMuteList,
				CreatedAt: 5,
				PubKey:    "pk5",
				ID:        "id52",
			},
		},
	)
	require.NoError(t, err)

	events := helperSelectEvents(t, db, model.Filter{
		Kinds:   []int{nostr.KindArticle},
		Authors: []string{"pk1", "pk2", "pk3", "pk4", "pk5"},
		Search:  "include:dependencies:kind30023>kind10002 include:dependencies:kind30023>kind10000",
	})
	require.Len(t, events, 15) // 5 articles, 10 genereted events, where 10 = (kind10000 + kind10002) * 5 (total event count).

	// pk1 has no KindRelayListMetadata and no KindMuteList.
	helperEventsMatchFilter(t, events, 2,
		model.Filter{
			Kinds: []int{model.CustomIONKindRelayListMetadata},
			Tags:  model.TagMap{}.SetLiterals("p", "pk1"),
		})

	// pk2 has both KindRelayListMetadata and KindMuteList.
	helperEventsMatchFilter(t, events, 2,
		model.Filter{
			Kinds:   []int{nostr.KindRelayListMetadata},
			Authors: []string{"pk2"},
		},
		model.Filter{
			Kinds:   []int{nostr.KindMuteList},
			Authors: []string{"pk2"},
		},
	)

	// pk3 has KindRelayListMetadata only.
	helperEventsMatchFilter(t, events, 2,
		model.Filter{
			Kinds:   []int{nostr.KindRelayListMetadata},
			Authors: []string{"pk3"},
		},
		model.Filter{
			Kinds: []int{model.CustomIONKindRelayListMetadata},
			Tags:  model.TagMap{}.SetLiterals("p", "pk3"),
		},
	)

	// pk4 has KindMuteList only.
	helperEventsMatchFilter(t, events, 2,
		model.Filter{
			Kinds:   []int{nostr.KindMuteList},
			Authors: []string{"pk4"},
		},
		model.Filter{
			Kinds: []int{model.CustomIONKindRelayListMetadata},
			Tags:  model.TagMap{}.SetLiterals("p", "pk4"),
		},
	)

	// pk5 has KindMuteList only.
	helperEventsMatchFilter(t, events, 2,
		model.Filter{
			Kinds:   []int{nostr.KindMuteList},
			Authors: []string{"pk5"},
		},
		model.Filter{
			Kinds: []int{model.CustomIONKindRelayListMetadata},
			Tags:  model.TagMap{}.SetLiterals("p", "pk5"),
		},
	)
}

func randomInt(n int) int {
	return rand.IntN(n) + 1
}

func TestDVMVoteResults(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	t.Run("Create", func(t *testing.T) {
		require.NoError(t, db.AcceptEvents(t.Context(),
			&model.Event{
				Event: nostr.Event{
					ID:     "poll1",
					Kind:   nostr.KindTextNote,
					PubKey: "pk1",
					Tags: model.Tags{
						{model.CustomIONTagPoll, "type multi", "title What is your favorite colors?", "options [\"Red\",\"Blue\",\"Green\",\"Yellow\"]"},
					},
				},
			},
			&model.Event{
				Event: nostr.Event{
					ID:     "poll2",
					Kind:   nostr.KindTextNote,
					PubKey: "pk1",
					Tags: model.Tags{
						{model.CustomIONTagPoll, "type single", "title What is your favorite color?", "options [\"Red\",\"Blue\",\"Green\",\"Yellow\"]"},
					},
				},
			},
			&model.Event{
				Event: nostr.Event{
					ID:     "poll3",
					Kind:   nostr.KindArticle,
					PubKey: "pk1",
					Tags: model.Tags{
						{model.CustomIONTagPoll, "type single", "title What is your favorite city?", "options [\"One\",\"Two\",\"Three\",\"Bar\"]"},
						{"d", "dtag3"},
					},
				},
			},
		))
	})

	results1 := map[string]int{
		"0":   randomInt(100),
		"1":   randomInt(99),
		"2":   randomInt(42),
		"3":   randomInt(666),
		"1,3": randomInt(50),
		"0,2": randomInt(70),
	}
	expected1 := map[string]int{
		"0": results1["0"] + results1["0,2"],
		"1": results1["1"] + results1["1,3"],
		"2": results1["2"] + results1["0,2"],
		"3": results1["3"] + results1["1,3"],
	}

	results2 := map[string]int{
		"0": randomInt(100),
		"1": randomInt(99),
	}

	results3 := map[string]int{
		"0": randomInt(100),
		"1": randomInt(99),
		"2": randomInt(42),
		"3": randomInt(666),
	}

	t.Run("Vote", func(t *testing.T) {
		cases := []struct {
			Results map[string]int
			ID      model.Tag
		}{
			{results1, model.Tag{"e", "poll1"}},
			{results2, model.Tag{"e", "poll2"}},
			{results3, model.Tag{"a", "30023:pk1:dtag3"}},
		}
		for _, c := range cases {
			t.Run(c.ID.Value(), func(t *testing.T) {
				for option, count := range c.Results {
					var events []*model.Event
					for range count {
						var ev model.Event
						ev.Kind = model.CustomIONKindPollVote
						ev.Content = `[` + option + `]`
						ev.Tags = model.Tags{c.ID}
						require.NoError(t, ev.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
						events = append(events, &ev)
					}
					require.NoError(t, db.AcceptEvents(t.Context(), events...))
				}
			})
		}
	})

	events := helperSelectEvents(t, db, model.Filter{
		Authors: []string{"pk1"},
		Search:  "include:dependencies:kind1>kind6400+kind1754+group+content include:dependencies:kind30023>kind6400+kind1754+group+content",
	})
	require.Len(t, events, 6) // 3 polls, 3 dvm events.

	t.Run("CheckResults", func(t *testing.T) {
		expected := map[string]map[string]int{
			"poll1":           expected1,
			"poll2":           results2,
			"30023:pk1:dtag3": results3,
		}
		for i := range events {
			if events[i].Kind != model.KindDVMCountResponse {
				continue
			}

			var counters map[string]int
			require.NoError(t, json.Unmarshal([]byte(events[i].Content), &counters))

			var request model.Event
			require.NoError(t, json.Unmarshal([]byte(events[i].GetTag("request").Value()), &request))

			var filters model.Filters
			require.NoError(t, json.Unmarshal([]byte(request.Content), &filters))
			require.Len(t, filters, 1)

			if filters[0].Tags.HasValues("e") {
				ids := filters[0].Tags.All("e")
				require.Len(t, ids, 1)
				require.Equal(t, expected[ids[0]], counters)
				delete(expected, ids[0])
			} else if filters[0].Tags.HasValues("a") {
				ids := filters[0].Tags.All("a")
				require.Len(t, ids, 1)
				require.Equal(t, expected[ids[0]], counters)
				delete(expected, ids[0])
			} else {
				t.Fatalf("unexpected filter: %s", filters[0].String())
			}
		}
		require.Empty(t, expected)
	})
}

func TestSelectDependencyWithAddressableEvents(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	user1Priv, user1Pub := model.GenerateKeyPair()

	var event1 model.Event
	event1.Kind = model.CustomIONKindEditableTextNote
	event1.Content = "Hey"
	event1.CreatedAt = 1
	event1.Tags = model.Tags{
		{"d", "dtag1"},
	}
	require.NoError(t, event1.SignWithAlg(user1Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	var event2 model.Event
	event2.Kind = model.CustomIONKindEditableTextNote
	event2.Content = "quote"
	event2.CreatedAt = 2
	event2.Tags = model.Tags{
		{"Q", event1.Address(), "", user1Pub},
		{"d", "dtag2"},
	}
	require.NoError(t, event2.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

	var event3 model.Event
	event3.Kind = model.CustomIONKindEditableTextNote
	event3.Content = "quote2"
	event3.CreatedAt = 3
	event3.Tags = model.Tags{
		{"Q", event1.Address(), "", user1Pub},
		{"d", "dtag3"},
	}
	require.NoError(t, event3.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

	require.NoError(t, db.AcceptEvents(t.Context(), &event1, &event2, &event3))

	events := helperSelectEvents(t, db, model.Filter{
		Kinds:  []int{model.CustomIONKindEditableTextNote},
		Search: "include:dependencies:kind30175>kind6400+kind30175+group+q",
		Limit:  10,
	})
	require.Len(t, events, 4) // 3 notes, 1 dvm event.

	t.Logf("dvm event: %+v", events[0]) // The last event is the DVM event.
	require.Equal(t, model.KindDVMCountResponse, events[0].Kind)
	require.Equal(t, "2", events[0].Content)
}

func TestSelectDependencyReactionAddressable(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	user1Priv, user1Pub := model.GenerateKeyPair()

	var event1 model.Event
	event1.Kind = model.CustomIONKindEditableTextNote
	event1.Content = "Hey"
	event1.CreatedAt = 1
	event1.Tags = model.Tags{
		{"d", "dtag1"},
	}
	require.NoError(t, event1.SignWithAlg(user1Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, db.AcceptEvents(t.Context(), &event1))

	var reaction1, reaction2 model.Event

	reaction1.ID = "id1"
	reaction1.Kind = nostr.KindReaction
	reaction1.PubKey = "pk1"
	reaction1.CreatedAt = 13
	reaction1.Content = "approve"
	reaction1.Tags = model.Tags{
		{"a", event1.Address()},
		{"p", user1Pub},
		{"k", strconv.Itoa(event1.Kind)},
	}

	reaction2.ID = "id2"
	reaction2.Kind = nostr.KindReaction
	reaction2.PubKey = "pk2"
	reaction2.CreatedAt = 11
	reaction2.Content = "minus"
	reaction2.Tags = model.Tags{
		{"a", event1.Address()},
		{"p", user1Pub},
		{"k", strconv.Itoa(event1.Kind)},
	}

	require.NoError(t, db.AcceptEvents(t.Context(), &reaction1, &reaction2))

	helperMustBePrecalculatedCount(t, db, 2, model.Filter{Tags: model.TagMap{}.Set("a", model.PointerOf(event1.Address())), Kinds: []int{nostr.KindReaction}})

	result, err := db.CountGroupedEventReactions(t.Context(), model.Filter{
		Kinds: []int{nostr.KindReaction},
		Tags: model.TagMap{}.
			Append("a", model.PointerOf(event1.Address())),
	})
	require.NoError(t, err)
	require.JSONEq(t, `{"approve":1,"minus":1}`, result)

	events := helperSelectEvents(t, db, model.Filter{
		IDs:    []string{event1.ID},
		Search: "include:dependencies:kind30175>kind6400+kind7+group+content",
	})

	require.Len(t, events, 2) // 1 event, 1 DVM (reaction count) event.
	require.Equal(t, event1.ID, events[1].ID)
	require.Equal(t, model.KindDVMCountResponse, events[0].Kind)
	require.JSONEq(t, `{"approve":1,"minus":1}`, events[0].Content)
}

func TestMostRelevantFollowers(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	t.Run("Create metadata", func(t *testing.T) {
		var bobMeta, aliceMeta, alexMeta, annaMeta, johnMeta, martinMeta model.Event
		bobMeta.ID = "id1"
		bobMeta.Kind = nostr.KindProfileMetadata
		bobMeta.PubKey = "bob"
		bobMeta.CreatedAt = 1
		bobMeta.Content = "{\"name\":\"Bob\"}"

		aliceMeta.ID = "id4"
		aliceMeta.Kind = nostr.KindProfileMetadata
		aliceMeta.PubKey = "alice"
		aliceMeta.CreatedAt = 2
		aliceMeta.Content = "{\"name\":\"Alice\"}"

		alexMeta.ID = "id2"
		alexMeta.Kind = nostr.KindProfileMetadata
		alexMeta.PubKey = "alex"
		alexMeta.CreatedAt = 3
		alexMeta.Content = "{\"name\":\"Alex\"}"

		annaMeta.ID = "id3"
		annaMeta.Kind = nostr.KindProfileMetadata
		annaMeta.PubKey = "anna"
		annaMeta.CreatedAt = 4
		annaMeta.Content = "{\"name\":\"Anna\"}"

		johnMeta.ID = "id5"
		johnMeta.Kind = nostr.KindProfileMetadata
		johnMeta.PubKey = "john"
		johnMeta.CreatedAt = 5
		johnMeta.Content = "{\"name\":\"John\"}"

		martinMeta.ID = "id6"
		martinMeta.Kind = nostr.KindProfileMetadata
		martinMeta.PubKey = "martin"
		martinMeta.CreatedAt = 6
		martinMeta.Content = "{\"name\":\"Martin\"}"

		require.NoError(t, db.AcceptEvents(t.Context(), &bobMeta, &aliceMeta, &alexMeta, &annaMeta, &johnMeta, &martinMeta))
	})
	t.Run("Create follow lists", func(t *testing.T) {
		var johnList, bobList, aliceList, alexList, annaList, martinList, joannaList model.Event
		bobList.Kind = nostr.KindFollowList
		bobList.PubKey = "bob"
		bobList.ID = "bob_id"
		bobList.CreatedAt = 1
		bobList.Tags = model.Tags{
			{"p", "john"},
			{"p", "alice"},
			{"p", "anna"},
			{"p", "joanna"},
		}

		aliceList.Kind = nostr.KindFollowList
		aliceList.PubKey = "alice"
		aliceList.ID = "alice_id"
		aliceList.CreatedAt = 2
		aliceList.Tags = model.Tags{
			{"p", "john"},
			{"p", "bob"},
			{"p", "alex"},
		}

		joannaList.Kind = nostr.KindFollowList
		joannaList.PubKey = "joanna"
		joannaList.ID = "joanna_id"
		joannaList.CreatedAt = 10
		joannaList.Tags = model.Tags{
			{"p", "bob"},
		}

		alexList.Kind = nostr.KindFollowList
		alexList.PubKey = "alex"
		alexList.ID = "alex_id"
		alexList.CreatedAt = 3
		alexList.Tags = model.Tags{
			{"p", "anna"},
			{"p", "john"},
		}

		annaList.Kind = nostr.KindFollowList
		annaList.PubKey = "anna"
		annaList.ID = "anna_id"
		annaList.CreatedAt = 4
		annaList.Tags = model.Tags{
			{"p", "alex"},
			{"p", "alice"},
		}

		martinList.Kind = nostr.KindFollowList
		martinList.PubKey = "martin"
		martinList.ID = "martin_id"
		martinList.CreatedAt = 5
		martinList.Tags = model.Tags{
			{"p", "john"},
			{"p", "bob"},
		}

		johnList.Kind = nostr.KindFollowList
		johnList.PubKey = "john"
		johnList.ID = "john_id"
		johnList.CreatedAt = 6
		johnList.Tags = model.Tags{
			{"p", "alex"},
			{"p", "anna"},
			{"p", "bob"},
			{"p", "alice"},
			{"p", "joanna"},
		}

		require.NoError(t, db.AcceptEvents(t.Context(),
			&johnList,
			&bobList,
			&aliceList,
			&alexList,
			&annaList,
			&martinList,
			&joannaList,
		))
	})

	// It's intersection between user's (Authors) follow list and users who follow X (include:dependencies:kind3>kind0+p+|X|).
	// Users who follow bob: martin, alice, john.
	// Users who follow alice: anna, bob, john.
	// John follows: alex, anna, bob, alice.
	t.Run("Find most relevant followers of john with alien", func(t *testing.T) {
		f := model.Filter{
			Kinds:   []int{nostr.KindFollowList},
			Authors: []string{"john"},
			Search:  "include:dependencies:kind3>kind0+p+|alien|",
			Limit:   10,
		}
		events := helperSelectEvents(t, db, f)
		require.Len(t, events, 1) // 1 main event (follow list).
	})
	t.Run("Find most relevant followers of john with bob", func(t *testing.T) {
		f := model.Filter{
			Kinds:   []int{nostr.KindFollowList},
			Authors: []string{"john"},
			Search:  "include:dependencies:kind3>kind0+p+|bob|",
			Limit:   10,
		}
		events := helperSelectEvents(t, db, f)
		require.Len(t, events, 3) // 1 main event (follow list), 1 kind 0 of relevant followers (alice), 1 ephemeral event with joanna.
		require.Equal(t, "alice", events[1].PubKey)
		require.Equal(t, "id4", events[1].ID)
		require.Equal(t, model.CustomIONKindEphemeralEmbedding, events[2].Kind)
		require.Equal(t, "joanna", events[2].GetTag("p").Value())
	})
	t.Run("Find most relevant followers of john with alice", func(t *testing.T) {
		f := model.Filter{
			Kinds:   []int{nostr.KindFollowList},
			Authors: []string{"john"},
			Search:  "include:dependencies:kind3>kind0+p+|alice|",
			Limit:   10,
		}
		events := helperSelectEvents(t, db, f)
		require.Len(t, events, 3) // 1 main event (follow list), 2 kind 0 of relevant followers.
		require.ElementsMatch(t, []string{"anna", "bob"}, []string{events[2].PubKey, events[1].PubKey})
		t.Run("Limited", func(t *testing.T) {
			f := model.Filter{
				Kinds:   []int{nostr.KindFollowList},
				Authors: []string{"john"},
				Search:  "include:dependencies:kind3>kind0+p+|alice|",
				Limit:   1,
			}
			events = helperSelectEvents(t, db, f)
			require.Len(t, events, 2) // 1 main event (follow list), 1 kind 0 of relevant followers.
		})
	})
}

func TestDependencyWithMasterAndAddress(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	userPriv, userPub := model.GenerateKeyPair()
	masterPriv, masterPub := model.GenerateKeyPair()

	t.Run("Create delegation", func(t *testing.T) {
		var ev model.Event
		ev.Kind = model.CustomIONKindAttestation
		ev.CreatedAt = 1
		ev.Tags = model.Tags{
			{model.TagAttestationName, userPub, "", model.CustomIONAttestationKindActive + ":1"},
		}
		require.NoError(t, ev.SignWithAlg(masterPriv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, db.AcceptEvents(t.Context(), &ev))
	})

	var root model.Event
	t.Run("Create root post", func(t *testing.T) {
		root.Kind = model.CustomIONKindEditableTextNote
		root.CreatedAt = 2
		root.Content = "root post"
		root.Tags = model.Tags{
			{"d", "rootpost"},
			{model.CustomIONTagOnBehalfOf, masterPub},
		}
		require.NoError(t, root.SignWithAlg(userPriv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, db.AcceptEvents(t.Context(), &root))
	})

	var repost model.Event
	t.Run("Create repost", func(t *testing.T) {
		repost.Kind = nostr.KindGenericRepost
		repost.CreatedAt = 3
		repost.Content = root.String()
		repost.Tags = model.Tags{
			{"a", root.Address()},
			{"p", userPub},
			{model.CustomIONTagOnBehalfOf, masterPub},
		}
		require.NoError(t, repost.SignWithAlg(userPriv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, db.AcceptEvents(t.Context(), &repost))
	})

	var like model.Event
	t.Run("Create like", func(t *testing.T) {
		like.Kind = nostr.KindReaction
		like.CreatedAt = 4
		like.Content = "+"
		like.Tags = model.Tags{
			{"a", root.Address()},
			{"p", userPub},
			{model.CustomIONTagOnBehalfOf, masterPub},
		}
		require.NoError(t, like.SignWithAlg(userPriv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, db.AcceptEvents(t.Context(), &like))
	})

	var reply model.Event
	t.Run("Create reply to root post", func(t *testing.T) {
		reply.Kind = model.CustomIONKindEditableTextNote
		reply.CreatedAt = 5
		reply.Content = "reply to root post"
		reply.Tags = model.Tags{
			{"a", root.Address(), "", "root"},
			{"p", userPub},
			{"d", "replytoroot"},
			{model.CustomIONTagOnBehalfOf, masterPub},
		}
		require.NoError(t, reply.SignWithAlg(userPriv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, db.AcceptEvents(t.Context(), &reply))
	})

	var replyToReply model.Event
	t.Run("Create reply to reply", func(t *testing.T) {
		replyToReply.Kind = model.CustomIONKindEditableTextNote
		replyToReply.CreatedAt = 6
		replyToReply.Content = "reply to reply"
		replyToReply.Tags = model.Tags{
			{"a", root.Address(), "", "root"},
			{"a", reply.Address(), "", "reply"},
			{"p", userPub},
			{"d", "replytoreply"},
			{model.CustomIONTagOnBehalfOf, masterPub},
		}
		require.NoError(t, replyToReply.SignWithAlg(userPriv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, db.AcceptEvents(t.Context(), &replyToReply))
	})

	f := model.Filter{
		Authors: []string{masterPub},
		Kinds:   []int{model.CustomIONKindEditableTextNote},
		Search: `include:dependencies:kind30175>` + masterPub + `@kind7` +
			` include:dependencies:kind30175>` + masterPub + `@kind16` +
			` include:dependencies:kind30175>` + masterPub + `@kind30175+e+root`,
		Tags: model.TagMap{}.SetLiterals("d", "rootpost"),
	}
	events := helperSelectEvents(t, db, f)
	require.Len(t, events, 4)
	for i, val := range []string{reply.ID, like.ID, repost.ID, root.ID} {
		require.Equal(t, val, events[i].ID)
		ok, err := events[i].CheckSignature()
		require.NoError(t, err)
		require.True(t, ok)
	}
}

func TestGenericKindWithProfileBadgeLookup(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	user1Priv := model.GeneratePrivateKey()
	user2Priv, user2Pub := model.GenerateKeyPair()

	t.Run("Create text notes", func(t *testing.T) {
		var note, article, editableNote model.Event

		note.Kind, editableNote.Kind = nostr.KindTextNote, model.CustomIONKindEditableTextNote
		note.CreatedAt, editableNote.CreatedAt = 1, 2
		note.Content, editableNote.Content = "note", "editable note"
		editableNote.Tags = model.Tags{{"d", "editable note"}}

		require.NoError(t, note.SignWithAlg(user1Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, editableNote.SignWithAlg(user2Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, db.AcceptEvents(t.Context(), &note, &editableNote))

		article.Kind = nostr.KindArticle
		article.CreatedAt = 3
		article.Content = "article"
		article.Tags = model.Tags{{"d", "article"}}
		require.NoError(t, article.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, db.AcceptEvents(t.Context(), &article))
	})

	var badgeDef, badgeAward, profileBadge model.Event
	t.Run("Create badge definition", func(t *testing.T) {
		badgeDef.Kind = nostr.KindBadgeDefinition
		badgeDef.CreatedAt = 4
		badgeDef.Tags = model.Tags{
			{"d", "testbadge"},
			{"name", "Test Badge"},
		}
		require.NoError(t, badgeDef.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, db.AcceptEvents(t.Context(), &badgeDef))
	})
	t.Run("Create badge award", func(t *testing.T) {
		badgeAward.Kind = nostr.KindBadgeAward
		badgeAward.CreatedAt = 5
		badgeAward.Tags = model.Tags{
			{"a", badgeDef.Address()},
			{"p", user2Pub},
		}
		require.NoError(t, badgeAward.SignWithAlg(user1Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, db.AcceptEvents(t.Context(), &badgeAward))
	})
	t.Run("Create profile badges of user2", func(t *testing.T) {
		profileBadge.Kind = nostr.KindProfileBadges
		profileBadge.CreatedAt = 6
		profileBadge.Tags = model.Tags{
			{"d", "profile_badges"},
			{"a", badgeAward.GetTag("a").Value()},
			{"e", badgeAward.ID},
		}
		require.NoError(t, profileBadge.SignWithAlg(user2Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, db.AcceptEvents(t.Context(), &profileBadge))
	})

	kinds := []int{nostr.KindTextNote, nostr.KindArticle, model.CustomIONKindEditableTextNote}
	notes := helperSelectEvents(t, db, model.Filter{Kinds: kinds})
	require.Len(t, notes, 3)

	for _, kind := range kinds {
		t.Run("Check kind "+strconv.Itoa(kind), func(t *testing.T) {
			events := helperSelectEvents(t, db, model.Filter{
				Kinds:  []int{kind},
				Search: "include:dependencies:kind" + strconv.Itoa(kind) + ">kind30008+profile_badges>kind30009>kind8",
			})
			switch kind {
			case nostr.KindTextNote, nostr.KindArticle:
				require.Len(t, events, 1)
				require.Equal(t, kind, events[0].Kind)
			case model.CustomIONKindEditableTextNote:
				require.Len(t, events, 4) // 1 editable note, 1 badge definition, 1 badge award, 1 profile badge.
				for i, expectedKind := range []int{nostr.KindProfileBadges, nostr.KindBadgeAward, nostr.KindBadgeDefinition, model.CustomIONKindEditableTextNote} {
					require.Equal(t, expectedKind, events[i].Kind)
				}
			default:
				t.Fatalf("unexpected kind: %d", kind)
			}
		})
	}
	t.Run("by kind", func(t *testing.T) {
		events := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{model.CustomIONKindEditableTextNote},
			Search: "include:dependencies:kind1>kind30008+profile_badges>kind30009>kind8",
		})
		require.Len(t, events, 1)
		require.Equal(t, model.CustomIONKindEditableTextNote, events[0].Kind)
	})
}

func TestSelectDependencyStoryCount(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	user1Priv, user1Pub := model.GenerateKeyPair()
	user2Priv, user2Pub := model.GenerateKeyPair()

	var userMeta1, userMeta2 model.Event
	userMeta1.Kind = nostr.KindProfileMetadata
	userMeta1.CreatedAt = nostr.Now().Add(-time.Minute)
	userMeta1.Content = `{"name":"User1"}`
	require.NoError(t, userMeta1.SignWithAlg(user1Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	userMeta2.Kind = nostr.KindProfileMetadata
	userMeta2.CreatedAt = nostr.Now().Add(-time.Minute)
	userMeta2.Content = `{"name":"User2"}`
	require.NoError(t, userMeta2.SignWithAlg(user2Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, db.AcceptEvents(t.Context(), &userMeta1, &userMeta2))

	var user1Story model.Event
	user1Story.Kind = model.CustomIONKindEditableTextNote
	user1Story.CreatedAt = nostr.Now()
	user1Story.Content = "User1 story"
	user1Story.Tags = model.Tags{
		{"d", "story_of_user1"},
		{"expiration", user1Story.CreatedAt.Add(time.Hour).String()},
	}
	require.NoError(t, user1Story.SignWithAlg(user1Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, db.AcceptEvents(t.Context(), &user1Story))

	var user1Post model.Event
	user1Post.Kind = nostr.KindTextNote
	user1Post.CreatedAt = nostr.Now()
	user1Post.Content = "User1 post"
	require.NoError(t, user1Post.SignWithAlg(user1Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, db.AcceptEvents(t.Context(), &user1Post))

	events := helperSelectEvents(t, db, model.Filter{
		Kinds:  []int{nostr.KindProfileMetadata},
		Search: "include:dependencies:kind0>kind6400+kind30175+expiration",
	})
	require.Len(t, events, 4, "2 dvm events, 2 profile metadata")
	for i := range events[:2] {
		var req model.Event

		require.Equal(t, model.KindDVMCountResponse, events[i].Kind)
		require.NoError(t, req.UnmarshalJSON([]byte(events[i].GetTag("request").Value())))

		userKey := events[i].GetTag("p").Value()
		require.NotEmpty(t, userKey)
		switch userKey {
		case user1Pub:
			require.Equal(t, "1", events[i].Content)
		case user2Pub:
			require.Equal(t, "0", events[i].Content)
		default:
			t.Fatalf("unexpected user pubkey: %s", userKey)
		}

		var reqFilters model.Filters
		require.NoError(t, json.Unmarshal([]byte(req.Content), &reqFilters))

		require.Len(t, reqFilters, 1)
		require.Len(t, reqFilters[0].Kinds, 1)
		require.Len(t, reqFilters[0].Authors, 1)
		require.Equal(t, "expiration:true", reqFilters[0].Search)
		require.Equal(t, userKey, reqFilters[0].Authors[0])
		require.Equal(t, model.CustomIONKindEditableTextNote, reqFilters[0].Kinds[0])
	}
}

func TestReduceKind3EventsFromFollowersQuery(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	userPub, _ := model.GenerateKeyPair()

	masterUser1Priv, masterUser1Pub := model.GenerateKeyPair()
	masterUser2Priv, masterUser2Pub := model.GenerateKeyPair()

	var meta1, meta2 model.Event
	t.Run("Create users", func(t *testing.T) {
		user1KeyPriv, user1KeyPub := model.GenerateKeyPair()
		user2KeyPriv, user2KeyPub := model.GenerateKeyPair()

		t.Run("Attestation", func(t *testing.T) {
			var user1Att, user2Att model.Event
			user1Att.Kind = model.CustomIONKindAttestation
			user1Att.CreatedAt = 1
			user1Att.Tags = model.Tags{
				{model.TagAttestationName, user1KeyPub, "", model.CustomIONAttestationKindActive + ":1"},
			}
			require.NoError(t, user1Att.SignWithAlg(masterUser1Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

			user2Att.Kind = model.CustomIONKindAttestation
			user2Att.CreatedAt = 2
			user2Att.Tags = model.Tags{
				{model.TagAttestationName, user2KeyPub, "", model.CustomIONAttestationKindActive + ":1"},
			}
			require.NoError(t, user2Att.SignWithAlg(masterUser2Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

			require.NoError(t, db.AcceptEvents(t.Context(), &user1Att, &user2Att))
		})

		meta1.Kind = nostr.KindProfileMetadata
		meta1.CreatedAt = 1
		meta1.Content = model.ProfileMetadataContent{
			Name:  "User1",
			About: "About User1",
		}.String()
		meta1.Tags = model.Tags{
			{model.CustomIONTagOnBehalfOf, masterUser1Pub},
		}

		meta2.Kind = nostr.KindProfileMetadata
		meta2.CreatedAt = 2
		meta2.Content = model.ProfileMetadataContent{
			Name:  "User2",
			About: "About User2",
		}.String()
		meta2.Tags = model.Tags{
			{model.CustomIONTagOnBehalfOf, masterUser2Pub},
		}

		require.NoError(t, meta1.SignWithAlg(user1KeyPriv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, meta2.SignWithAlg(user2KeyPriv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		require.NoError(t, db.AcceptEvents(t.Context(), &meta1, &meta2))

		var follow1, follow2 model.Event
		follow1.Kind = nostr.KindFollowList
		follow1.CreatedAt = 3
		follow1.Tags = model.Tags{
			{model.CustomIONTagOnBehalfOf, masterUser1Pub},
			{"p", userPub},
			{"p", meta2.PubKey},
		}
		require.NoError(t, follow1.SignWithAlg(user1KeyPriv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		follow2.Kind = nostr.KindFollowList
		follow2.CreatedAt = 4
		follow2.Tags = model.Tags{
			{model.CustomIONTagOnBehalfOf, masterUser2Pub},
			{"p", userPub},
			{"p", meta1.PubKey},
		}
		require.NoError(t, follow2.SignWithAlg(user2KeyPriv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		require.NoError(t, db.AcceptEvents(t.Context(), &follow1, &follow2))
	})
	t.Run("Get metadata via dependencies", func(t *testing.T) {
		f := model.Filter{
			Kinds:  []int{nostr.KindFollowList},
			Search: "include:dependencies:kind3>kind0",
			Tags:   model.TagMap{}.SetLiterals("p", userPub),
		}
		events := helperSelectEvents(t, db, f)
		require.Len(t, events, 4) // 2 follow lists embedding, 2 metadata.
		require.EqualValues(t, []*model.Event{&meta2, &meta1}, events[2:])
		require.Equal(t, model.CustomIONKindEphemeralEmbedding, events[0].Kind)
		require.Equal(t, model.CustomIONKindEphemeralEmbedding, events[1].Kind)

		f.Kinds = nil
		events = helperSelectEvents(t, db, f)
		require.Len(t, events, 4) // 2 metadata, 2 follow lists.
	})
}
