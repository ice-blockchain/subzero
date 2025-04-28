// SPDX-License-Identifier: ice License 1.0

package command

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/cometbft/multiplex/client"
	"github.com/ice-blockchain/cometbft/multiplex/server"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/validation"
)

type (
	consensus struct {
		server     server.Server
		client     client.Client
		cfg        *Config
		shutdownCh chan struct{}
	}
)

const consensusTimeout = time.Second * 25

var (
	ErrMultipleMasterKeys = errors.New("cannot broadcast single batch to multiple master keys")
)

func (c *consensus) AcceptBroadcastTx(ctx context.Context, userAddress string, transactions ...client.Transaction) error {
	events := make([]*model.Event, 0, len(transactions))
	for _, tx := range transactions {
		evs, err := mapTxToEvent(tx)
		if err != nil {
			return errors.Wrapf(err, "failed to transform tx into event: %v", tx.Data)
		}
		events = append(events, evs...)
	}
	ctx = context.WithValue(ctx, "consensusPort", c.cfg.DiscoveryPort)
	if consensusEventListener != nil && len(events) > 0 {
		err := consensusEventListener(ctx, events...)
		if err != nil {
			return errors.Wrapf(err, "failed to accept broadcasted txs %v", func() string {
				res := []string{}
				for _, tx := range transactions {
					res = append(res, string(tx.Data))
				}
				return "[" + strings.Join(res, ", ") + "]"
			}())
		}
	}
	return nil
}

func (c *consensus) RollbackTx(ctx context.Context, userAddress string, transactions ...client.Transaction) error {
	events := make([]*model.Event, 0, len(transactions))
	for _, tx := range transactions {
		evs, err := mapTxToEvent(tx)
		if err != nil {
			return errors.Wrapf(err, "failed to transform tx into event")
		}
		events = append(events, evs...)
	}
	ctx = context.WithValue(ctx, "consensusPort", c.cfg.DiscoveryPort)
	return errors.Wrapf(rollback(ctx, events...), "failed to rollback non-accepted txs")
}

func (c *consensus) AcceptBroadcastTxRemoval(ctx context.Context, userAddress string, transactions ...client.Transaction) error {
	events := make([]*model.Event, 0, len(transactions))
	for _, tx := range transactions {
		evs, err := mapTxToEvent(tx)
		if err != nil {
			return errors.Wrapf(err, "failed to transform removal tx into event")
		}
		for _, ev := range evs {
			if err = validation.Validate(ctx, ev); err != nil {
				return errors.Wrapf(err, "failed to validate removal tx")
			}
		}
		events = append(events, evs...)
	}
	ctx = context.WithValue(ctx, "consensusPort", c.cfg.DiscoveryPort)
	if consensusEventListener != nil && len(events) > 0 {
		err := consensusEventListener(ctx, events...)
		if err != nil {
			return errors.Wrapf(err, "failed to accept broadcasted txs %v", func() string {
				res := []string{}
				for _, tx := range transactions {
					res = append(res, string(tx.Data))
				}
				return "[" + strings.Join(res, ", ") + "]"
			}())
		}
	}
	return nil
}
func (c *consensus) RollbackTxRemoval(ctx context.Context, userAddress string, transactions ...client.Transaction) error {
	return nil
}

func (c *consensus) broadcastUserEvents(ctx context.Context, events ...*model.Event) error {
	userMasterKeys := map[string]bool{}
	relays := []string{}
	var profileDeletion *model.Event
	for _, ev := range events {
		if ev.IsEphemeral() {
			continue
		}
		if ev.Kind == nostr.KindRelayListMetadata {
			relays = collectRelaysFromRelayEvent(ev)
		}
		if ev.Kind == nostr.KindDeletion && (len(ev.Tags) == 0 || (len(ev.Tags) == 1 && ev.GetTag("b").Value() != "")) && ev.PubKey == ev.GetMasterPublicKey() {
			profileDeletion = ev
		}
		userMasterKeys[ev.GetMasterPublicKey()] = true
	}
	if len(userMasterKeys) > 1 {
		return ErrMultipleMasterKeys
	}
	var userMasterKey string
	for userKey, _ := range userMasterKeys {
		userMasterKey = userKey
		break
	}
	var err error
	if len(relays) == 0 {
		if relays, err = c.fetchUserRelays(ctx, userMasterKey); err != nil {
			return errors.Wrapf(err, "failed to get relay list for user %v", userMasterKey)
		}
	}
	broadcastCtx, broadcastCancel := context.WithTimeout(ctx, consensusTimeout)
	defer broadcastCancel()
	notifier := make(chan client.BroadcastStatus, 1)
	if profileDeletion != nil {
		c.client.BroadcastTxRemoval(broadcastCtx, userMasterKey, c.convertRelaysToBroadcastEndpoints(relays...), notifier)
		res := <-notifier
		return errors.Wrapf(res.Error, "failed to delete chains for user deletion %v", profileDeletion)
	}
	txs, err := mapEventsToTXs(ctx, events)
	if err != nil {
		return errors.Wrapf(err, "failed serialize events: %v", events)
	}
	userMasterKeyBytes, err := hex.DecodeString(userMasterKey)
	if err != nil {
		return errors.Wrapf(err, "failed to transform user key to address")
	}
	if len(txs) == 0 {
		return nil
	}
	userAddr, err := client.PubKeyToAddress(string(userMasterKeyBytes))
	if err != nil {
		return errors.Wrapf(err, "failed to transform user key to address")
	}
	c.client.BroadcastTx(broadcastCtx, userAddr, c.convertRelaysToBroadcastEndpoints(relays...), notifier, txs...)
	err = c.rollbackIfErr(ctx, userMasterKey, notifier, events...)

	return errors.Wrapf(err, "failed to broadcast user events for %v to %#v", userMasterKeys, relays)
}

func (c *consensus) rollbackIfErr(ctx context.Context, userMasterKey string, notifier <-chan client.BroadcastStatus, events ...*model.Event) error {
	var err error
	rollbackContext, rollbackCancel := context.WithTimeout(context.Background(), 30*time.Second)
	rollbackContext = context.WithValue(rollbackContext, "consensusPort", c.cfg.DiscoveryPort)
	defer rollbackCancel()
	select {
	case res := <-notifier:
		if res.Error != nil {
			err = errors.Wrapf(res.Error, "failed to broadcast txs for %v", userMasterKey)
			rErr := errors.Wrapf(rollback(rollbackContext, events...), "failed to rollback changes due to failed consensus")
			if rErr != nil {
				err = errors.Join(err, rErr)
			}
			return err
		}
	case <-ctx.Done():
		err = context.Canceled
		rErr := errors.Wrapf(rollback(rollbackContext, events...), "failed to rollback changes due to failed consensus")
		if rErr != nil {
			err = errors.Join(err, rErr)
		}
	}
	return err
}

func (c *consensus) fetchUserRelays(ctx context.Context, userMasterKey string) (relays []string, err error) {
	evIt := query.GetStoredEvents(ctx, &model.Subscription{
		Filters: model.Filters{
			model.Filter{Authors: []string{userMasterKey}, Kinds: []int{nostr.KindRelayListMetadata}},
		},
	})
	for ev, iErr := range evIt {
		if iErr != nil {
			return nil, errors.Wrapf(err, "failed to fetch user's relays for user %v", userMasterKey)
		}
		relays = collectRelaysFromRelayEvent(ev)
		break
	}
	return relays, nil
}

func collectRelaysFromRelayEvent(ev *model.Event) []string {
	relays := make([]string, 0, len(ev.Tags))
	for _, tag := range ev.Tags {
		if tag.Key() == "r" {
			relays = append(relays, tag[1])
		}
	}
	return relays
}

func mapEventKindToChainFingerprint(kind int) (fingerprint string) {
	switch kind {
	case
		nostr.KindTextNote,
		nostr.KindRepost,
		nostr.KindReaction,
		nostr.KindGenericRepost,
		nostr.KindReactionToWebsite,
		nostr.KindArticle,
		model.CustomIONKindEditableTextNote,
		nostr.KindDraftArticle,
		nostr.KindFileMetadata:
		fingerprint = client.GetFingerprint("feed")
	case
		nostr.KindProfileMetadata,
		nostr.KindRelayListMetadata,
		nostr.KindSearchRelayList,
		nostr.KindInterestSets,
		nostr.KindInterestList,
		nostr.KindFollowList,
		model.CustomIONKindAttestation,
		nostr.KindDMRelayList,
		nostr.KindBookmarkSets,
		nostr.KindBadgeAward,
		nostr.KindMuteList,
		nostr.KindPinList,
		nostr.KindBookmarkList,
		nostr.KindBlockedRelayList,
		nostr.KindProfileBadges:
		fingerprint = client.GetFingerprint("profile")
	case nostr.KindPublicChatList,
		nostr.KindSimpleGroupList,
		nostr.KindGiftWrap,
		nostr.KindSeal,
		model.CustomIONKindCommunityJoin,
		model.CustomIONKindCommunityOwnershipTransferring,
		model.CustomIONKindCommunityBanUser,
		model.CustomIONKindCommunityChangeDefinition,
		model.CustomIONKindCommunityDefinition,
		nostr.KindDirectMessage:
		fingerprint = client.GetFingerprint("chat")
	case model.CustomIONKindFundSendNotify,
		model.CustomIONKindFundReceive:
		fingerprint = client.GetFingerprint("wallet")
	default:
		fingerprint = client.GetFingerprint("others")
	}
	return
}

func mapTxToEvent(tx client.Transaction) ([]*model.Event, error) {
	var env nostr.EventEnvelope
	err := env.UnmarshalJSON(tx.Data)

	var events []*model.Event
	for i := range env.Events {
		events = append(events, &model.Event{Event: *env.Events[i]})
	}

	return events, err
}

func mapEventsToTXs(ctx context.Context, events []*model.Event) (txs []client.Transaction, err error) {
	txs = make([]client.Transaction, 0, len(events))
	encodedEvents := map[string]nostr.EventEnvelope{}
	for _, ev := range events {
		if ev.IsEphemeral() {
			continue
		}
		fingerprint := mapEventKindToChainFingerprint(ev.Kind)
		var env nostr.EventEnvelope
		ok := false
		if env, ok = encodedEvents[fingerprint]; !ok {
			env = nostr.EventEnvelope{}
		}
		env.Events = append(env.Events, &ev.Event)
		encodedEvents[fingerprint] = env
	}
	for f, e := range encodedEvents {
		jBytes, err := e.MarshalJSON()
		if err != nil {
			return nil, errors.Wrapf(err, "failed to serialize events json")
		}
		txs = append(txs, client.Transaction{
			Data:        jBytes,
			Fingerprint: f,
		})
	}

	return txs, nil
}

func (c *consensus) convertRelaysToBroadcastEndpoints(relays ...string) []string {
	discoveryAddresses := make([]string, 0, len(relays))
	for _, relay := range relays {
		u, err := url.Parse(relay)
		if err != nil {
			log.Printf("malformed relay: %v", relay)
			continue
		}
		port, err := strconv.ParseUint(u.Port(), 10, 64)
		if err != nil {
			log.Printf("malformed relay: %v", relay)
			continue
		}
		discoveryPort := (port + 10000)
		discoveryAddresses = append(discoveryAddresses, fmt.Sprintf("%v:%v", u.Hostname(), discoveryPort))
	}
	return discoveryAddresses
}
