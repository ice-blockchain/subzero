// SPDX-License-Identifier: ice License 1.0

package query

import (
	"cmp"
	"database/sql"
	"slices"
	"strconv"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/database/query/internal/connector"
	"github.com/ice-blockchain/subzero/model"
)

func TestFlowTC_GetAndDelete(t *testing.T) {
	t.Parallel()

	db, _ := helperEnsureDatabaseWithData(t)
	defer db.Close()

	user1Priv, user2Priv := model.GeneratePrivateKey(), model.GeneratePrivateKey()

	var evPost, evAction, evAction2, evDefiniton model.Event
	evPost.Kind = model.CustomIONKindEditableTextNote
	evPost.Content = "This is a post"
	evPost.CreatedAt = nostr.Now()
	evPost.Tags = model.Tags{
		{"d", "post1"},
	}
	require.NoError(t, evPost.SignWithAlg(user1Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	evDefiniton.Kind = model.CustomIONKindTokenizedCommunityDefination
	evDefiniton.CreatedAt = nostr.Now()
	evDefiniton.Tags = model.Tags{
		{"a", evPost.Address()},
		{"d", "def1"},
		{"k", strconv.Itoa(evPost.Kind)},
	}
	require.NoError(t, evDefiniton.SignWithAlg(user1Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	evAction.Kind = model.CustomIONKindTokenizedCommunityAction
	evAction.CreatedAt = nostr.Now()
	evAction.Content = "This is action 1"
	evAction.Tags = model.Tags{
		{"a", evDefiniton.Address()},
		{"tx_type", "test_buy"},
		{"network", "testnet"},
		{"token_address", "0xTokenAddress"},
	}
	require.NoError(t, evAction.SignWithAlg(user2Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	evActionFirstBuyAnotherUser := evAction
	evActionFirstBuyAnotherUser.CreatedAt--
	evActionFirstBuyAnotherUser.Content = "This is first buy by another user"
	require.NoError(t, evActionFirstBuyAnotherUser.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

	require.NoError(t, db.AcceptEvents(t.Context(), &evPost, &evDefiniton, &evActionFirstBuyAnotherUser))
	require.NoError(t, db.AcceptEvents(t.Context(), &evAction))

	evAction2.Kind = model.CustomIONKindTokenizedCommunityAction
	evAction2.CreatedAt = evAction.CreatedAt + 1
	evAction2.Content = "This is action 2"
	evAction2.Tags = model.Tags{
		{"a", evDefiniton.Address()},
		{"tx_type", "test_buy2"},
		{"network", "testnet"},
		{"token_address", "0xTokenAddress"},
	}
	require.NoError(t, evAction2.SignWithAlg(user2Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, db.AcceptEvents(t.Context(), &evAction2))

	evAction3 := evAction2
	evAction3.CreatedAt++
	evAction3.Content = "This is action 3" // Should be totally ignored in the query.
	require.NoError(t, evAction3.SignWithAlg(user2Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, db.AcceptEvents(t.Context(), &evAction3))

	events := helperSelectEvents(t, db, model.Filter{
		Since:  &evAction2.CreatedAt,
		Until:  &evAction2.CreatedAt,
		Search: "include:dependencies:kind1175>kind31175 include:dependencies:kind1>kind10100",
	})
	require.Len(t, events, 4) // 2 action (first buy + requested one), 1 definition, 1 post.

	t.Run("1175 first buy is ephemeral embedding", func(t *testing.T) {
		firstBuyIndex := slices.IndexFunc(events, func(e *model.Event) bool { return e.GetTag("e").Value() == evAction.ID })
		require.Greater(t, firstBuyIndex, -1)
		firstBuyEvent := events[firstBuyIndex]
		require.Equal(t, model.CustomIONKindEphemeralEmbedding, firstBuyEvent.Kind)
		require.EqualValues(t, evAction.String(), firstBuyEvent.Content)

		events = slices.Delete(events, firstBuyIndex, firstBuyIndex+1)
	})
	t.Run("31175 definition is ephemeral embedding", func(t *testing.T) {
		defIndex := slices.IndexFunc(events, func(e *model.Event) bool { return e.GetTag("e").Value() == evDefiniton.ID })
		require.Greater(t, defIndex, -1)
		receivedDefEvent := events[defIndex]
		require.Equal(t, model.CustomIONKindEphemeralEmbedding, receivedDefEvent.Kind)
		require.EqualValues(t, evDefiniton.String(), receivedDefEvent.Content)

		events = slices.Delete(events, defIndex, defIndex+1)
	})

	require.ElementsMatch(t, []*model.Event{&evAction2, &evPost}, events)

	t.Run("Delete is not allowed", func(t *testing.T) {
		t.Run("Action", func(t *testing.T) {
			var evActionDelete model.Event

			evActionDelete.Kind = nostr.KindDeletion
			evActionDelete.CreatedAt = nostr.Now()
			evActionDelete.Tags = model.Tags{
				{"e", evAction.ID},
			}
			require.NoError(t, evActionDelete.SignWithAlg(user2Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.NoError(t, db.AcceptEvents(t.Context(), &evActionDelete))

			events := helperSelectEvents(t, db, model.Filter{IDs: []string{evAction.ID}})
			require.Len(t, events, 1)
			require.Equal(t, &evAction, events[0])
		})
		t.Run("Post", func(t *testing.T) {
			var evPostDelete model.Event

			evPostDelete.Kind = nostr.KindDeletion
			evPostDelete.CreatedAt = nostr.Now()
			evPostDelete.Tags = model.Tags{
				{"e", evPost.ID},
			}
			require.NoError(t, evPostDelete.SignWithAlg(user1Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.NoError(t, db.AcceptEvents(t.Context(), &evPostDelete))

			events := helperSelectEvents(t, db, model.Filter{IDs: []string{evPost.ID}})
			require.Len(t, events, 1)
			require.Equal(t, &evPost, events[0])
		})
		t.Run("Account Delete", func(t *testing.T) {
			var evAccountDelete model.Event

			evAccountDelete.Kind = nostr.KindDeletion
			evAccountDelete.CreatedAt = nostr.Now()
			require.NoError(t, evAccountDelete.SignWithAlg(user1Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			err := db.AcceptEvents(t.Context(), &evAccountDelete)
			require.ErrorIs(t, err, ErrForbidden)
		})
	})
}

func helperSelectTCActionID(t *testing.T, db *dbClient, eventID string) string {
	t.Helper()

	value, err := connector.GetNamed[sql.NullString](t.Context(), db.db, "select first_1175_address from events where id = :event_id", map[string]any{
		"event_id": eventID,
	})
	if errors.Is(err, connector.ErrNotFound) {
		return ""
	}
	require.NoError(t, err)
	require.NotNil(t, value)

	t.Logf("TC Action ID for event %s: %v", eventID, cmp.Or(value.String, "<NULL>"))

	return value.String
}

func TestFlowTC_LinkActionID(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	user1Priv, user2Priv := model.GeneratePrivateKey(), model.GeneratePrivateKey()

	var evPost, evAction, evDefiniton model.Event
	evPost.Kind = model.CustomIONKindEditableTextNote
	evPost.Content = "This is a post"
	evPost.CreatedAt = nostr.Now()
	evPost.Tags = model.Tags{
		{"d", "post1"},
	}
	require.NoError(t, evPost.SignWithAlg(user1Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	evDefiniton.Kind = model.CustomIONKindTokenizedCommunityDefination
	evDefiniton.CreatedAt = nostr.Now()
	evDefiniton.Tags = model.Tags{
		{"a", evPost.Address()},
		{"d", "def1"},
		{"k", strconv.Itoa(evPost.Kind)},
	}
	require.NoError(t, evDefiniton.SignWithAlg(user1Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	evAction.Kind = model.CustomIONKindTokenizedCommunityAction
	evAction.CreatedAt = nostr.Now()
	evAction.Tags = model.Tags{
		{"a", evDefiniton.Address()},
		{"tx_type", "test_buy"},
		{"network", "testnet"},
		{"token_address", "0xTokenAddress"},
	}
	require.NoError(t, evAction.SignWithAlg(user2Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, db.AcceptEvents(t.Context(), &evPost, &evDefiniton, &evAction))
	require.Empty(t, helperSelectTCActionID(t, db, evAction.ID))

	var evAction2 model.Event
	evAction2.Kind = model.CustomIONKindTokenizedCommunityAction
	evAction2.CreatedAt = nostr.Now()
	evAction2.Tags = model.Tags{
		{"a", evDefiniton.Address()},
		{"tx_type", "test_buy_2"},
		{"network", "testnet"},
		{"token_address", "0xTokenAddress"},
	}
	require.NoError(t, evAction2.SignWithAlg(user2Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, db.AcceptEvents(t.Context(), &evAction2))
	require.Equal(t, evAction.ID, helperSelectTCActionID(t, db, evAction2.ID))

	var evAction3 model.Event
	evAction3.Kind = model.CustomIONKindTokenizedCommunityAction
	evAction3.CreatedAt = nostr.Now()
	evAction3.Tags = model.Tags{
		{"a", evDefiniton.Address()},
		{"tx_type", "test_buy_prod"},
		{"network", "prod"},
		{"token_address", "0xTokenAddress"},
	}
	require.NoError(t, evAction3.SignWithAlg(user2Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, db.AcceptEvents(t.Context(), &evAction3))
	require.Empty(t, helperSelectTCActionID(t, db, evAction3.ID))
}

func TestFlowTC_FirstBuyActionFromPost(t *testing.T) {
	t.Parallel()

	db := helperNewDatabase(t)
	defer db.Close()

	postAuthor := model.GeneratePrivateKey()
	user2Priv := model.GeneratePrivateKey() // Action user 1.
	user3Priv := model.GeneratePrivateKey() // Action user 2.
	user4Priv := model.GeneratePrivateKey() // Action user 3.

	var evPost1, evPost2, evPost3 model.Event

	evPost1.Kind = model.CustomIONKindEditableTextNote
	evPost1.Content = "Post 1 - Editable text note"
	evPost1.CreatedAt = nostr.Now()
	evPost1.Tags = model.Tags{{"d", "post1"}}
	require.NoError(t, evPost1.SignWithAlg(postAuthor, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	evPost2.Kind = nostr.KindTextNote
	evPost2.Content = "Post 2 - Text note"
	evPost2.CreatedAt = nostr.Now()
	evPost2.Tags = model.Tags{{"d", "post2"}}
	require.NoError(t, evPost2.SignWithAlg(postAuthor, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	evPost3.Kind = nostr.KindArticle
	evPost3.Content = "Post 3 - Article (no definition)"
	evPost3.CreatedAt = nostr.Now()
	evPost3.Tags = model.Tags{{"d", "post3"}}
	require.NoError(t, evPost3.SignWithAlg(postAuthor, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	var evDef1, evDef2 model.Event

	evDef1.Kind = model.CustomIONKindTokenizedCommunityDefination
	evDef1.CreatedAt = nostr.Now()
	evDef1.Tags = model.Tags{
		{"a", evPost1.Address()},
		{"d", "def1"},
		{"k", strconv.Itoa(evPost1.Kind)},
	}
	require.NoError(t, evDef1.SignWithAlg(postAuthor, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	evDef2.Kind = model.CustomIONKindTokenizedCommunityDefination
	evDef2.CreatedAt = nostr.Now()
	evDef2.Tags = model.Tags{
		{"e", evPost2.ID},
		{"d", "def2"},
		{"k", strconv.Itoa(evPost2.Kind)},
	}
	require.NoError(t, evDef2.SignWithAlg(postAuthor, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	var evAction1_1, evAction1_2, evAction1_3 model.Event

	evAction1_1.Kind = model.CustomIONKindTokenizedCommunityAction
	evAction1_1.CreatedAt = nostr.Now()
	evAction1_1.Content = "Action 1 for def1 (first buy)"
	evAction1_1.Tags = model.Tags{
		{"a", evDef1.Address()},
		{"tx_type", "buy"},
		{"network", "testnet"},
		{"token_address", "0xToken1"},
	}
	require.NoError(t, evAction1_1.SignWithAlg(user2Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	evAction1_2.Kind = model.CustomIONKindTokenizedCommunityAction
	evAction1_2.CreatedAt = evAction1_1.CreatedAt + 1
	evAction1_2.Content = "Action 2 for def1"
	evAction1_2.Tags = model.Tags{
		{"a", evDef1.Address()},
		{"tx_type", "buy"},
		{"network", "testnet"},
		{"token_address", "0xToken1"},
	}
	require.NoError(t, evAction1_2.SignWithAlg(user3Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	evAction1_3.Kind = model.CustomIONKindTokenizedCommunityAction
	evAction1_3.CreatedAt = evAction1_2.CreatedAt + 1
	evAction1_3.Content = "Action 3 for def1"
	evAction1_3.Tags = model.Tags{
		{"a", evDef1.Address()},
		{"tx_type", "buy"},
		{"network", "testnet"},
		{"token_address", "0xToken1"},
	}
	require.NoError(t, evAction1_3.SignWithAlg(user4Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	var evAction2_1, evAction2_2 model.Event

	evAction2_1.Kind = model.CustomIONKindTokenizedCommunityAction
	evAction2_1.CreatedAt = nostr.Now()
	evAction2_1.Content = "Action 1 for def2 (first buy)"
	evAction2_1.Tags = model.Tags{
		{"a", evDef2.Address()},
		{"tx_type", "buy"},
		{"network", "testnet"},
		{"token_address", "0xToken2"},
	}
	require.NoError(t, evAction2_1.SignWithAlg(user2Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	evAction2_2.Kind = model.CustomIONKindTokenizedCommunityAction
	evAction2_2.CreatedAt = evAction2_1.CreatedAt + 1
	evAction2_2.Content = "Action 2 for def2"
	evAction2_2.Tags = model.Tags{
		{"a", evDef2.Address()},
		{"tx_type", "buy"},
		{"network", "testnet"},
		{"token_address", "0xToken2"},
	}
	require.NoError(t, evAction2_2.SignWithAlg(user2Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	require.NoError(t, db.AcceptEvents(t.Context(), &evPost1, &evPost2, &evPost3))
	require.NoError(t, db.AcceptEvents(t.Context(), &evDef1, &evDef2))
	require.NoError(t, db.AcceptEvents(t.Context(), &evAction1_1)) // First buy for def1.
	require.NoError(t, db.AcceptEvents(t.Context(), &evAction1_2, &evAction1_3))
	require.NoError(t, db.AcceptEvents(t.Context(), &evAction2_1)) // First buy for def2.
	require.NoError(t, db.AcceptEvents(t.Context(), &evAction2_2))

	require.Empty(t, helperSelectTCActionID(t, db, evAction1_1.ID), "first action should have NULL first_1175_address")
	require.Equal(t, evAction1_1.ID, helperSelectTCActionID(t, db, evAction1_2.ID))
	require.Equal(t, evAction1_1.ID, helperSelectTCActionID(t, db, evAction1_3.ID))
	require.Empty(t, helperSelectTCActionID(t, db, evAction2_1.ID), "first action should have NULL first_1175_address")
	require.Equal(t, evAction2_1.ID, helperSelectTCActionID(t, db, evAction2_2.ID))

	// Select posts with dependency filter to get first buy actions.
	// Using kind30175>kind1175 to find first buy actions for posts.
	events := helperSelectEvents(t, db, model.Filter{
		Kinds:  []int{evPost1.Kind, evPost2.Kind, evPost3.Kind},
		Search: "include:dependencies:kind30175>kind1175 include:dependencies:kind1>kind1175 include:dependencies:kind0>kind1175",
	})

	// 3 posts + 2 first buy actions (one per definition).
	require.Len(t, events, 5)

	// Extract posts and actions.
	var posts, actions []*model.Event
	for _, ev := range events {
		switch ev.Kind {
		case model.CustomIONKindEditableTextNote, nostr.KindTextNote, nostr.KindArticle:
			posts = append(posts, ev)
		case model.CustomIONKindTokenizedCommunityAction:
			actions = append(actions, ev)
		}
	}

	// Verify all 3 posts are returned.
	require.Len(t, posts, 3)
	require.ElementsMatch(t, []*model.Event{&evPost1, &evPost2, &evPost3}, posts)

	// Verify only 2 first buy actions are returned.
	require.Len(t, actions, 2)
	require.ElementsMatch(t, []*model.Event{&evAction1_1, &evAction2_1}, actions)
}
