// SPDX-License-Identifier: ice License 1.0

package command

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/cometbft/multiplex/client"
	"github.com/ice-blockchain/cometbft/multiplex/server"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/validation"
)

type (
	Consensus interface {
	}
	consensus struct {
		server server.Server
		client client.Client
	}
)

var (
	ErrMultipleMasterKeys = errors.New("cannot broadcast single batch to multiple master keys")
)

func (c *consensus) AcceptBroadcastTx(ctx context.Context, userAddress string, transactions ...client.Transaction) error {
	events := make([]*model.Event, 0, len(transactions))
	for _, tx := range transactions {
		ev, err := mapTxToEvent(tx)
		if err != nil {
			return errors.Wrapf(err, "failed to transform tx into event")
		}
		if err = validation.ValidateIncomingEvent(ctx, ev, globalCfg.NIP13MinLeadingZeroBits); err != nil {
			return errors.Wrapf(err, "failed to validate tx")
		}
		events = append(events, ev)
	}
	return errors.Wrapf(consensusEventListener(ctx, events...), "failed to accept broadcasted txs")
}

func (c *consensus) RollbackTx(ctx context.Context, userAddress string, transactions ...client.Transaction) error {
	events := make([]*model.Event, 0, len(transactions))
	for _, tx := range transactions {
		ev, err := mapTxToEvent(tx)
		if err != nil {
			return errors.Wrapf(err, "failed to transform tx into event")
		}
		events = append(events, ev)
	}
	return errors.Wrapf(rollback(ctx, events...), "failed to rollback non-accepted txs")
}

func (c *consensus) AcceptBroadcastTxRemoval(ctx context.Context, userAddress string, transactions ...client.Transaction) error {
	for _, tx := range transactions {
		ev, err := mapTxToEvent(tx)
		if err != nil {
			return errors.Wrapf(err, "failed to transform removal tx into event")
		}
		if err = validation.Validate(ctx, ev); err != nil {
			return errors.Wrapf(err, "failed to validate removal tx")
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
		if ev.Kind == nostr.KindRelayListMetadata {
			relays = collectRelaysFromRelayEvent(ev)
		}
		if ev.Kind == nostr.KindDeletion && (len(ev.Tags) == 0 || (len(ev.Tags) == 1 && ev.GetTag("b").Value() != "")) && ev.GetMasterPublicKey() != "" {
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
	notifier := make(chan client.BroadcastStatus, 10)
	if profileDeletion != nil {
		c.client.BroadcastTxRemoval(ctx, userMasterKey, convertRelaysToBroadcastEndpoints(relays...), notifier)
		res := <-notifier
		return errors.Wrapf(res.Error, "failed to delete chains for user deletion %v", profileDeletion)
	}
	txs, otherUserTxDueToLinkedEvents := mapEventsToTXs(ctx, events)
	userMasterKeyBytes, err := hex.DecodeString(userMasterKey)
	if err != nil {
		return errors.Wrapf(err, "failed to transform user key to address")
	}
	userAddr, err := client.PubKeyToAddress(string(userMasterKeyBytes))
	if err != nil {
		return errors.Wrapf(err, "failed to transform user key to address")
	}
	c.client.BroadcastTx(ctx, userAddr, convertRelaysToBroadcastEndpoints(relays...), notifier, txs...)
	res := <-notifier
	if res.Error != nil {
		err = errors.Wrapf(res.Error, "failed to broadcast txs for %v to %#v", userMasterKey, relays)
		rErr := errors.Wrapf(rollback(ctx, events...), "failed to rollback changes due to failed consensus")
		if rErr != nil {
			err = errors.Join(err, rErr)
		}
		return err
	}
	for otherUserMasterKey, otherUserTx := range otherUserTxDueToLinkedEvents {
		otherUserRelays, err := c.fetchUserRelays(ctx, otherUserMasterKey)
		if err != nil {
			return errors.Wrapf(err, "failed to get relay list for user %v", userMasterKey)
		}
		otherUserNotifier := make(chan client.BroadcastStatus, 1)
		otherUserMasterKeyBytes, err := hex.DecodeString(otherUserMasterKey)
		if err != nil {
			return errors.Wrapf(err, "failed to transform user key to address")
		}
		otherUserAddr, err := client.PubKeyToAddress(string(otherUserMasterKeyBytes))
		if err != nil {
			return errors.Wrapf(err, "failed to transform user key to address")
		}
		c.client.BroadcastTx(ctx, otherUserAddr, convertRelaysToBroadcastEndpoints(otherUserRelays...), otherUserNotifier, otherUserTx...)
		res = <-notifier
		if res.Error != nil {
			err = errors.Wrapf(res.Error, "failed to broadcast txs for %v to %#v", otherUserMasterKey, otherUserRelays)
			otherUserEvents := []*model.Event{}
			for _, otherUserTransaction := range otherUserTx {
				var ev model.Event
				if jErr := ev.UnmarshalJSON(otherUserTransaction.Data); jErr != nil {
					return errors.Wrapf(jErr, "failed to unmarshal event %v", string(otherUserTransaction.Data))
				}
				otherUserEvents = append(otherUserEvents, &ev)
			}
			rErr := errors.Wrapf(rollback(ctx, otherUserEvents...), "failed to rollback changes due to failed consensus")
			if rErr != nil {
				err = errors.Join(err, rErr)
			}
			return err
		}
	}
	return nil
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
		nostr.KindDraftArticle:
		fingerprint = client.GetFingerprint("posts")
	case nostr.KindProfileMetadata,
		nostr.KindFollowList,
		nostr.KindBadgeAward,
		nostr.KindMuteList,
		nostr.KindPinList,
		nostr.KindBookmarkList,
		nostr.KindBlockedRelayList,
		nostr.KindRelayListMetadata,
		nostr.KindSearchRelayList,
		nostr.KindProfileBadges:
		fingerprint = client.GetFingerprint("profile")
	case nostr.KindPublicChatList,
		nostr.KindSimpleGroupList,
		nostr.KindDMRelayList:
		fingerprint = client.GetFingerprint("chat")
	case nostr.KindFileMetadata:
		fingerprint = client.GetFingerprint("files")
	default:
		fingerprint = client.GetFingerprint("others")
	}
	return
}

func mapLinkedEventToTx(ctx context.Context, event *model.Event) (linkedMasterKey string, transaction *client.Transaction, err error) {
	var linkedEvent *model.Event
	switch event.Kind {
	case nostr.KindRepost:
		err = json.Unmarshal([]byte(event.Content), &linkedEvent)
	case nostr.KindReaction, nostr.KindTextNote:
		if eTag := event.GetTag("e"); eTag != nil && eTag.Value() != "" {
			it := query.GetStoredEvents(ctx, &model.Subscription{Filters: model.Filters{
				model.Filter{IDs: eTag},
			}})
			for ev, iErr := range it {
				if iErr != nil {
					return "", nil, errors.Wrapf(iErr, "failed to fetch linked event for %+v", event)
				}
				linkedEvent = ev
				break
			}
		}
	default:
		return "", nil, ErrUserIsNotPresentedOnRelay
	}
	if err != nil {
		return "", nil, errors.Wrapf(err, "failed to fetch linked event for %+v", event)
	}
	if linkedEvent == nil {
		return "", nil, ErrUserIsNotPresentedOnRelay
	}
	return linkedEvent.GetMasterPublicKey(), &client.Transaction{
		Data:        event.Serialize(),
		Fingerprint: mapEventKindToChainFingerprint(linkedEvent.Kind),
	}, nil
}
func mapTxToEvent(tx client.Transaction) (*model.Event, error) {
	var ev model.Event
	if err := json.Unmarshal(tx.Data, &ev); err != nil {
		return nil, errors.Wrapf(err, "failed to parse transaction %v into event", hex.EncodeToString(tx.Data))
	}
	return &ev, nil
}

func mapEventsToTXs(ctx context.Context, events []*model.Event) (txs []client.Transaction, otherUserTxs map[string][]client.Transaction) {
	txs = make([]client.Transaction, 0, len(events))
	otherUserTxs = make(map[string][]client.Transaction)
	for _, ev := range events {
		linkedMasterKey, otherUserTx, err := mapLinkedEventToTx(ctx, ev)
		if err == nil {
			otherUserTxs[linkedMasterKey] = append(otherUserTxs[linkedMasterKey], *otherUserTx)
			continue
		}
		fingerprint := mapEventKindToChainFingerprint(ev.Kind)
		txs = append(txs, client.Transaction{
			Data:        []byte(ev.String()),
			Fingerprint: fingerprint,
		})
	}
	return txs, otherUserTxs
}

func convertRelaysToBroadcastEndpoints(relays ...string) []string {
	rpcAddresses := make([]string, 0, len(relays))
	for _, relay := range relays {
		u, err := url.Parse(relay)
		if err != nil {
			continue
		}
		port, err := strconv.ParseUint(u.Port(), 10, 64)
		if err != nil {
			continue
		}
		globalRPCPort := (port + 10000)
		rpcAddresses = append(rpcAddresses, fmt.Sprintf("%v:%v", u.Hostname(), globalRPCPort))
	}
	return rpcAddresses
}
