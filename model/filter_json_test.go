// SPDX-License-Identifier: ice License 1.0

package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFilterTagsJSON(t *testing.T) {
	t.Parallel()

	var cases = []struct {
		In  *Filter
		Out string
	}{
		{
			In:  &Filter{},
			Out: `{}`,
		},
		{
			In: &Filter{
				Tags: TagMap{
					"foo": {},
				},
			},
			Out: `{"#foo":[]}`,
		},
		{
			In: &Filter{
				Tags: TagMap{
					"foo": {{nil, PointerOf("bar")}},
				},
			},
			Out: `{"#foo":[[null, "bar"]]}`,
		},
		{
			In: &Filter{
				Tags: TagMap{
					"foo": {{nil, PointerOf("bar")}, {PointerOf("baz"), nil, PointerOf("qux")}},
				},
			},
			Out: `{"#foo":[[null, "bar"], ["baz", null, "qux"]]}`,
		},
		{
			In: &Filter{
				Tags: TagMap{
					"foo": {{nil, PointerOf("bar")}, {PointerOf("baz"), nil, PointerOf("qux")}},
					"bar": {{}, {PointerOf("baz")}},
				},
			},
			Out: `{"#bar":[[],["baz"]],"#foo":[[null, "bar"], ["baz", null, "qux"]]}`,
		},
	}

	t.Run("Encode", func(t *testing.T) {
		for idx, c := range cases {
			data, err := json.Marshal(c.In)
			require.NoError(t, err, "case %d", idx)
			require.JSONEq(t, c.Out, string(data), "case %d", idx)
		}
	})
	t.Run("Decode", func(t *testing.T) {
		for idx, c := range cases {
			var f Filter
			err := json.Unmarshal([]byte(c.Out), &f)
			require.NoError(t, err, "case %d", idx)
			require.Equal(t, c.In, &f, "case %d", idx)
		}
		t.Run("Compatibility", func(t *testing.T) {
			var compCases = []struct {
				In  string
				Out *Filter
			}{
				{
					In: `{"#foo":["bar"]}`,
					Out: &Filter{
						Tags: TagMap{
							"foo": {{PointerOf("bar")}},
						},
					},
				},
				{
					In: `{"#foo":["bar", "baz"]}`,
					Out: &Filter{
						Tags: TagMap{
							"foo": {{PointerOf("bar")}, {PointerOf("baz")}},
						},
					},
				},
				{
					In: `{"#foo":["bar", "baz"], "#bar":["baz"]}`,
					Out: &Filter{
						Tags: TagMap{
							"foo": {{PointerOf("bar")}, {PointerOf("baz")}},
							"bar": {{PointerOf("baz")}},
						},
					},
				},
			}
			for idx, c := range compCases {
				var f Filter
				err := json.Unmarshal([]byte(c.In), &f)
				require.NoError(t, err, "case %d", idx)
				require.Equal(t, c.Out, &f, "case %d", idx)
			}
		})
	})
}
