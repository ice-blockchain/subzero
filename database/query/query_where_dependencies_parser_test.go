// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"testing"

	"github.com/ice-blockchain/subzero/model"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
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
			Input: "kind1>publickey@kind1+e+root",
			Expected: filterDependencies{
				Start: filterDependenciesStart{
					Kind: 1,
				},
				Reduce: filterDependenciesReduce{
					Kinds:   []int{1},
					Author:  "publickey",
					Tag:     "e",
					Context: "root",
				},
			},
		},
		{
			Input: "kind1>publickey@kind1+q",
			Expected: filterDependencies{
				Start: filterDependenciesStart{
					Kind: 1,
				},
				Reduce: filterDependenciesReduce{
					Kinds:  []int{1},
					Author: "publickey",
					Tag:    "q",
				},
			},
		},
		{
			Input: "kind1>publickey@kind6",
			Expected: filterDependencies{
				Start: filterDependenciesStart{
					Kind: 1,
				},
				Reduce: filterDependenciesReduce{
					Kinds:  []int{6},
					Author: "publickey",
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
		require.Len(t, events, 4)
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
			{"e", "t2id2"},
		}
		err := db.AcceptEvents(context.Background(), &ev)
		require.NoError(t, err)

		ev.ID = "t3id2"
		ev.Kind = nostr.KindReaction
		ev.PubKey = "t3pk2"
		ev.CreatedAt = 13
		ev.Content = "+"
		ev.Tags = model.Tags{
			{"e", "t2id3"},
		}
		err = db.AcceptEvents(context.Background(), &ev)
		require.NoError(t, err)

		events := helperSelectEvents(t, db, model.Filter{
			IDs:    []string{"t2id2", "t2id3"},
			Search: "include:dependencies:kind1>kind6400+kind7+group+content",
		})
		require.Len(t, events, 4)
		require.Equal(t, "t2id3", events[0].ID)
		require.Equal(t, "t2id2", events[1].ID)
		require.Condition(t, func() bool {
			return events[2].Content == "1" &&
				events[3].Content == "1" &&
				events[2].Kind == model.KindDVMCount &&
				events[3].Kind == model.KindDVMCount &&
				events[3].ID == "t2id3" &&
				events[2].ID == "t2id2"
		})
	})
}
