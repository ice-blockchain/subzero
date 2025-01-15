// SPDX-License-Identifier: ice License 1.0

package query

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func TestIsFilterEmpty(t *testing.T) {
	t.Parallel()

	require.True(t, isFilterEmpty(&databaseFilterSearch{}))

	f := model.Filter{
		IDs: []string{"123"},
	}

	dbFilter, err := parseNostrFilter(f)
	require.NoError(t, err)
	require.False(t, isFilterEmpty(dbFilter))
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
		q, params, err := newWhereBuilder().Build(model.Filter{
			IDs: []string{"123"},
		})
		require.NoError(t, err)
		t.Logf("stmt: %s (%+v)", q, params)
		require.Len(t, params, 1)
		helperEnsureParams(t, q, params)
	})
	t.Run("WithMoreIDs", func(t *testing.T) {
		q, params, err := newWhereBuilder().Build(model.Filter{
			IDs: []string{generateHexString(), "789"},
		})
		require.NoError(t, err)
		t.Logf("stmt: %s (%+v)", q, params)
		require.Len(t, params, 2)
		helperEnsureParams(t, q, params)
	})
	t.Run("WithKind", func(t *testing.T) {
		q, params, err := newWhereBuilder().Build(model.Filter{
			IDs:   []string{generateHexString()},
			Kinds: []int{1, 2},
		})
		require.NoError(t, err)
		t.Logf("stmt: %s (%+v)", q, params)
		require.Len(t, params, 3)
		helperEnsureParams(t, q, params)
	})
	t.Run("WithAuthors", func(t *testing.T) {
		q, params, err := newWhereBuilder().Build(model.Filter{
			IDs:     []string{generateHexString()},
			Kinds:   []int{1},
			Authors: []string{"author1", "author2"},
		})
		require.NoError(t, err)
		t.Logf("stmt: %s (%+v)", q, params)
		require.Len(t, params, 4)
		helperEnsureParams(t, q, params)
	})
	t.Run("WithTimeRange", func(t *testing.T) {
		ts1 := model.Timestamp(generateCreatedAt())
		ts2 := model.Timestamp(generateCreatedAt())
		filter := model.Filter{
			Since: &ts1,
			Until: &ts2,
		}
		helperBenchEnsureValidRange(t, &filter)
		q, params, err := newWhereBuilder().Build(filter)
		require.NoError(t, err)
		t.Logf("stmt: %s (%+v)", q, params)
		require.Len(t, params, 2)
		helperEnsureParams(t, q, params)
	})
	t.Run("WithTimestamp", func(t *testing.T) {
		ts1 := model.Timestamp(generateCreatedAt())
		q, params, err := newWhereBuilder().Build(model.Filter{
			Since: &ts1,
			Until: &ts1,
		})
		require.NoError(t, err)
		t.Logf("stmt: %s (%+v)", q, params)
		require.Len(t, params, 1)
		helperEnsureParams(t, q, params)
	})
	t.Run("WithInvalidTimeRange", func(t *testing.T) {
		ts1 := model.Timestamp(1)
		ts2 := model.Timestamp(2)
		q, params, err := newWhereBuilder().Build(model.Filter{
			Since: &ts2,
			Until: &ts1,
		})
		require.ErrorIs(t, err, ErrWhereBuilderInvalidTimeRange)
		require.Empty(t, q)
		require.Len(t, params, 0)
	})
}

func TestWhereBuilderSingleWithTags(t *testing.T) {
	t.Parallel()

	t.Run("OneTag", func(t *testing.T) {
		q, params, err := newWhereBuilder().Build(model.Filter{
			IDs: []string{"123"},
			Tags: model.TagMap{}.
				SetLiterals("e", "value1", "value2", "value3", "value4"),
		})
		t.Logf("stmt: %s (%+v)", q, params)
		require.NoError(t, err)
		require.Len(t, params, 6)
		helperEnsureParams(t, q, params)
	})
	t.Run("TwoTagsShink", func(t *testing.T) {
		var valuesMax []string

		for range 30 {
			valuesMax = append(valuesMax, generateRandomString(4))
		}

		q, params, err := newWhereBuilder().Build(model.Filter{
			IDs: []string{"123"},
			Tags: model.TagMap{}.
				SetLiterals("e", "value1", "value2", "value3", generateRandomString(4)).
				SetLiterals("p", valuesMax...),
		})
		require.NoError(t, err)
		t.Logf("stmt: %s (%+v)", q, params)
		require.Len(t, params, 28)
		helperEnsureParams(t, q, params)
	})
}

func TestWhereBuilderMulti(t *testing.T) {
	t.Parallel()

	ts1 := model.Timestamp(generateCreatedAt())
	ts2 := model.Timestamp(generateCreatedAt())
	filters := model.Filters{
		{
			IDs: []string{"123"},
			Tags: model.TagMap{}.
				SetLiterals("e", "value1", "value2", "value3", "value4"),
		},
		{
			IDs: []string{"456"},
			Tags: model.TagMap{}.
				SetLiterals("d", "value1", "value2", "value3", "value4"),
			Until: &ts2,
		},
		{
			Authors: []string{"author1", "author2"},
			Since:   &ts1,
		},
	}

	builder := newWhereBuilder()
	q, params, err := builder.Build(filters...)
	require.NoError(t, err)
	t.Logf("stmt: %s (%+v)", q, params)
	require.Len(t, params, 16)
	helperEnsureParams(t, q, params)
}

func TestWhereBuilderMultiTagsOnly(t *testing.T) {
	t.Parallel()

	filters := model.Filters{
		model.Filter{
			Tags: model.TagMap{}.
				SetLiterals("e", "value1", generateRandomString(3), "value3", "value4"),
		},
		model.Filter{
			Tags: model.TagMap{}.
				SetLiterals("d", "value1", "value2", generateRandomString(4), generateRandomString(4)),
		},
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

	filter := model.Filter{
		IDs:     []string{"123", "456", "123"},
		Authors: []string{"111", "222", "222"},
	}

	builder := newWhereBuilder()
	q, params, err := builder.Build(filter)
	require.NoError(t, err)
	t.Logf("stmt: %s (%+v)", q, params)
	t.Logf("params: %+v", params)
	require.Len(t, params, 4)
	helperEnsureParams(t, q, params)
}

func TestWhereBuilderMimeType(t *testing.T) {
	t.Parallel()

	builder := newWhereBuilder()
	q, params, err := builder.Build(model.Filter{
		Search: "images:true videos:false",
	})
	require.NoError(t, err)
	t.Logf("stmt: %s (%+v)", q, params)
	t.Logf("params: %+v", params)
	require.Len(t, params, 0)
}

func TestParseNostrFilter(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		f, err := parseNostrFilter(model.Filter{})
		require.NoError(t, err)
		require.Empty(t, f.Filter)
		require.Nil(t, f.Quotes)
		require.Nil(t, f.Images)
	})
	t.Run("Images", func(t *testing.T) {
		f, err := parseNostrFilter(model.Filter{
			Search: "images:true",
		})
		require.NoError(t, err)
		require.Empty(t, f.Filter)
		require.NotNil(t, f.Images)
		require.True(t, *f.Images)
	})
	t.Run("ImagesWithQuotes", func(t *testing.T) {
		f, err := parseNostrFilter(model.Filter{
			Search: "images:true quoteS:off",
		})
		require.NoError(t, err)
		require.NotNil(t, f.Images)
		require.True(t, *f.Images)
		require.NotNil(t, f.Quotes)
		require.False(t, *f.Quotes)
		require.Empty(t, f.Filter)
	})
	t.Run("ImagesWithQuotesWithRef", func(t *testing.T) {
		f, err := parseNostrFilter(model.Filter{
			Search: "images:true quoteS:off references:yes",
		})
		require.NoError(t, err)
		require.NotNil(t, f.Images)
		require.True(t, *f.Images)
		require.NotNil(t, f.Quotes)
		require.False(t, *f.Quotes)
		require.NotNil(t, f.References)
		require.True(t, *f.References)
		require.Empty(t, f.Filter)
	})
	t.Run("ImagesWithUnknownValue", func(t *testing.T) {
		f, err := parseNostrFilter(model.Filter{
			Search: "images:true quoteS:foo",
		})
		require.NoError(t, err)
		require.NotNil(t, f.Images)
		require.True(t, *f.Images)
		require.Nil(t, f.Quotes)
		require.Equal(t, "quoteS:foo", f.Filter.Search)
	})
	t.Run("ImagesWithQuotesWithRefWithContent", func(t *testing.T) {
		f, err := parseNostrFilter(model.Filter{
			Search: "images:true quoteS:off some content here references:yes",
		})
		require.NoError(t, err)
		require.NotNil(t, f.Images)
		require.True(t, *f.Images)
		require.NotNil(t, f.Quotes)
		require.False(t, *f.Quotes)
		require.NotNil(t, f.References)
		require.True(t, *f.References)
		require.Equal(t, "some content here", f.Filter.Search)
	})
	t.Run("Image with dependencies", func(t *testing.T) {
		f, err := parseNostrFilter(model.Filter{
			Search: "images:true some content here include:dependencies:kind1>kind2",
		})
		require.NoError(t, err)
		require.NotNil(t, f.Images)
		require.True(t, *f.Images)
		require.Len(t, f.Dependencies, 1)
		require.Equal(t, &filterDependencies{
			Start: filterDependenciesStart{
				Kind: 1,
			},
			Reduce: filterDependenciesReduce{
				Kinds: []int{2},
			},
		}, f.Dependencies[0])
		require.Equal(t, "some content here", f.Filter.Search)
	})
	t.Run("Image with dependencies in the beginning", func(t *testing.T) {
		f, err := parseNostrFilter(model.Filter{
			Search: "include:dependencies:kind1>kind3 some content here2 images:false",
		})
		require.NoError(t, err)
		require.NotNil(t, f.Images)
		require.False(t, *f.Images)
		require.Len(t, f.Dependencies, 1)
		require.Equal(t, &filterDependencies{
			Start: filterDependenciesStart{
				Kind: 1,
			},
			Reduce: filterDependenciesReduce{
				Kinds: []int{3},
			},
		}, f.Dependencies[0])
		require.Equal(t, "some content here2", f.Filter.Search)
	})
	t.Run("E marker with reply and images", func(t *testing.T) {
		f, err := parseNostrFilter(model.Filter{
			Search: "images:false some content here emarker:reply",
		})
		require.NoError(t, err)
		require.NotNil(t, f.Images)
		require.False(t, *f.Images)
		require.Len(t, f.TagMarkers, 1)
		require.Equal(t, "e", f.TagMarkers[0].Tag)
		require.Equal(t, "reply", f.TagMarkers[0].Marker)
		require.Equal(t, "some content here", f.Filter.Search)
	})
	t.Run("Three markers with videos", func(t *testing.T) {
		f, err := parseNostrFilter(model.Filter{
			Search: "amarker:aval videos:true some bmarker:bval content here cmarker:cval marker:invalid x",
		})
		require.NoError(t, err)
		require.NotNil(t, f.Videos)
		require.True(t, *f.Videos)
		require.Equal(t, "some content here marker:invalid x", f.Filter.Search)
		require.Len(t, f.TagMarkers, 3)
		require.Equal(t, []databaseFilterMarker{{Tag: "a", Marker: "aval"}, {Tag: "b", Marker: "bval"}, {Tag: "c", Marker: "cval"}}, f.TagMarkers)
	})
	t.Run("One negative marker and one positive and images", func(t *testing.T) {
		f, err := parseNostrFilter(model.Filter{
			Search: "amarker:aval images:true some !bmarker:bval content here marker:invalid x",
		})
		require.NoError(t, err)
		require.NotNil(t, f.Images)
		require.True(t, *f.Images)
		require.Equal(t, "some content here marker:invalid x", f.Filter.Search)
		require.Len(t, f.TagMarkers, 2)
		require.Equal(t, []databaseFilterMarker{{Tag: "a", Marker: "aval"}, {Tag: "b", Marker: "bval", Exclude: true}}, f.TagMarkers)
	})
}

func applyDeleteFilter(t *testing.T) {
	t.Parallel()

	t.Run("Simple", func(t *testing.T) {
		filter := databaseFilterDelete{
			Author: "author1",
			IDs:    []string{"123", "456"},
		}
		stmt, param, err := newWhereBuilder().BuildForDelete(filter)
		require.NoError(t, err)
		t.Logf("stmt: %s (%+v)", stmt, param)
		require.Len(t, param, 3)
	})
	t.Run("Complex", func(t *testing.T) {
		filter := databaseFilterDelete{
			Author: "author1",
			IDs:    []string{"123", "456"},
			Events: []databaseEventAddress{
				{Kind: 13, Pubkey: "author2", Dtag: "value1"},
			},
		}
		stmt, param, err := newWhereBuilder().BuildForDelete(filter)
		require.NoError(t, err)
		t.Logf("stmt: %s (%+v)", stmt, param)
		require.Len(t, param, 6)
	})
	t.Run("TwoSimple", func(t *testing.T) {
		filters := []databaseFilterDelete{
			{Author: "author1", IDs: []string{"123", "456"}},
			{Author: "author2", IDs: []string{"789"}},
		}

		stmt, param, err := newWhereBuilder().BuildForDelete(filters...)
		require.NoError(t, err)
		t.Logf("stmt: %s (%+v)", stmt, param)
		require.Len(t, param, 5)
	})
	t.Run("OnlyOwner", func(t *testing.T) {
		filter := databaseFilterDelete{
			Author: "author1",
		}

		stmt, param, err := newWhereBuilder().BuildForDelete(filter)
		require.NoError(t, err)
		t.Logf("stmt: %s (%+v)", stmt, param)
		require.Len(t, param, 1)
	})
	t.Run("Empty", func(t *testing.T) {
		_, _, err := newWhereBuilder().BuildForDelete()
		require.ErrorIs(t, err, ErrEmptyFilter)
	})
}
