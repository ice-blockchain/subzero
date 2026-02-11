// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"strconv"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/server/auth"
)

func TestValidateOnBehalfAccess(t *testing.T) {
	t.Parallel()

	t.Run("Invalid attestation record", func(t *testing.T) {
		masterPrivKey, masterPubKey := model.GenerateKeyPair()
		priv, pub := model.GenerateKeyPair()
		attestationEv := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindAttestation,
				Tags: model.Tags{
					{"p", pub},
					{"invalid", "tag"},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, attestationEv, masterPrivKey)

		ev := &model.Event{
			Event: nostr.Event{
				Kind: nostr.KindTextNote,
				Tags: model.Tags{{model.CustomIONTagOnBehalfOf, masterPubKey}},
			},
		}
		require.NoError(t, ev.SignWithAlg(priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		_, err := auth.ValidateUserAttestation(t.Context(), ev, attestationEv)
		require.Error(t, err)
	})

	t.Run("No matching attestation record", func(t *testing.T) {
		masterPrivKey, masterPubKey := model.GenerateKeyPair()
		attestationEv := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindAttestation,
				Tags: model.Tags{
					{"p", "foo", "", model.CustomIONAttestationKindActive + ":1"},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, attestationEv, masterPrivKey)

		ev := &model.Event{
			Event: nostr.Event{
				Kind: nostr.KindTextNote,
				Tags: model.Tags{{model.CustomIONTagOnBehalfOf, masterPubKey}},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, model.GeneratePrivateKey())

		_, err := auth.ValidateUserAttestation(t.Context(), ev, attestationEv)
		require.ErrorIs(t, err, model.ErrAttestationRecordNotFound)
	})

	t.Run("Revoked attestation", func(t *testing.T) {
		revokedTime := time.Now().Add(-1 * time.Hour)
		masterPrivKey, masterPubKey := model.GenerateKeyPair()
		priv, pub := model.GenerateKeyPair()
		attestationEv := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindAttestation,
				Tags: model.Tags{
					{"p", pub, "", model.CustomIONAttestationKindRevoked + ":" + strconv.FormatInt(revokedTime.Unix(), 10)},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, attestationEv, masterPrivKey)

		ev := &model.Event{
			Event: nostr.Event{
				Kind: nostr.KindTextNote,
				Tags: model.Tags{{model.CustomIONTagOnBehalfOf, masterPubKey}},
			},
		}
		require.NoError(t, ev.SignWithAlg(priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		_, err := auth.ValidateUserAttestation(t.Context(), ev, attestationEv)
		require.ErrorIs(t, err, model.ErrAttestationRecordRevoked)
	})

	t.Run("Expired attestation", func(t *testing.T) {
		expiredTime := time.Now().Add(-1 * time.Hour)
		masterPrivKey, masterPubKey := model.GenerateKeyPair()
		priv, pub := model.GenerateKeyPair()
		attestationEv := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindAttestation,
				Tags: model.Tags{
					{"p", pub, "", model.CustomIONAttestationKindInactive + ":" + strconv.FormatInt(expiredTime.Unix(), 10)},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, attestationEv, masterPrivKey)

		ev := &model.Event{
			Event: nostr.Event{
				Kind: nostr.KindTextNote,
				Tags: model.Tags{{model.CustomIONTagOnBehalfOf, masterPubKey}},
			},
		}
		require.NoError(t, ev.SignWithAlg(priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		_, err := auth.ValidateUserAttestation(t.Context(), ev, attestationEv)
		require.ErrorIs(t, err, model.ErrAttestationRecordExpired)
	})

	t.Run("Not yet active attestation", func(t *testing.T) {
		startTime := time.Now().Add(1 * time.Hour)
		masterPrivKey, masterPubKey := model.GenerateKeyPair()
		priv, pub := model.GenerateKeyPair()
		attestationEv := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindAttestation,
				Tags: model.Tags{
					{"p", pub, "", model.CustomIONAttestationKindActive + ":" + strconv.FormatInt(startTime.Unix(), 10)},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, attestationEv, masterPrivKey)

		ev := &model.Event{
			Event: nostr.Event{
				Kind: nostr.KindTextNote,
				Tags: model.Tags{{model.CustomIONTagOnBehalfOf, masterPubKey}},
			},
		}
		require.NoError(t, ev.SignWithAlg(priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		_, err := auth.ValidateUserAttestation(t.Context(), ev, attestationEv)
		require.ErrorIs(t, err, model.ErrAttestationRecordIsNotActive)
	})

	t.Run("Valid attestation", func(t *testing.T) {
		masterPrivKey, masterPubKey := model.GenerateKeyPair()
		priv, pub := model.GenerateKeyPair()
		attestationEv := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindAttestation,
				Tags: model.Tags{
					{"p", pub, "", model.CustomIONAttestationKindActive + ":1:1,10,22,33"},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, attestationEv, masterPrivKey)

		ev := &model.Event{
			Event: nostr.Event{
				Kind: nostr.KindTextNote,
				Tags: model.Tags{{model.CustomIONTagOnBehalfOf, masterPubKey}},
			},
		}
		require.NoError(t, ev.SignWithAlg(priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		kinds, err := auth.ValidateUserAttestation(t.Context(), ev, attestationEv)
		require.NoError(t, err)
		require.Contains(t, kinds, 10)
		require.Contains(t, kinds, 22)
		require.Contains(t, kinds, 33)
	})
}
