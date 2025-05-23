// SPDX-License-Identifier: ice License 1.0

package command

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/ice-blockchain/cometbft/multiplex/client"
	"github.com/ice-blockchain/subzero/database/command/fixture"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

func TestMain(m *testing.M) {
	code := m.Run()
	if code == 0 {
		if err := goleak.Find(
			goleak.IgnoreAnyFunction("github.com/ice-blockchain/cometbft/multiplex.(*MultiplexBackend).metricsReporter"),
		); err != nil {
			fmt.Printf("goleak found issues: %v\n", err)
			code = 1
		}
	}
	os.Exit(code)
}

func TestRollBackOnTxError(t *testing.T) {
	var rolledBack bool
	RegisterRollbackListener(func(ctx context.Context, event ...*model.Event) error {
		rolledBack = true
		return nil
	})
	RegisterAcceptListener(func(ctx context.Context, event ...*model.Event) error {
		require.Fail(t, "Should not be called")
		return nil
	})
	privKeyOfOriginalNote := model.GeneratePrivateKey()
	masterPrivKey, masterPubkey := model.GenerateKeyPair()
	originalEvent := &model.Event{Event: nostr.Event{
		CreatedAt: nostr.Now(),
		Kind:      nostr.KindTextNote,
		Content:   "validEvent",
		Tags: model.Tags{
			{model.CustomIONTagOnBehalfOf, masterPubkey},
		},
	}}
	relaysList := &model.Event{Event: nostr.Event{
		CreatedAt: nostr.Now(),
		Kind:      nostr.KindRelayListMetadata,
		Tags: model.Tags{
			{model.CustomIONTagOnBehalfOf, masterPubkey},
			{"r", "wss://localhost:9988"},
			{"r", "wss://localhost:9977"},
		},
	}}
	require.NoError(t, relaysList.SignWithAlg(masterPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, originalEvent.SignWithAlg(privKeyOfOriginalNote, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	node, release := newConsensusNode(t.Context(), nil, 19999, WithClient(fixture.NewErrornousClient()))
	defer release()
	err := node.broadcastUserEvents(t.Context(), relaysList, originalEvent)
	t.Logf("%v", err)
	require.Error(t, err)
	require.True(t, rolledBack)
}

func TestBroadcastProfileDeletion(t *testing.T) {
	masterPrivKey, masterPubkey := model.GenerateKeyPair()
	delegatedPrivKey, pk := model.GenerateKeyPair()
	attestationEvent := &model.Event{Event: nostr.Event{
		Kind:      model.CustomIONKindAttestation,
		CreatedAt: nostr.Now(),
		Tags: model.Tags{
			{model.TagAttestationName, pk, "", model.CustomIONAttestationKindActive + ":1"},
		},
	}}
	relaysList := &model.Event{Event: nostr.Event{
		CreatedAt: nostr.Now(),
		Kind:      nostr.KindRelayListMetadata,
		Tags: model.Tags{
			{model.CustomIONTagOnBehalfOf, masterPubkey},
			{"r", "wss://localhost:9988"},
			{"r", "wss://localhost:9977"},
		},
	}}
	require.NoError(t, relaysList.SignWithAlg(masterPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, attestationEvent.SignWithAlg(masterPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	profileEvent := &model.Event{Event: nostr.Event{
		CreatedAt: nostr.Now(),
		Kind:      nostr.KindProfileMetadata,
		Tags:      model.Tags{}.AppendUnique(model.Tag{model.CustomIONTagOnBehalfOf, masterPubkey}),
		Content:   "{\"name\": \"bogus\", \"about\":\"bogus\", \"picture\": \"https://bogus.com/pic.jpg\"}",
	}}
	require.NoError(t, profileEvent.SignWithAlg(delegatedPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	var memdb query.MemDB
	require.NoError(t, memdb.AcceptEvents(t.Context(), attestationEvent, relaysList, profileEvent))
	consensusClient := fixture.NewCallbackClient(func(userAddress string, relays []string, transactions ...client.Transaction) {
		require.Fail(t, "Accept should not be called")
	}, func(userAddress string, relays []string, transactions ...client.Transaction) {
		require.Equal(t, masterPubkey, userAddress)
	})
	node, release := newConsensusNode(t.Context(), nil, 19999,
		WithClient(consensusClient),
		WithQuery(memdb.SelectEvents),
	)
	defer release()
	deletionProfile := &model.Event{Event: nostr.Event{
		CreatedAt: nostr.Now(),
		Kind:      nostr.KindDeletion,
		Tags:      model.Tags{},
		PubKey:    masterPubkey,
	}}
	require.NoError(t, profileEvent.SignWithAlg(masterPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, node.broadcastUserEvents(t.Context(), deletionProfile))
}

func TestBroadcastLinkedEvent(t *testing.T) {
	var memdb query.MemDB
	masterPrivKey, masterPubkey := model.GenerateKeyPair()
	priv, pk := model.GenerateKeyPair()
	attestationEvent := &model.Event{Event: nostr.Event{
		Kind:      model.CustomIONKindAttestation,
		CreatedAt: 1,
		Tags: model.Tags{
			{model.TagAttestationName, pk, "", model.CustomIONAttestationKindActive + ":1"},
		},
	}}
	require.NoError(t, attestationEvent.SignWithAlg(masterPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	relaysList := &model.Event{Event: nostr.Event{
		CreatedAt: nostr.Now(),
		Kind:      nostr.KindRelayListMetadata,
		Tags: model.Tags{
			{model.CustomIONTagOnBehalfOf, masterPubkey},
			{"r", "wss://localhost:9988"},
			{"r", "wss://localhost:9977"},
		},
	}}
	require.NoError(t, relaysList.SignWithAlg(masterPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, memdb.AcceptEvents(t.Context(), attestationEvent, relaysList))

	privkeyOfRepostedNote, pubKeyOfRepostedNote := model.GenerateKeyPair()
	masterPrivKeyOfRepostedNote, masterPubkeyOfRepostedNote := model.GenerateKeyPair()

	consensusClient := fixture.NewCallbackClient(func(userAddress string, relays []string, transactions ...client.Transaction) {
		for _, tx := range transactions {
			evs, err := mapTxToEvent(tx)
			require.NoError(t, err)
			unhex, err := hex.DecodeString(masterPubkeyOfRepostedNote)
			require.NoError(t, err)
			addr, err := client.PubKeyToAddress(string(unhex))
			require.NoError(t, err)
			expected := map[int]string{
				nostr.KindTextNote: addr,
				nostr.KindRepost:   addr,
				nostr.KindReaction: addr,
			}
			hasEphemeralAck := false
			for _, ev := range evs {
				if ev.Kind == model.CustomIONKindEphemeralEmbeddding {
					hasEphemeralAck = true
					continue
				}
				require.Equal(t, expected[ev.Kind], userAddress)
			}
			require.True(t, hasEphemeralAck)
		}
	}, func(userAddress string, relays []string, transactions ...client.Transaction) {
		require.Fail(t, "Rollback should not be called")
	})
	node, release := newConsensusNode(t.Context(), nil, 19999,
		WithClient(consensusClient),
		WithQuery(memdb.SelectEvents),
	)
	defer release()

	t.Run("repost", func(t *testing.T) {
		otherUserAttestation := &model.Event{Event: nostr.Event{
			Kind:      model.CustomIONKindAttestation,
			CreatedAt: 1,
			Tags: model.Tags{
				{model.TagAttestationName, pubKeyOfRepostedNote, "", model.CustomIONAttestationKindActive + ":1"},
			},
		}}
		require.NoError(t, otherUserAttestation.SignWithAlg(masterPrivKeyOfRepostedNote, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		otherUserRelaysList := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindRelayListMetadata,
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, masterPubkeyOfRepostedNote},
				{"r", "wss://localhost:9988"},
				{"r", "wss://localhost:9977"},
			},
		}}
		require.NoError(t, otherUserRelaysList.SignWithAlg(masterPrivKeyOfRepostedNote, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, memdb.AcceptEvents(t.Context(), otherUserAttestation, otherUserRelaysList))

		repostedEvent := model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, masterPubkeyOfRepostedNote},
			},
		}}
		require.NoError(t, repostedEvent.SignWithAlg(privkeyOfRepostedNote, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		repostEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindRepost,
			Tags: model.Tags{
				{"e", repostedEvent.ID, "relay"},
				{"p", repostedEvent.GetMasterPublicKey()},
				{model.CustomIONTagOnBehalfOf, masterPubkey}},
			Content: repostedEvent.String(),
		}}
		require.NoError(t, repostEvent.SignWithAlg(priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		ack := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      model.CustomIONKindEphemeralEmbeddding,
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, masterPubkey},
				{"e", repostEvent.ID},
			},
			Content: attestationEvent.String(),
		}}
		require.NoError(t, ack.SignWithAlg(priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, memdb.AcceptEvents(t.Context(), repostEvent, ack))
		require.NoError(t, node.broadcastUserEvents(t.Context(), repostEvent, ack))
	})

	t.Run("reaction", func(t *testing.T) {
		originalEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, masterPubkeyOfRepostedNote},
			},
			Content: "validEvent",
		}}
		require.NoError(t, originalEvent.SignWithAlg(privkeyOfRepostedNote, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, memdb.AcceptEvents(t.Context(), originalEvent))
		reactionEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindReaction,
			Tags: model.Tags{
				{"e", originalEvent.ID},
				{"k", strconv.Itoa(originalEvent.Kind)},
				{"p", originalEvent.PubKey},
				{model.CustomIONTagOnBehalfOf, masterPubkey}},
			Content: "+",
		}}
		require.NoError(t, reactionEvent.SignWithAlg(priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		ack := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      model.CustomIONKindEphemeralEmbeddding,
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, masterPubkey},
				{"e", reactionEvent.ID}},
			Content: attestationEvent.String(),
		}}
		require.NoError(t, ack.SignWithAlg(priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, node.broadcastUserEvents(t.Context(), reactionEvent, ack))
	})
}

func TestServerRestart(t *testing.T) {
	t.Parallel()

	var memdb query.MemDB
	node, release := newConsensusNode(t.Context(), nil, 13999, WithQuery(memdb.SelectEvents))
	defer release()

	for range 5 {
		require.NoError(t, node.Stop(t.Context(), time.Second))
		node.Start(t.Context())
	}
}

func TestBroadcastLinkedEventBadges(t *testing.T) {
	var memdb query.MemDB
	heimdallPrivKey, heimdallPubkey := model.GenerateKeyPair()
	userPrivKey, userPubkey := model.GenerateKeyPair()
	relaysList := &model.Event{Event: nostr.Event{
		CreatedAt: nostr.Now(),
		Kind:      nostr.KindRelayListMetadata,
		Tags: nostr.Tags{
			[]string{model.CustomIONTagOnBehalfOf, userPubkey},
			[]string{"r", "wss://localhost:9988"},
			[]string{"r", "wss://localhost:9977"},
		},
	}}
	require.NoError(t, relaysList.SignWithAlg(userPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, memdb.AcceptEvents(t.Context(), relaysList))

	attestationEvent := &model.Event{Event: nostr.Event{
		Kind:      model.CustomIONKindAttestation,
		CreatedAt: 1,
		Tags: model.Tags{
			{model.TagAttestationName, userPubkey, "", model.CustomIONAttestationKindActive + ":" + strconv.Itoa(int(time.Now().Unix()-10))},
		},
	}}
	require.NoError(t, attestationEvent.SignWithAlg(userPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, memdb.AcceptEvents(t.Context(), attestationEvent))

	consensusClient := fixture.NewCallbackClient(func(userAddress string, relays []string, transactions ...client.Transaction) {
		for _, tx := range transactions {
			evs, err := mapTxToEvent(tx)
			require.NoError(t, err)
			unhex, err := hex.DecodeString(userPubkey)
			require.NoError(t, err)
			addr, err := client.PubKeyToAddress(string(unhex))
			require.NoError(t, err)
			expected := map[int]string{
				nostr.KindBadgeDefinition: addr,
				nostr.KindBadgeAward:      addr,
				nostr.KindProfileMetadata: addr,
			}
			require.Equal(t, expected[evs[0].Kind], userAddress)
		}
	}, func(userAddress string, relays []string, transactions ...client.Transaction) {
		require.Fail(t, "Rollback should not be called")
	})
	node, release := newConsensusNode(t.Context(), nil, 19999,
		WithClient(consensusClient),
		WithQuery(memdb.SelectEvents),
	)
	defer release()

	t.Run("badge", func(t *testing.T) {
		badgeDefinitionEvent := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindBadgeDefinition,
				Tags: nostr.Tags{
					{"d", "verified"},
					{"name", "verified"},
					{"description", "verified"},
					{"image", "https://bogus.com/pic.jpg", "1024x1024"},
					{"thumb", "https://bogus.com/pic.jpg", "256x256"},
				},
			},
		}
		require.NoError(t, badgeDefinitionEvent.SignWithAlg(heimdallPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, memdb.AcceptEvents(t.Context(), badgeDefinitionEvent))

		badgeAwardEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindBadgeAward,
			Tags: nostr.Tags{
				[]string{model.CustomIONTagOnBehalfOf, heimdallPubkey},
				[]string{"a", fmt.Sprintf("30009:%v:verified", heimdallPubkey)},
				[]string{"p", userPubkey},
			},
		}}
		require.NoError(t, badgeAwardEvent.SignWithAlg(heimdallPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, memdb.AcceptEvents(t.Context(), badgeAwardEvent))
		require.NoError(t, node.broadcastUserEvents(t.Context(), badgeDefinitionEvent, badgeAwardEvent))
	})

	t.Run("change_display_name_only", func(t *testing.T) {
		username := "stableusername"
		originalProfile := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now().Add(-time.Hour),
			Kind:      nostr.KindProfileMetadata,
			Tags:      model.Tags{{model.CustomIONTagOnBehalfOf, userPubkey}},
			Content:   fmt.Sprintf(`{"name":"%s","display_name":"Original Display Name"}`, username),
		}}
		require.NoError(t, originalProfile.SignWithAlg(userPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, memdb.AcceptEvents(t.Context(), originalProfile))

		updatedProfile := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindProfileMetadata,
			Tags:      model.Tags{{model.CustomIONTagOnBehalfOf, userPubkey}},
			Content:   fmt.Sprintf(`{"name":"%s","display_name":"New Display Name Only"}`, username),
		}}
		require.NoError(t, updatedProfile.SignWithAlg(userPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		var oldMeta model.ProfileMetadataContent
		var newMeta model.ProfileMetadataContent

		t.Logf("Original profile content: %s", originalProfile.Content)
		t.Logf("Updated profile content: %s", updatedProfile.Content)

		require.NoError(t, json.Unmarshal([]byte(originalProfile.Content), &oldMeta))
		require.NoError(t, json.Unmarshal([]byte(updatedProfile.Content), &newMeta))

		t.Logf("Old metadata: Name=%s, DisplayName=%s", oldMeta.Name, oldMeta.DisplayName)
		t.Logf("New metadata: Name=%s, DisplayName=%s", newMeta.Name, newMeta.DisplayName)

		require.Equal(t, oldMeta.Name, newMeta.Name)
		require.NotEqual(t, oldMeta.DisplayName, newMeta.DisplayName)

		err := node.broadcastUserEvents(t.Context(), updatedProfile)
		require.NoError(t, err)
	})

	t.Run("mismatch_between_profile_and_badge_username", func(t *testing.T) {
		profileUsername := "profileusername"
		badgeUsername := "badgeusername"

		profileMetadata := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindProfileMetadata,
			Tags:      model.Tags{{model.CustomIONTagOnBehalfOf, userPubkey}},
			Content:   fmt.Sprintf(`{"name":"%s","display_name":"Profile User"}`, profileUsername),
		}}
		require.NoError(t, profileMetadata.SignWithAlg(userPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, memdb.AcceptEvents(t.Context(), profileMetadata))

		badgeDefinitionEvent := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindBadgeDefinition,
				Tags: nostr.Tags{
					{"d", fmt.Sprintf("username_proof_of_ownership:%s", badgeUsername)},
					{"name", "Username Verification Mismatch"},
					{"description", "This badge has a different username than the profile"},
				},
			},
		}
		require.NoError(t, badgeDefinitionEvent.SignWithAlg(heimdallPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		badgeAwardEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindBadgeAward,
			Tags: nostr.Tags{
				[]string{model.CustomIONTagOnBehalfOf, heimdallPubkey},
				[]string{"a", fmt.Sprintf("30009:%v:username_proof_of_ownership:%s", heimdallPubkey, badgeUsername)},
				[]string{"p", userPubkey},
			},
		}}
		require.NoError(t, badgeAwardEvent.SignWithAlg(heimdallPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		err := node.broadcastUserEvents(t.Context(), profileMetadata, badgeDefinitionEvent, badgeAwardEvent)
		require.Error(t, err)
	})

	t.Run("username_proof_of_ownership", func(t *testing.T) {
		username := "testuser123"
		profileMetadata := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindProfileMetadata,
			Tags:      model.Tags{{model.CustomIONTagOnBehalfOf, userPubkey}},
			Content:   fmt.Sprintf(`{"name":"%s","display_name":"Test User"}`, username),
		}}
		require.NoError(t, profileMetadata.SignWithAlg(userPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, memdb.AcceptEvents(t.Context(), profileMetadata))

		badgeDefinitionEvent := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindBadgeDefinition,
				Tags: nostr.Tags{
					{"d", fmt.Sprintf("username_proof_of_ownership:%s", username)},
					{"name", "Username Verification"},
					{"description", "Proof of ownership for username"},
					{"image", "https://bogus.com/verified.jpg", "1024x1024"},
					{"thumb", "https://bogus.com/verified_thumb.jpg", "256x256"},
				},
			},
		}
		require.NoError(t, badgeDefinitionEvent.SignWithAlg(heimdallPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, memdb.AcceptEvents(t.Context(), badgeDefinitionEvent))

		badgeAwardEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindBadgeAward,
			Tags: nostr.Tags{
				[]string{model.CustomIONTagOnBehalfOf, heimdallPubkey},
				[]string{"a", fmt.Sprintf("30009:%v:username_proof_of_ownership:%s", heimdallPubkey, username)},
				[]string{"p", userPubkey},
			},
		}}
		require.NoError(t, badgeAwardEvent.SignWithAlg(heimdallPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, memdb.AcceptEvents(t.Context(), badgeAwardEvent))

		require.NoError(t, node.broadcastUserEvents(t.Context(), badgeDefinitionEvent, badgeAwardEvent, profileMetadata))

		wrongUsername := "wrongusername"
		badDefinitionWrongUsername := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindBadgeDefinition,
				Tags: nostr.Tags{
					{"d", fmt.Sprintf("username_proof_of_ownership:%s", wrongUsername)},
					{"name", "Wrong Username"},
					{"description", "This should fail"},
				},
			},
		}
		require.NoError(t, badDefinitionWrongUsername.SignWithAlg(heimdallPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		badgeAwardWrongUsername := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindBadgeAward,
			Tags: nostr.Tags{
				[]string{model.CustomIONTagOnBehalfOf, heimdallPubkey},
				[]string{"a", fmt.Sprintf("30009:%v:username_proof_of_ownership:%s", heimdallPubkey, wrongUsername)},
				[]string{"p", userPubkey},
			},
		}}
		require.NoError(t, badgeAwardWrongUsername.SignWithAlg(heimdallPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.Error(t, node.broadcastUserEvents(t.Context(), badDefinitionWrongUsername, badgeAwardWrongUsername))
	})

	t.Run("missing_profile_metadata", func(t *testing.T) {
		username := "verifyUsername"
		badgeDefinitionEvent := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindBadgeDefinition,
				Tags: nostr.Tags{
					{"d", fmt.Sprintf("username_proof_of_ownership:%s", username)},
					{"name", "Username Verification"},
					{"description", "Proof of ownership for username without profile"},
				},
			},
		}
		require.NoError(t, badgeDefinitionEvent.SignWithAlg(heimdallPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, memdb.AcceptEvents(t.Context(), badgeDefinitionEvent))

		badgeAwardEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindBadgeAward,
			Tags: nostr.Tags{
				[]string{model.CustomIONTagOnBehalfOf, heimdallPubkey},
				[]string{"a", fmt.Sprintf("30009:%v:username_proof_of_ownership:%s", heimdallPubkey, username)},
				[]string{"p", userPubkey},
			},
		}}
		require.NoError(t, badgeAwardEvent.SignWithAlg(heimdallPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, memdb.AcceptEvents(t.Context(), badgeAwardEvent))
		require.Error(t, node.broadcastUserEvents(t.Context(), badgeDefinitionEvent, badgeAwardEvent))
	})

	t.Run("create_profile_with_proof_ownership", func(t *testing.T) {
		username := "newusername"
		profileMetadata := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindProfileMetadata,
			Tags:      model.Tags{{model.CustomIONTagOnBehalfOf, userPubkey}},
			Content:   fmt.Sprintf(`{"name":"%s","display_name":"New User"}`, username),
		}}
		require.NoError(t, profileMetadata.SignWithAlg(userPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		badgeDefinitionEvent := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindBadgeDefinition,
				Tags: nostr.Tags{
					{"d", fmt.Sprintf("username_proof_of_ownership:%s", username)},
					{"name", "Username Verification"},
					{"description", "Proof of ownership for username"},
				},
			},
		}
		require.NoError(t, badgeDefinitionEvent.SignWithAlg(heimdallPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		badgeAwardEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindBadgeAward,
			Tags: nostr.Tags{
				[]string{model.CustomIONTagOnBehalfOf, heimdallPubkey},
				[]string{"a", fmt.Sprintf("30009:%v:username_proof_of_ownership:%s", heimdallPubkey, username)},
				[]string{"p", userPubkey},
			},
		}}
		require.NoError(t, badgeAwardEvent.SignWithAlg(heimdallPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, node.broadcastUserEvents(t.Context(), profileMetadata, badgeDefinitionEvent, badgeAwardEvent))
	})

	t.Run("change_username_with_proof_ownership", func(t *testing.T) {
		oldUsername := "oldusername"
		originalProfile := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now().Add(-time.Hour),
			Kind:      nostr.KindProfileMetadata,
			Tags:      model.Tags{{model.CustomIONTagOnBehalfOf, userPubkey}},
			Content:   fmt.Sprintf(`{"name":"%s","display_name":"Old User"}`, oldUsername),
		}}
		require.NoError(t, originalProfile.SignWithAlg(userPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, memdb.AcceptEvents(t.Context(), originalProfile))

		newUsername := "updatedjdoe"
		updatedProfile := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindProfileMetadata,
			Tags:      model.Tags{{model.CustomIONTagOnBehalfOf, userPubkey}},
			Content:   fmt.Sprintf(`{"name":"%s","display_name":"Updated User"}`, newUsername),
		}}
		require.NoError(t, updatedProfile.SignWithAlg(userPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		badgeDefinitionEvent := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindBadgeDefinition,
				Tags: nostr.Tags{
					{"d", fmt.Sprintf("username_proof_of_ownership:%s", newUsername)},
					{"name", "Username Verification"},
					{"description", "Proof of ownership for updated username"},
				},
			},
		}
		require.NoError(t, badgeDefinitionEvent.SignWithAlg(heimdallPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		badgeAwardEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindBadgeAward,
			Tags: nostr.Tags{
				[]string{model.CustomIONTagOnBehalfOf, heimdallPubkey},
				[]string{"a", fmt.Sprintf("30009:%v:username_proof_of_ownership:%s", heimdallPubkey, newUsername)},
				[]string{"p", userPubkey},
			},
		}}
		require.NoError(t, badgeAwardEvent.SignWithAlg(heimdallPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, node.broadcastUserEvents(t.Context(), updatedProfile, badgeDefinitionEvent, badgeAwardEvent))
	})
}

func TestExtractUsernameFromProofBadge(t *testing.T) {
	t.Parallel()

	t.Run("badge_award_with_username_proof", func(t *testing.T) {
		username := "testuser123"
		event := &model.Event{Event: nostr.Event{
			Kind: nostr.KindBadgeAward,
			Tags: model.Tags{
				[]string{"a", fmt.Sprintf("30009:pubkey:username_proof_of_ownership:%s", username)},
			},
		}}

		isProofBadge, extractedUsername := extractUsernameFromProofBadge(event)
		require.True(t, isProofBadge)
		require.Equal(t, username, extractedUsername)
	})

	t.Run("badge_definition_with_username_proof", func(t *testing.T) {
		username := "testuser123"
		event := &model.Event{Event: nostr.Event{
			Kind: nostr.KindBadgeDefinition,
			Tags: model.Tags{
				[]string{"d", fmt.Sprintf("username_proof_of_ownership:%s", username)},
			},
		}}

		isProofBadge, extractedUsername := extractUsernameFromProofBadge(event)
		require.True(t, isProofBadge)
		require.Equal(t, username, extractedUsername)
	})

	t.Run("badge_award_without_username_proof", func(t *testing.T) {
		event := &model.Event{Event: nostr.Event{
			Kind: nostr.KindBadgeAward,
			Tags: model.Tags{
				[]string{"a", "30009:pubkey:regular_badge"},
			},
		}}

		isProofBadge, extractedUsername := extractUsernameFromProofBadge(event)
		require.False(t, isProofBadge)
		require.Empty(t, extractedUsername)
	})

	t.Run("badge_definition_without_username_proof", func(t *testing.T) {
		event := &model.Event{Event: nostr.Event{
			Kind: nostr.KindBadgeDefinition,
			Tags: model.Tags{
				[]string{"d", "regular_badge"},
			},
		}}

		isProofBadge, extractedUsername := extractUsernameFromProofBadge(event)
		require.False(t, isProofBadge)
		require.Empty(t, extractedUsername)
	})

	t.Run("other_event_type", func(t *testing.T) {
		event := &model.Event{Event: nostr.Event{
			Kind: nostr.KindTextNote,
			Tags: model.Tags{
				[]string{"d", "username_proof_of_ownership:testuser"},
			},
		}}

		isProofBadge, extractedUsername := extractUsernameFromProofBadge(event)
		require.False(t, isProofBadge)
		require.Empty(t, extractedUsername)
	})
}
