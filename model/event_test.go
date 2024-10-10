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
			err := ev.SignWithAlg("", EventSignAlg("unknown"), EventKeyAlg("unknown"))
			require.ErrorIs(t, err, ErrUnsupportedAlg)
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
