package query

import (
	"context"
	"slices"
	"testing"

	"github.com/nbd-wtf/go-nostr"
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

func TestSaveEventTagsRedone(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	t.Run("Regular", func(t *testing.T) {
		var event model.Event

		tags := model.Tags{{"imeta", "m video"}}
		event.Kind = nostr.KindTextNote
		event.ID = generateHexString()
		event.PubKey = "1"
		event.Tags = slices.Clone(tags)
		event.CreatedAt = 1

		err := db.SaveEvent(context.TODO(), &event)
		require.NoError(t, err)

		t.Run("CheckSelect", func(t *testing.T) {
			var event2 model.Event
			for ev, err := range db.SelectEvents(context.TODO(), helperNewFilterSubscription(func(apply *model.Filter) {
				apply.IDs = []string{event.ID}
			})) {
				require.NoError(t, err)
				event2 = *ev

				break
			}

			event.Tags = slices.Clone(tags)
			require.Equal(t, event, event2)
		})

		t.Run("CheckTags", func(t *testing.T) {
			var mime string

			err := db.QueryRow("SELECT "+tagValueMimeType+" FROM event_tags WHERE event_id = $1", event.ID).Scan(&mime)
			require.NoError(t, err)
			require.Equal(t, "m video", mime)
		})
	})

	t.Run("Repost", func(t *testing.T) {
		var event model.Event

		event.Kind = nostr.KindRepost
		event.ID = generateHexString()
		event.PubKey = "2"
		event.Tags = model.Tags{{"e", "2"}}
		event.CreatedAt = 2
		event.Content = `{"id":"3","pubkey":"4","created_at":1712594952,"kind":1,"tags":[["imeta","url https://example.com/foo.jpg","ox f63ccef25fcd9b9a181ad465ae40d282eeadd8a4f5c752434423cb0539f73e69 https://nostr.build","x f9c8b660532a6e8236779283950d875fbfbdc6f4dbc7c675bc589a7180299c30","m image/jpeg","dim 1066x1600","bh L78C~=$%0%ERjENbWX$g0jNI}:-S","blurhash L78C~=$%0%ERjENbWX$g0jNI}:-S"]],"content":"foo","sig":"sig"}`

		err := db.saveRepost(context.TODO(), &event)
		require.NoError(t, err)

		t.Run("CheckTags", func(t *testing.T) {
			var (
				mime string
				url  string
			)

			err := db.QueryRow("SELECT "+tagValueURL+", "+tagValueMimeType+" FROM event_tags WHERE event_id = $1", "3").Scan(&url, &mime)
			require.NoError(t, err)
			require.Equal(t, "m image/jpeg", mime)
			require.Equal(t, "url https://example.com/foo.jpg", url)
		})
	})
}
