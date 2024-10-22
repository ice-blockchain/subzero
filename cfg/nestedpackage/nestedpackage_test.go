// SPDX-License-Identifier: ice License 1.0

package nestedpackage

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/cfg"
)

func TestMustGet(t *testing.T) {
	t.Parallel()
	type testCfg struct {
		AA string `yaml:"xx"`
	}
	require.Equal(t, "yy", cfg.MustGet[testCfg]().AA)
}
