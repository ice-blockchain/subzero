// SPDX-License-Identifier: ice License 1.0

package command

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
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

func TestBroadcastLinkedEventBadges(t *testing.T) {
	heimdallPrivKey, heimdallPubkey := model.GenerateKeyPair()
	relaysList := &model.Event{Event: nostr.Event{
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      nostr.KindRelayListMetadata,
		Tags: nostr.Tags{
			[]string{model.CustomIONTagOnBehalfOf, heimdallPubkey},
			[]string{"r", "wss://localhost:9988"},
			[]string{"r", "wss://localhost:9977"},
		},
	}}
	require.NoError(t, relaysList.SignWithAlg(heimdallPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, query.AcceptEvents(t.Context(), relaysList))
	consensusHeimClient := fixture.NewCallbackClient(func(userAddress string, relays []string, transactions ...client.Transaction) {
		for _, tx := range transactions {
			evs, err := mapTxToEvent(tx)
			require.NoError(t, err)
			unhex, err := hex.DecodeString(heimdallPubkey)
			require.NoError(t, err)
			addr, err := client.PubKeyToAddress(string(unhex))
			require.NoError(t, err)
			expected := map[int]string{
				nostr.KindBadgeAward: addr,
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
	c.client = consensusHeimClient

	t.Run("badge", func(t *testing.T) {
		attestationForHeimdall := &model.Event{Event: nostr.Event{
			Kind:      model.CustomIONKindAttestation,
			CreatedAt: 1,
			Tags: model.Tags{
				{model.TagAttestationName, heimdallPubkey, "", model.CustomIONAttestationKindActive + ":" + strconv.Itoa(int(time.Now().Unix()-10))},
			},
		}}
		require.NoError(t, attestationForHeimdall.SignWithAlg(heimdallPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		heimdallRelaysList := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindRelayListMetadata,
			Tags: nostr.Tags{
				[]string{model.CustomIONTagOnBehalfOf, heimdallPubkey},
				[]string{"r", "wss://localhost:9988"},
				[]string{"r", "wss://localhost:9977"},
			},
		}}
		require.NoError(t, heimdallRelaysList.SignWithAlg(heimdallPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		badgeEvent := &model.Event{
			Event: nostr.Event{
				PubKey:    heimdallPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindBadgeDefinition,
				Tags: nostr.Tags{
					{"d", "verified"},
					{"name", "verified"},
					{"description", "verified"},
					{"image", "https://bogus.com/pic.jpg", "1024x1024"},
					{"thumb", "https://bogus.com/pic.jpg", "256x256"},
					[]string{model.CustomIONTagOnBehalfOf, heimdallPubkey},
				},
			},
		}
		require.NoError(t, badgeEvent.SignWithAlg(heimdallPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		badgeAwardEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindBadgeAward,
			Tags: nostr.Tags{
				[]string{model.CustomIONTagOnBehalfOf, heimdallPubkey},
				[]string{"a", fmt.Sprintf("30009:%v:verified", heimdallPubkey)},
				[]string{"p", heimdallPubkey},
			},
		}}
		require.NoError(t, badgeAwardEvent.SignWithAlg(heimdallPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		heimdalProfileMetadataEvt := &model.Event{
			Event: nostr.Event{
				PubKey:    heimdallPubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindProfileMetadata,
				Content:   `{"name":"heimdall","display_name":"heimdall"}`,
			},
		}
		require.NoError(t, heimdalProfileMetadataEvt.SignWithAlg(heimdallPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		ack := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      model.CustomIONKindEphemeralEmbeddding,
				Tags: nostr.Tags{
					[]string{model.CustomIONTagOnBehalfOf, heimdallPubkey},
					[]string{"e", badgeAwardEvent.ID},
					[]string{"e", badgeEvent.ID},
				},
				Content: heimdalProfileMetadataEvt.String(),
			}}
		require.NoError(t, ack.SignWithAlg(heimdallPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, query.AcceptEvents(t.Context(), attestationForHeimdall, heimdallRelaysList, badgeAwardEvent, badgeEvent, ack))
		require.NoError(t, c.broadcastUserEvents(t.Context(), badgeAwardEvent, ack))
	})
}
