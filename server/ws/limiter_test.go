// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func TestApplySubscriptionLimit(t *testing.T) {
	t.Parallel()

	t.Run("returns error when no filters", func(t *testing.T) {
		s := &model.Subscription{
			Filters: []model.Filter{},
		}

		err := applySubscriptionLimit(s)
		require.ErrorIs(t, err, errNoFilter)
	})
	t.Run("applies default limit per filter when limit is zero or negative", func(t *testing.T) {
		s := &model.Subscription{
			Filters: []model.Filter{
				{Limit: 0},
				{Limit: 0},
				{Limit: -1},
			},
		}

		err := applySubscriptionLimit(s)
		require.NoError(t, err)
		require.Equal(t, 33, s.Filters[0].Limit)
		require.Equal(t, 33, s.Filters[1].Limit)
		require.Equal(t, 33, s.Filters[2].Limit)
	})

	t.Run("keeps existing positive limits", func(t *testing.T) {
		s := &model.Subscription{
			Filters: []model.Filter{
				{Limit: 30},
				{Limit: 40},
			},
		}

		err := applySubscriptionLimit(s)
		require.NoError(t, err)
		require.Equal(t, 30, s.Filters[0].Limit)
		require.Equal(t, 40, s.Filters[1].Limit)
	})

	t.Run("returns error when total limit exceeds maximum", func(t *testing.T) {
		s := &model.Subscription{
			Filters: []model.Filter{
				{Limit: 60},
				{Limit: 50},
			},
		}

		err := applySubscriptionLimit(s)
		require.ErrorIs(t, err, errLimitExceeded)
	})

	t.Run("handles single filter with default limit", func(t *testing.T) {
		s := &model.Subscription{
			Filters: []model.Filter{
				{Limit: 0},
			},
		}

		err := applySubscriptionLimit(s)
		require.NoError(t, err)
		require.Equal(t, 100, s.Filters[0].Limit)
	})

	t.Run("handles mixed positive and zero limits", func(t *testing.T) {
		s := &model.Subscription{
			Filters: []model.Filter{
				{Limit: 25},
				{Limit: 0},
				{Limit: 25},
			},
		}

		err := applySubscriptionLimit(s)
		require.NoError(t, err)
		require.Equal(t, 25, s.Filters[0].Limit)
		require.Equal(t, 50, s.Filters[1].Limit)
		require.Equal(t, 25, s.Filters[2].Limit)
	})

	t.Run("handles overflow", func(t *testing.T) {
		s := &model.Subscription{
			Filters: []model.Filter{
				{Limit: 25},
				{Limit: 0},
				{Limit: 70},
			},
		}

		err := applySubscriptionLimit(s)
		require.NoError(t, err)
		require.Equal(t, 25, s.Filters[0].Limit)
		require.Equal(t, 5, s.Filters[1].Limit)
		require.Equal(t, 70, s.Filters[2].Limit)
	})

	t.Run("fair share", func(t *testing.T) {
		s := &model.Subscription{
			Filters: []model.Filter{
				{Limit: 99},
				{Limit: 0},
			},
		}

		err := applySubscriptionLimit(s)
		require.NoError(t, err)
		require.Equal(t, 99, s.Filters[0].Limit)
		require.Equal(t, 1, s.Filters[1].Limit)
	})
}
