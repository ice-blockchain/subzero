// SPDX-License-Identifier: ice License 1.0

package cfg

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMustGet(t *testing.T) {
	t.Parallel()
	type testCfg struct{ A string }
	require.Equal(t, "b", MustGet[testCfg]().A)
}
