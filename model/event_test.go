// SPDX-License-Identifier: ice License 1.0

package model

import (
	"crypto/ed25519"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/nbd-wtf/go-nostr"
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
