// SPDX-License-Identifier: ice License 1.0

package query

import (
	"cmp"
	"database/sql"
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
	require.NoError(t, db.AcceptEvents(t.Context(), &evPost, &evDefiniton, &evAction))

	evAction2.Kind = model.CustomIONKindTokenizedCommunityAction
	evAction2.CreatedAt = 3
	evAction2.Content = "This is action 2"
	evAction2.Tags = model.Tags{
		{"a", evDefiniton.Address()},
		{"tx_type", "test_buy2"},
		{"network", "testnet"},
		{"token_address", "0xTokenAddress"},
	}
	require.NoError(t, evAction2.SignWithAlg(user2Priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, db.AcceptEvents(t.Context(), &evAction2))

	events := helperSelectEvents(t, db, model.Filter{
		Since:  &evAction2.CreatedAt,
		Until:  &evAction2.CreatedAt,
		Search: "include:dependencies:kind1175>kind31175 include:dependencies:kind1>kind10100",
	})
	require.Len(t, events, 4) // 2 action (first buy + requested one), 1 definition, 1 post.
	require.ElementsMatch(t, []*model.Event{&evAction, &evAction2, &evDefiniton, &evPost}, events)

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

	value, err := connector.GetNamed[sql.NullString](t.Context(), db.db, "select tc_action_id from events where id = :event_id", map[string]any{
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
