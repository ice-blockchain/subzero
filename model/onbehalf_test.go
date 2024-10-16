// SPDX-License-Identifier: ice License 1.0

package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseAttestationString(t *testing.T) {
	t.Parallel()

	cases := []struct {
		In     string
		Action string
		Ts     time.Time
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
			Ts:     time.Unix(123, 0),
		},
		{
			In:  "action:foo",
			Err: true,
		},
		{
			In:     "action:123:1,2,3",
			Action: "action",
			Ts:     time.Unix(123, 0),
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

func TestAttestationUpdateIsAllowed(t *testing.T) {
	t.Parallel()

	var cases = []struct {
		Old     Tags
		New     Tags
		Allowed bool
	}{
		{
			Allowed: true,
		},
		{
			Old:     nil,
			New:     Tags{{TagAttestationName, "pub", "", "foo:123"}},
			Allowed: true,
		},
		{
			Old:     nil,
			New:     Tags{{TagAttestationName, "pub", "", "foo"}},
			Allowed: false,
		},
		{
			Old:     nil,
			New:     Tags{{TagAttestationName, "pub", "", "foo:bar"}},
			Allowed: false,
		},
		{
			Old:     nil,
			New:     Tags{{TagAttestationName, "pub", "foo:bar"}},
			Allowed: false,
		},
		{
			Old:     nil,
			New:     Tags{{TagAttestationName, "pub", "", "foo:123:foo"}},
			Allowed: false,
		},
		{
			Old:     nil,
			New:     Tags{{TagAttestationName, "pub", "", "foo:123:1,2,3"}},
			Allowed: true,
		},
		{
			Old:     nil,
			New:     Tags{{TagAttestationName, "pub", "", "foo:123"}, {TagAttestationName, "pub", "", "bar:123"}},
			Allowed: true,
		},
		{
			Old:     Tags{{TagAttestationName, "pub", "", "foo:123"}},
			New:     Tags{{TagAttestationName, "pub", "", "foo:123"}, {TagAttestationName, "pub", "", "bar:123"}},
			Allowed: true,
		},
		{
			Old:     Tags{{TagAttestationName, "pub", "", "foo:123"}},
			New:     Tags{{TagAttestationName, "pub", "", "foo:1234"}, {TagAttestationName, "pub", "", "bar:123"}},
			Allowed: false,
		},
		{
			Old:     Tags{{TagAttestationName, "pub", "", "foo:123"}},
			New:     Tags{{TagAttestationName, "pub", "", "foo:1234"}},
			Allowed: false,
		},
		{
			Old:     Tags{{TagAttestationName, "pub", "", "foo:123"}, {TagAttestationName, "pub", "", "bar:1234"}},
			New:     Tags{{TagAttestationName, "pub", "", "foo:123"}},
			Allowed: false,
		},
		{
			Old:     Tags{{TagAttestationName, "pub", "", "foo:123"}},
			New:     Tags{{TagAttestationName, "pub", "", "foo:123"}, {"foo", "bar"}, {TagAttestationName, "pub", "", "bar:123"}},
			Allowed: true,
		},
		{
			Old:     Tags{{TagAttestationName, "pub", "", "foo:123"}, {TagAttestationName, "pub2", "", IceAttestationKindRevoked + ":123"}},
			New:     Tags{{TagAttestationName, "pub", "", "foo:123"}, {TagAttestationName, "pub2", "", IceAttestationKindRevoked + ":123"}, {TagAttestationName, "pub2", "", IceAttestationKindActive + ":123"}},
			Allowed: false,
		},
		{
			Old:     Tags{{TagAttestationName, "pub", "", "foo:123"}},
			New:     Tags{{TagAttestationName, "pub", "", "foo:123"}, {TagAttestationName, "pub2", "", IceAttestationKindRevoked + ":123"}, {TagAttestationName, "pub2", "", IceAttestationKindActive + ":123"}},
			Allowed: false,
		},
	}

	for idx, c := range cases {
		allowed := AttestationUpdateIsAllowed(c.Old, c.New)
		require.Equalf(t, c.Allowed, allowed, "test case %v: in=%#v, want=%#v", idx, c, c.Allowed)
	}
}
