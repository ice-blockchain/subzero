package query

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func TestPrepareTag(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		require.Nil(t, prepareTag(nil))
		require.Equal(t, model.Tag{}, prepareTag(model.Tag{}))
		require.Equal(t, model.Tag{"a"}, prepareTag(model.Tag{"a"}))
	})
	t.Run("URL", func(t *testing.T) {
		tags := model.Tag{"imeta", "url http://example.com", "foo bar"}
		require.Equal(t, tags, prepareTag(tags))

		tagsRaw := model.Tag{"imeta", "foo bar", "url http://example.com"}
		require.Equal(t, tags, prepareTag(tagsRaw))
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
			require.Equal(t, c.Expected, prepareTag(c.Data), "test case %v: in=%#v, want=%#v", idx, c.Data, c.Expected)
		}
	})
}