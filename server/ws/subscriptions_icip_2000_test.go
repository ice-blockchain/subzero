// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

func TestValidateOnBehalfAccess(t *testing.T) {
	ctx := context.Background()
	t.Run("No attestation events", func(t *testing.T) {
		_, masterPubKey := model.GenerateKeyPair()
		ev := &model.Event{
			Event: nostr.Event{
				Kind: nostr.KindTextNote,
				Tags: model.Tags{{model.CustomIONTagOnBehalfOf, masterPubKey}},
			},
		}
		require.NoError(t, ev.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

		_, err := validateOnBehalfAccess(ctx, ev)
		require.ErrorIs(t, err, errAttestationRecordNotFound)
	})

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
		require.NoError(t, attestationEv.SignWithAlg(masterPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, query.AcceptEvents(ctx, attestationEv))

		ev := &model.Event{
			Event: nostr.Event{
				Kind: nostr.KindTextNote,
				Tags: model.Tags{{model.CustomIONTagOnBehalfOf, masterPubKey}},
			},
		}
		require.NoError(t, ev.SignWithAlg(priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		_, err := validateOnBehalfAccess(ctx, ev)
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
		require.NoError(t, attestationEv.SignWithAlg(masterPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, query.AcceptEvents(ctx, attestationEv))

		ev := &model.Event{
			Event: nostr.Event{
				Kind: nostr.KindTextNote,
				Tags: model.Tags{{model.CustomIONTagOnBehalfOf, masterPubKey}},
			},
		}
		require.NoError(t, ev.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

		_, err := validateOnBehalfAccess(ctx, ev)
		require.ErrorIs(t, err, errAttestationRecordNotFound)
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
		require.NoError(t, attestationEv.SignWithAlg(masterPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, query.AcceptEvents(ctx, attestationEv))

		ev := &model.Event{
			Event: nostr.Event{
				Kind: nostr.KindTextNote,
				Tags: model.Tags{{model.CustomIONTagOnBehalfOf, masterPubKey}},
			},
		}
		require.NoError(t, ev.SignWithAlg(priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		_, err := validateOnBehalfAccess(ctx, ev)
		require.ErrorIs(t, err, errAttestationRecordRevoked)
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
		require.NoError(t, attestationEv.SignWithAlg(masterPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, query.AcceptEvents(ctx, attestationEv))

		ev := &model.Event{
			Event: nostr.Event{
				Kind: nostr.KindTextNote,
				Tags: model.Tags{{model.CustomIONTagOnBehalfOf, masterPubKey}},
			},
		}
		require.NoError(t, ev.SignWithAlg(priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		_, err := validateOnBehalfAccess(ctx, ev)
		require.ErrorIs(t, err, errAttestationRecordExpired)
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
		require.NoError(t, attestationEv.SignWithAlg(masterPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, query.AcceptEvents(ctx, attestationEv))

		ev := &model.Event{
			Event: nostr.Event{
				Kind: nostr.KindTextNote,
				Tags: model.Tags{{model.CustomIONTagOnBehalfOf, masterPubKey}},
			},
		}
		require.NoError(t, ev.SignWithAlg(priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		_, err := validateOnBehalfAccess(ctx, ev)
		require.ErrorIs(t, err, errAttestationRecordIsNotActive)
	})

	t.Run("Valid attestation", func(t *testing.T) {
		masterPrivKey, masterPubKey := model.GenerateKeyPair()
		priv, pub := model.GenerateKeyPair()
		attestationEv := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindAttestation,
				Tags: model.Tags{
					{"p", pub, "", model.CustomIONAttestationKindActive + ":1:10,22,33"},
				},
			},
		}
		require.NoError(t, attestationEv.SignWithAlg(masterPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, query.AcceptEvents(ctx, attestationEv))

		ev := &model.Event{
			Event: nostr.Event{
				Kind: nostr.KindTextNote,
				Tags: model.Tags{{model.CustomIONTagOnBehalfOf, masterPubKey}},
			},
		}
		require.NoError(t, ev.SignWithAlg(priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		kinds, err := validateOnBehalfAccess(ctx, ev)
		require.NoError(t, err)
		require.Contains(t, kinds, 10)
		require.Contains(t, kinds, 22)
		require.Contains(t, kinds, 33)
	})
}
