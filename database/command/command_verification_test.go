// SPDX-License-Identifier: ice License 1.0

package command

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/ice-blockchain/cometbft/multiplex/client"
	"github.com/ice-blockchain/subzero/database/command/fixture"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

func TestBroadcastLinkedEventBadges(t *testing.T) {
	t.Parallel()
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

func TestBroadcastUserEvents_TextNote_WithoutAck(t *testing.T) {
	t.Parallel()
	var memdb query.MemDB
	userPrivKey, userPubkey := model.GenerateKeyPair()
	rootAuthorPrivKey, rootAuthorPubkey := model.GenerateKeyPair()
	_, badgeIssuerPubkey := model.GenerateKeyPair()

	relaysList := &model.Event{Event: nostr.Event{
		CreatedAt: nostr.Now(),
		Kind:      nostr.KindRelayListMetadata,
		Tags: model.Tags{
			{model.CustomIONTagOnBehalfOf, userPubkey},
			{"r", "wss://localhost:9988"},
			{"r", "wss://localhost:9977"},
		},
	}}
	require.NoError(t, relaysList.SignWithAlg(userPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, memdb.AcceptEvents(t.Context(), relaysList))

	rootAuthorRelaysList := &model.Event{Event: nostr.Event{
		CreatedAt: nostr.Now(),
		Kind:      nostr.KindRelayListMetadata,
		Tags: model.Tags{
			{model.CustomIONTagOnBehalfOf, rootAuthorPubkey},
			{"r", "wss://localhost:9988"},
			{"r", "wss://localhost:9977"},
		},
	}}
	require.NoError(t, rootAuthorRelaysList.SignWithAlg(rootAuthorPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, memdb.AcceptEvents(t.Context(), rootAuthorRelaysList))

	var broadcastedEvents []*model.Event
	consensusClient := fixture.NewCallbackClient(func(userAddress string, relays []string, transactions ...client.Transaction) {
		for _, tx := range transactions {
			evs, err := mapTxToEvent(tx)
			require.NoError(t, err)
			broadcastedEvents = append(broadcastedEvents, evs...)
		}
	}, func(userAddress string, relays []string, transactions ...client.Transaction) {
		require.Fail(t, "Rollback should not be called")
	})

	node, release := newConsensusNode(t.Context(), nil, 19999,
		WithClient(consensusClient),
		WithQuery(memdb.SelectEvents),
	)
	defer release()

	t.Run("root post without restrictions", func(t *testing.T) {
		broadcastedEvents = nil
		rootPost := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, rootAuthorPubkey},
			},
			Content: "Root post without restrictions",
		}}
		require.NoError(t, rootPost.SignWithAlg(rootAuthorPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, node.broadcastUserEvents(t.Context(), rootPost))
		require.Len(t, broadcastedEvents, 1)
		require.Equal(t, rootPost.ID, broadcastedEvents[0].ID)
	})

	t.Run("direct reply to unrestricted post", func(t *testing.T) {
		broadcastedEvents = nil
		rootPost := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, rootAuthorPubkey},
			},
			Content: "Unrestricted root post",
		}}
		require.NoError(t, rootPost.SignWithAlg(rootAuthorPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, memdb.AcceptEvents(t.Context(), rootPost))

		replyPost := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, userPubkey},
				{"e", rootPost.ID, "", model.TagMarkerRoot},
				{"p", rootAuthorPubkey},
			},
			Content: "Direct reply to unrestricted post",
		}}
		require.NoError(t, replyPost.SignWithAlg(userPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, node.broadcastUserEvents(t.Context(), replyPost))
		require.Len(t, broadcastedEvents, 1)
		require.Equal(t, replyPost.ID, broadcastedEvents[0].ID)
	})

	t.Run("reply to reply with root reference", func(t *testing.T) {
		broadcastedEvents = nil
		rootPost := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, rootAuthorPubkey},
			},
			Content: "Unrestricted root post",
		}}
		require.NoError(t, rootPost.SignWithAlg(rootAuthorPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, memdb.AcceptEvents(t.Context(), rootPost))

		firstReply := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, userPubkey},
				{"e", rootPost.ID, "", model.TagMarkerRoot},
				{"p", rootAuthorPubkey},
			},
			Content: "First reply",
		}}
		require.NoError(t, firstReply.SignWithAlg(userPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, memdb.AcceptEvents(t.Context(), firstReply))

		replyToReply := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, userPubkey},
				{"e", rootPost.ID, "", model.TagMarkerRoot},
				{"e", firstReply.ID, "", model.TagMarkerReply},
				{"p", userPubkey},
			},
			Content: "Reply to reply with root reference",
		}}
		require.NoError(t, replyToReply.SignWithAlg(userPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, node.broadcastUserEvents(t.Context(), replyToReply))
		require.Len(t, broadcastedEvents, 1)
		require.Equal(t, replyToReply.ID, broadcastedEvents[0].ID)
	})

	t.Run("direct reply to restricted post should fail", func(t *testing.T) {
		broadcastedEvents = nil
		restrictedRootPost := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, rootAuthorPubkey},
				{"settings", model.WhoCanReplySettings, fmt.Sprintf("%s|%d:%s:verified", model.BadgeWhoCanReplySettingsPrefix, nostr.KindBadgeDefinition, badgeIssuerPubkey), strconv.FormatInt(time.Now().Unix(), 10)},
			},
			Content: "Badge restricted root post",
		}}
		require.NoError(t, restrictedRootPost.SignWithAlg(rootAuthorPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, memdb.AcceptEvents(t.Context(), restrictedRootPost))

		replyPost := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, userPubkey},
				{"e", restrictedRootPost.ID, "", model.TagMarkerRoot},
				{"p", rootAuthorPubkey},
			},
			Content: "Direct reply to restricted post",
		}}
		require.NoError(t, replyPost.SignWithAlg(userPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.Error(t, node.broadcastUserEvents(t.Context(), replyPost))
		require.Len(t, broadcastedEvents, 0)
	})
}

func TestBroadcastUserEvents_TextNote_WithAck(t *testing.T) {
	t.Parallel()
	var memdb query.MemDB
	userPrivKey, userPubkey := model.GenerateKeyPair()
	rootAuthorPrivKey, rootAuthorPubkey := model.GenerateKeyPair()
	badgeIssuerPrivKey, badgeIssuerPubkey := model.GenerateKeyPair()

	relaysList := &model.Event{Event: nostr.Event{
		CreatedAt: nostr.Now(),
		Kind:      nostr.KindRelayListMetadata,
		Tags: model.Tags{
			{model.CustomIONTagOnBehalfOf, userPubkey},
			{"r", "wss://localhost:9988"},
			{"r", "wss://localhost:9977"},
		},
	}}
	require.NoError(t, relaysList.SignWithAlg(userPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, memdb.AcceptEvents(t.Context(), relaysList))

	rootAuthorRelaysList := &model.Event{Event: nostr.Event{
		CreatedAt: nostr.Now(),
		Kind:      nostr.KindRelayListMetadata,
		Tags: model.Tags{
			{model.CustomIONTagOnBehalfOf, rootAuthorPubkey},
			{"r", "wss://localhost:9988"},
			{"r", "wss://localhost:9977"},
		},
	}}
	require.NoError(t, rootAuthorRelaysList.SignWithAlg(rootAuthorPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, memdb.AcceptEvents(t.Context(), rootAuthorRelaysList))

	var broadcastedEvents []*model.Event
	consensusClient := fixture.NewCallbackClient(func(userAddress string, relays []string, transactions ...client.Transaction) {
		for _, tx := range transactions {
			evs, err := mapTxToEvent(tx)
			require.NoError(t, err)
			broadcastedEvents = append(broadcastedEvents, evs...)
		}
	}, func(userAddress string, relays []string, transactions ...client.Transaction) {
		require.Fail(t, "Rollback should not be called")
	})

	node, release := newConsensusNode(t.Context(), nil, 19999,
		WithClient(consensusClient),
		WithQuery(memdb.SelectEvents),
	)
	defer release()

	t.Run("valid badge acks for direct reply to restricted post", func(t *testing.T) {
		broadcastedEvents = nil
		restrictedRootPost := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, rootAuthorPubkey},
				{"settings", model.WhoCanReplySettings, fmt.Sprintf("%s|%d:%s:verified", model.BadgeWhoCanReplySettingsPrefix, nostr.KindBadgeDefinition, badgeIssuerPubkey), strconv.FormatInt(time.Now().Unix(), 10)},
			},
			Content: "Badge restricted root post",
		}}
		require.NoError(t, restrictedRootPost.SignWithAlg(rootAuthorPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, memdb.AcceptEvents(t.Context(), restrictedRootPost))

		replyEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, userPubkey},
				{"e", restrictedRootPost.ID, "", model.TagMarkerRoot},
				{"p", rootAuthorPubkey},
			},
			Content: "Direct reply with valid badge acks",
		}}
		require.NoError(t, replyEvent.SignWithAlg(userPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		badgeDefinition := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindBadgeDefinition,
			Tags: model.Tags{
				{"d", "verified"},
				{"name", "Verified Badge"},
			},
		}}
		require.NoError(t, badgeDefinition.SignWithAlg(badgeIssuerPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		badgeAward := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindBadgeAward,
			Tags: model.Tags{
				{"a", fmt.Sprintf("%d:%s:verified", nostr.KindBadgeDefinition, badgeIssuerPubkey)},
				{"p", userPubkey},
			},
		}}
		require.NoError(t, badgeAward.SignWithAlg(badgeIssuerPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		badgeDefAckContent, err := badgeDefinition.MarshalJSON()
		require.NoError(t, err)
		badgeDefAck := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      model.CustomIONKindEphemeralEmbeddding,
			Tags: model.Tags{
				{"e", replyEvent.ID},
			},
			Content: string(badgeDefAckContent),
		}}
		require.NoError(t, badgeDefAck.SignWithAlg(userPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		badgeAwardAckContent, err := badgeAward.MarshalJSON()
		require.NoError(t, err)
		badgeAwardAck := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      model.CustomIONKindEphemeralEmbeddding,
			Tags: model.Tags{
				{"e", replyEvent.ID},
			},
			Content: string(badgeAwardAckContent),
		}}
		require.NoError(t, badgeAwardAck.SignWithAlg(userPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, node.broadcastUserEvents(t.Context(), replyEvent, badgeDefAck, badgeAwardAck))

		require.GreaterOrEqual(t, len(broadcastedEvents), 1)

		var replyFound bool
		for _, ev := range broadcastedEvents {
			if ev.ID == replyEvent.ID {
				replyFound = true
				break
			}
		}
		require.True(t, replyFound, "Reply event should be broadcasted")
	})

	t.Run("valid badge acks for reply to reply", func(t *testing.T) {
		broadcastedEvents = nil

		restrictedRootPost := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, rootAuthorPubkey},
				{"settings", model.WhoCanReplySettings, fmt.Sprintf("%s|%d:%s:verified", model.BadgeWhoCanReplySettingsPrefix, nostr.KindBadgeDefinition, badgeIssuerPubkey), strconv.FormatInt(time.Now().Unix(), 10)},
			},
			Content: "Badge restricted root post",
		}}
		require.NoError(t, restrictedRootPost.SignWithAlg(rootAuthorPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, memdb.AcceptEvents(t.Context(), restrictedRootPost))

		firstReply := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, userPubkey},
				{"e", restrictedRootPost.ID, "", model.TagMarkerRoot},
				{"p", rootAuthorPubkey},
			},
			Content: "First reply",
		}}
		require.NoError(t, firstReply.SignWithAlg(userPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, memdb.AcceptEvents(t.Context(), firstReply))

		replyToReply := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, userPubkey},
				{"e", restrictedRootPost.ID, "", model.TagMarkerRoot},
				{"e", firstReply.ID, "", model.TagMarkerReply},
				{"p", userPubkey},
			},
			Content: "Reply to reply with valid badge acks",
		}}
		require.NoError(t, replyToReply.SignWithAlg(userPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		badgeDefinition := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindBadgeDefinition,
			Tags: model.Tags{
				{"d", "verified"},
				{"name", "Verified Badge"},
			},
		}}
		require.NoError(t, badgeDefinition.SignWithAlg(badgeIssuerPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		badgeAward := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindBadgeAward,
			Tags: model.Tags{
				{"a", fmt.Sprintf("%d:%s:verified", nostr.KindBadgeDefinition, badgeIssuerPubkey)},
				{"p", userPubkey},
			},
		}}
		require.NoError(t, badgeAward.SignWithAlg(badgeIssuerPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		badgeDefAckContent, err := badgeDefinition.MarshalJSON()
		require.NoError(t, err)
		badgeDefAck := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      model.CustomIONKindEphemeralEmbeddding,
			Tags: model.Tags{
				{"e", replyToReply.ID},
			},
			Content: string(badgeDefAckContent),
		}}
		require.NoError(t, badgeDefAck.SignWithAlg(userPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		badgeAwardAckContent, err := badgeAward.MarshalJSON()
		require.NoError(t, err)
		badgeAwardAck := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      model.CustomIONKindEphemeralEmbeddding,
			Tags: model.Tags{
				{"e", replyToReply.ID},
			},
			Content: string(badgeAwardAckContent),
		}}
		require.NoError(t, badgeAwardAck.SignWithAlg(userPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		err = node.broadcastUserEvents(t.Context(), replyToReply, badgeDefAck, badgeAwardAck)
		require.NoError(t, err)

		require.GreaterOrEqual(t, len(broadcastedEvents), 1)

		var replyFound bool
		for _, ev := range broadcastedEvents {
			if ev.ID == replyToReply.ID {
				replyFound = true
				break
			}
		}
		require.True(t, replyFound, "Reply to reply event should be broadcasted")
	})

	t.Run("missing badge definition ack should fail", func(t *testing.T) {
		broadcastedEvents = nil

		restrictedRootPost := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, rootAuthorPubkey},
				{"settings", model.WhoCanReplySettings, fmt.Sprintf("%s|%d:%s:verified", model.BadgeWhoCanReplySettingsPrefix, nostr.KindBadgeDefinition, badgeIssuerPubkey), strconv.FormatInt(time.Now().Unix(), 10)},
			},
			Content: "Badge restricted root post",
		}}
		require.NoError(t, restrictedRootPost.SignWithAlg(rootAuthorPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, memdb.AcceptEvents(t.Context(), restrictedRootPost))

		replyEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, userPubkey},
				{"e", restrictedRootPost.ID, "", model.TagMarkerRoot},
				{"p", rootAuthorPubkey},
			},
			Content: "Direct reply without badge definition ack",
		}}
		require.NoError(t, replyEvent.SignWithAlg(userPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		badgeAward := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindBadgeAward,
			Tags: model.Tags{
				{"a", fmt.Sprintf("%d:%s:verified", nostr.KindBadgeDefinition, badgeIssuerPubkey)},
				{"p", userPubkey},
			},
		}}
		require.NoError(t, badgeAward.SignWithAlg(badgeIssuerPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		badgeAwardAckContent, err := badgeAward.MarshalJSON()
		require.NoError(t, err)
		badgeAwardAck := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      model.CustomIONKindEphemeralEmbeddding,
			Tags: model.Tags{
				{"e", replyEvent.ID},
			},
			Content: string(badgeAwardAckContent),
		}}
		require.NoError(t, badgeAwardAck.SignWithAlg(userPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.Error(t, node.broadcastUserEvents(t.Context(), replyEvent, badgeAwardAck))
		require.Len(t, broadcastedEvents, 0)
	})

	t.Run("post without root tags should succeed", func(t *testing.T) {
		broadcastedEvents = nil

		replyEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: model.Tags{
				{model.CustomIONTagOnBehalfOf, userPubkey},
				{"p", rootAuthorPubkey},
			},
			Content: "Post without root tags",
		}}
		require.NoError(t, replyEvent.SignWithAlg(userPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		err := node.broadcastUserEvents(t.Context(), replyEvent)
		require.NoError(t, err)
		require.Len(t, broadcastedEvents, 1)
		require.Equal(t, replyEvent.ID, broadcastedEvents[0].ID)
	})
}
