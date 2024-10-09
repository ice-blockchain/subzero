// SPDX-License-Identifier: ice License 1.0

package model

import (
	"crypto/ed25519"
	"encoding/hex"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

func helperTestSignVerify(t *testing.T, ev *Event, pk string, alg EventSignAlg) {
	t.Helper()

	t.Run("Sign", func(t *testing.T) {
		err := ev.SignWithAlg(pk, alg)
		require.NoError(t, err)
		t.Logf("%v: signature: %s", alg, ev.Sig)
	})
	t.Run("Verify", func(t *testing.T) {
		ok, err := ev.CheckSignature()
		require.NoError(t, err)
		require.True(t, ok)
		t.Logf("%v: signature: %s: OK", alg, ev.Sig)
	})
}

func TestEventSignVerify(t *testing.T) {
	t.Run(string(SignAlgSchnorr), func(t *testing.T) {
		var ev Event
		ev.Kind = nostr.KindTextNote
		ev.CreatedAt = 1
		ev.Content = string(SignAlgSchnorr)

		pk := nostr.GeneratePrivateKey()
		require.NotEmpty(t, pk)
		helperTestSignVerify(t, &ev, pk, SignAlgSchnorr)
	})
	t.Run(string(SignAlgEDDSA), func(t *testing.T) {
		var ev Event
		ev.Kind = nostr.KindTextNote
		ev.CreatedAt = 1
		ev.Content = string(SignAlgEDDSA)

		_, pk, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)
		helperTestSignVerify(t, &ev, hex.EncodeToString(pk), SignAlgEDDSA)
	})
	t.Run(string(SignAlgECDSA), func(t *testing.T) {
		var ev Event
		ev.Kind = nostr.KindTextNote
		ev.CreatedAt = 1
		ev.Content = string(SignAlgECDSA)

		pk, err := btcec.NewPrivateKey()
		require.NoError(t, err)
		require.NotNil(t, pk)

		helperTestSignVerify(t, &ev, hex.EncodeToString(pk.Serialize()), SignAlgECDSA)
	})
	t.Run("Unknown", func(t *testing.T) {
		t.Run("Sign", func(t *testing.T) {
			var ev Event
			err := ev.SignWithAlg("", EventSignAlg("unknown"))
			require.ErrorIs(t, err, ErrUnsupportedSignatureAlg)
		})
		t.Run("Verify", func(t *testing.T) {
			var ev Event
			ev.Sig = "unknown:" + hex.EncodeToString([]byte("signature"))
			ok, err := ev.CheckSignature()
			require.ErrorIs(t, err, ErrUnsupportedSignatureAlg)
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
		helperTestSignVerify(t, &ev, pk, "")
	})
}
