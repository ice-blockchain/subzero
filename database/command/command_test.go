// SPDX-License-Identifier: ice License 1.0

package command

import (
	"context"
	"encoding/hex"
	"log"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/ice-blockchain/cometbft/config"
	"github.com/ice-blockchain/cometbft/multiplex/client"
	"github.com/ice-blockchain/subzero/cfg"
	"github.com/ice-blockchain/subzero/database/command/fixture"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

var c, c2 *consensus

func TestMain(m *testing.M) {
	serverCtx, serverCancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer serverCancel()
	conn, closeDb := query.NewTestDatabase(serverCtx)
	query.MustInit(serverCtx, query.WithConfig(&query.Config{
		URL: conn,
	}))
	c = mustInit(serverCtx, config.DefaultConfig()).(*consensus)
	defer func() {
		if err := os.RemoveAll(globalCfg.AbsoluteRootPath); err != nil {
			log.Panic(err)
		}
	}()
	cfg.MustInit("./.testdata/application2.yaml")
	c2 = mustInit(serverCtx, config.DefaultConfig()).(*consensus)
	code := m.Run()
	serverCancel()
	closeDb()
	defer func() {
		if err := os.RemoveAll(globalCfg.AbsoluteRootPath); err != nil {
			log.Panic(err)
		}
	}()
	if code == 0 {
		if err := goleak.Find(); err != nil {
			log.Printf("goleak: %v", err)
			code = 1
		}
	}
	defer func() {
		if err := os.RemoveAll("../../.cometbft"); err != nil {
			log.Printf("err cleanup: %v", err)
		}
		if err := os.RemoveAll("../../.cometbft2"); err != nil {
			log.Printf("err cleanup: %v", err)
		}
	}()
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
	consensusClient := fixture.NewErrornousClient()
	privKeyOfOriginalNote := model.GeneratePrivateKey()
	originalEvent := &model.Event{Event: nostr.Event{
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      nostr.KindTextNote,
		Content:   "validEvent",
	}}
	require.NoError(t, originalEvent.SignWithAlg(privKeyOfOriginalNote, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	c.client = consensusClient
	require.Error(t, c.broadcastUserEvents(t.Context(), originalEvent))
	require.True(t, rolledBack)
}

func TestBroadcastProfileDeletion(t *testing.T) {
	masterPrivKey, masterPubkey := model.GenerateKeyPair()
	delegatedPrivKey, pk := model.GenerateKeyPair()
	attestationEvent := &model.Event{Event: nostr.Event{
		Kind:      model.CustomIONKindAttestation,
		CreatedAt: 3,
		Tags: model.Tags{
			{model.TagAttestationName, pk, "", model.CustomIONAttestationKindActive + ":" + strconv.Itoa(int(time.Now().Unix()-10))},
		},
	}}
	require.NoError(t, attestationEvent.SignWithAlg(masterPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	profileEvent := &model.Event{Event: nostr.Event{
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      nostr.KindProfileMetadata,
		Tags:      nostr.Tags{}.AppendUnique(nostr.Tag{"b", masterPubkey}),
		Content:   "{\"name\": \"bogus\", \"about\":\"bogus\", \"picture\": \"https://bogus.com/pic.jpg\"}",
	}}
	require.NoError(t, profileEvent.SignWithAlg(delegatedPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, query.AcceptEvents(t.Context(), attestationEvent, profileEvent))
	consensusClient := fixture.NewCallbackClient(func(userAddress string, relays []string, transactions ...client.Transaction) {
		require.Fail(t, "Accept should not be called")
	}, func(userAddress string, relays []string, transactions ...client.Transaction) {
		require.Equal(t, masterPubkey, userAddress)
	})
	c.client = consensusClient
	deletionProfile := &model.Event{Event: nostr.Event{
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      nostr.KindDeletion,
		Tags:      nostr.Tags{},
		PubKey:    masterPubkey,
	}}
	require.NoError(t, profileEvent.SignWithAlg(masterPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, c.broadcastUserEvents(t.Context(), deletionProfile))
}

func TestBroadcastLinkedEvent(t *testing.T) {
	masterPrivKey, masterPubkey := model.GenerateKeyPair()
	priv, pk := model.GenerateKeyPair()
	attestationEvent := &model.Event{Event: nostr.Event{
		Kind:      model.CustomIONKindAttestation,
		CreatedAt: 1,
		Tags: model.Tags{
			{model.TagAttestationName, pk, "", model.CustomIONAttestationKindActive + ":" + strconv.Itoa(int(time.Now().Unix()-10))},
		},
	}}
	require.NoError(t, attestationEvent.SignWithAlg(masterPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	relaysList := &model.Event{Event: nostr.Event{
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      nostr.KindRelayListMetadata,
		Tags: nostr.Tags{
			[]string{model.CustomIONTagOnBehalfOf, masterPubkey},
			[]string{"r", "wss://localhost:9988"},
			[]string{"r", "wss://localhost:9977"},
		},
	}}
	require.NoError(t, relaysList.SignWithAlg(masterPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, query.AcceptEvents(t.Context(), attestationEvent, relaysList))

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
			for _, ev := range evs {
				require.Equal(t, expected[ev.Kind], userAddress)
			}
		}
	}, func(userAddress string, relays []string, transactions ...client.Transaction) {
		require.Fail(t, "Rollback should not be called")
	})
	c.client = consensusClient

	t.Run("repost", func(t *testing.T) {
		otherUserAttestation := &model.Event{Event: nostr.Event{
			Kind:      model.CustomIONKindAttestation,
			CreatedAt: 1,
			Tags: model.Tags{
				{model.TagAttestationName, pubKeyOfRepostedNote, "", model.CustomIONAttestationKindActive + ":" + strconv.Itoa(int(time.Now().Unix()-10))},
			},
		}}
		require.NoError(t, otherUserAttestation.SignWithAlg(masterPrivKeyOfRepostedNote, model.SignAlgEDDSA, model.KeyAlgCurve25519))

		otherUserRelaysList := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindRelayListMetadata,
			Tags: nostr.Tags{
				[]string{model.CustomIONTagOnBehalfOf, masterPubkeyOfRepostedNote},
				[]string{"r", "wss://localhost:9988"},
				[]string{"r", "wss://localhost:9977"},
			},
		}}
		require.NoError(t, otherUserRelaysList.SignWithAlg(masterPrivKeyOfRepostedNote, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, query.AcceptEvents(t.Context(), otherUserAttestation, otherUserRelaysList))

		repostedEvent := model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindTextNote,
			Tags: nostr.Tags{
				[]string{model.CustomIONTagOnBehalfOf, masterPubkeyOfRepostedNote},
			},
		}}
		require.NoError(t, repostedEvent.SignWithAlg(privkeyOfRepostedNote, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		repostEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindRepost,
			Tags: nostr.Tags{
				[]string{"e", repostedEvent.ID, "relay"},
				[]string{"p", repostedEvent.GetMasterPublicKey()},
				[]string{model.CustomIONTagOnBehalfOf, masterPubkey}},
			Content: repostedEvent.String(),
		}}
		ack := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.CustomIONKindEphemeralEmbeddding,
			Tags: nostr.Tags{
				[]string{model.CustomIONTagOnBehalfOf, masterPubkey}},
			Content: relaysList.String(),
		}}
		require.NoError(t, repostEvent.SignWithAlg(priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, ack.SignWithAlg(priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, query.AcceptEvents(t.Context(), repostEvent, ack))
		require.NoError(t, c.broadcastUserEvents(t.Context(), repostEvent, ack))
	})

	t.Run("reaction", func(t *testing.T) {
		originalEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindTextNote,
			Tags: nostr.Tags{
				[]string{model.CustomIONTagOnBehalfOf, masterPubkeyOfRepostedNote},
			},
			Content: "validEvent",
		}}
		require.NoError(t, originalEvent.SignWithAlg(privkeyOfRepostedNote, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, query.AcceptEvents(t.Context(), originalEvent))
		reactionEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindReaction,
			Tags: nostr.Tags{
				[]string{"e", originalEvent.ID},
				[]string{"k", strconv.Itoa(originalEvent.Kind)},
				[]string{"p", originalEvent.PubKey},
				[]string{model.CustomIONTagOnBehalfOf, masterPubkey}},
			Content: "+",
		}}
		ack := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      model.CustomIONKindEphemeralEmbeddding,
			Tags: nostr.Tags{
				[]string{model.CustomIONTagOnBehalfOf, masterPubkey}},
			Content: relaysList.String(),
		}}
		require.NoError(t, reactionEvent.SignWithAlg(priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, ack.SignWithAlg(priv, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, c.broadcastUserEvents(t.Context(), reactionEvent, ack))
	})
}
