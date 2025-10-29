// SPDX-License-Identifier: ice License 1.0

package connector_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/appcontext"
	"github.com/ice-blockchain/subzero/database/query/internal/connector"
	"github.com/ice-blockchain/subzero/database/query/internal/postgres/fixture"
)

func TestStorageMasterSwitch(t *testing.T) {
	t.Parallel()

	master1 := fixture.New(t.Context())
	defer master1.Close(t.Context())

	master2 := fixture.New(t.Context())
	defer master2.Close(t.Context())

	master1Addr, _ := master1.MustTempDB(t.Context(), "master1")
	master2Addr, _ := master2.MustTempDB(t.Context(), "master2")

	conn, err := connector.New(appcontext.TestContext(t),
		connector.WithWriteURLs(master1Addr, master2Addr),
	)
	require.NoError(t, err)
	require.NotNil(t, conn)

	ptr, err := connector.ExecOne[string](t.Context(), conn, `select current_database()`)
	require.NoError(t, err)
	require.NotEmpty(t, ptr)
	require.Equal(t, "master1", *ptr)

	// Close the first master to trigger a switch.
	require.NoError(t, master1.Close(t.Context()))

	ptr, err = connector.ExecOne[string](t.Context(), conn, `select current_database()`)
	require.NoError(t, err)
	require.NotEmpty(t, ptr)
	require.Equal(t, "master2", *ptr)

	require.NoError(t, conn.Close())
}
func TestCalculateConnectOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		items        []string
		currentIndex int
		expected     []int
	}{
		{
			name:         "single item",
			items:        []string{"item1"},
			currentIndex: 0,
			expected:     []int{},
		},
		{
			name:         "two items, start from index 0",
			items:        []string{"item1", "item2"},
			currentIndex: 0,
			expected:     []int{1},
		},
		{
			name:         "two items, start from index 1",
			items:        []string{"item1", "item2"},
			currentIndex: 1,
			expected:     []int{0},
		},
		{
			name:         "three items, start from index 0",
			items:        []string{"item1", "item2", "item3"},
			currentIndex: 0,
			expected:     []int{1, 2},
		},
		{
			name:         "three items, start from index 1",
			items:        []string{"item1", "item2", "item3"},
			currentIndex: 1,
			expected:     []int{2, 0},
		},
		{
			name:         "three items, start from index 2",
			items:        []string{"item1", "item2", "item3"},
			currentIndex: 2,
			expected:     []int{0, 1},
		},
		{
			name:         "five items, start from middle",
			items:        []string{"item1", "item2", "item3", "item4", "item5"},
			currentIndex: 2,
			expected:     []int{3, 4, 0, 1},
		},
		{
			name:         "five items, start from last",
			items:        []string{"item1", "item2", "item3", "item4", "item5"},
			currentIndex: 4,
			expected:     []int{0, 1, 2, 3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := connector.CalculateConnectOrder(tt.items, tt.currentIndex)
			require.Equal(t, tt.expected, result)
		})
	}
}
