package query

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func TestIsFilterEmpty(t *testing.T) {
	t.Parallel()

	require.True(t, isFilterEmpty(&model.Filter{}))
	require.False(t, isFilterEmpty(&model.Filter{IDs: []string{"1"}}))
}

func TestWhereBuilderEmpty(t *testing.T) {
	t.Parallel()

	builder := newWhereBuilder()
	q, params := builder.Build()
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
		builder := newWhereBuilder()
		builder.Build(model.Filter{})
		q, params := builder.Build()
		require.Empty(t, params)
		require.Equal(t, whereBuilderDefaultWhere, q)
	})
	t.Run("WithID", func(t *testing.T) {
		builder := newWhereBuilder()
		builder.Build(model.Filter{IDs: []string{"123"}})
		q, params := builder.Build()
		t.Logf("stmt: %s (%+v)", q, params)
		require.Len(t, params, 1)
		helperEnsureParams(t, q, params)
	})
	t.Run("WithMoreIDs", func(t *testing.T) {
		builder := newWhereBuilder()
		builder.Build(model.Filter{IDs: []string{generateHexString(), "789"}})
		q, params := builder.Build()
		t.Logf("stmt: %s (%+v)", q, params)
		require.Len(t, params, 2)
		helperEnsureParams(t, q, params)
	})
	t.Run("WithKind", func(t *testing.T) {
		builder := newWhereBuilder()
		builder.Build(model.Filter{IDs: []string{generateHexString()}, Kinds: []int{1, generateKind()}})
		q, params := builder.Build()
		t.Logf("stmt: %s (%+v)", q, params)
		require.Len(t, params, 3)
		helperEnsureParams(t, q, params)
	})
	t.Run("WithAuthors", func(t *testing.T) {
		builder := newWhereBuilder()
		builder.Build(model.Filter{IDs: []string{generateHexString()}, Kinds: []int{1}, Authors: []string{"author1", "author2"}})
		q, params := builder.Build()
		t.Logf("stmt: %s (%+v)", q, params)
		require.Len(t, params, 4)
		helperEnsureParams(t, q, params)
	})
	t.Run("WithTimeRange", func(t *testing.T) {
		builder := newWhereBuilder()
		ts1 := model.Timestamp(generateCreatedAt())
		ts2 := model.Timestamp(generateCreatedAt())
		builder.Build(model.Filter{Since: &ts1, Until: &ts2})
		q, params := builder.Build()
		t.Logf("stmt: %s (%+v)", q, params)
		require.Len(t, params, 2)
		helperEnsureParams(t, q, params)
	})
}

func TestWhereBuilderSingleWithTags(t *testing.T) {
	t.Parallel()

	t.Run("OneTag", func(t *testing.T) {
		builder := newWhereBuilder()
		q, params := builder.Build(model.Filter{
			IDs: []string{"123"},
			Tags: map[string][]string{
				"#e": {"value1", "value2", "value3", "value4"},
			},
		})
		t.Logf("stmt: %s (%+v)", q, params)
		require.Len(t, params, 7)
		helperEnsureParams(t, q, params)
	})
	t.Run("TwoTags", func(t *testing.T) {
		builder := newWhereBuilder()
		q, params := builder.Build(model.Filter{
			IDs: []string{"123"},
			Tags: map[string][]string{
				"#e": {"value1", "value2", "value3", generateRandomString(4)},
				"#p": {"value1", "value2", "value3", "value4", generateRandomString(5)},
			},
		})
		t.Logf("stmt: %s (%+v)", q, params)
		require.Len(t, params, 12)
		helperEnsureParams(t, q, params)
	})
}

func TestWhereBuilderMulti(t *testing.T) {
	t.Parallel()

	ts1 := model.Timestamp(generateCreatedAt())
	ts2 := model.Timestamp(generateCreatedAt())
	filters := []model.Filter{
		{
			IDs:  []string{"123"},
			Tags: map[string][]string{"#e": {"value1", "value2", "value3", "value4"}},
		},
		{
			IDs:   []string{"456"},
			Tags:  map[string][]string{"#d": {"value1", "value2", "value3", "value4"}},
			Until: &ts2,
		},
		{
			Authors: []string{"author1", "author2"},
			Since:   &ts1,
		},
	}

	builder := newWhereBuilder()
	q, params := builder.Build(filters...)
	t.Logf("stmt: %s (%+v)", q, params)
	require.Len(t, params, 18)
	helperEnsureParams(t, q, params)
}

func TestWhereBuilderMultiTagsOnly(t *testing.T) {
	t.Parallel()

	filters := []model.Filter{
		{
			Tags: map[string][]string{"#e": {"value1", generateRandomString(3), "value3", "value4"}},
		},
		{
			Tags: map[string][]string{"#d": {"value1", "value2", generateRandomString(4), generateRandomString(4)}},
		},
	}

	builder := newWhereBuilder()
	q, params := builder.Build(filters...)
	t.Logf("stmt: %s (%+v)", q, params)
	require.Len(t, params, 10)
	helperEnsureParams(t, q, params)
}

func TestWhereBuilderSameElements(t *testing.T) {
	t.Parallel()

	filters := []model.Filter{
		{
			IDs:     []string{"123", "456", "123"},
			Authors: []string{"111", "222", "222"},
		},
	}

	builder := newWhereBuilder()
	q, params := builder.Build(filters...)
	t.Logf("stmt: %s (%+v)", q, params)
	t.Logf("params: %+v", params)
	require.Len(t, params, 4)
	helperEnsureParams(t, q, params)
}
