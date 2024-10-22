// SPDX-License-Identifier: ice License 1.0

package cfg

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMustGet(t *testing.T) {
	t.Parallel()
	type embeddedCfg1 struct {
		C1 string
	}
	type embeddedCfg2 struct {
		C2 string
	}
	type testCfg struct {
		embeddedCfg1
		Embedded2 embeddedCfg2 `yaml:",squash"`
		AA        string       `yaml:"a"`
	}
	result := MustGet[testCfg]()
	require.Equal(t, "b", result.AA)
	require.Equal(t, "cc1", result.C1)
	require.Equal(t, "cc2", result.Embedded2.C2)
}
