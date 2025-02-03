// SPDX-License-Identifier: ice License 1.0

package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUserDataContext_IsFiltersAllowed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		kinds   map[int]struct{}
		filters Filters
		want    bool
	}{
		{
			name:    "empty kinds allows all filters",
			kinds:   map[int]struct{}{},
			filters: Filters{{Kinds: []int{1, 2, 3}}},
			want:    true,
		},
		{
			name: "matching kinds allows filters",
			kinds: map[int]struct{}{
				1: {},
				2: {},
				3: {},
			},
			filters: Filters{{Kinds: []int{1, 2}}},
			want:    true,
		},
		{
			name: "non-matching kinds denies filters",
			kinds: map[int]struct{}{
				1: {},
				2: {},
			},
			filters: Filters{{Kinds: []int{1, 3}}},
			want:    false,
		},
		{
			name: "multiple filters all need to match",
			kinds: map[int]struct{}{
				1: {},
				2: {},
			},
			filters: Filters{
				{Kinds: []int{1}},
				{Kinds: []int{2}},
				{Kinds: []int{3}},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := UserDataContext{
				Kinds: tt.kinds,
			}
			require.Equal(t, tt.want, u.IsFilterAllowed(tt.filters...))
		})
	}
}
