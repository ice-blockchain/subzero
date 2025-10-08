// SPDX-License-Identifier: ice License 1.0

package query

import (
	"strconv"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/database/query/internal/connector"
	"github.com/ice-blockchain/subzero/model"
)

func helperGetEventPointsAndScore(t *testing.T, client *dbClient, eventID string) (points int, score int) {
	t.Helper()

	data, err := connector.Get[struct {
		Points int `db:"points"`
		Score  int `db:"score"`
	}](t.Context(), client.db, "SELECT points, score FROM ranked_events WHERE event_id = $1", eventID)
	require.NoError(t, err)
	require.NotNil(t, data)

	points, score = data.Points, data.Score

	t.Logf("event %s: points=%d, score=%d", eventID, points, score)

	return points, score
}

func helperSetEventPointsAndScore(t *testing.T, client *dbClient, eventID string, points, score, created_at int, verified bool) {
	t.Helper()

	const stmt = `
INSERT INTO ranked_events (event_id, event_kind, event_created_at, points, score, event_verified)
VALUES ($1, 0, $2, $3, $4, $5)
ON CONFLICT (event_id) DO UPDATE SET points = EXCLUDED.points, score = EXCLUDED.score, event_verified = EXCLUDED.event_verified;
`
	_, err := connector.Exec(t.Context(), client.db, stmt, eventID, created_at, points, score, verified)
	require.NoError(t, err)
}

func helperPointsScoreEqual(t *testing.T, db *dbClient, eventID string, points int, score float64) {
	t.Helper()

	p, s := helperGetEventPointsAndScore(t, db, eventID)
	require.EqualValues(t, points, p)
	require.InDelta(t, score, s, 300)
}

func TestEventScore(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	ts := nostr.Now().Add(time.Hour)

	var evNote, evArticle model.Event
	evNote.ID = "note"
	evNote.Kind = nostr.KindTextNote
	evNote.PubKey = "note_pub"
	evNote.CreatedAt = ts
	evNote.Content = "note content"

	evArticle.ID = "article"
	evArticle.Kind = nostr.KindArticle
	evArticle.PubKey = "article_pub"
	evArticle.CreatedAt = ts
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
			helperPointsScoreEqual(t, db, target.ID, 1, 1e4)
		}
	})
	var reposts []model.Event
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
			reposts = append(reposts, ev)
			require.NoError(t, db.AcceptEvents(t.Context(), &ev))
			helperPointsScoreEqual(t, db, target.ID, 4, 4e4) // like (1) + repost (3).
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
			helperPointsScoreEqual(t, db, target.ID, 8, 8e4) // like (1) + repost (3) + quote (4).
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
				helperPointsScoreEqual(t, db, target.ID, 10, 10e4) // like (1) + repost (3) + quote (4) + root comment (2).
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
				helperPointsScoreEqual(t, db, target.ID, 10, 10e4) // like (1) + repost (3) + quote (4) + root comment (2).
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
				helperPointsScoreEqual(t, db, target.ID, 6, 6e4) // like (1) + repost (3) + root comment (2).
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
			helperPointsScoreEqual(t, db, evArticle.ID, 4, 4e4) // like (1) + repost (3).
			helperPointsScoreEqual(t, db, evNote.ID, 6, 6e4)    // like (1) + repost (3) + root comment (2).
		})
		t.Run("rollback deletes", func(t *testing.T) {
			for _, id := range quotes {
				var ev model.Event
				ev.Kind = nostr.KindDeletion
				ev.ID = "delete_" + id
				ev.PubKey = "quote_pub"
				ev.Tags = model.Tags{
					{"e", id},
				}
				require.NoError(t, db.RollbackEvents(t.Context(), &ev))
			}
			helperPointsScoreEqual(t, db, evArticle.ID, 8, 8e4) // like (1) + repost (3) + quote(4)
			helperPointsScoreEqual(t, db, evNote.ID, 10, 10e4)  // like (1) + repost (3) + root comment (2) + quote(4)
		})
		t.Run("rollback reposts", func(t *testing.T) {
			for _, repost := range reposts {
				require.NoError(t, db.RollbackEvents(t.Context(), &repost))
			}
			helperPointsScoreEqual(t, db, evArticle.ID, 5, 5e4) // like (1) + quote(4)
			helperPointsScoreEqual(t, db, evNote.ID, 7, 7e4)    // like (1) + root comment (2) + quote(4)
		})
	})
}

func TestEventRankByVerifiedAndScore(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	var post1, post2, post3 model.Event
	post1.ID = "post1"
	post1.Kind = nostr.KindTextNote
	post1.PubKey = "post1_pub"

	post2.ID = "post2"
	post2.Kind = nostr.KindTextNote
	post2.PubKey = "post2_pub"

	post3.ID = "post3"
	post3.Kind = nostr.KindTextNote
	post3.PubKey = "post3_pub"

	require.NoError(t, db.AcceptEvents(t.Context(), &post1, &post2, &post3))

	helperSetEventPointsAndScore(t, db, post1.ID, 10, 10e4, 1, false)
	helperSetEventPointsAndScore(t, db, post2.ID, 20, 20e4, 2, true)
	helperSetEventPointsAndScore(t, db, post3.ID, 30, 30e4, 3, false)

	events := helperSelectEvents(t, db, model.Filter{
		Limit:  10,
		Search: "top",
	})
	require.Len(t, events, 3)

	// Expected: post2 (verified, score 20e4), post3 (not verified, score 30e4), post1 (not verified, score 10e4).
	require.Equal(t, post2.ID, events[0].ID)
	require.Equal(t, post3.ID, events[1].ID)
	require.Equal(t, post1.ID, events[2].ID)
}
