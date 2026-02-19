// SPDX-License-Identifier: ice License 1.0

package eventmatcher

import (
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func TestEventMatcherSetAndGet(t *testing.T) {
	t.Parallel()

	matcher := newEventMatcher[*model.Subscription]()
	require.NotNil(t, matcher)

	t.Run("Empty", func(t *testing.T) {
		sub := model.NewSubscription("sub-empty-filter", model.Filters{})

		matcher.Index(sub.Filters, sub)
		require.EqualValues(t, 1, matcher.Generic.GetCardinality())
		require.EqualValues(t, 1, matcher.Values.Size())

		data := matcher.Get(new(model.Event))
		require.Len(t, data, 1)
		require.Equal(t, sub, data[0])

		require.True(t, matcher.Remove(sub))
	})
	t.Run("Generic", func(t *testing.T) {
		sub := model.NewSubscription("sub-author-filter", model.Filters{{Authors: []string{"root"}}})

		matcher.Index(sub.Filters, sub)
		require.EqualValues(t, 1, matcher.Generic.GetCardinality())
		require.EqualValues(t, 1, matcher.Values.Size())

		data := matcher.Get(new(model.Event))
		require.Len(t, data, 1)
		require.True(t, matcher.Remove(sub))
	})

	t.Run("By kind", func(t *testing.T) {
		sub := model.NewSubscription("sub-kinds-only", model.Filters{{Kinds: []int{1, 2, 3}}})

		matcher.Index(sub.Filters, sub)
		require.EqualValues(t, 1, matcher.Values.Size())
		require.Len(t, matcher.ByKind, 3)

		var ev model.Event
		ev.Kind = 1
		data := matcher.Get(&ev)
		require.Len(t, data, 1)
		require.Equal(t, sub, data[0])

		ev.Kind = 2
		data = matcher.Get(&ev)
		require.Len(t, data, 1)
		require.Equal(t, sub, data[0])

		ev.Kind = 4
		data = matcher.Get(&ev)
		require.Empty(t, data) // No match.

		require.True(t, matcher.Remove(sub))
	})
	t.Run("By p tag", func(t *testing.T) {
		sub := model.NewSubscription("sub-p-tag-only", model.Filters{{Tags: model.TagMap{}.SetLiterals("p", "root")}})

		matcher.Index(sub.Filters, sub)
		require.EqualValues(t, 1, matcher.Values.Size())
		require.Len(t, matcher.ByDestination, 1)

		var ev model.Event
		ev.Tags = model.Tags{{"p", "root"}}
		data := matcher.Get(&ev)
		require.Len(t, data, 1)
		require.Equal(t, sub, data[0])

		ev.Tags = model.Tags{{"p", "non-root"}}
		data = matcher.Get(&ev)
		require.Empty(t, data) // No match.

		require.True(t, matcher.Remove(sub))
	})
	t.Run("By Q tag", func(t *testing.T) {
		sub := model.NewSubscription("sub-Q-tag-only", model.Filters{{Tags: model.TagMap{}.Set("Q", nil, nil, new("relay.example.com"))}})

		matcher.Index(sub.Filters, sub)
		require.EqualValues(t, 1, matcher.Values.Size())
		require.Len(t, matcher.ByDestination, 1)

		var ev model.Event
		ev.Tags = model.Tags{{"Q", "", "", "relay.example.com"}}
		data := matcher.Get(&ev)
		require.Len(t, data, 1)
		require.Equal(t, sub, data[0])

		ev.Tags = model.Tags{{"Q", "", "", "other-relay.example.com"}}
		data = matcher.Get(&ev)
		require.Empty(t, data) // No match.

		ev.Kind = 10
		ev.Tags = model.Tags{}
		data = matcher.Get(&ev)
		require.Empty(t, data) // No match.

		require.True(t, matcher.Remove(sub))
	})
	t.Run("By kind and p tag", func(t *testing.T) {
		sub := model.NewSubscription("sub-kinds-and-p-tag", model.Filters{{Kinds: []int{10, 11}, Tags: model.TagMap{}.SetLiterals("p", "root")}})

		matcher.Index(sub.Filters, sub)
		require.EqualValues(t, 1, matcher.Values.Size())
		require.Len(t, matcher.ByKindDestination, 2)

		var ev model.Event
		ev.Kind = 10
		ev.Tags = model.Tags{{"p", "root"}}
		data := matcher.Get(&ev)
		require.Len(t, data, 1)
		require.Equal(t, sub, data[0])

		ev.Kind = 11
		ev.Tags = model.Tags{{"p", "root"}}
		data = matcher.Get(&ev)
		require.Len(t, data, 1)
		require.Equal(t, sub, data[0])

		ev.Kind = 12
		ev.Tags = model.Tags{{"p", "root"}}
		data = matcher.Get(&ev)
		require.Empty(t, data) // No match.

		ev.Kind = 10
		ev.Tags = model.Tags{{"p", "non-root"}}
		data = matcher.Get(&ev)
		require.Empty(t, data) // No match.

		require.True(t, matcher.Remove(sub))
	})
	t.Run("By kind and Q tag", func(t *testing.T) {
		sub := model.NewSubscription("sub-kinds-and-Q-tag", model.Filters{
			{
				Kinds: []int{10, 11},
				Tags: model.TagMap{}.
					Set("Q", nil, nil, new("relay.example.com")),
			}},
		)

		matcher.Index(sub.Filters, sub)

		var ev model.Event
		ev.Kind = 10
		ev.Tags = model.Tags{{"Q", "", "", "relay.example.com"}}
		data := matcher.Get(&ev)
		require.Len(t, data, 1)
		require.Equal(t, sub, data[0]) // Match by kind and Q tag.

		ev.Kind = 11
		ev.Tags = model.Tags{{"Q", "", "", "relay.example.com"}}
		data = matcher.Get(&ev)
		require.Len(t, data, 1)
		require.Equal(t, sub, data[0]) // Match by kind and Q tag.

		ev.Kind = 12
		ev.Tags = model.Tags{{"Q", "", "", "relay.example.com"}}
		data = matcher.Get(&ev)
		require.Empty(t, data) // No match, kind 12 not in index.

		ev.Kind = 10
		ev.Tags = model.Tags{{"Q", "", "", "fooo.example.com"}}
		data = matcher.Get(&ev)
		require.Empty(t, data) // No match, Q tag does not match.

		require.True(t, matcher.Remove(sub))
	})
	t.Run("By p and Q tags", func(t *testing.T) {
		sub := model.NewSubscription("sub-p-and-Q-tags", model.Filters{
			{
				Tags: model.TagMap{}.
					Set("p", new("root")).
					Set("Q", nil, nil, new("relay.example.com")),
			},
		})

		matcher.Index(sub.Filters, sub)
		require.EqualValues(t, 1, matcher.Values.Size())

		var ev model.Event
		ev.Kind = 10
		ev.Tags = model.Tags{{"Q", "", "", "relay.example.com"}}
		data := matcher.Get(&ev)
		require.Len(t, data, 1) // At least one tag match.

		ev.Kind = 11
		ev.Tags = model.Tags{{"p", "root"}}
		data = matcher.Get(&ev)
		require.Len(t, data, 1) // At least one tag match

		ev.Kind = 12
		ev.Tags = model.Tags{
			{"Q", "", "", "relay.example.com"},
			{"p", "root"},
		}
		data = matcher.Get(&ev)
		require.Len(t, data, 1) // Match by p and Q tags.
		require.Equal(t, sub, data[0])

		require.True(t, matcher.RemoveByHash(sub.Hash()))
	})

	t.Run("By kind p and Q tags", func(t *testing.T) {
		sub := model.NewSubscription("sub-kinds-p-and-Q-tags", model.Filters{
			{
				Kinds: []int{1, 2},
				Tags: model.TagMap{}.
					Set("p", new("root")).
					Set("Q", nil, nil, new("relay.example.com")),
			},
		})

		matcher.Index(sub.Filters, sub)
		require.EqualValues(t, 1, matcher.Values.Size())

		var ev model.Event
		ev.Kind = 10
		ev.Tags = model.Tags{{"Q", "", "", "relay.example.com"}}
		data := matcher.Get(&ev)
		require.Empty(t, data) // No match, kind 10 not in index.

		ev.Kind = 11
		ev.Tags = model.Tags{{"p", "root"}}
		data = matcher.Get(&ev)
		require.Empty(t, data) // No match, kind 10 not in index.

		ev.Kind = 12
		ev.Tags = model.Tags{
			{"Q", "", "", "relay.example.com"},
			{"p", "root"},
		}
		data = matcher.Get(&ev)
		require.Empty(t, data) // No match, kind 10 not in index.

		ev.Kind = 1
		ev.Tags = model.Tags{
			{"Q", "", "", "relay.example.com"},
			{"p", "root"},
		}
		data = matcher.Get(&ev)
		require.Len(t, data, 1) // Match by kind and tags.
		require.Equal(t, sub, data[0])

		require.True(t, matcher.Remove(sub))
	})
	t.Run("By kind with author", func(t *testing.T) {
		sub := model.NewSubscription("sub-kinds-and-author", model.Filters{
			{
				Kinds:   []int{1, 2},
				Authors: []string{"root"},
			},
		})

		matcher.Index(sub.Filters, sub)
		require.EqualValues(t, 1, matcher.Values.Size())
		require.Len(t, matcher.ByKindAuthor, 2)

		var ev model.Event
		ev.Kind = 1
		ev.PubKey = "root"
		data := matcher.Get(&ev)
		require.Len(t, data, 1)
		require.Equal(t, sub, data[0]) // Match by kind and author.

		ev.Kind = 2
		ev.PubKey = "root"
		data = matcher.Get(&ev)
		require.Len(t, data, 1)
		require.Equal(t, sub, data[0]) // Match by kind and author.

		ev.Kind = 3
		ev.PubKey = "root"
		data = matcher.Get(&ev)
		require.Empty(t, data) // No match, kind 3 not in index.

		ev.Kind = 1
		ev.PubKey = "non-root"
		data = matcher.Get(&ev)
		require.Empty(t, data) // No match, author does not match.

		ev.Kind = 1
		ev.PubKey = "beta"
		ev.Tags = model.Tags{{"b", "root"}} // Author matches by b tag.
		data = matcher.Get(&ev)
		require.Len(t, data, 1)

		require.True(t, matcher.Remove(sub))
	})
	t.Run("Kind-only sub with tagged ev", func(t *testing.T) {
		sub := model.NewSubscription("kind-only", model.Filters{{Kinds: []int{1}}})

		matcher.Index(sub.Filters, sub)

		var ev model.Event
		ev.Kind = 1
		ev.Tags = model.Tags{{"p", "foo"}} // Additional tag.
		data := matcher.Get(&ev)
		require.Len(t, data, 1) // Should match.

		require.True(t, matcher.Remove(sub))
	})
	t.Run("Multi-filter OR", func(t *testing.T) {
		sub := model.NewSubscription("sub-mixed-or-kind-p-tag",
			model.Filters{
				{Kinds: []int{1}},
				{Tags: model.TagMap{}.SetLiterals("p", "root")},
			})

		matcher.Index(sub.Filters, sub)
		require.EqualValues(t, 1, matcher.Values.Size())
		require.Len(t, matcher.ByDestination, 1)

		var ev1 model.Event
		ev1.Kind = 1
		data1 := matcher.Get(&ev1)
		require.Len(t, data1, 1) // Matches first filter.
		require.Equal(t, sub, data1[0])

		var ev2 model.Event
		ev2.Kind = 2
		ev2.Tags = model.Tags{{"p", "root"}}
		data2 := matcher.Get(&ev2)
		require.Len(t, data2, 1) // Matches second filter.
		require.Equal(t, sub, data2[0])

		var ev3 model.Event
		ev3.Kind = 2
		data3 := matcher.Get(&ev3)
		require.Empty(t, data3) // No match.

		var ev4 model.Event
		ev4.Kind = 1
		ev4.Tags = model.Tags{{"p", "root"}}
		data4 := matcher.Get(&ev4)
		require.Len(t, data4, 1) // Matches both filters.
		require.Equal(t, sub, data4[0])

		require.True(t, matcher.Remove(sub))
	})
	t.Run("Multi keys OR", func(t *testing.T) {
		sub := model.NewSubscription("sub-mixed-or-kind-p-tag",
			model.Filters{
				{
					Kinds: []int{1},
					Tags:  model.TagMap{}.SetLiterals("p", "root", "", "device"),
				},
				{
					Kinds: []int{2},
					Tags:  model.TagMap{}.Set("Q", nil, nil, new("root")),
				},
				{
					Kinds: []int{3},
					Tags:  model.TagMap{}.SetLiterals("p", "root"),
				},
				{
					Kinds: []int{4},
					Tags:  model.TagMap{}.SetLiterals("e", "id"), // Should be ignored.
				},
				{
					Kinds: []int{5},
					Tags: model.TagMap{
						"x": []model.TagValues{
							{new("val1"), new("val2")}, // Should be ignored.
							{new("val3")},
						},
					},
				},
			})

		matcher.Index(sub.Filters, sub)

		var ev model.Event

		ev.Kind = 1
		ev.Tags = model.Tags{{"p", "device"}}
		data := matcher.Get(&ev)
		require.Empty(t, data) // No match, want kind 1 with "root".

		ev.Kind = 1
		ev.Tags = model.Tags{{"p", "root"}}
		data = matcher.Get(&ev)
		require.Len(t, data, 1)
		require.Equal(t, sub, data[0])

		ev.Kind = 4
		ev.Tags = model.Tags{
			{"p", "some-key"},
			{"e", "id2"},
		}
		data = matcher.Get(&ev)
		require.Len(t, data, 1) // Match by kind only, `e`/`p` tags ignored.

		ev.Kind = 2
		ev.Tags = model.Tags{{"p", "root"}}
		data = matcher.Get(&ev)
		require.Empty(t, data) // No match.

		ev.Kind = 2
		ev.Tags = model.Tags{{"x", "root"}}
		data = matcher.Get(&ev)
		require.Empty(t, data) // No match.

		require.True(t, matcher.Remove(sub))
	})
}

func TestMatcherStorageIterators(t *testing.T) {
	t.Parallel()

	const subCount = 1000

	var subs []*model.Subscription
	for i := range subCount {
		subs = append(subs, model.NewSubscription("sub-"+strconv.Itoa(i), model.Filters{
			{Kinds: []int{1}},
		}))
	}

	t.Run("Matcher iterator", func(t *testing.T) {
		matcher := newEventMatcher[*model.Subscription]()
		for _, sub := range subs {
			matcher.Index(sub.Filters, sub)
		}
		require.EqualValues(t, subCount, matcher.Values.Size())
		var ev model.Event
		ev.Kind = 1
		t.Run("Full", func(t *testing.T) {
			var found int
			matcher.Lookup(&ev, func(s *model.Subscription) bool {
				found++
				return true
			})
			require.EqualValues(t, subCount, found)
		})
		t.Run("Partial", func(t *testing.T) {
			var found int
			matcher.Lookup(&ev, func(s *model.Subscription) bool {
				found++
				return found < subCount/2
			})
			require.EqualValues(t, subCount/2, found)
		})
	})
	t.Run("Storage iterator", func(t *testing.T) {
		const shardCount = 42

		storage := NewMatcherStorage[*model.Subscription](shardCount)
		for _, sub := range subs {
			storage.Index(sub.Filters, sub)
		}
		require.EqualValues(t, subCount, storage.Size())
		var ev model.Event
		ev.Kind = 1
		t.Run("Full", func(t *testing.T) {
			var found int
			for range storage.Lookup(&ev) {
				found++
			}
			require.EqualValues(t, subCount, found)
		})
		t.Run("Partial", func(t *testing.T) {
			left := subCount / 2
			for range storage.Lookup(&ev) {
				left--
				if left == 0 {
					break
				}
			}
			require.Zero(t, left)
		})
		t.Run("Partial Range", func(t *testing.T) {
			left := subCount / 2
			for range storage.Range() {
				left--
				if left == 0 {
					break
				}
			}
			require.Zero(t, left)
		})
		t.Run("Full Range", func(t *testing.T) {
			var count int
			for range storage.Range() {
				count++
			}
			require.EqualValues(t, subCount, count)
		})
	})
}

func BenchmarkMatcherStorageInsert(b *testing.B) {
	const (
		numberOfShards        = 16
		numberOfSubscriptions = 50_000
	)

	storage := NewMatcherStorage[*model.Subscription](numberOfShards)

	var subs []*model.Subscription
	for i := range numberOfSubscriptions {
		var filters model.Filters

		numKinds := 1 + (i % 2) // 1 or 2 kinds.
		kinds := make([]int, numKinds)
		for j := range numKinds {
			kinds[j] = 1 + (i*31+j)%20000 // pseudo-random kind in range 1...20k.
		}

		filter := model.Filter{Kinds: kinds}

		// If subIdx % 2 == 0, add p tag.
		if i%2 == 0 {
			filter.Tags = model.TagMap{}.SetLiterals("p", "ptagvalue_"+strconv.Itoa(i))
		}

		filters = append(filters, filter)
		sub := model.NewSubscription("sub-test-num-"+strconv.Itoa(i), filters)
		subs = append(subs, sub)

	}

	zerolog.SetGlobalLevel(zerolog.Disabled)

	randSource := rand.NewChaCha8([32]byte{42, 42})
	require.NotNil(b, randSource)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(p *testing.PB) {
		for p.Next() {
			idx := randSource.Uint64() % uint64(len(subs))
			storage.Index(subs[int(idx)].Filters, subs[int(idx)])
		}
	})
	for i, shard := range storage.shards {
		b.ReportMetric(float64(shard.Values.Size()), "subs_count/shard_"+strconv.Itoa(i))
	}
}
