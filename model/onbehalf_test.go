// SPDX-License-Identifier: ice License 1.0

package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseAttestationString(t *testing.T) {
	t.Parallel()

	cases := []struct {
		In     string
		Action string
		Ts     Timestamp
		Kinds  []int
		Err    bool
	}{
		{
			In:  "action",
			Err: true,
		},
		{
			In:     "action:123",
			Action: "action",
			Ts:     123,
		},
		{
			In:  "action:foo",
			Err: true,
		},
		{
			In:     "action:123:1,2,3",
			Action: "action",
			Ts:     123,
			Kinds:  []int{1, 2, 3},
		},
		{
			In:     "action:1749219680077637000:1,2,3",
			Action: "action",
			Ts:     1749219680077637000,
			Kinds:  []int{1, 2, 3},
		},
		{
			In:  "action:123:1,foo,3",
			Err: true,
		},
	}

	for i, c := range cases {
		t.Logf("case: %v = %v", i, c.In)
		action, ts, kinds, err := ParseAttestationString(c.In)
		if c.Err {
			require.Error(t, err)
			continue
		}

		require.NoError(t, err)
		require.Equal(t, c.Action, action)
		require.Equal(t, c.Ts, ts)
		require.Equal(t, c.Kinds, kinds)
	}
}
