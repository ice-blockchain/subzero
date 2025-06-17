// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func TestApplySubscriptionLimit(t *testing.T) {
	t.Parallel()

	t.Run("applies default limit per filter when limit is zero or negative", func(t *testing.T) {
		s := &model.Subscription{
			Filters: []model.Filter{
				{Limit: 0},
				{Limit: 0},
				{Limit: -1},
			},
		}

		applySubscriptionLimit(s)
		require.Equal(t, 33, s.Filters[0].Limit)
		require.Equal(t, 33, s.Filters[1].Limit)
		require.Equal(t, 33, s.Filters[2].Limit)
	})
	t.Run("handles overflow", func(t *testing.T) {
		s := &model.Subscription{
			Filters: []model.Filter{
				{Limit: 25},
				{Limit: 5},
				{Limit: 70},
				{Limit: 0},
			},
		}

		applySubscriptionLimit(s)
		for i := range s.Filters {
			require.Equal(t, 25, s.Filters[i].Limit)
		}
	})
	t.Run("single filter with limit", func(t *testing.T) {
		s := &model.Subscription{
			Filters: []model.Filter{
				{Limit: 50},
			},
		}

		applySubscriptionLimit(s)
		require.Equal(t, defaultLimitREQ, s.Filters[0].Limit)
	})
	t.Run("no filters", func(t *testing.T) {
		s := &model.Subscription{}

		applySubscriptionLimit(s)
		require.Len(t, s.Filters, 1)
		require.Equal(t, defaultLimitREQ, s.Filters[0].Limit)
	})
}
