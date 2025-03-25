// SPDX-License-Identifier: ice License 1.0

package command

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/ice-blockchain/cometbft/multiplex/client"
	"github.com/ice-blockchain/subzero/database/command/fixture"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

var c *consensus

func TestMain(m *testing.M) {
	serverCtx, serverCancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer serverCancel()
	query.MustInit(serverCtx)
	c = mustInit(serverCtx).(*consensus)
	code := m.Run()
	serverCancel()
	defer func() {
		if err := os.RemoveAll("./../../.cometbft"); err != nil {
			log.Panic(err)
		}
	}()
	if code == 0 {
		time.Sleep(10 * time.Second)
		if err := goleak.Find(); err != nil {
			log.Printf("goleak: %v", err)
			code = 1
		}
	}

	os.Exit(code)
}

func TestBroadcastLinkedEvent(t *testing.T) {
	masterPubkey, masterPrivkey, _ := ed25519.GenerateKey(nil)
	relaysList := &model.Event{Event: nostr.Event{
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      nostr.KindRelayListMetadata,
		Tags: nostr.Tags{
			[]string{model.CustomIONTagOnBehalfOf, hex.EncodeToString(masterPubkey)},
			[]string{"r", "wss://localhost:9998"},
			[]string{"r", "wss://localhost:9997"},
		},
	}}
	require.NoError(t, relaysList.SignWithAlg(hex.EncodeToString(masterPrivkey), model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, query.AcceptEvents(context.TODO(), relaysList))
	t.Run("repost", func(t *testing.T) {
		repostedEventID := uuid.NewString()
		pubKeyOfRepostedNote, _ := nostr.GetPublicKey(nostr.GeneratePrivateKey())
		masterPubkeyOfRepostedNote, _ := hex.DecodeString("7e6dc029a5512d8047a7b4d2e00803b1cb2e78782f4588b6061df2df54658db1") // Has chainID
		repostEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindRepost,
			Tags:      nostr.Tags{[]string{"e", repostedEventID, "relay"}, []string{"p", pubKeyOfRepostedNote}, []string{model.CustomIONTagOnBehalfOf, hex.EncodeToString(masterPubkey)}},
			Content:   fmt.Sprintf(`{"kind":1,"id":"%v","pubkey":"%v","tags":[["b","%x"]]}`, repostedEventID, pubKeyOfRepostedNote, masterPubkeyOfRepostedNote),
		}}
		require.NoError(t, repostEvent.Sign(nostr.GeneratePrivateKey()))
		require.NoError(t, c.broadcastUserEvents(context.TODO(), repostEvent))
	})
	t.Run("reaction", func(t *testing.T) {
		masterPubkeyOfOriginalNote, _ := hex.DecodeString("2084109897af017d3cf01ca73ea493e37f6cb0c2af0e9abd7f0f70d45e70161a")                                                                  // Has chainID
		masterPrivKeyOfOriginalNote, _ := hex.DecodeString("3e4dc3b7d14ce46677ac98d5918a49b94c5f93ca5a72244a9f745dc266882c302084109897af017d3cf01ca73ea493e37f6cb0c2af0e9abd7f0f70d45e70161a") // Has chainID
		privKeyOfOriginalNote := nostr.GeneratePrivateKey()
		pk, _ := nostr.GetPublicKey(privKeyOfOriginalNote)
		attestationEvent := &model.Event{Event: nostr.Event{
			Kind:      model.CustomIONKindAttestation,
			CreatedAt: 2,
			Tags: model.Tags{
				{model.TagAttestationName, pk, "", model.CustomIONAttestationKindActive + ":" + strconv.Itoa(int(time.Now().Unix()-10))},
			},
		}}
		require.NoError(t, attestationEvent.SignWithAlg(hex.EncodeToString(masterPrivKeyOfOriginalNote), model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, query.AcceptEvents(context.TODO(), attestationEvent))
		originalEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindTextNote,
			Tags: nostr.Tags{
				[]string{model.CustomIONTagOnBehalfOf, hex.EncodeToString(masterPubkeyOfOriginalNote)},
			},
			Content: "validEvent",
		}}
		require.NoError(t, originalEvent.Sign(privKeyOfOriginalNote))
		require.NoError(t, query.AcceptEvents(context.TODO(), originalEvent))
		reactionEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindReaction,
			Tags:      nostr.Tags{[]string{"e", originalEvent.ID}, []string{"p", originalEvent.PubKey}, []string{model.CustomIONTagOnBehalfOf, hex.EncodeToString(masterPubkey)}},
			Content:   "+",
		}}
		require.NoError(t, reactionEvent.Sign(nostr.GeneratePrivateKey()))
		require.NoError(t, c.broadcastUserEvents(context.TODO(), reactionEvent))
	})
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
	privKeyOfOriginalNote := nostr.GeneratePrivateKey()
	originalEvent := &model.Event{Event: nostr.Event{
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      nostr.KindTextNote,
		Content:   "validEvent",
	}}
	require.NoError(t, originalEvent.Sign(privKeyOfOriginalNote))
	c.client = consensusClient
	require.Error(t, c.broadcastUserEvents(context.TODO(), originalEvent))
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
	require.NoError(t, query.AcceptEvents(context.TODO(), attestationEvent, profileEvent))
	consensusClient := fixture.NewCallbackClient(func(userAddress string, relays []string, transactions ...client.Transaction) {
		require.Fail(t, "Accept should not be called")
	}, func(userAddress string, relays []string, transactions ...client.Transaction) {
		require.Equal(t, masterPubkey, userAddress)
	})
	c.client = consensusClient
	deletionProfile := &model.Event{Event: nostr.Event{
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      nostr.KindDeletion,
		Tags: nostr.Tags{}.
			AppendUnique(nostr.Tag{"b", masterPubkey}),
	}}
	require.NoError(t, profileEvent.Sign(delegatedPrivKey))
	require.NoError(t, c.broadcastUserEvents(context.TODO(), deletionProfile))
}
