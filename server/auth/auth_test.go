// SPDX-License-Identifier: ice License 1.0

package auth

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/validation"
)

func TestMain(m *testing.M) {
	validation.MustInit(context.Background(), validation.WithIONIdentityPublicKeys(func() []string { return []string{} }))
	os.Exit(m.Run())
}

type attestationOptions struct {
	kind           int
	attestedPubKey string
	action         string
	allowedKinds   string
}

func helperNewAttestationEvent(t *testing.T, masterPrivKey string, opts attestationOptions) *model.Event {
	t.Helper()

	if opts.kind == 0 {
		opts.kind = model.CustomIONKindAttestation
	}
	if opts.action == "" {
		opts.action = model.CustomIONAttestationKindActive
	}

	actionStr := opts.action + ":1"
	if opts.allowedKinds != "" {
		actionStr += ":" + opts.allowedKinds
	}

	event := &model.Event{Event: nostr.Event{
		Kind:      opts.kind,
		CreatedAt: nostr.Now(),
	}}

	if opts.attestedPubKey != "" {
		event.Tags = append(event.Tags, model.Tag{model.TagAttestationName, opts.attestedPubKey, "", actionStr})
	}

	require.NoError(t, event.SignWithAlg(masterPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	return event
}

type relayMetadataOptions struct {
	kind         int
	masterPubKey string
	relayURLs    []string
}

func helperNewRelayMetadataEvent(t *testing.T, signerPrivKey string, opts relayMetadataOptions) *model.Event {
	t.Helper()

	if opts.kind == 0 {
		opts.kind = nostr.KindRelayListMetadata
	}

	tags := model.Tags{}
	if opts.masterPubKey != "" {
		tags = append(tags, model.Tag{model.CustomIONTagOnBehalfOf, opts.masterPubKey})
	}
	for _, url := range opts.relayURLs {
		tags = append(tags, model.Tag{"r", url})
	}

	event := &model.Event{Event: nostr.Event{
		Kind:      opts.kind,
		CreatedAt: nostr.Now(),
		Tags:      tags,
	}}
	require.NoError(t, event.SignWithAlg(signerPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	return event
}

func TestValidateUserAccessFromTags(t *testing.T) {
	t.Parallel()

	const mainRelayURL = "wss://example.com"

	masterPrivKey, masterPubKey := model.GenerateKeyPair()
	delegatedPrivKey, delegatedPubKey := model.GenerateKeyPair()

	t.Run("Successful validation with relay in list", func(t *testing.T) {
		attestationEvent := helperNewAttestationEvent(t, masterPrivKey, attestationOptions{
			attestedPubKey: delegatedPubKey,
		})

		relayMetadataEvent := helperNewRelayMetadataEvent(t, masterPrivKey, relayMetadataOptions{
			masterPubKey: masterPubKey,
			relayURLs:    []string{mainRelayURL, "wss://other-relay.com"},
		})

		userEvent := &model.Event{Event: nostr.Event{
			Kind:      nostr.KindTextNote,
			CreatedAt: nostr.Now(),
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, masterPubKey},
			},
		}}
		require.NoError(t, userEvent.SignWithAlg(delegatedPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		allowedKinds, authoritative, err := validateUserAccessFromTags(t.Context(), mainRelayURL, userEvent, attestationEvent, relayMetadataEvent)

		require.NoError(t, err)
		require.True(t, authoritative)
		require.Empty(t, allowedKinds)
	})
	t.Run("Relay not in user's relay list", func(t *testing.T) {
		attestationEvent := helperNewAttestationEvent(t, masterPrivKey, attestationOptions{
			attestedPubKey: delegatedPubKey,
		})

		relayMetadataEvent := helperNewRelayMetadataEvent(t, masterPrivKey, relayMetadataOptions{
			masterPubKey: masterPubKey,
			relayURLs:    []string{"wss://other-relay.com", "wss://another-relay.com"},
		})

		userEvent := &model.Event{Event: nostr.Event{
			Kind:      nostr.KindTextNote,
			CreatedAt: nostr.Now(),
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, masterPubKey},
			},
		}}
		require.NoError(t, userEvent.SignWithAlg(delegatedPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		allowedKinds, authoritative, err := validateUserAccessFromTags(t.Context(), mainRelayURL, userEvent, attestationEvent, relayMetadataEvent)

		require.NoError(t, err)
		require.False(t, authoritative)
		require.Empty(t, allowedKinds)
	})
	t.Run("Invalid attestation event kind", func(t *testing.T) {
		attestationEvent := helperNewAttestationEvent(t, masterPrivKey, attestationOptions{
			kind:           nostr.KindTextNote, // Wrong kind.
			attestedPubKey: delegatedPubKey,
		})

		relayMetadataEvent := helperNewRelayMetadataEvent(t, masterPrivKey, relayMetadataOptions{
			masterPubKey: masterPubKey,
			relayURLs:    []string{mainRelayURL},
		})

		userEvent := &model.Event{Event: nostr.Event{
			Kind:      nostr.KindTextNote,
			CreatedAt: nostr.Now(),
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, masterPubKey},
			},
		}}
		require.NoError(t, userEvent.SignWithAlg(delegatedPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		allowedKinds, authoritative, err := validateUserAccessFromTags(t.Context(), mainRelayURL, userEvent, attestationEvent, relayMetadataEvent)

		require.Error(t, err)
		require.Contains(t, err.Error(), "unexpected kind")
		require.False(t, authoritative)
		require.Empty(t, allowedKinds)
	})
	t.Run("Invalid relay metadata event kind", func(t *testing.T) {
		attestationEvent := helperNewAttestationEvent(t, masterPrivKey, attestationOptions{
			attestedPubKey: delegatedPubKey,
		})

		relayMetadataEvent := helperNewRelayMetadataEvent(t, masterPrivKey, relayMetadataOptions{
			kind:         nostr.KindTextNote, // Wrong kind.
			masterPubKey: masterPubKey,
			relayURLs:    []string{mainRelayURL},
		})

		userEvent := &model.Event{Event: nostr.Event{
			Kind:      nostr.KindTextNote,
			CreatedAt: nostr.Now(),
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, masterPubKey},
			},
		}}
		require.NoError(t, userEvent.SignWithAlg(delegatedPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		allowedKinds, authoritative, err := validateUserAccessFromTags(t.Context(), mainRelayURL, userEvent, attestationEvent, relayMetadataEvent)

		require.Error(t, err)
		require.Contains(t, err.Error(), "relay metadata event has unexpected kind")
		require.False(t, authoritative)
		require.Empty(t, allowedKinds)
	})
	t.Run("Attestation author mismatch with user master key", func(t *testing.T) {
		otherPrivKey := model.GeneratePrivateKey()

		// Attestation signed by different master key.
		attestationEvent := helperNewAttestationEvent(t, otherPrivKey, attestationOptions{
			attestedPubKey: delegatedPubKey,
		})

		relayMetadataEvent := helperNewRelayMetadataEvent(t, masterPrivKey, relayMetadataOptions{
			masterPubKey: masterPubKey,
			relayURLs:    []string{mainRelayURL},
		})

		userEvent := &model.Event{Event: nostr.Event{
			Kind:      nostr.KindTextNote,
			CreatedAt: nostr.Now(),
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, masterPubKey},
			},
		}}
		require.NoError(t, userEvent.SignWithAlg(delegatedPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		allowedKinds, authoritative, err := validateUserAccessFromTags(t.Context(), mainRelayURL, userEvent, attestationEvent, relayMetadataEvent)

		require.Error(t, err)
		require.Contains(t, err.Error(), "unexpected author")
		require.False(t, authoritative)
		require.Empty(t, allowedKinds)
	})
	t.Run("Relay metadata author mismatch with attestation", func(t *testing.T) {
		otherPrivKey, otherPubKey := model.GenerateKeyPair()

		attestationEvent := helperNewAttestationEvent(t, masterPrivKey, attestationOptions{
			attestedPubKey: delegatedPubKey,
		})

		// Relay metadata signed by different user.
		relayMetadataEvent := helperNewRelayMetadataEvent(t, otherPrivKey, relayMetadataOptions{
			masterPubKey: otherPubKey,
			relayURLs:    []string{mainRelayURL},
		})

		userEvent := &model.Event{Event: nostr.Event{
			Kind:      nostr.KindTextNote,
			CreatedAt: nostr.Now(),
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, otherPubKey},
			},
		}}
		require.NoError(t, userEvent.SignWithAlg(delegatedPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		allowedKinds, authoritative, err := validateUserAccessFromTags(t.Context(), mainRelayURL, userEvent, attestationEvent, relayMetadataEvent)

		require.Error(t, err)
		require.Contains(t, err.Error(), "unexpected author")
		require.False(t, authoritative)
		require.Empty(t, allowedKinds)
	})
	t.Run("Relay metadata author mismatch with user master key", func(t *testing.T) {
		_, otherMasterPubKey := model.GenerateKeyPair()

		attestationEvent := helperNewAttestationEvent(t, masterPrivKey, attestationOptions{
			attestedPubKey: delegatedPubKey,
		})

		relayMetadataEvent := helperNewRelayMetadataEvent(t, masterPrivKey, relayMetadataOptions{
			masterPubKey: masterPubKey,
			relayURLs:    []string{mainRelayURL},
		})

		// User event with different master key.
		userEvent := &model.Event{Event: nostr.Event{
			Kind:      nostr.KindTextNote,
			CreatedAt: nostr.Now(),
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, otherMasterPubKey},
			},
		}}
		require.NoError(t, userEvent.SignWithAlg(delegatedPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		allowedKinds, authoritative, err := validateUserAccessFromTags(t.Context(), mainRelayURL, userEvent, attestationEvent, relayMetadataEvent)

		require.Error(t, err)
		require.Contains(t, err.Error(), "attestation event has unexpected author")
		require.False(t, authoritative)
		require.Empty(t, allowedKinds)
	})
	t.Run("Revoked attestation", func(t *testing.T) {
		attestationEvent := helperNewAttestationEvent(t, masterPrivKey, attestationOptions{
			attestedPubKey: delegatedPubKey,
			action:         model.CustomIONAttestationKindRevoked,
		})

		relayMetadataEvent := helperNewRelayMetadataEvent(t, masterPrivKey, relayMetadataOptions{
			masterPubKey: masterPubKey,
			relayURLs:    []string{mainRelayURL},
		})

		userEvent := &model.Event{Event: nostr.Event{
			Kind:      nostr.KindTextNote,
			CreatedAt: nostr.Now(),
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, masterPubKey},
			},
		}}
		require.NoError(t, userEvent.SignWithAlg(delegatedPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		allowedKinds, authoritative, err := validateUserAccessFromTags(t.Context(), mainRelayURL, userEvent, attestationEvent, relayMetadataEvent)

		require.Error(t, err)
		require.False(t, authoritative)
		require.Empty(t, allowedKinds)
	})
	t.Run("Attestation with specific allowed kinds", func(t *testing.T) {
		attestationEvent := helperNewAttestationEvent(t, masterPrivKey, attestationOptions{
			attestedPubKey: delegatedPubKey,
			allowedKinds:   "1,6,7",
		})

		relayMetadataEvent := helperNewRelayMetadataEvent(t, masterPrivKey, relayMetadataOptions{
			masterPubKey: masterPubKey,
			relayURLs:    []string{mainRelayURL},
		})

		userEvent := &model.Event{Event: nostr.Event{
			Kind:      nostr.KindTextNote,
			CreatedAt: nostr.Now(),
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, masterPubKey},
			},
		}}
		require.NoError(t, userEvent.SignWithAlg(delegatedPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		allowedKinds, authoritative, err := validateUserAccessFromTags(t.Context(), mainRelayURL, userEvent, attestationEvent, relayMetadataEvent)

		require.NoError(t, err)
		require.True(t, authoritative)
		require.NotEmpty(t, allowedKinds)
		require.Contains(t, allowedKinds, 1)
		require.Contains(t, allowedKinds, 6)
		require.Contains(t, allowedKinds, 7)
		require.NotContains(t, allowedKinds, 0)
	})
	t.Run("Relay URL comparison case insensitive", func(t *testing.T) {
		attestationEvent := helperNewAttestationEvent(t, masterPrivKey, attestationOptions{
			attestedPubKey: delegatedPubKey,
		})

		relayMetadataEvent := helperNewRelayMetadataEvent(t, masterPrivKey, relayMetadataOptions{
			masterPubKey: masterPubKey,
			relayURLs:    []string{strings.ToUpper(mainRelayURL)},
		})

		userEvent := &model.Event{Event: nostr.Event{
			Kind:      nostr.KindTextNote,
			CreatedAt: nostr.Now(),
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, masterPubKey},
			},
		}}
		require.NoError(t, userEvent.SignWithAlg(delegatedPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		allowedKinds, authoritative, err := validateUserAccessFromTags(t.Context(), mainRelayURL, userEvent, attestationEvent, relayMetadataEvent)

		require.NoError(t, err)
		require.True(t, authoritative)
		require.Empty(t, allowedKinds)
	})
	t.Run("Empty relay list", func(t *testing.T) {
		attestationEvent := helperNewAttestationEvent(t, masterPrivKey, attestationOptions{
			attestedPubKey: delegatedPubKey,
		})

		relayMetadataEvent := helperNewRelayMetadataEvent(t, masterPrivKey, relayMetadataOptions{
			masterPubKey: masterPubKey,
			relayURLs:    []string{}, // Empty.
		})

		userEvent := &model.Event{Event: nostr.Event{
			Kind:      nostr.KindTextNote,
			CreatedAt: nostr.Now(),
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, masterPubKey},
			},
		}}
		require.NoError(t, userEvent.SignWithAlg(delegatedPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		allowedKinds, authoritative, err := validateUserAccessFromTags(t.Context(), mainRelayURL, userEvent, attestationEvent, relayMetadataEvent)

		require.Error(t, err)
		require.False(t, authoritative)
		require.Empty(t, allowedKinds)
	})
	t.Run("No relay metadata", func(t *testing.T) {
		attestationEvent := helperNewAttestationEvent(t, masterPrivKey, attestationOptions{
			attestedPubKey: delegatedPubKey,
		})

		userEvent := &model.Event{Event: nostr.Event{
			Kind:      nostr.KindTextNote,
			CreatedAt: nostr.Now(),
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, masterPubKey},
			},
		}}
		require.NoError(t, userEvent.SignWithAlg(delegatedPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		allowedKinds, authoritative, err := validateUserAccessFromTags(t.Context(), mainRelayURL, userEvent, attestationEvent, nil)

		require.NoError(t, err)
		require.False(t, authoritative)
		require.Empty(t, allowedKinds)
	})
	t.Run("Master user without on-behalf-of tag", func(t *testing.T) {
		attestationEvent := helperNewAttestationEvent(t, masterPrivKey, attestationOptions{})
		relayMetadataEvent := helperNewRelayMetadataEvent(t, masterPrivKey, relayMetadataOptions{
			relayURLs: []string{mainRelayURL},
		})

		// User event without on-behalf-of tag (master user).
		userEvent := &model.Event{Event: nostr.Event{
			Kind:      nostr.KindTextNote,
			CreatedAt: nostr.Now(),
			Content:   "Master user posting directly",
			Tags:      model.Tags{},
		}}
		require.NoError(t, userEvent.SignWithAlg(masterPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		allowedKinds, authoritative, err := validateUserAccessFromTags(t.Context(), mainRelayURL, userEvent, attestationEvent, relayMetadataEvent)

		require.NoError(t, err)
		require.True(t, authoritative)
		require.Empty(t, allowedKinds)
	})
	t.Run("Multiple relays in list", func(t *testing.T) {
		attestationEvent := helperNewAttestationEvent(t, masterPrivKey, attestationOptions{
			attestedPubKey: delegatedPubKey,
		})

		relayMetadataEvent := helperNewRelayMetadataEvent(t, masterPrivKey, relayMetadataOptions{
			masterPubKey: masterPubKey,
			relayURLs:    []string{"wss://first-relay.com", "wss://second-relay.com", mainRelayURL, "wss://fourth-relay.com"},
		})

		userEvent := &model.Event{Event: nostr.Event{
			Kind:      nostr.KindTextNote,
			CreatedAt: nostr.Now(),
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, masterPubKey},
			},
		}}
		require.NoError(t, userEvent.SignWithAlg(delegatedPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		allowedKinds, authoritative, err := validateUserAccessFromTags(t.Context(), mainRelayURL, userEvent, attestationEvent, relayMetadataEvent)

		require.NoError(t, err)
		require.True(t, authoritative)
		require.Empty(t, allowedKinds)
	})
	t.Run("Relay metadata master pubkey differs from attestation author", func(t *testing.T) {
		_, differentPubKey := model.GenerateKeyPair()

		attestationEvent := helperNewAttestationEvent(t, masterPrivKey, attestationOptions{
			attestedPubKey: delegatedPubKey,
		})

		// Relay metadata has on-behalf-of tag pointing to a different master pubkey than attestation author.
		relayMetadataEvent := helperNewRelayMetadataEvent(t, masterPrivKey, relayMetadataOptions{
			masterPubKey: differentPubKey,
			relayURLs:    []string{mainRelayURL},
		})

		userEvent := &model.Event{Event: nostr.Event{
			Kind:      nostr.KindTextNote,
			CreatedAt: nostr.Now(),
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, masterPubKey}, // Matches attestation.PubKey so attestation validation passes.
			},
		}}
		require.NoError(t, userEvent.SignWithAlg(delegatedPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		allowedKinds, authoritative, err := validateUserAccessFromTags(t.Context(), mainRelayURL, userEvent, attestationEvent, relayMetadataEvent)

		require.Error(t, err)
		require.Contains(t, err.Error(), "relay metadata event has unexpected author")
		require.Contains(t, err.Error(), differentPubKey)
		require.Contains(t, err.Error(), masterPubKey)
		require.False(t, authoritative)
		require.Empty(t, allowedKinds)
	})
}
