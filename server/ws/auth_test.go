// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"context"
	"encoding/json"
	"math/rand/v2"
	"net"
	"sync"
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/server/auth"
	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
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
		require.Contains(t, resp.Reason, "no challenge")
	})

	t.Run("Already authenticated with different public key", func(t *testing.T) {
		state := model.UserDataContext{
			Authenticated: true,
			PublicKey:     "foo",
		}
		h := newHandler("wss://relay.example.com", "")
		w := &mockWriter{}

		challenge := authConnGenerateChallenge(w)
		t.Logf("Generated challenge: %s", challenge)
		authConnSet(w, state)
		authEvent := createValidAuthEvent(t, model.GeneratePrivateKey(), challenge, "wss://relay.example.com")
		resp := h.handleAuth(t.Context(), w, authEvent)
		require.False(t, resp.OK)
		require.Contains(t, resp.Reason, "already authenticated with a different public key")
	})

	t.Run("Invalid auth event", func(t *testing.T) {
		h := newHandler("wss://relay.example.com", "")
		w := &mockWriter{}

		authConnGenerateChallenge(w)
		authEvent := createValidAuthEvent(t, model.GeneratePrivateKey(), "wrong-challenge", "wss://relay.example.com")
		resp := h.handleAuth(t.Context(), w, authEvent)

		require.False(t, resp.OK)
		require.Contains(t, resp.Reason, "failed to validate auth event:")

		state := authConnGetState(w)
		require.False(t, state.Authenticated)
	})

	t.Run("Successful authentication - master key", func(t *testing.T) {
		h := newHandler("wss://relay.example.com", "")
		w := &mockWriter{}

		challenge := authConnGenerateChallenge(w)
		priv, pub := model.GenerateKeyPair()
		authEvent := createValidAuthEvent(t, priv, challenge, "wss://relay.example.com")
		resp := h.handleAuth(t.Context(), w, authEvent)
		require.True(t, resp.OK)
		require.Empty(t, resp.Reason)

		storedData := authConnGetState(w)
		require.True(t, storedData.Authenticated)
		require.Equal(t, pub, storedData.PublicKey)
		require.Equal(t, pub, storedData.MasterPublicKey)
		require.Equal(t, challenge, authConnGetChallenge(w))
	})

	t.Run("Successful authentication - different ports - master key", func(t *testing.T) {
		h := newHandler("wss://relay.example.com", "")
		w := &mockWriter{}

		challenge := authConnGenerateChallenge(w)
		priv, pub := model.GenerateKeyPair()
		authEvent := createValidAuthEvent(t, priv, challenge, "wss://relay.example.com:898")
		resp := h.handleAuth(t.Context(), w, authEvent)
		require.True(t, resp.OK)
		require.Empty(t, resp.Reason)

		storedData := authConnGetState(w)
		require.True(t, storedData.Authenticated)
		require.Equal(t, pub, storedData.PublicKey)
		require.Equal(t, pub, storedData.MasterPublicKey)
		require.Equal(t, challenge, authConnGetChallenge(w))
	})

	t.Run("Successful authentication - with delegation", func(t *testing.T) {
		h := newHandler("wss://relay.example.com", "")
		w := &mockWriter{}

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

		challenge := authConnGenerateChallenge(w)

		authEvent := &model.Event{
			Event: nostr.Event{
				Kind:      nostr.KindClientAuthentication,
				CreatedAt: nostr.Now(),
				Tags: model.Tags{
					{model.CustomIONTagOnBehalfOf, masterPub},
					{"attestation", string(attestationJSON)},
					{"challenge", challenge},
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
		storedData := authConnGetState(w)
		require.True(t, storedData.Authenticated)
		require.Equal(t, pub, storedData.PublicKey)
		require.Equal(t, masterPub, storedData.MasterPublicKey)
		require.Equal(t, "test-client", storedData.UserAgent)
		require.Contains(t, storedData.Kinds, 10)
		require.Contains(t, storedData.Kinds, 22)
		require.Contains(t, storedData.Kinds, 33)
	})

	t.Run("Failed delegation - invalid attestation", func(t *testing.T) {
		h := newHandler("wss://relay.example.com", "")
		w := &mockWriter{}

		_, masterPub := model.GenerateKeyPair()
		priv := model.GeneratePrivateKey()

		challenge := authConnGenerateChallenge(w)
		authEvent := &model.Event{
			Event: nostr.Event{
				Kind:      nostr.KindClientAuthentication,
				CreatedAt: nostr.Now(),
				Tags: model.Tags{
					{model.CustomIONTagOnBehalfOf, masterPub},
					{"attestation", "invalid-json"},
					{"challenge", challenge},
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
		h := newHandler("wss://relay.example.com", "")
		w := &mockWriter{}

		_, masterPub := model.GenerateKeyPair()
		priv := model.GeneratePrivateKey()

		challenge := authConnGenerateChallenge(w)

		authEvent := &model.Event{
			Event: nostr.Event{
				Kind:      nostr.KindClientAuthentication,
				CreatedAt: nostr.Now(),
				Tags: model.Tags{
					{model.CustomIONTagOnBehalfOf, masterPub},
					{"challenge", challenge},
					{"relay", "wss://relay.example.com"},
				},
			},
		}
		require.NoError(t, authEvent.SignWithAlg(priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		resp := h.handleAuth(t.Context(), w, authEvent)
		require.False(t, resp.OK)
		require.Equal(t, auth.ErrRelayNotAuthoritative.Error(), resp.Reason)
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

type mockWriter struct {
	adapters.MetadataHander
	Remote     net.Addr
	RemoteOnce sync.Once
}

func (*mockWriter) WriteMessage(context.Context, int, []byte) error { return nil }
func (*mockWriter) Close() error                                    { return nil }

func (m *mockWriter) RemoteAddr() net.Addr {
	m.RemoteOnce.Do(func() {
		m.Remote = &net.TCPAddr{IP: net.IPv4(127, 0, 0, byte(rand.Uint32N(253)+1)), Port: rand.IntN(0xffff) + 1}
	})
	return m.Remote
}
