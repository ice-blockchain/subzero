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

	var evPost, evAction, evAction2, evDefinition model.Event
	evPost.Kind = model.CustomIONKindEditableTextNote
	evPost.Content = "This is a post"
	evPost.CreatedAt = nostr.Now()
	evPost.Tags = model.Tags{
		{"d", "post1"},
	}
	require.NoError(t, evPost.SignWithAlg(user1Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	evDefinition.Kind = model.CustomIONKindTokenizedCommunityDefinition
	evDefinition.CreatedAt = nostr.Now()
	evDefinition.Tags = model.Tags{
		{"a", evPost.Address()},
		{"d", "def1"},
		{"k", strconv.Itoa(evPost.Kind)},
	}
	require.NoError(t, evDefinition.SignWithAlg(user1Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	evAction.Kind = model.CustomIONKindTokenizedCommunityAction
	evAction.CreatedAt = nostr.Now()
	evAction.Content = "This is action 1"
	evAction.Tags = model.Tags{
		{"a", evDefinition.Address()},
		{"tx_type", "test_buy"},
		{"network", "testnet"},
		{"token_address", "0xTokenAddress"},
	}
	require.NoError(t, evAction.SignWithAlg(user2Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	evActionFirstBuyAnotherUser := evAction
	evActionFirstBuyAnotherUser.CreatedAt--
	evActionFirstBuyAnotherUser.Content = "This is first buy by another user"
	require.NoError(t, evActionFirstBuyAnotherUser.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

	require.NoError(t, db.AcceptEvents(t.Context(), &evPost, &evDefinition, &evActionFirstBuyAnotherUser))
	require.NoError(t, db.AcceptEvents(t.Context(), &evAction))

	evAction2.Kind = model.CustomIONKindTokenizedCommunityAction
	evAction2.CreatedAt = evAction.CreatedAt + 1
	evAction2.Content = "This is action 2"
	evAction2.Tags = model.Tags{
		{"a", evDefinition.Address()},
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

	t.Run("Find definition for the given post", func(t *testing.T) {
		events := helperSelectEvents(t, db, model.Filter{
			IDs:    []string{evPost.ID},
			Search: "include:dependencies:kind30175>kind31175",
		})

		require.Len(t, events, 2) // 1 post + 1 definition.
		for _, ev := range events {
			switch ev.Kind {
			case model.CustomIONKindEditableTextNote:
				require.Equal(t, &evPost, ev)

			case model.CustomIONKindEphemeralEmbedding:
				var nested model.Event
				require.NoError(t, nested.UnmarshalJSON([]byte(ev.Content)))
				switch nested.Kind {
				case model.CustomIONKindTokenizedCommunityDefinition:
					require.Equal(t, evDefinition, nested)

				default:
					require.Failf(t, "unexpected nested event kind", "got %d", nested.Kind)
				}

			default:
				require.Failf(t, "unexpected event kind", "got %d", ev.Kind)
			}
		}
	})

	t.Run("Find definition for the given repost", func(t *testing.T) {
		var repostEvent model.Event

		repostEvent.Kind = nostr.KindGenericRepost
		repostEvent.Content = evPost.String()
		repostEvent.CreatedAt = nostr.Now()
		repostEvent.Tags = model.Tags{
			{"a", evPost.Address()},
		}
		require.NoError(t, repostEvent.SignWithAlg(user2Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, db.AcceptEvents(t.Context(), &repostEvent))

		events := helperSelectEvents(t, db, model.Filter{
			IDs:    []string{repostEvent.ID},
			Search: "include:dependencies:kind16>kind31175",
		})

		require.Len(t, events, 2) // 1 repost + 1 definition.
		for _, ev := range events {
			switch ev.Kind {
			case nostr.KindGenericRepost:
				require.Equal(t, &repostEvent, ev)

			case model.CustomIONKindEphemeralEmbedding:
				var nested model.Event
				require.NoError(t, nested.UnmarshalJSON([]byte(ev.Content)))
				switch nested.Kind {
				case model.CustomIONKindTokenizedCommunityDefinition:
					require.Equal(t, evDefinition, nested)

				default:
					require.Failf(t, "unexpected nested event kind", "got %d", nested.Kind)
				}

			default:
				require.Failf(t, "unexpected event kind", "got %d", ev.Kind)
			}
		}
	})

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

	var evPost, evAction, evDefinition model.Event
	evPost.Kind = model.CustomIONKindEditableTextNote
	evPost.Content = "This is a post"
	evPost.CreatedAt = nostr.Now()
	evPost.Tags = model.Tags{
		{"d", "post1"},
	}
	require.NoError(t, evPost.SignWithAlg(user1Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	evDefinition.Kind = model.CustomIONKindTokenizedCommunityDefinition
	evDefinition.CreatedAt = nostr.Now()
	evDefinition.Tags = model.Tags{
		{"a", evPost.Address()},
		{"d", "def1"},
		{"k", strconv.Itoa(evPost.Kind)},
	}
	require.NoError(t, evDefinition.SignWithAlg(user1Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	// Insert a reset event for user2 to establish the position reset baseline.
	var evReset model.Event
	evReset.Kind = model.CustomIONKindTokenizedCommunityAction
	evReset.CreatedAt = nostr.Now()
	evReset.Tags = model.Tags{
		{"a", evDefinition.Address()},
		{"t", "community_token_position_reset"},
	}
	require.NoError(t, evReset.SignWithAlg(user2Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, db.AcceptEvents(t.Context(), &evPost, &evDefinition, &evReset))
	require.Empty(t, helperSelectTCActionID(t, db, evReset.ID), "reset event should have NULL first_1175_address")

	evAction.Kind = model.CustomIONKindTokenizedCommunityAction
	evAction.CreatedAt = nostr.Now()
	evAction.Tags = model.Tags{
		{"a", evDefinition.Address()},
		{"tx_type", "test_buy"},
		{"network", "testnet"},
		{"token_address", "0xTokenAddress"},
	}
	require.NoError(t, evAction.SignWithAlg(user2Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, db.AcceptEvents(t.Context(), &evAction))
	require.Equal(t, evReset.ID, helperSelectTCActionID(t, db, evAction.ID))

	var evAction2 model.Event
	evAction2.Kind = model.CustomIONKindTokenizedCommunityAction
	evAction2.CreatedAt = nostr.Now()
	evAction2.Tags = model.Tags{
		{"a", evDefinition.Address()},
		{"tx_type", "test_buy_2"},
		{"network", "testnet"},
		{"token_address", "0xTokenAddress"},
	}
	require.NoError(t, evAction2.SignWithAlg(user2Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, db.AcceptEvents(t.Context(), &evAction2))
	require.Equal(t, evReset.ID, helperSelectTCActionID(t, db, evAction2.ID))
}

func TestFlowTC_FirstBuyFromPostOrAction(t *testing.T) {
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

	evDef1.Kind = model.CustomIONKindTokenizedCommunityDefinition
	evDef1.CreatedAt = nostr.Now()
	evDef1.Tags = model.Tags{
		{"a", evPost1.Address()},
		{"d", "def1"},
		{"k", strconv.Itoa(evPost1.Kind)},
	}
	require.NoError(t, evDef1.SignWithAlg(postAuthor, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	evDef2.Kind = model.CustomIONKindTokenizedCommunityDefinition
	evDef2.CreatedAt = nostr.Now()
	evDef2.Tags = model.Tags{
		{"e", evPost2.ID},
		{"d", "def2"},
		{"k", strconv.Itoa(evPost2.Kind)},
	}
	require.NoError(t, evDef2.SignWithAlg(postAuthor, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	var evDef1FirstBuy, evDef2FirstBuy model.Event

	evDef1FirstBuy = evDef1
	evDef1FirstBuy.CreatedAt++
	evDef1FirstBuy.Content = "First buy for def1 by user2"
	evDef1FirstBuy.Tags = slices.Clone(evDef1.Tags)
	evDef1FirstBuy.Tags = append(evDef1FirstBuy.Tags, model.Tag{"p", evDef1.GetMasterPublicKey()})
	require.NoError(t, evDef1FirstBuy.SignWithAlg(user2Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	evDef2FirstBuy = evDef2
	evDef2FirstBuy.CreatedAt++
	evDef2FirstBuy.Content = "First buy for def2 by user3"
	evDef2FirstBuy.Tags = slices.Clone(evDef2.Tags)
	evDef2FirstBuy.Tags = append(evDef2FirstBuy.Tags, model.Tag{"p", evDef2.GetMasterPublicKey()})
	require.NoError(t, evDef2FirstBuy.SignWithAlg(user3Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	var evAction1_1, evAction1_2, evAction1_3 model.Event

	evAction1_1.Kind = model.CustomIONKindTokenizedCommunityAction
	evAction1_1.CreatedAt = nostr.Now()
	evAction1_1.Content = "Action 1 for def1 (first buy) by user2"
	evAction1_1.Tags = model.Tags{
		{"a", evDef1.Address()},
		{"tx_type", "buy"},
		{"network", "testnet"},
		{"token_address", "0xToken1"},
	}
	require.NoError(t, evAction1_1.SignWithAlg(user2Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	evAction1_2.Kind = model.CustomIONKindTokenizedCommunityAction
	evAction1_2.CreatedAt = evAction1_1.CreatedAt + 1
	evAction1_2.Content = "Action 2 for def1 by user3"
	evAction1_2.Tags = model.Tags{
		{"a", evDef1.Address()},
		{"tx_type", "buy"},
		{"network", "testnet"},
		{"token_address", "0xToken1"},
	}
	require.NoError(t, evAction1_2.SignWithAlg(user3Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	evAction1_3.Kind = model.CustomIONKindTokenizedCommunityAction
	evAction1_3.CreatedAt = evAction1_2.CreatedAt + 1
	evAction1_3.Content = "Action 3 for def1 by user4"
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
	evAction2_1.Content = "Action 1 for def2 (first buy) by user2"
	evAction2_1.Tags = model.Tags{
		{"a", evDef2.Address()},
		{"tx_type", "buy"},
		{"network", "testnet"},
		{"token_address", "0xToken2"},
	}
	require.NoError(t, evAction2_1.SignWithAlg(user2Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	evAction2_2.Kind = model.CustomIONKindTokenizedCommunityAction
	evAction2_2.CreatedAt = evAction2_1.CreatedAt + 1
	evAction2_2.Content = "Action 2 for def2 by user2"
	evAction2_2.Tags = model.Tags{
		{"a", evDef2.Address()},
		{"tx_type", "buy"},
		{"network", "testnet"},
		{"token_address", "0xToken2"},
	}
	require.NoError(t, evAction2_2.SignWithAlg(user2Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	// Create reset events for each (user, definition) pair so the trigger can link actions.
	var resetDef1User2, resetDef1User3, resetDef1User4, resetDef2User2 model.Event

	resetDef1User2.Kind = model.CustomIONKindTokenizedCommunityAction
	resetDef1User2.CreatedAt = evAction1_1.CreatedAt - 1
	resetDef1User2.Tags = model.Tags{{"a", evDef1.Address()}, {"t", "community_token_position_reset"}}
	require.NoError(t, resetDef1User2.SignWithAlg(user2Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	resetDef1User3.Kind = model.CustomIONKindTokenizedCommunityAction
	resetDef1User3.CreatedAt = evAction1_2.CreatedAt - 1
	resetDef1User3.Tags = model.Tags{{"a", evDef1.Address()}, {"t", "community_token_position_reset"}}
	require.NoError(t, resetDef1User3.SignWithAlg(user3Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	resetDef1User4.Kind = model.CustomIONKindTokenizedCommunityAction
	resetDef1User4.CreatedAt = evAction1_3.CreatedAt - 1
	resetDef1User4.Tags = model.Tags{{"a", evDef1.Address()}, {"t", "community_token_position_reset"}}
	require.NoError(t, resetDef1User4.SignWithAlg(user4Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	resetDef2User2.Kind = model.CustomIONKindTokenizedCommunityAction
	resetDef2User2.CreatedAt = evAction2_1.CreatedAt - 1
	resetDef2User2.Tags = model.Tags{{"a", evDef2.Address()}, {"t", "community_token_position_reset"}}
	require.NoError(t, resetDef2User2.SignWithAlg(user2Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	require.NoError(t, db.AcceptEvents(t.Context(), &evPost1, &evPost2, &evPost3))
	require.NoError(t, db.AcceptEvents(t.Context(), &evDef1, &evDef2))
	require.NoError(t, db.AcceptEvents(t.Context(), &evDef1FirstBuy, &evDef2FirstBuy))
	require.NoError(t, db.AcceptEvents(t.Context(), &resetDef1User2, &resetDef1User3, &resetDef1User4, &resetDef2User2))
	require.NoError(t, db.AcceptEvents(t.Context(), &evAction1_1))
	require.NoError(t, db.AcceptEvents(t.Context(), &evAction1_2, &evAction1_3))
	require.NoError(t, db.AcceptEvents(t.Context(), &evAction2_1)) // First buy for def2.
	require.NoError(t, db.AcceptEvents(t.Context(), &evAction2_2))

	require.Empty(t, helperSelectTCActionID(t, db, resetDef1User2.ID), "reset event should have NULL first_1175_address")
	require.Empty(t, helperSelectTCActionID(t, db, resetDef1User3.ID), "reset event should have NULL first_1175_address")
	require.Empty(t, helperSelectTCActionID(t, db, resetDef1User4.ID), "reset event should have NULL first_1175_address")
	require.Empty(t, helperSelectTCActionID(t, db, resetDef2User2.ID), "reset event should have NULL first_1175_address")

	require.Equal(t, resetDef1User2.ID, helperSelectTCActionID(t, db, evAction1_1.ID))
	require.Equal(t, resetDef1User3.ID, helperSelectTCActionID(t, db, evAction1_2.ID))
	require.Equal(t, resetDef1User4.ID, helperSelectTCActionID(t, db, evAction1_3.ID))
	require.Equal(t, resetDef2User2.ID, helperSelectTCActionID(t, db, evAction2_1.ID))
	require.Equal(t, resetDef2User2.ID, helperSelectTCActionID(t, db, evAction2_2.ID))

	t.Run("post>kind31175", func(t *testing.T) {
		events := helperSelectEvents(t, db, model.Filter{
			Kinds:  []int{evPost1.Kind, evPost2.Kind, evPost3.Kind},
			Search: "include:dependencies:kind30175>kind31175 include:dependencies:kind1>kind31175 include:dependencies:kind0>kind31175",
		})

		// 3 posts + 2 definitions + 2 def first buys = 7 events.
		require.Len(t, events, 7)

		// Extract posts and actions.
		var posts, defs, defFirstBuy []*model.Event
		for _, ev := range events {
			switch ev.Kind {
			case model.CustomIONKindEditableTextNote, nostr.KindTextNote, nostr.KindArticle:
				posts = append(posts, ev)

			case model.CustomIONKindEphemeralEmbedding:
				var nested model.Event
				ok, err := ev.CheckSignature()
				require.NoError(t, err)
				require.True(t, ok)

				require.NoError(t, nested.UnmarshalJSON([]byte(ev.Content)))
				ok, err = nested.CheckSignature()
				require.NoError(t, err)
				require.True(t, ok)
				switch nested.Kind {
				case model.CustomIONKindTokenizedCommunityDefinition:
					if nested.GetTag("p").Value() != "" {
						defFirstBuy = append(defFirstBuy, &nested)
					} else {
						defs = append(defs, &nested)
					}
				default:
					require.Failf(t, "unexpected nested event kind", "got %d", nested.Kind)
				}
			default:
				require.Failf(t, "unexpected event kind", "got %d", ev.Kind)
			}
		}

		// Verify all 3 posts are returned.
		require.Len(t, posts, 3)
		require.ElementsMatch(t, []*model.Event{&evPost1, &evPost2, &evPost3}, posts)

		// Verify both definitions are returned.
		require.Len(t, defs, 2)
		require.ElementsMatch(t, []*model.Event{&evDef1, &evDef2}, defs)

		// Verify both definition first buys are returned.
		require.Len(t, defFirstBuy, 2)
		require.ElementsMatch(t, []*model.Event{&evDef1FirstBuy, &evDef2FirstBuy}, defFirstBuy)
	})
	t.Run("kind1175>kind31175", func(t *testing.T) {
		events := helperSelectEvents(t, db, model.Filter{
			IDs:    []string{evAction1_1.ID, evAction2_2.ID},
			Search: "include:dependencies:kind1175>kind31175",
		})

		// 2 actions + 2 definitions + 2 posts + 2 reset events = 8 events.
		require.Len(t, events, 8)

		var original, def, posts, resets []*model.Event

		for _, ev := range events {
			switch ev.Kind {
			case model.CustomIONKindTokenizedCommunityAction:
				original = append(original, ev)

			case model.CustomIONKindEphemeralEmbedding:
				var nested model.Event
				require.NoError(t, nested.UnmarshalJSON([]byte(ev.Content)))
				switch nested.Kind {
				case model.CustomIONKindTokenizedCommunityDefinition:
					def = append(def, &nested)

				case model.CustomIONKindEditableTextNote, nostr.KindTextNote:
					posts = append(posts, &nested)

				case model.CustomIONKindTokenizedCommunityAction:
					resets = append(resets, &nested)

				default:
					require.Failf(t, "unexpected nested event kind", "got %d", nested.Kind)
				}
			default:
				require.Failf(t, "unexpected event kind", "got %d", ev.Kind)
			}
		}

		require.ElementsMatch(t, []*model.Event{&evAction1_1, &evAction2_2}, original)
		require.ElementsMatch(t, []*model.Event{&evPost1, &evPost2}, posts)
		require.ElementsMatch(t, []*model.Event{&evDef1, &evDef2}, def)
		require.ElementsMatch(t, []*model.Event{&resetDef1User2, &resetDef2User2}, resets)
	})
}
