// SPDX-License-Identifier: ice License 1.0

package eventmatcher

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

type dummyIntValue int

func (d dummyIntValue) Hash() uint64 {
	return uint64(d)
}

func TestParseFiltersWithAuthorsAndKTags(t *testing.T) {
	t.Parallel()

	var complexFilter = model.Filter{
		Kinds: []int{model.CustomIONKindTokenizedCommunityAction},
		Tags: model.TagMap{
			"p": []model.TagValues{{new("pOne")}, {new("pTwo")}},
			"k": []model.TagValues{{new("1")}, {new("2")}, {new("3")}},
		},
	}

	t.Run("Parse", func(t *testing.T) {
		keys := parseFilters(model.Filters{complexFilter})
		require.Len(t, keys, 5) // 2 from "p" tags and 3 from "k" tags.
		var kFound, pFound int
		for i, key := range keys {
			t.Logf("Key %d: %s", i, key.String())
			switch key.Dimension {
			case dimTagP:
				pFound++
				require.Contains(t, []string{"pOne", "pTwo"}, key.Value)
			case dimTagK:
				kFound++
				require.Contains(t, []string{"1", "2", "3"}, key.Value)
			default:
				t.Errorf("Unexpected dimension: %c", key.Dimension)
			}
		}
		require.Equal(t, 2, pFound)
		require.Equal(t, 3, kFound)
	})
	t.Run("Index", func(t *testing.T) {
		ev := newEventMatcher[dummyIntValue]()
		ev.Index(model.Filters{complexFilter}, dummyIntValue(42))
		require.Equal(t, 1, ev.Size())
		require.Len(t, ev.Indexes, 5)
		t.Run("Match", func(t *testing.T) {
			var event model.Event
			event.Kind = model.CustomIONKindTokenizedCommunityAction
			event.Tags = model.Tags{
				{"k", "42"},
			}
			matches := ev.Get(&event)
			require.Empty(t, matches)

			event.Tags = model.Tags{
				{"p", "pOne"},
			}
			matches = ev.Get(&event)
			require.Len(t, matches, 1)
			require.EqualValues(t, 42, matches[0])

			event.Tags = model.Tags{
				{"k", "3"},
			}
			matches = ev.Get(&event)
			require.Len(t, matches, 1)
			require.EqualValues(t, 42, matches[0])

			event.Tags = model.Tags{
				{"p", "XXX"},
				{"k", "2"},
			}
			for _, key := range parseEvent(&event, make([]indexKey, 0, 10)) {
				t.Logf("Event key: %s", key.String())
			}
			matches = ev.Get(&event)
			require.Len(t, matches, 1)
			require.EqualValues(t, 42, matches[0]) // Match on "k" tag even if "p" tag doesn't match.
		})
		t.Run("Remove", func(t *testing.T) {
			removed := ev.Remove(dummyIntValue(42))
			require.True(t, removed)
			require.Zero(t, ev.Size())
			require.Zero(t, ev.IndexSize())
		})
	})
}
