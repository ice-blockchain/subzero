// SPDX-License-Identifier: ice License 1.0

package query

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func TestEventTagsReorder(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		require.Nil(t, eventTagsReorder(nil))
		require.Equal(t, model.Tag{}, eventTagsReorder(model.Tag{}))
		require.Equal(t, model.Tag{"a"}, eventTagsReorder(model.Tag{"a"}))
	})
	t.Run("URL", func(t *testing.T) {
		tags := model.Tag{"imeta", "url http://example.com", "foo bar"}
		require.Equal(t, tags, eventTagsReorder(tags))

		tagsRaw := model.Tag{"imeta", "foo bar", "url http://example.com"}
		require.Equal(t, tags, eventTagsReorder(tagsRaw))
	})

	t.Run("URLAndMeta", func(t *testing.T) {
		var cases = []struct {
			Data     model.Tag
			Expected model.Tag
		}{
			{
				Data:     model.Tag{"imeta", "url http://example.com", "foo bar"},
				Expected: model.Tag{"imeta", "url http://example.com", "foo bar"},
			},
			{
				Data:     model.Tag{"imeta", "foo bar", "url http://example.com"},
				Expected: model.Tag{"imeta", "url http://example.com", "foo bar"},
			},
			{
				Data:     model.Tag{"foo bar", "url http://example.com"},
				Expected: model.Tag{"url http://example.com", "foo bar"},
			},
			{
				Data:     model.Tag{"imeta", "m video", "url http://example.com"},
				Expected: model.Tag{"imeta", "url http://example.com", "m video"},
			},
			{
				Data:     model.Tag{"imeta", "foo bar", "m video", "x y", "url http://example.com"},
				Expected: model.Tag{"imeta", "url http://example.com", "m video", "x y", "foo bar"},
			},
			{
				Data:     model.Tag{"imeta", "M video"},
				Expected: model.Tag{"imeta", "", "M video"},
			},
			{
				Data:     model.Tag{"M video"},
				Expected: model.Tag{"", "M video"},
			},
		}

		for idx, c := range cases {
			t.Logf("running test case %v: in=%#v, want=%#v", idx, c.Data, c.Expected)
			require.Equal(t, c.Expected, eventTagsReorder(c.Data), "test case %v: in=%#v, want=%#v", idx, c.Data, c.Expected)
		}
	})
}

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
		action, ts, kinds, err := parseAttestationString(c.In)
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
		Old     model.Tags
		New     model.Tags
		Allowed bool
	}{
		{
			Allowed: true,
		},
		{
			Old:     nil,
			New:     model.Tags{{model.IceTagAttestation, "pub", "", "foo:123"}},
			Allowed: true,
		},
		{
			Old:     nil,
			New:     model.Tags{{model.IceTagAttestation, "pub", "", "foo"}},
			Allowed: false,
		},
		{
			Old:     nil,
			New:     model.Tags{{model.IceTagAttestation, "pub", "", "foo:bar"}},
			Allowed: false,
		},
		{
			Old:     nil,
			New:     model.Tags{{model.IceTagAttestation, "pub", "foo:bar"}},
			Allowed: false,
		},
		{
			Old:     nil,
			New:     model.Tags{{model.IceTagAttestation, "pub", "", "foo:123:foo"}},
			Allowed: false,
		},
		{
			Old:     nil,
			New:     model.Tags{{model.IceTagAttestation, "pub", "", "foo:123:1,2,3"}},
			Allowed: true,
		},
		{
			Old:     nil,
			New:     model.Tags{{model.IceTagAttestation, "pub", "", "foo:123"}, {model.IceTagAttestation, "pub", "", "bar:123"}},
			Allowed: true,
		},
		{
			Old:     model.Tags{{model.IceTagAttestation, "pub", "", "foo:123"}},
			New:     model.Tags{{model.IceTagAttestation, "pub", "", "foo:123"}, {model.IceTagAttestation, "pub", "", "bar:123"}},
			Allowed: true,
		},
		{
			Old:     model.Tags{{model.IceTagAttestation, "pub", "", "foo:123"}},
			New:     model.Tags{{model.IceTagAttestation, "pub", "", "foo:1234"}, {model.IceTagAttestation, "pub", "", "bar:123"}},
			Allowed: false,
		},
		{
			Old:     model.Tags{{model.IceTagAttestation, "pub", "", "foo:123"}},
			New:     model.Tags{{model.IceTagAttestation, "pub", "", "foo:1234"}},
			Allowed: false,
		},
		{
			Old:     model.Tags{{model.IceTagAttestation, "pub", "", "foo:123"}, {model.IceTagAttestation, "pub", "", "bar:1234"}},
			New:     model.Tags{{model.IceTagAttestation, "pub", "", "foo:123"}},
			Allowed: false,
		},
		{
			Old:     model.Tags{{model.IceTagAttestation, "pub", "", "foo:123"}},
			New:     model.Tags{{model.IceTagAttestation, "pub", "", "foo:123"}, {"foo", "bar"}, {model.IceTagAttestation, "pub", "", "bar:123"}},
			Allowed: true,
		},
	}

	for idx, c := range cases {
		allowed := attestationUpdateIsAllowed(c.Old, c.New)
		require.Equalf(t, c.Allowed, allowed, "test case %v: in=%#v, want=%#v", idx, c, c.Allowed)
	}
}
