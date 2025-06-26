// SPDX-License-Identifier: ice License 1.0

package command

import (
	"context"
	"encoding/hex"
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
	dbfix "github.com/ice-blockchain/subzero/database/query/fixture"
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
	var memdb dbfix.MemDB
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
	var memdb dbfix.MemDB
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

	var memdb dbfix.MemDB
	node, release := newConsensusNode(t.Context(), nil, 13999, WithQuery(memdb.SelectEvents))
	defer release()

	for range 5 {
		require.NoError(t, node.Stop(t.Context(), time.Second))
		node.Start(t.Context())
	}
}

func TestBroadcastUserEvents_BasicFunctionality(t *testing.T) {
	var memdb dbfix.MemDB
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

	var broadcastedUserAddress string
	var broadcastedRelays []string
	var broadcastedEvents []*model.Event

	consensusClient := fixture.NewCallbackClient(func(userAddress string, relays []string, transactions ...client.Transaction) {
		broadcastedUserAddress = userAddress
		broadcastedRelays = relays
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

	t.Run("simple_text_note", func(t *testing.T) {
		broadcastedEvents = nil
		textNote := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags:      model.Tags{{model.CustomIONTagOnBehalfOf, userPubkey}},
			Content:   "Hello, world!",
		}}
		require.NoError(t, textNote.SignWithAlg(userPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		require.NoError(t, node.broadcastUserEvents(t.Context(), textNote))
		unhex, err := hex.DecodeString(userPubkey)
		require.NoError(t, err)
		expectedAddr, err := client.PubKeyToAddress(string(unhex))
		require.NoError(t, err)
		require.Equal(t, expectedAddr, broadcastedUserAddress)
		require.ElementsMatch(t, []string{"localhost:19988", "localhost:19977"}, broadcastedRelays)
		require.Len(t, broadcastedEvents, 1)
		require.Equal(t, textNote.ID, broadcastedEvents[0].ID)
	})

	t.Run("profile_metadata", func(t *testing.T) {
		broadcastedEvents = nil
		profileMetadata := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindProfileMetadata,
			Tags:      model.Tags{{model.CustomIONTagOnBehalfOf, userPubkey}},
			Content:   `{"name":"testuser","display_name":"Test User"}`,
		}}
		require.NoError(t, profileMetadata.SignWithAlg(userPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		require.NoError(t, node.broadcastUserEvents(t.Context(), profileMetadata))

		require.Len(t, broadcastedEvents, 1)
		require.Equal(t, profileMetadata.ID, broadcastedEvents[0].ID)
	})

	t.Run("multiple_events_same_user", func(t *testing.T) {
		broadcastedEvents = nil
		textNote1 := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags:      model.Tags{{model.CustomIONTagOnBehalfOf, userPubkey}},
			Content:   "First note",
		}}
		require.NoError(t, textNote1.SignWithAlg(userPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		textNote2 := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags:      model.Tags{{model.CustomIONTagOnBehalfOf, userPubkey}},
			Content:   "Second note",
		}}
		require.NoError(t, textNote2.SignWithAlg(userPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		require.NoError(t, node.broadcastUserEvents(t.Context(), textNote1, textNote2))
		require.Len(t, broadcastedEvents, 2)
		eventIDs := []string{broadcastedEvents[0].ID, broadcastedEvents[1].ID}
		require.ElementsMatch(t, []string{textNote1.ID, textNote2.ID}, eventIDs)
	})
}

func TestBroadcastUserEvents_MasterKeyDetection(t *testing.T) {
	var memdb dbfix.MemDB
	userPrivKey, userPubkey := model.GenerateKeyPair()
	otherUserPrivKey, otherUserPubkey := model.GenerateKeyPair()
	userRelaysList := &model.Event{Event: nostr.Event{
		CreatedAt: nostr.Now(),
		Kind:      nostr.KindRelayListMetadata,
		Tags: nostr.Tags{
			[]string{model.CustomIONTagOnBehalfOf, userPubkey},
			[]string{"r", "wss://localhost:9988"},
		},
	}}
	require.NoError(t, userRelaysList.SignWithAlg(userPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, memdb.AcceptEvents(t.Context(), userRelaysList))

	otherUserRelaysList := &model.Event{Event: nostr.Event{
		CreatedAt: nostr.Now(),
		Kind:      nostr.KindRelayListMetadata,
		Tags: nostr.Tags{
			[]string{model.CustomIONTagOnBehalfOf, otherUserPubkey},
			[]string{"r", "wss://localhost:9977"},
		},
	}}
	require.NoError(t, otherUserRelaysList.SignWithAlg(otherUserPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, memdb.AcceptEvents(t.Context(), otherUserRelaysList))

	var broadcastedUserAddress string
	consensusClient := fixture.NewCallbackClient(func(userAddress string, relays []string, transactions ...client.Transaction) {
		broadcastedUserAddress = userAddress
	}, func(userAddress string, relays []string, transactions ...client.Transaction) {
		require.Fail(t, "Rollback should not be called")
	})

	node, release := newConsensusNode(t.Context(), nil, 19999,
		WithClient(consensusClient),
		WithQuery(memdb.SelectEvents),
	)
	defer release()

	t.Run("reply_to_other_user", func(t *testing.T) {
		broadcastedUserAddress = ""
		rootPost := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags:      model.Tags{{model.CustomIONTagOnBehalfOf, otherUserPubkey}},
			Content:   "Root post",
		}}
		require.NoError(t, rootPost.SignWithAlg(otherUserPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, memdb.AcceptEvents(t.Context(), rootPost))
		replyPost := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: nostr.Tags{
				{model.CustomIONTagOnBehalfOf, userPubkey},
				{"e", rootPost.ID, "", model.TagMarkerReply},
			},
			Content: "Reply to root post",
		}}
		require.NoError(t, replyPost.SignWithAlg(userPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		require.NoError(t, node.broadcastUserEvents(t.Context(), replyPost))
		require.NotEmpty(t, broadcastedUserAddress)
	})

	t.Run("mention_other_user", func(t *testing.T) {
		broadcastedUserAddress = ""
		mentionPost := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags: nostr.Tags{
				{model.CustomIONTagOnBehalfOf, userPubkey},
				{"p", otherUserPubkey},
			},
			Content: "Mentioning other user",
		}}
		require.NoError(t, mentionPost.SignWithAlg(userPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		require.NoError(t, node.broadcastUserEvents(t.Context(), mentionPost))
		require.NotEmpty(t, broadcastedUserAddress)
	})
}

func TestBroadcastUserEvents_BadgeEvents(t *testing.T) {
	var memdb dbfix.MemDB
	userPrivKey, userPubkey := model.GenerateKeyPair()
	badgeIssuerPrivKey, badgeIssuerPubkey := model.GenerateKeyPair()

	userRelaysList := &model.Event{Event: nostr.Event{
		CreatedAt: nostr.Now(),
		Kind:      nostr.KindRelayListMetadata,
		Tags: nostr.Tags{
			[]string{model.CustomIONTagOnBehalfOf, userPubkey},
			[]string{"r", "wss://localhost:9988"},
		},
	}}
	require.NoError(t, userRelaysList.SignWithAlg(userPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, memdb.AcceptEvents(t.Context(), userRelaysList))

	badgeIssuerRelaysList := &model.Event{Event: nostr.Event{
		CreatedAt: nostr.Now(),
		Kind:      nostr.KindRelayListMetadata,
		Tags: nostr.Tags{
			[]string{model.CustomIONTagOnBehalfOf, badgeIssuerPubkey},
			[]string{"r", "wss://localhost:9977"},
		},
	}}
	require.NoError(t, badgeIssuerRelaysList.SignWithAlg(badgeIssuerPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, memdb.AcceptEvents(t.Context(), badgeIssuerRelaysList))

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

	t.Run("badge_definition_and_award", func(t *testing.T) {
		broadcastedEvents = nil

		badgeDefinition := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindBadgeDefinition,
			Tags: nostr.Tags{
				{"d", "verified"},
				{"name", "Verified Badge"},
				{"description", "Verification badge"},
			},
		}}
		require.NoError(t, badgeDefinition.SignWithAlg(badgeIssuerPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		badgeAward := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindBadgeAward,
			Tags: nostr.Tags{
				[]string{model.CustomIONTagOnBehalfOf, badgeIssuerPubkey},
				[]string{"a", fmt.Sprintf("30009:%s:verified", badgeIssuerPubkey)},
				[]string{"p", userPubkey},
			},
		}}
		require.NoError(t, badgeAward.SignWithAlg(badgeIssuerPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		require.NoError(t, node.broadcastUserEvents(t.Context(), badgeDefinition, badgeAward))
		require.Len(t, broadcastedEvents, 2)
		eventIDs := []string{broadcastedEvents[0].ID, broadcastedEvents[1].ID}
		require.ElementsMatch(t, []string{badgeDefinition.ID, badgeAward.ID}, eventIDs)
	})

	t.Run("profile_badges", func(t *testing.T) {
		broadcastedEvents = nil

		profileBadges := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindProfileBadges,
			Tags: nostr.Tags{
				{"d", "profile_badges"},
				[]string{model.CustomIONTagOnBehalfOf, userPubkey},
				[]string{"a", fmt.Sprintf("30009:%s:verified", badgeIssuerPubkey)},
				[]string{"e", "some_badge_award_id"},
			},
		}}
		require.NoError(t, profileBadges.SignWithAlg(userPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, node.broadcastUserEvents(t.Context(), profileBadges))
		require.Len(t, broadcastedEvents, 1)
		require.Equal(t, profileBadges.ID, broadcastedEvents[0].ID)
	})
}

func TestMapEventsToTXs_AckEventsFiltering(t *testing.T) {
	t.Parallel()
	privkeyPostOwner, pubkeyPostOwner := model.GenerateKeyPair()
	privkeyUser1, _ := model.GenerateKeyPair()
	privkeyUser2, pubkeyUser2 := model.GenerateKeyPair()
	dBadgeTagVal := "verified"

	t.Run("badge_events_with_ephemeral_acks", func(t *testing.T) {
		var post, replyEvent *model.Event
		var badgeDefinition, badgeAward, profileEvent, attestationEvent, textNoteEvent *model.Event
		var badgeDefAck, badgeAwardAck, profileAck, attestationAck, textNoteAck *model.Event

		t.Run("create_main_post_with_badge_restrictions", func(t *testing.T) {
			post = &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Content:   "Post with badge restrictions",
				Tags: nostr.Tags{
					{"settings", model.WhoCanReplySettings, fmt.Sprintf("%v|%v:%v:%v", model.BadgeWhoCanReplySettingsPrefix, nostr.KindBadgeDefinition, pubkeyPostOwner, dBadgeTagVal), strconv.FormatInt(time.Now().Unix(), 10)},
				},
			}}
			require.NoError(t, post.SignWithAlg(privkeyPostOwner, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.NotEmpty(t, post.ID)
		})

		t.Run("create_reply_event", func(t *testing.T) {
			replyEvent = &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Tags: nostr.Tags{
					{"e", post.GetID(), "", model.TagMarkerRoot},
					{"e", post.GetID(), "", model.TagMarkerReply},
					{"p", post.GetMasterPublicKey()},
				},
				Content: "Reply with badge acks",
			}}
			require.NoError(t, replyEvent.SignWithAlg(privkeyUser2, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.NotEmpty(t, replyEvent.ID)
		})

		t.Run("create_badge_definition_content", func(t *testing.T) {
			badgeDefinition = &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindBadgeDefinition,
				Tags: nostr.Tags{
					{"d", dBadgeTagVal},
					{"name", "Verified Badge"},
					{"description", "User verification badge"},
				},
			}}
			require.NoError(t, badgeDefinition.SignWithAlg(privkeyPostOwner, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.Equal(t, nostr.KindBadgeDefinition, badgeDefinition.Kind)
		})

		t.Run("create_badge_award_content", func(t *testing.T) {
			badgeAward = &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindBadgeAward,
				Tags: nostr.Tags{
					{"a", fmt.Sprintf("%d:%s:%s", nostr.KindBadgeDefinition, pubkeyPostOwner, dBadgeTagVal)},
					{"p", pubkeyUser2},
				},
			}}
			require.NoError(t, badgeAward.SignWithAlg(privkeyPostOwner, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.Equal(t, nostr.KindBadgeAward, badgeAward.Kind)
		})

		t.Run("create_profile_metadata_content", func(t *testing.T) {
			profileEvent = &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindProfileMetadata,
				Content:   `{"name": "Test User", "about": "Testing badges"}`,
			}}
			require.NoError(t, profileEvent.SignWithAlg(privkeyUser2, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.Equal(t, nostr.KindProfileMetadata, profileEvent.Kind)
		})

		t.Run("create_attestation_content", func(t *testing.T) {
			attestationEvent = &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindAttestation,
				Content:   "Attestation content",
			}}
			require.NoError(t, attestationEvent.SignWithAlg(privkeyUser1, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.Equal(t, model.CustomIONKindAttestation, attestationEvent.Kind)
		})

		t.Run("create_unsupported_content", func(t *testing.T) {
			textNoteEvent = &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Content:   "Just a regular text note",
			}}
			require.NoError(t, textNoteEvent.SignWithAlg(privkeyUser1, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.Equal(t, nostr.KindTextNote, textNoteEvent.Kind)
		})

		t.Run("create_ephemeral_acks", func(t *testing.T) {
			badgeDefAckContent, err := badgeDefinition.MarshalJSON()
			require.NoError(t, err)
			badgeDefAck = &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindEphemeralEmbeddding,
				Tags: nostr.Tags{
					{"e", replyEvent.ID},
				},
				Content: string(badgeDefAckContent),
			}}
			require.NoError(t, badgeDefAck.SignWithAlg(privkeyPostOwner, model.SignAlgEDDSA, model.KeyAlgCurve25519))

			badgeAwardAckContent, err := badgeAward.MarshalJSON()
			require.NoError(t, err)
			badgeAwardAck = &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindEphemeralEmbeddding,
				Tags: nostr.Tags{
					{"e", replyEvent.ID},
				},
				Content: string(badgeAwardAckContent),
			}}
			require.NoError(t, badgeAwardAck.SignWithAlg(privkeyPostOwner, model.SignAlgEDDSA, model.KeyAlgCurve25519))

			profileAckContent, err := profileEvent.MarshalJSON()
			require.NoError(t, err)
			profileAck = &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindEphemeralEmbeddding,
				Tags: nostr.Tags{
					{"e", replyEvent.ID},
				},
				Content: string(profileAckContent),
			}}
			require.NoError(t, profileAck.SignWithAlg(privkeyUser2, model.SignAlgEDDSA, model.KeyAlgCurve25519))

			attestationAckContent, err := attestationEvent.MarshalJSON()
			require.NoError(t, err)
			attestationAck = &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindEphemeralEmbeddding,
				Tags: nostr.Tags{
					{"e", replyEvent.ID},
				},
				Content: string(attestationAckContent),
			}}
			require.NoError(t, attestationAck.SignWithAlg(privkeyUser1, model.SignAlgEDDSA, model.KeyAlgCurve25519))

			textNoteAckContent, err := textNoteEvent.MarshalJSON()
			require.NoError(t, err)
			textNoteAck = &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindEphemeralEmbeddding,
				Tags: nostr.Tags{
					{"e", replyEvent.ID},
				},
				Content: string(textNoteAckContent),
			}}
			require.NoError(t, textNoteAck.SignWithAlg(privkeyUser1, model.SignAlgEDDSA, model.KeyAlgCurve25519))

			require.NotEmpty(t, badgeDefAck.ID)
			require.NotEmpty(t, badgeAwardAck.ID)
			require.NotEmpty(t, profileAck.ID)
			require.NotEmpty(t, attestationAck.ID)
			require.NotEmpty(t, textNoteAck.ID)
		})

		t.Run("test_mapEventsToTXs_filtering", func(t *testing.T) {
			allEvents := []*model.Event{replyEvent, badgeDefAck, badgeAwardAck, profileAck, attestationAck, textNoteAck}
			ephemeralAckEvents, err := model.ParseEphemeralEmbeddingEvents(allEvents...)
			require.NoError(t, err)

			txs, err := mapEventsToTXs([]*model.Event{replyEvent}, ephemeralAckEvents)
			require.NoError(t, err)
			require.Len(t, txs, 1)

			var env nostr.EventEnvelope
			err = env.UnmarshalJSON(txs[0].Data)
			require.NoError(t, err)

			require.Len(t, env.Events, 5, "Should have reply + 4 filtered ephemeral events")

			eventKinds := make(map[int]int)
			ephemeralIDs := make(map[string]bool)

			for _, ev := range env.Events {
				eventKinds[ev.Kind]++
				if ev.Kind == model.CustomIONKindEphemeralEmbeddding {
					ephemeralIDs[ev.ID] = true
				}
			}
			require.Equal(t, 1, eventKinds[nostr.KindTextNote], "Should have 1 text note (the reply)")
			require.Equal(t, 4, eventKinds[model.CustomIONKindEphemeralEmbeddding], "Should have 4 ephemeral events")

			require.True(t, ephemeralIDs[badgeDefAck.ID], "Badge definition ephemeral ack should be included")
			require.True(t, ephemeralIDs[badgeAwardAck.ID], "Badge award ephemeral ack should be included")
			require.True(t, ephemeralIDs[profileAck.ID], "Profile ephemeral ack should be included")
			require.True(t, ephemeralIDs[attestationAck.ID], "Attestation ephemeral ack should be included")
			require.False(t, ephemeralIDs[textNoteAck.ID], "Text note ephemeral ack should be filtered out")
		})
	})
}
