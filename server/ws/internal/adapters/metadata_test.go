// SPDX-License-Identifier: ice License 1.0

package adapters

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMetadatahandler(t *testing.T) {
	t.Parallel()

	var meta MetadataHander

	for i, v := range []string{"foo", "bar", "baz"} {
		meta.Set(v, i)

		got, loaded := meta.Get(v)
		require.True(t, loaded)
		require.Equal(t, i, got)
	}

	var total int
	for range meta.Range() {
		total++
	}
	require.Equal(t, 3, total)
}
