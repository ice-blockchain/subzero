// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func TestHandleAuth(t *testing.T) {
	t.Parallel()

	t.Run("No challenge sent", func(t *testing.T) {
		h := newHandler("wss://relay.example.com", "")

		priv := model.GeneratePrivateKey()
		authEvent := createValidAuthEvent(t, priv, "challenge", "wss://relay.example.com")

		mockWriter := &mockWriter{}
		resp := h.handleAuth(t.Context(), mockWriter, authEvent)

		require.False(t, resp.OK)
	})

	t.Run("Already authenticated with different public key", func(t *testing.T) {
		state := connAuthData{
			UserDataContext: model.UserDataContext{
				Authenticated: true,
				PublicKey:     "foo",
			},
			Challenge: "challenge123",
		}
		h := newHandler("wss://relay.example.com", "")
		w := &mockWriter{}
		h.ConnAuth.Store(w, state)

		authEvent := createValidAuthEvent(t, model.GeneratePrivateKey(), "challenge123", "wss://relay.example.com")
		resp := h.handleAuth(t.Context(), w, authEvent)
		require.False(t, resp.OK)
	})

	t.Run("Invalid auth event", func(t *testing.T) {
		state := connAuthData{
			Challenge: "challenge123",
		}
		h := newHandler("wss://relay.example.com", "")
		w := &mockWriter{}
		h.ConnAuth.Store(w, state)

		authEvent := createValidAuthEvent(t, model.GeneratePrivateKey(), "wrong-challenge", "wss://relay.example.com")
		resp := h.handleAuth(t.Context(), w, authEvent)

		require.False(t, resp.OK)
		require.Contains(t, resp.Reason, "failed to validate auth event:")
	})

	t.Run("Successful authentication - master key", func(t *testing.T) {
		state := connAuthData{
			Challenge: "challenge123",
		}
		h := newHandler("wss://relay.example.com", "")
		w := &mockWriter{}
		h.ConnAuth.Store(w, state)

		priv, pub := model.GenerateKeyPair()
		authEvent := createValidAuthEvent(t, priv, "challenge123", "wss://relay.example.com")
		resp := h.handleAuth(t.Context(), w, authEvent)
		require.True(t, resp.OK)
		require.Empty(t, resp.Reason)

		storedData, ok := h.ConnAuth.Load(w)
		require.True(t, ok)
		require.True(t, storedData.Authenticated)
		require.Equal(t, pub, storedData.PublicKey)
		require.Equal(t, pub, storedData.MasterPublicKey)
		require.Equal(t, "challenge123", storedData.Challenge)
	})

	t.Run("Successful authentication - with delegation", func(t *testing.T) {
		state := connAuthData{
			Challenge: "challenge123",
		}
		h := newHandler("wss://relay.example.com", "")
		w := &mockWriter{}
		h.ConnAuth.Store(w, state)

		masterPriv, masterPub := model.GenerateKeyPair()
		priv, pub := model.GenerateKeyPair()

		// Create attestation event
		attestationEvent := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindAttestation,
				Tags: model.Tags{
					{"p", pub, "", model.CustomIONAttestationKindActive + ":1:10,22,33"},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, attestationEvent, masterPriv)

		attestationJSON, err := json.Marshal(attestationEvent)
		require.NoError(t, err)

		authEvent := &model.Event{
			Event: nostr.Event{
				Kind:      nostr.KindClientAuthentication,
				CreatedAt: nostr.Now(),
				Tags: model.Tags{
					{model.CustomIONTagOnBehalfOf, masterPub},
					{"attestation", string(attestationJSON)},
					{"challenge", "challenge123"},
					{"relay", "wss://relay.example.com"},
					{"user-agent", "test-client"},
				},
			},
		}
		require.NoError(t, authEvent.SignWithAlg(priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		resp := h.handleAuth(t.Context(), w, authEvent)
		require.True(t, resp.OK)
		require.Empty(t, resp.Reason)

		// Verify stored auth data
		storedData, ok := h.ConnAuth.Load(w)
		require.True(t, ok)
		require.True(t, storedData.Authenticated)
		require.Equal(t, pub, storedData.PublicKey)
		require.Equal(t, masterPub, storedData.MasterPublicKey)
		require.Equal(t, "test-client", storedData.UserAgent)
		require.Contains(t, storedData.Kinds, 10)
		require.Contains(t, storedData.Kinds, 22)
		require.Contains(t, storedData.Kinds, 33)
	})

	t.Run("Failed delegation - invalid attestation", func(t *testing.T) {
		state := connAuthData{
			Challenge: "challenge123",
		}
		h := newHandler("wss://relay.example.com", "")
		w := &mockWriter{}
		h.ConnAuth.Store(w, state)

		_, masterPub := model.GenerateKeyPair()
		priv := model.GeneratePrivateKey()

		authEvent := &model.Event{
			Event: nostr.Event{
				Kind:      nostr.KindClientAuthentication,
				CreatedAt: nostr.Now(),
				Tags: model.Tags{
					{model.CustomIONTagOnBehalfOf, masterPub},
					{"attestation", "invalid-json"},
					{"challenge", "challenge123"},
					{"relay", "wss://relay.example.com"},
				},
			},
		}
		require.NoError(t, authEvent.SignWithAlg(priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		resp := h.handleAuth(t.Context(), w, authEvent)

		require.False(t, resp.OK)
		require.Contains(t, resp.Reason, "failed to validate on-behalf access:")
	})

	t.Run("Failed delegation - attestation not found", func(t *testing.T) {
		state := connAuthData{
			Challenge: "challenge123",
		}
		h := newHandler("wss://relay.example.com", "")
		w := &mockWriter{}
		h.ConnAuth.Store(w, state)

		_, masterPub := model.GenerateKeyPair()
		priv := model.GeneratePrivateKey()

		authEvent := &model.Event{
			Event: nostr.Event{
				Kind:      nostr.KindClientAuthentication,
				CreatedAt: nostr.Now(),
				Tags: model.Tags{
					{model.CustomIONTagOnBehalfOf, masterPub},
					{"challenge", "challenge123"},
					{"relay", "wss://relay.example.com"},
				},
			},
		}
		require.NoError(t, authEvent.SignWithAlg(priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		resp := h.handleAuth(t.Context(), w, authEvent)
		require.False(t, resp.OK)
		require.Equal(t, errRelayNotAuthoritative.Error(), resp.Reason)
	})
}

func createValidAuthEvent(t *testing.T, priv string, challenge, relayURL string) *model.Event {
	t.Helper()

	authEvent := &model.Event{
		Event: nostr.Event{
			Kind:      nostr.KindClientAuthentication,
			CreatedAt: nostr.Now(),
			Tags: model.Tags{
				{"challenge", challenge},
				{"relay", relayURL},
			},
		},
	}

	require.NoError(t, authEvent.SignWithAlg(priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	return authEvent
}

type mockWriter struct{}

func (*mockWriter) WriteMessage(context.Context, int, []byte) error { return nil }
func (*mockWriter) Close() error                                    { return nil }
