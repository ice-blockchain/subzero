// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
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
		Expected filterDependencies
		Err      error
	}{
		{
			Input: "kind30008+profile_badges>kind30009>kind8",
			Expected: filterDependencies{
				Start: filterDependenciesStart{
					Kind:          30008,
					ProfileBadges: true,
				},
				Reduce: filterDependenciesReduce{
					Kinds: []int{30009, 8},
				},
			},
		},
		{
			Input: "kind6>kind10002",
			Expected: filterDependencies{
				Start: filterDependenciesStart{
					Kind: 6,
				},
				Reduce: filterDependenciesReduce{
					Kinds: []int{10002},
				},
			},
		},
		{
			Input: "kind3>kind0",
			Expected: filterDependencies{
				Start: filterDependenciesStart{
					Kind: 3,
				},
				Reduce: filterDependenciesReduce{
					Kinds: []int{0},
				},
			},
		},
		{
			Input: "kind1+q>kind10002",
			Expected: filterDependencies{
				Start: filterDependenciesStart{
					Kind: 1,
					Tag:  "q",
				},
				Reduce: filterDependenciesReduce{
					Kinds: []int{10002},
				},
			},
		},
		{
			Input: "kind1>e4f0cf865fd1b24845b694a9dbe5296f663b0c4d449308c3afbd9f319ecbbcd4@kind1+e+root",
			Expected: filterDependencies{
				Start: filterDependenciesStart{
					Kind: 1,
				},
				Reduce: filterDependenciesReduce{
					Kinds:   []int{1},
					Author:  "e4f0cf865fd1b24845b694a9dbe5296f663b0c4d449308c3afbd9f319ecbbcd4",
					Tag:     "e",
					Context: "root",
				},
			},
		},
		{
			Input: "kind1>3cfb1533dd7534bc0bbd60ad40492a4f131c2cb05ca47994d12ea530d7c40183@kind1+e+root",
			Expected: filterDependencies{
				Start: filterDependenciesStart{
					Kind: 1,
				},
				Reduce: filterDependenciesReduce{
					Kinds:   []int{1},
					Author:  "3cfb1533dd7534bc0bbd60ad40492a4f131c2cb05ca47994d12ea530d7c40183",
					Tag:     "e",
					Context: "root",
				},
			},
		},
		{
			Input: "kind1>3cfb1533dd7534bc0bbd60ad40492a4f131c2cb05ca47994d12ea530d7c40183@kind1+e+reply",
			Expected: filterDependencies{
				Start: filterDependenciesStart{
					Kind: 1,
				},
				Reduce: filterDependenciesReduce{
					Kinds:   []int{1},
					Author:  "3cfb1533dd7534bc0bbd60ad40492a4f131c2cb05ca47994d12ea530d7c40183",
					Tag:     "e",
					Context: "reply",
				},
			},
		},
		{
			Input: "kind1>3cfb1533dd7534bc0bbd60ad40492a4f131c2cb05ca47994d12ea530d7c40183@kind1+q",
			Expected: filterDependencies{
				Start: filterDependenciesStart{
					Kind: 1,
				},
				Reduce: filterDependenciesReduce{
					Kinds:  []int{1},
					Author: "3cfb1533dd7534bc0bbd60ad40492a4f131c2cb05ca47994d12ea530d7c40183",
					Tag:    "q",
				},
			},
		},
		{
			Input: "kind1>3cfb1533dd7534bc0bbd60ad40492a4f131c2cb05ca47994d12ea530d7c40183@kind6",
			Expected: filterDependencies{
				Start: filterDependenciesStart{
					Kind: 1,
				},
				Reduce: filterDependenciesReduce{
					Kinds:  []int{6},
					Author: "3cfb1533dd7534bc0bbd60ad40492a4f131c2cb05ca47994d12ea530d7c40183",
				},
			},
		},
		{
			Input: "kind1>kind6400+kind1+group+root",
			Expected: filterDependencies{
				Start: filterDependenciesStart{
					Kind: 1,
				},
				Reduce: filterDependenciesReduce{
					Kinds:   []int{6400, 1},
					Group:   true,
					Context: "root",
				},
			},
		},
		{
			Input: "kind1>kind6400+kind1+group+q",
			Expected: filterDependencies{
				Start: filterDependenciesStart{
					Kind: 1,
				},
				Reduce: filterDependenciesReduce{
					Kinds: []int{6400, 1},
					Group: true,
					Tag:   "q",
				},
			},
		},
		{
			Input: "kind0>kind6400+kind3+group+p",
			Expected: filterDependencies{
				Start: filterDependenciesStart{
					Kind: 0,
				},
				Reduce: filterDependenciesReduce{
					Kinds: []int{6400, 3},
					Group: true,
					Tag:   "p",
				},
			},
		},
		{
			Input: "kind1>kind6400+kind7+group+content",
			Expected: filterDependencies{
				Start: filterDependenciesStart{
					Kind: 1,
				},
				Reduce: filterDependenciesReduce{
					Kinds:   []int{6400, 7},
					Group:   true,
					Context: "content",
				},
			},
		},
		{
			Input: "kind30023>kind6400+kind1754+group+content",
			Expected: filterDependencies{
				Start: filterDependenciesStart{
					Kind: 30023,
				},
				Reduce: filterDependenciesReduce{
					Kinds:   []int{6400, 1754},
					Group:   true,
					Context: "content",
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
		err := db.AcceptEvents(context.Background(), &ev)
		require.NoError(t, err)

		ev.ID = "id2"
		ev.Kind = nostr.KindTextNote
		ev.PubKey = "pk1"
		ev.CreatedAt = 2
		ev.Content = "content of the note"
		err = db.AcceptEvents(context.Background(), &ev)
		require.NoError(t, err)

		events := helperSelectEvents(t, db, model.Filter{
			IDs:    []string{"id2"},
			Search: "include:dependencies:kind1>kind0",
		})
		require.Len(t, events, 2)
		require.Equal(t, "id2", events[0].ID)
		require.Equal(t, "id1", events[1].ID)
	})
	t.Run("kind1>$logged_in_user_pubkey@kind1+e+root", func(t *testing.T) {
		var ev model.Event

		ev.ID = "t2id1"
		ev.Kind = nostr.KindTextNote
		ev.PubKey = "t2pk1"
		ev.CreatedAt = 1
		ev.Content = "text note 1"
		err := db.AcceptEvents(context.Background(), &ev)
		require.NoError(t, err)

		ev.ID = "t2id4"
		ev.Kind = nostr.KindTextNote
		ev.PubKey = "t2pk1"
		ev.CreatedAt = 1
		err = db.AcceptEvents(context.Background(), &ev)
		require.NoError(t, err)

		ev.ID = "t2id2"
		ev.Kind = nostr.KindTextNote
		ev.PubKey = "t2pk2"
		ev.CreatedAt = 2
		ev.Tags = model.Tags{
			{"e", "t2id1", "", "root"},
		}
		err = db.AcceptEvents(context.Background(), &ev)
		require.NoError(t, err)

		ev.ID = "t2id3"
		ev.Kind = nostr.KindTextNote
		ev.PubKey = "t2pk2"
		ev.CreatedAt = 3
		ev.Tags = model.Tags{
			{"e", "t2id1", "", "root"},
		}
		err = db.AcceptEvents(context.Background(), &ev)
		require.NoError(t, err)

		ev.ID = "t2id5"
		ev.Kind = nostr.KindTextNote
		ev.PubKey = "t2pk2"
		ev.CreatedAt = 4
		ev.Tags = model.Tags{
			{"e", "id2", "", "root"},
		}
		err = db.AcceptEvents(context.Background(), &ev)
		require.NoError(t, err)

		events := helperSelectEvents(t, db, model.Filter{
			IDs:    []string{"t2id1", "id2"},
			Search: "include:dependencies:kind1>t2pk2@kind1+e+root",
		})
		require.Len(t, events, 4) // Two original notes, two replies (only one single reply per note). t2id3 must be excluded.
		require.Equal(t, "t2id1", events[0].ID)
		require.Equal(t, "id2", events[1].ID)
		require.Equal(t, "t2id2", events[2].ID)
		require.Equal(t, "t2id5", events[3].ID)
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
		err := db.AcceptEvents(context.Background(), &ev)
		require.NoError(t, err)

		ev.ID = "t3id2"
		ev.Kind = nostr.KindReaction
		ev.PubKey = "t3pk2"
		ev.CreatedAt = 13
		ev.Content = "*"
		ev.Tags = model.Tags{
			{"e", "t2id3"},
		}
		err = db.AcceptEvents(context.Background(), &ev)
		require.NoError(t, err)
		helperMustBePrecalculatedCount(t, db, 2, model.Filter{Tags: model.TagMap{}.Set("e", model.PointerOf("t2id3")), Kinds: []int{nostr.KindReaction}})

		result, err := db.CountGroupedEventReactions(context.Background(), model.Filter{
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
		require.Equal(t, "t2id3", events[0].ID)
		require.Equal(t, "t2id2", events[1].ID)
		for _, ev := range events[2:] {
			t.Logf("dvm event: %+v", ev)
			require.Equal(t, model.KindDVMCountResponse, ev.Kind)
			require.Equal(t, `{"*":1,"+":1}`, ev.Content)
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
		err := db.AcceptEvents(context.Background(), &ev)
		require.NoError(t, err)

		// t4pk2 wants to award t4pk3 the badge `testbadge`.
		ev.ID = "t4id2"
		ev.Kind = nostr.KindBadgeAward
		ev.PubKey = "t4pk2"
		ev.CreatedAt = 2
		ev.Tags = model.Tags{
			{"a", "30009:t4pk1:testbadge"},
		}
		err = db.AcceptEvents(context.Background(), &ev)
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
		err = db.AcceptEvents(context.Background(), &ev)

		events := helperSelectEvents(t, db, model.Filter{
			Authors: []string{"t4pk3"},
			Search:  "include:dependencies:kind30008+profile_badges>kind30009>kind8",
		})
		require.Len(t, events, 3)
		require.Equal(t, nostr.KindProfileBadges, events[0].Kind)
		require.Equal(t, nostr.KindBadgeDefinition, events[1].Kind)
		require.Equal(t, nostr.KindBadgeAward, events[2].Kind)
	})
	t.Run("Combined dependencies", func(t *testing.T) {
		events := helperSelectEvents(t, db, model.Filter{
			Authors: []string{"t4pk3", "pk1"},
			Search:  "include:dependencies:kind1>kind0 include:dependencies:kind30008+profile_badges>kind30009>kind8",
		})
		require.Len(t, events, 5) // 2 from the first search, 3 from the second.
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
			err := db.AcceptEvents(context.Background(), &ev1, &ev2)
			require.NoError(t, err)

			evRelayMetadata := model.Event{}
			evRelayMetadata.ID = "t6id3"
			evRelayMetadata.Kind = nostr.KindRelayListMetadata
			evRelayMetadata.PubKey = "t6pk2"
			evRelayMetadata.CreatedAt = 15
			evRelayMetadata.Tags = model.Tags{
				{"r", "wss://foo.bar"},
			}
			err = db.AcceptEvents(context.Background(), &evRelayMetadata)
			require.NoError(t, err)
			events := helperSelectEvents(t, db, model.Filter{
				IDs:    []string{"t6id1", "t6id2", "t2id5"},
				Search: "include:dependencies:kind6>kind10002",
			})
			require.Len(t, events, 5) // 2 reposts, 1 note, 2 relay metadata.
			for i, k := range []int{nostr.KindRepost, nostr.KindRepost, nostr.KindTextNote, nostr.KindRelayListMetadata, model.CustomIONKindRelayListMetadata} {
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

			err := db.AcceptEvents(context.Background(), &ev1, &ev2)
			require.NoError(t, err)

			ev1.ID = "t7id3"
			ev1.Kind = nostr.KindTextNote
			ev1.PubKey = "t7pk3"
			ev1.CreatedAt = 16
			ev1.Tags = model.Tags{}
			err = db.AcceptEvents(context.Background(), &ev1)
			require.NoError(t, err)

			events := helperSelectEvents(t, db, model.Filter{
				IDs:    []string{"t7id1", "t7id3", "t2id5"},
				Search: "include:dependencies:kind1+q>kind10002",
			})
			require.Len(t, events, 4) // 3 notes, 1 relay metadata.
			for i, k := range []int{nostr.KindTextNote, nostr.KindTextNote, nostr.KindTextNote, nostr.KindRelayListMetadata} {
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
		err := db.AcceptEvents(context.Background(), &ev)
		require.NoError(t, err)

		ev.ID = "t8id2"
		ev.Kind = nostr.KindArticle
		ev.PubKey = "t8pk1"
		ev.CreatedAt = 2
		ev.Content = "content of the article"
		err = db.AcceptEvents(context.Background(), &ev)
		require.NoError(t, err)

		events := helperSelectEvents(t, db, model.Filter{
			IDs:    []string{"t8id2"},
			Search: "include:dependencies:kind30023>kind0",
		})
		require.Len(t, events, 2)
		require.Equal(t, "t8id2", events[0].ID)
		require.Equal(t, "t8id1", events[1].ID)
	})
	t.Run("kind0>kind6400+kind3+group+p", func(t *testing.T) {
		// Celebrity 1, and two fans.
		require.NoError(t, db.AcceptEvents(context.Background(),
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
		require.NoError(t, db.AcceptEvents(context.Background(),
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
			require.NoError(t, db.AcceptEvents(context.TODO(), ev))

			meta := &model.Event{
				Event: nostr.Event{
					Kind:      nostr.KindProfileMetadata,
					CreatedAt: 2,
					Tags:      model.Tags{{model.CustomIONTagOnBehalfOf, masterPublic}},
				},
			}
			require.NoError(t, meta.SignWithAlg(userPrivate, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.NoError(t, db.AcceptEvents(context.TODO(), meta))

			require.NoError(t, db.AcceptEvents(context.Background(),
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
			for i, kind := range []int{nostr.KindProfileMetadata, model.CustomIONKindAttestation, model.KindDVMCountResponse} {
				require.Equalf(t, kind, events[i].Kind, "event %d: %v", i, events[i])
			}
			require.Equal(t, "3", events[2].Content)
		})
	})
}

func TestSelectExpirationWithDependencies(t *testing.T) {
	t.Parallel()
	db := helperNewDatabase(t)
	defer db.Close()

	now := time.Now().Unix()
	expiration := strconv.FormatInt(now+3600, 10)

	t.Run("Insert", func(t *testing.T) {
		require.NoError(t, db.AcceptEvents(context.Background(),
			&model.Event{
				Event: nostr.Event{
					ID:        "t1id1",
					PubKey:    "t1pk1",
					Content:   "content1",
					Kind:      nostr.KindTextNote,
					CreatedAt: 1,
					Tags:      model.Tags{{"expiration", expiration}},
				},
			},
			&model.Event{
				Event: nostr.Event{
					ID:        "t1id2",
					PubKey:    "t1pk2",
					Content:   "content2",
					Kind:      nostr.KindTextNote,
					CreatedAt: 2,
					Tags:      model.Tags{{"expiration", expiration}},
				},
			},
			&model.Event{
				Event: nostr.Event{
					ID:        "t1id3",
					PubKey:    "t1pk2",
					Content:   "content3",
					Kind:      nostr.KindTextNote,
					CreatedAt: 3,
					Tags:      model.Tags{{"expiration", expiration}},
				},
			},
			&model.Event{
				Event: nostr.Event{
					ID:        "t1id4",
					PubKey:    "t1pk4",
					Content:   "content4",
					Kind:      nostr.KindTextNote,
					CreatedAt: 4,
					Tags:      model.Tags{{"expiration", expiration}},
				},
			},
			&model.Event{
				Event: nostr.Event{
					ID:        "t1id5",
					PubKey:    "t1pk1",
					Content:   "content5",
					Kind:      nostr.KindTextNote,
					CreatedAt: 5,
					Tags:      model.Tags{{"expiration", expiration}},
				},
			},
		))
	})

	events := helperSelectEvents(t, db, model.Filter{Limit: 2, Search: "expiration:true"})
	require.Len(t, events, 3) // main: (t1pk1 + t1pk4) + dep: (t1pk1 / t1id1).
	for i, pubkey := range []string{"t1pk1", "t1pk4", "t1pk1"} {
		t.Logf("event %d: %+v", i, events[i])
		require.Equal(t, pubkey, events[i].PubKey)
	}
}

func TestSelectDependenciesQuote(t *testing.T) {
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
	require.NoError(t, db.AcceptEvents(context.Background(), &event1, &event2))

	events := helperSelectEvents(t, db, model.Filter{
		Kinds:  []int{nostr.KindTextNote, nostr.KindRepost},
		Search: "include:dependencies:kind1>kind6400+kind1+group+q",
		Limit:  10,
	})
	require.Len(t, events, 3) // 2 notes, 1 dvm event.
	t.Logf("dvm event: %+v", events[2])
	require.Equal(t, model.KindDVMCountResponse, events[2].Kind)
	require.Equal(t, "1", events[2].Content)
}

func TestSelectDepsAuthorTags(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	// Original note.
	err := db.AcceptEvents(context.Background(), &model.Event{
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
	err = db.AcceptEvents(context.Background(),
		&model.Event{
			Event: nostr.Event{
				ID:        "id2",
				Kind:      nostr.KindTextNote,
				PubKey:    "pk2",
				CreatedAt: 2,
				Content:   "content of the reply from pk2",
				Tags:      model.Tags{{"e", "id1", "", "root"}},
			},
		},
		&model.Event{
			Event: nostr.Event{
				ID:        "id3",
				Kind:      nostr.KindTextNote,
				PubKey:    "pk3",
				CreatedAt: 3,
				Content:   "content of the reply from pk3",
				Tags:      model.Tags{{"e", "id1", "", "root"}},
			},
		},
	)
	require.NoError(t, err)

	events := helperSelectEvents(t, db, model.Filter{
		IDs:    []string{"id1"},
		Search: "include:dependencies:kind1>pk3@kind1+e+root",
	})
	require.Len(t, events, 2) // Original note, one reply.
	require.Equal(t, "id1", events[0].ID)
	// No pk2 (id2) reply.
	require.Equal(t, "id3", events[1].ID)
}

func helperEventsMatch(t *testing.T, events []*model.Event, expectedCount int, filters ...model.Filter) {
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

	err := db.AcceptEvents(context.Background(),
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
	helperEventsMatch(t, events, 2,
		model.Filter{
			Kinds: []int{model.CustomIONKindRelayListMetadata},
			Tags:  model.TagMap{}.SetLiterals("p", "pk1"),
		})

	// pk2 has both KindRelayListMetadata and KindMuteList.
	helperEventsMatch(t, events, 2,
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
	helperEventsMatch(t, events, 2,
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
	helperEventsMatch(t, events, 2,
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
	helperEventsMatch(t, events, 2,
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
		require.NoError(t, db.AcceptEvents(context.Background(),
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
			ID      string
		}{
			{results1, "poll1"},
			{results2, "poll2"},
			{results3, "30023:pk1:dtag3"},
		}
		for _, c := range cases {
			t.Run(c.ID, func(t *testing.T) {
				for option, count := range c.Results {
					var events []*model.Event
					for range count {
						var ev model.Event
						ev.Kind = model.CustomIONKindPollVote
						ev.Content = `[` + option + `]`
						ev.Tags = model.Tags{
							{"e", c.ID},
						}
						require.NoError(t, ev.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
						events = append(events, &ev)
					}
					require.NoError(t, db.AcceptEvents(context.Background(), events...))
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
		events = events[3:]
		var cases = []struct {
			ID       string
			Expected map[string]int
		}{
			{"poll1", expected1},
			{"poll2", results2},
			{"poll3", results3},
		}
		for i := range events {
			require.Equal(t, model.KindDVMCountResponse, events[i].Kind)

			var counters map[string]int
			require.NoError(t, json.Unmarshal([]byte(events[i].Content), &counters))
			require.Equal(t, cases[i].Expected, counters)

			switch cases[i].ID {
			case "poll1", "poll2":
				require.Contains(t, events[i].String(), `#e\\\":\\\"`+cases[i].ID+`\\\"`)
			case "poll3":
				require.Contains(t, events[i].String(), `#a\\\":\\\"30023:pk1:dtag3\\\"`)
			}
		}
	})
}

func TestSelectDependenciesWithAddressableEvents(t *testing.T) {
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

	require.NoError(t, db.AcceptEvents(context.Background(), &event1, &event2, &event3))

	events := helperSelectEvents(t, db, model.Filter{
		Kinds:  []int{model.CustomIONKindEditableTextNote},
		Search: "include:dependencies:kind30175>kind6400+kind30175+group+q",
		Limit:  10,
	})
	require.Len(t, events, 4) // 3 notes, 1 dvm event.

	t.Logf("dvm event: %+v", events[len(events)-1]) // The last event is the DVM event.
	require.Equal(t, model.KindDVMCountResponse, events[len(events)-1].Kind)
	require.Equal(t, "2", events[len(events)-1].Content)
}

func TestSelectDependenciesReactionAddressable(t *testing.T) {
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
	require.NoError(t, db.AcceptEvents(context.Background(), &event1))

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

	require.NoError(t, db.AcceptEvents(context.Background(), &reaction1, &reaction2))

	helperMustBePrecalculatedCount(t, db, 2, model.Filter{Tags: model.TagMap{}.Set("a", model.PointerOf(event1.Address())), Kinds: []int{nostr.KindReaction}})

	result, err := db.CountGroupedEventReactions(context.Background(), model.Filter{
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
	require.Equal(t, event1.ID, events[0].ID)
	require.Equal(t, model.KindDVMCountResponse, events[1].Kind)
	require.JSONEq(t, `{"approve":1,"minus":1}`, events[1].Content)
}
