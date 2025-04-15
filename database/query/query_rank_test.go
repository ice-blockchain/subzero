// SPDX-License-Identifier: ice License 1.0

package query

import (
	"strconv"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func helperGetEventPointsAndScore(t *testing.T, db *dbClient, eventID string) (points int, score float64) {
	t.Helper()

	err := db.QueryRow("SELECT points, score FROM ranked_events WHERE event_id = $1", eventID).Scan(&points, &score)
	require.NoError(t, err)

	t.Logf("event %s: points=%d, score=%f", eventID, points, score)

	return points, score
}

func helperPointsScoreEqual(t *testing.T, db *dbClient, eventID string, points int, score float64) {
	t.Helper()

	p, s := helperGetEventPointsAndScore(t, db, eventID)
	require.EqualValues(t, points, p)
	require.InDelta(t, score, s, 0.3)
}

func TestEventScore(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	ts := time.Now().Add(time.Hour).Unix()

	var evNote, evArticle model.Event
	evNote.ID = "note"
	evNote.Kind = nostr.KindTextNote
	evNote.PubKey = "note_pub"
	evNote.CreatedAt = model.Timestamp(ts)
	evNote.Content = "note content"

	evArticle.ID = "article"
	evArticle.Kind = nostr.KindArticle
	evArticle.PubKey = "article_pub"
	evArticle.CreatedAt = model.Timestamp(ts)
	evArticle.Content = "article content"
	evArticle.Tags = model.Tags{
		{"d", "my article"},
	}
	require.NoError(t, db.AcceptEvents(t.Context(), &evNote, &evArticle))

	targetEvents := []struct {
		Tag     string
		ID      string
		Address string
		JSON    string
	}{
		{"a", evArticle.ID, evArticle.Address(), evArticle.String()},
		{"e", evNote.ID, evNote.Address(), evNote.String()},
	}
	t.Logf("target events: %v", targetEvents)

	t.Run("Like", func(t *testing.T) {
		for _, target := range targetEvents {
			var ev model.Event
			ev.Content = "+"
			ev.ID = "like_" + target.ID
			ev.PubKey = "like_pub_" + target.ID
			ev.Kind = nostr.KindReaction
			ev.CreatedAt = nostr.Now()
			ev.Tags = model.Tags{
				{target.Tag, target.Address},
			}
			require.NoError(t, db.AcceptEvents(t.Context(), &ev))
			helperPointsScoreEqual(t, db, target.ID, 1, 1.0)
		}
	})
	t.Run("Repost", func(t *testing.T) {
		for _, target := range targetEvents {
			var ev model.Event
			ev.Content = target.JSON
			ev.ID = "repost_" + target.ID
			ev.PubKey = "repost_pub_" + target.ID
			ev.Kind = nostr.KindRepost
			if target.Tag == "a" {
				ev.Kind = nostr.KindGenericRepost
			}
			ev.CreatedAt = nostr.Now()
			ev.Tags = model.Tags{
				{target.Tag, target.Address},
			}
			require.NoError(t, db.AcceptEvents(t.Context(), &ev))
			helperPointsScoreEqual(t, db, target.ID, 4, 4.0) // like (1) + repost (3).
		}
	})
	var quotes []string
	t.Run("Quote", func(t *testing.T) {
		for _, target := range targetEvents {
			var ev model.Event
			ev.Content = "quote"
			ev.ID = "quote" + target.ID
			ev.PubKey = "quote_pub"
			ev.Kind = nostr.KindTextNote
			if target.Tag == "a" {
				ev.Kind = model.CustomIONKindEditableTextNote
				ev.Tags = model.Tags{
					{model.CustomIONTagAddressableQ, target.Address},
					{"d", "quote"},
				}
			} else {
				ev.Tags = model.Tags{
					{"q", target.Address},
				}
			}
			ev.CreatedAt = nostr.Now()
			quotes = append(quotes, ev.ID)
			require.NoError(t, db.AcceptEvents(t.Context(), &ev))
			helperPointsScoreEqual(t, db, target.ID, 8, 8.0) // like (1) + repost (3) + quote (4).
		}
	})
	t.Run("Comment", func(t *testing.T) {
		var replies []*model.Event
		t.Run("Root", func(t *testing.T) {
			for _, target := range targetEvents {
				var ev model.Event
				ev.Content = "root comment"
				ev.ID = "comment_root_" + target.ID
				ev.PubKey = "comment_root_pub_" + target.ID
				ev.Kind = nostr.KindTextNote
				if target.Tag == "a" {
					ev.Kind = nostr.KindArticle
				}
				ev.CreatedAt = nostr.Now()
				ev.Tags = model.Tags{
					{target.Tag, target.Address, "", "root"},
					{target.Tag, target.Address, "", "reply"},
				}
				require.NoError(t, db.AcceptEvents(t.Context(), &ev))
				helperPointsScoreEqual(t, db, target.ID, 10, 10.0) // like (1) + repost (3) + quote (4) + root comment (2).
				replies = append(replies, &ev)
			}
		})
		t.Run("Reply", func(t *testing.T) {
			for i, target := range targetEvents {
				var ev model.Event
				ev.Content = "reply comment"
				ev.ID = "comment_reply_" + target.ID
				ev.PubKey = "comment_reply_pub_" + target.ID
				ev.Kind = nostr.KindTextNote
				if target.Tag == "a" {
					ev.Kind = nostr.KindArticle
				}
				ev.CreatedAt = nostr.Now()
				ev.Tags = model.Tags{
					{target.Tag, target.Address, "", "root"},
					{target.Tag, replies[i].Address(), "", "reply"},
				}
				require.NoError(t, db.AcceptEvents(t.Context(), &ev))
				// Should not affect the score of the target event.
				helperPointsScoreEqual(t, db, target.ID, 10, 10.0) // like (1) + repost (3) + quote (4) + root comment (2).
			}
		})
	})
	t.Run("Delete", func(t *testing.T) {
		t.Run("Quote", func(t *testing.T) {
			for _, id := range quotes {
				var ev model.Event
				ev.Kind = nostr.KindDeletion
				ev.ID = "delete_" + id
				ev.PubKey = "quote_pub"
				ev.Tags = model.Tags{
					{"e", id},
				}
				require.NoError(t, db.AcceptEvents(t.Context(), &ev))
			}
			for _, target := range targetEvents {
				helperPointsScoreEqual(t, db, target.ID, 6, 6.0) // like (1) + repost (3) + root comment (2).
			}
		})
		t.Run("Soft delete root comment", func(t *testing.T) {
			var ev model.Event
			ev.ID = "comment_root_delete" + evArticle.ID
			ev.PubKey = "comment_root_pub_" + evArticle.ID
			ev.Kind = nostr.KindArticle
			ev.CreatedAt = nostr.Now()
			ev.Tags = model.Tags{
				{"a", evArticle.Address(), "", "root"},
				{"a", evArticle.Address(), "", "reply"},
				{"published_at", strconv.FormatInt(int64(nostr.Now())-1, 10)},
			}
			require.NoError(t, db.AcceptEvents(t.Context(), &ev))
			helperPointsScoreEqual(t, db, evArticle.ID, 4, 4.0) // like (1) + repost (3).
			helperPointsScoreEqual(t, db, evNote.ID, 6, 6.0)    // like (1) + repost (3) + root comment (2).
		})
	})
}
