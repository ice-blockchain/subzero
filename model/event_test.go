// SPDX-License-Identifier: ice License 1.0

package model

import (
	"crypto/ed25519"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip44"
	"github.com/stretchr/testify/require"
)

func helperTestSignVerify(t *testing.T, ev *Event, pk string, signAlg EventSignAlg, keyAlg EventKeyAlg) {
	t.Helper()

	t.Run("Sign", func(t *testing.T) {
		err := ev.SignWithAlg(pk, signAlg, keyAlg)
		require.NoError(t, err)
		t.Logf("%v: signature: %s", signAlg, ev.Sig)
	})
	t.Run("Verify", func(t *testing.T) {
		ok, err := ev.CheckSignature()
		require.NoError(t, err)
		require.True(t, ok)
		t.Logf("%v: signature: %s: OK", signAlg, ev.Sig)
	})
}

func TestEventSignVerify(t *testing.T) {
	t.Parallel()

	t.Run(strings.Join([]string{string(SignAlgSchnorr), string(KeyAlgSecp256k1)}, "_"), func(t *testing.T) {
		var ev Event
		ev.Kind = nostr.KindTextNote
		ev.CreatedAt = 1
		ev.Content = string(SignAlgSchnorr)

		pk := nostr.GeneratePrivateKey()
		require.NotEmpty(t, pk)
		helperTestSignVerify(t, &ev, pk, SignAlgSchnorr, KeyAlgSecp256k1)
	})
	t.Run(strings.Join([]string{string(SignAlgEDDSA), string(KeyAlgCurve25519)}, "_"), func(t *testing.T) {
		var ev Event
		ev.Kind = nostr.KindTextNote
		ev.CreatedAt = 1
		ev.Content = string(SignAlgEDDSA)

		_, pk, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)
		helperTestSignVerify(t, &ev, hex.EncodeToString(pk), SignAlgEDDSA, KeyAlgCurve25519)
	})
	t.Run("ArgError", func(t *testing.T) {
		t.Run("Sign", func(t *testing.T) {
			var ev Event
			err := ev.SignWithAlg("", SignAlgEDDSA, "")
			require.ErrorIs(t, err, ErrUnsupportedAlg)
		})
		t.Run("Verify", func(t *testing.T) {
			testData := []string{
				"foo",
				"/foo",
				"/",
				"foo/",
				"",
				"/",
				"//",
			}
			for i := range testData {
				var ev Event
				ev.Sig = testData[i] + ":" + hex.EncodeToString([]byte("signature"))
				ok, err := ev.CheckSignature()
				require.ErrorIsf(t, err, ErrUnsupportedAlg, "testData[%d]: %s", i, testData[i])
				require.False(t, ok)

			}
		})
	})
	t.Run("Unknown", func(t *testing.T) {
		t.Run("Sign", func(t *testing.T) {
			var ev Event
			pk := hex.EncodeToString([]byte("private key"))
			require.ErrorIs(t, ev.SignWithAlg(pk, EventSignAlg("unknown"), EventKeyAlg("unknown")), ErrUnsupportedAlg)
			require.ErrorIs(t, ev.SignWithAlg(pk, SignAlgSchnorr, KeyAlgCurve25519), ErrUnsupportedAlg)
			require.ErrorIs(t, ev.SignWithAlg(pk, SignAlgEDDSA, KeyAlgSecp256k1), ErrUnsupportedAlg)
		})
		t.Run("Verify", func(t *testing.T) {
			var ev Event
			ev.Sig = "unknown/foo:" + hex.EncodeToString([]byte("signature"))
			ok, err := ev.CheckSignature()
			require.ErrorIs(t, err, ErrUnsupportedAlg)
			require.False(t, ok)
		})
	})
	t.Run("Default", func(t *testing.T) {
		var ev Event
		ev.Kind = nostr.KindTextNote
		ev.CreatedAt = 1
		ev.Content = "default"

		pk := nostr.GeneratePrivateKey()
		require.NotEmpty(t, pk)
		helperTestSignVerify(t, &ev, pk, "", "")
	})
}

func TestDeduplicateSlice(t *testing.T) {
	t.Parallel()

	t.Run("Events", func(t *testing.T) {
		events := []*Event{
			{
				Event: nostr.Event{
					Kind: nostr.KindTextNote,
					ID:   "id1",
				},
			},
			{
				Event: nostr.Event{
					ID:   "id2",
					Kind: nostr.KindTextNote,
				},
			},
			{
				Event: nostr.Event{
					ID:   "id1",
					Kind: nostr.KindTextNote,
				},
			},
			{
				Event: nostr.Event{
					ID:   "id3",
					Kind: nostr.KindTextNote,
				},
			},
			{
				Event: nostr.Event{
					ID:   "id2",
					Kind: nostr.KindTextNote,
				},
			},
			{
				Event: nostr.Event{
					ID:   "id4",
					Kind: nostr.KindTextNote,
				},
			},
			{
				Event: nostr.Event{
					ID:   "id3",
					Kind: nostr.KindTextNote,
				},
			},
		}
		deduplicated := DeduplicateSlice(events, func(e *Event) string { return e.ID })
		require.Len(t, deduplicated, 4)
		require.Equal(t, "id1", deduplicated[0].ID)
		require.Equal(t, "id2", deduplicated[1].ID)
		require.Equal(t, "id3", deduplicated[2].ID)
		require.Equal(t, "id4", deduplicated[3].ID)
	})

	t.Run("String", func(t *testing.T) {
		ids := []string{"id1", "id2", "id1", "id3", "id2", "id4", "id3"}
		deduplicated := DeduplicateSlice(ids, func(e string) string { return e })
		require.Len(t, deduplicated, 4)
		require.Equal(t, "id1", deduplicated[0])
		require.Equal(t, "id2", deduplicated[1])
		require.Equal(t, "id3", deduplicated[2])
		require.Equal(t, "id4", deduplicated[3])
	})
}
func TestSplitBatch(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		result := SplitBatch([]int{}, 2)
		require.Len(t, result, 1)
		require.Empty(t, result[0])
	})

	t.Run("Smaller than batch", func(t *testing.T) {
		input := []int{1, 2}
		result := SplitBatch(input, 3)
		require.Len(t, result, 1)
		require.Equal(t, input, result[0])
	})

	t.Run("Equal to batch", func(t *testing.T) {
		input := []int{1, 2, 3}
		result := SplitBatch(input, 3)
		require.Len(t, result, 1)
		require.Equal(t, input, result[0])
	})

	t.Run("Multiple batches", func(t *testing.T) {
		input := []int{1, 2, 3, 4, 5, 6, 7}
		result := SplitBatch(input, 3)
		require.Len(t, result, 3)
		require.Equal(t, []int{1, 2, 3}, result[0])
		require.Equal(t, []int{4, 5, 6}, result[1])
		require.Equal(t, []int{7}, result[2])
	})
}

func TestDecryptToken(t *testing.T) {
	t.Parallel()

	privKey, pubKey := GenerateKeyPair()

	t.Run("successful decryption of token", func(t *testing.T) {
		t.Parallel()
		var ev Event
		ev.Kind = nostr.KindTextNote
		ev.PubKey = pubKey

		privKeyX25519, err := nip44.ConvertEd25519PrivateKeyToX25519(privKey)
		require.NoError(t, err)

		conversationKey, err := nip44.GenerateConversationKeyX25519(privKeyX25519, pubKey)
		require.NoError(t, err)

		originalToken := "test-device-token-123456"
		encryptedToken, err := nip44.EncryptX25519(originalToken, conversationKey, nil)
		require.NoError(t, err)

		ev.Tags = Tags{{"token", encryptedToken}}

		decryptedToken, err := ev.DecryptToken(privKey)
		require.NoError(t, err)
		require.Equal(t, originalToken, decryptedToken)
	})

	t.Run("token is missing", func(t *testing.T) {
		t.Parallel()
		var ev Event
		ev.Kind = nostr.KindTextNote
		ev.PubKey = pubKey

		decryptedToken, err := ev.DecryptToken(privKey)
		require.NoError(t, err)
		require.Empty(t, decryptedToken)
	})

	t.Run("wrong private key", func(t *testing.T) {
		t.Parallel()
		var ev Event
		ev.Kind = nostr.KindTextNote
		ev.PubKey = pubKey

		privKeyX25519, err := nip44.ConvertEd25519PrivateKeyToX25519(privKey)
		require.NoError(t, err)

		conversationKey, err := nip44.GenerateConversationKeyX25519(privKeyX25519, pubKey)
		require.NoError(t, err)

		originalToken := "test-device-token-123456"
		encryptedToken, err := nip44.EncryptX25519(originalToken, conversationKey, nil)
		require.NoError(t, err)

		ev.Tags = Tags{{"token", encryptedToken}}
		wrongPrivKey := GeneratePrivateKey()

		_, err = ev.DecryptToken(wrongPrivKey)
		require.Error(t, err)
	})

	t.Run("wrong token format", func(t *testing.T) {
		t.Parallel()
		var ev Event
		ev.Kind = nostr.KindTextNote
		ev.PubKey = pubKey

		ev.Tags = Tags{{"token", "not-a-valid-encrypted-token"}}

		_, err := ev.DecryptToken(privKey)
		require.Error(t, err)
	})
}
