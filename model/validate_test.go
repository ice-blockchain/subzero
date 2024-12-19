// SPDX-License-Identifier: ice License 1.0

package model

import (
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

func TestValidateGiftWrap(t *testing.T) {
	t.Parallel()

	key := GeneratePrivateKey()

	var ev Event
	ev.Kind = nostr.KindGiftWrap
	ev.CreatedAt = 1
	require.NoError(t, ev.SignWithAlg(key, SignAlgEDDSA, KeyAlgCurve25519))
	require.Error(t, ev.Validate())

	ev.Tags = append(ev.Tags, Tag{"p", "foop"}, Tag{"k", "123"})
	require.NoError(t, ev.SignWithAlg(key, SignAlgEDDSA, KeyAlgCurve25519))
	require.Error(t, ev.Validate())

	ev.Tags = append(ev.Tags, Tag{"expiration", "foo"})
	require.NoError(t, ev.SignWithAlg(key, SignAlgEDDSA, KeyAlgCurve25519))
	require.Error(t, ev.Validate())

	ev.Tags = append(ev.Tags[:len(ev.Tags)-2], Tag{"expiration", "123"})
	require.NoError(t, ev.SignWithAlg(key, SignAlgEDDSA, KeyAlgCurve25519))
	require.Error(t, ev.Validate())
}
