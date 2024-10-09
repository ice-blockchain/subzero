// SPDX-License-Identifier: ice License 1.0

package query

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func TestIsFilterEmpty(t *testing.T) {
	t.Parallel()

	require.True(t, isFilterEmpty(&databaseFilter{}))

	f := helperNewFilter(func(apply *model.Filter) {
		apply.IDs = []string{"123"}
	})
	require.False(t, isFilterEmpty(parseNostrFilter(f)))
}

func TestWhereBuilderEmpty(t *testing.T) {
	t.Parallel()

	builder := newWhereBuilder()
	q, params, err := builder.Build()
	require.NoError(t, err)
	require.Equal(t, whereBuilderDefaultWhere, q)
	require.Empty(t, params)
}

func helperEnsureParams(t *testing.T, stmt string, params map[string]any) {
	t.Helper()

	for k := range params {
		require.Contains(t, stmt, ":"+k)
	}
}

func TestWhereBuilderSingleNoTags(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		q, params, err := newWhereBuilder().Build()
		require.NoError(t, err)
		require.Empty(t, params)
		require.Equal(t, whereBuilderDefaultWhere, q)
	})
	t.Run("WithID", func(t *testing.T) {
		q, params, err := newWhereBuilder().Build(helperNewFilter(func(apply *model.Filter) {
			apply.IDs = []string{"123"}
		}))
		require.NoError(t, err)
		t.Logf("stmt: %s (%+v)", q, params)
		require.Len(t, params, 1)
		helperEnsureParams(t, q, params)
	})
	t.Run("WithMoreIDs", func(t *testing.T) {
		q, params, err := newWhereBuilder().Build(helperNewFilter(func(apply *model.Filter) {
			apply.IDs = []string{generateHexString(), "789"}
		}))
		require.NoError(t, err)
		t.Logf("stmt: %s (%+v)", q, params)
		require.Len(t, params, 2)
		helperEnsureParams(t, q, params)
	})
	t.Run("WithKind", func(t *testing.T) {
		q, params, err := newWhereBuilder().Build(helperNewFilter(func(apply *model.Filter) {
			apply.IDs = []string{generateHexString()}
			apply.Kinds = []int{1, 2}
		}))
		require.NoError(t, err)
		t.Logf("stmt: %s (%+v)", q, params)
		require.Len(t, params, 3)
		helperEnsureParams(t, q, params)
	})
	t.Run("WithAuthors", func(t *testing.T) {
		q, params, err := newWhereBuilder().Build(helperNewFilter(func(apply *model.Filter) {
			apply.IDs = []string{generateHexString()}
			apply.Kinds = []int{1}
			apply.Authors = []string{"author1", "author2"}
		}))
		require.NoError(t, err)
		t.Logf("stmt: %s (%+v)", q, params)
		require.Len(t, params, 4)
		helperEnsureParams(t, q, params)
	})
	t.Run("WithTimeRange", func(t *testing.T) {
		ts1 := model.Timestamp(generateCreatedAt())
		ts2 := model.Timestamp(generateCreatedAt())
		filter := helperNewFilter(func(apply *model.Filter) {
			apply.Since = &ts1
			apply.Until = &ts2
		})
		helperBenchEnsureValidRange(t, &filter)
		q, params, err := newWhereBuilder().Build(filter)
		require.NoError(t, err)
		t.Logf("stmt: %s (%+v)", q, params)
		require.Len(t, params, 2)
		helperEnsureParams(t, q, params)
	})
	t.Run("WithTimestamp", func(t *testing.T) {
		ts1 := model.Timestamp(generateCreatedAt())
		q, params, err := newWhereBuilder().Build(helperNewFilter(func(apply *model.Filter) {
			apply.Since = &ts1
			apply.Until = &ts1
		}))
		require.NoError(t, err)
		t.Logf("stmt: %s (%+v)", q, params)
		require.Len(t, params, 1)
		helperEnsureParams(t, q, params)
	})
	t.Run("WithInvalidTimeRange", func(t *testing.T) {
		ts1 := model.Timestamp(1)
		ts2 := model.Timestamp(2)
		q, params, err := newWhereBuilder().Build(helperNewFilter(func(apply *model.Filter) {
			apply.Since = &ts2
			apply.Until = &ts1
		}))
		require.ErrorIs(t, err, ErrWhereBuilderInvalidTimeRange)
		require.Empty(t, q)
		require.Len(t, params, 0)
	})
}

func TestWhereBuilderSingleWithTags(t *testing.T) {
	t.Parallel()

	t.Run("OneTag", func(t *testing.T) {
		q, params, err := newWhereBuilder().Build(helperNewFilter(func(apply *model.Filter) {
			apply.IDs = []string{"123"}
			apply.Tags = model.TagMap{
				"#e": {"value1", "value2", "value3", "value4"},
			}
		}))
		t.Logf("stmt: %s (%+v)", q, params)
		require.NoError(t, err)
		require.Len(t, params, 7)
		helperEnsureParams(t, q, params)
	})
	t.Run("TwoTagsShink", func(t *testing.T) {
		var valuesMax []string

		for range 30 {
			valuesMax = append(valuesMax, generateRandomString(4))
		}

		q, params, err := newWhereBuilder().Build(helperNewFilter(func(apply *model.Filter) {
			apply.IDs = []string{"123"}
			apply.Tags = model.TagMap{
				"#e": {"value1", "value2", "value3", generateRandomString(4)},
				"#p": valuesMax,
			}
		}))

		require.NoError(t, err)
		t.Logf("stmt: %s (%+v)", q, params)
		require.Len(t, params, 29)
		helperEnsureParams(t, q, params)
	})
}

func TestWhereBuilderMulti(t *testing.T) {
	t.Parallel()

	ts1 := model.Timestamp(generateCreatedAt())
	ts2 := model.Timestamp(generateCreatedAt())
	filters := []model.Filter{
		helperNewFilter(func(apply *model.Filter) {
			apply.IDs = []string{"123"}
			apply.Tags = model.TagMap{"#e": {"value1", "value2", "value3", "value4"}}
		}),
		helperNewFilter(func(apply *model.Filter) {
			apply.IDs = []string{"456"}
			apply.Tags = model.TagMap{"#d": {"value1", "value2", "value3", "value4"}}
			apply.Until = &ts2
		}),
		helperNewFilter(func(apply *model.Filter) {
			apply.Authors = []string{"author1", "author2"}
			apply.Since = &ts1
		}),
	}

	builder := newWhereBuilder()
	q, params, err := builder.Build(filters...)
	require.NoError(t, err)
	t.Logf("stmt: %s (%+v)", q, params)
	require.Len(t, params, 18)
	helperEnsureParams(t, q, params)
}

func TestWhereBuilderMultiTagsOnly(t *testing.T) {
	t.Parallel()

	filters := []model.Filter{
		helperNewFilter(func(apply *model.Filter) {
			apply.Tags = model.TagMap{"#e": {"value1", generateRandomString(3), "value3", "value4"}}
		}),
		helperNewFilter(func(apply *model.Filter) {
			apply.Tags = model.TagMap{"#d": {"value1", "value2", generateRandomString(4), generateRandomString(4)}}
		}),
	}

	builder := newWhereBuilder()
	q, params, err := builder.Build(filters...)
	require.NoError(t, err)
	t.Logf("stmt: %s (%+v)", q, params)
	require.Len(t, params, 10)
	helperEnsureParams(t, q, params)
}

func TestWhereBuilderSameElements(t *testing.T) {
	t.Parallel()

	filters := helperNewSingleFilter(func(apply *model.Filter) {
		apply.IDs = []string{"123", "456", "123"}
		apply.Authors = []string{"111", "222", "222"}
	})

	builder := newWhereBuilder()
	q, params, err := builder.Build(filters...)
	require.NoError(t, err)
	t.Logf("stmt: %s (%+v)", q, params)
	t.Logf("params: %+v", params)
	require.Len(t, params, 4)
	helperEnsureParams(t, q, params)
}

func TestWhereBuilderMimeType(t *testing.T) {
	t.Parallel()

	filters := helperNewSingleFilter(func(apply *model.Filter) {
		apply.Search = "images:true videos:false"
	})

	builder := newWhereBuilder()
	q, params, err := builder.Build(filters...)
	require.NoError(t, err)
	t.Logf("stmt: %s (%+v)", q, params)
	t.Logf("params: %+v", params)
	require.Len(t, params, 0)
}

func TestParseNostrFilter(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		f := parseNostrFilter(model.Filter{})
		require.Empty(t, f.Filter)
		require.Nil(t, f.Quotes)
		require.Nil(t, f.Images)
	})
	t.Run("Images", func(t *testing.T) {
		f := parseNostrFilter(model.Filter{
			Search: "images:true",
		})
		require.Empty(t, f.Filter)
		require.NotNil(t, f.Images)
		require.Equal(t, ExtensionOn, *f.Images)
	})
	t.Run("ImagesWithQuotes", func(t *testing.T) {
		f := parseNostrFilter(model.Filter{
			Search: "images:true quoteS:off",
		})
		require.NotNil(t, f.Images)
		require.Equal(t, ExtensionOn, *f.Images)
		require.NotNil(t, f.Quotes)
		require.Equal(t, ExtensionOff, *f.Quotes)
		require.Empty(t, f.Filter)
	})
	t.Run("ImagesWithQuotesWithRef", func(t *testing.T) {
		f := parseNostrFilter(model.Filter{
			Search: "images:true quoteS:off references:yes",
		})
		require.NotNil(t, f.Images)
		require.Equal(t, ExtensionOn, *f.Images)
		require.NotNil(t, f.Quotes)
		require.Equal(t, ExtensionOff, *f.Quotes)
		require.NotNil(t, f.References)
		require.Equal(t, ExtensionOn, *f.References)
		require.Empty(t, f.Filter)
	})
	t.Run("ImagesWithUnknownValue", func(t *testing.T) {
		f := parseNostrFilter(model.Filter{
			Search: "images:true quoteS:foo",
		})
		require.NotNil(t, f.Images)
		require.Equal(t, ExtensionOn, *f.Images)
		require.Nil(t, f.Quotes)
		require.Equal(t, "quoteS:foo", f.Filter.Search)
	})
	t.Run("ImagesWithQuotesWithRefWithContent", func(t *testing.T) {
		f := parseNostrFilter(model.Filter{
			Search: "images:true quoteS:off some content here references:yes",
		})
		require.NotNil(t, f.Images)
		require.Equal(t, ExtensionOn, *f.Images)
		require.NotNil(t, f.Quotes)
		require.Equal(t, ExtensionOff, *f.Quotes)
		require.NotNil(t, f.References)
		require.Equal(t, ExtensionOn, *f.References)
		require.Equal(t, "some content here", f.Filter.Search)
	})
}
