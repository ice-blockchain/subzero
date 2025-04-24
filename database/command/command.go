// SPDX-License-Identifier: ice License 1.0

package command

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
		for _, ev := range evs {
			if err = validation.ValidateIncomingEvent(ctx, ev, globalCfg.NIP13MinLeadingZeroBits); err != nil {
				return errors.Wrapf(err, "failed to validate tx %x", tx.Data)
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
	broadcastCtx, broadcastCancel := context.WithTimeout(ctx, consensusTimeout)
	defer broadcastCancel()
	notifier := make(chan client.BroadcastStatus, 100)
	if profileDeletion != nil {
		c.client.BroadcastTxRemoval(broadcastCtx, userMasterKey, c.convertRelaysToBroadcastEndpoints(relays...), notifier)
		res := <-notifier
		return errors.Wrapf(res.Error, "failed to delete chains for user deletion %v", profileDeletion)
	}
	txs, otherUserTxDueToLinkedEvents, err := mapEventsToTXs(ctx, events)
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
	if err == nil {
		for otherUserMasterKey, otherUserTx := range otherUserTxDueToLinkedEvents {
			otherUserRelays, err := c.fetchUserRelays(ctx, otherUserMasterKey)
			if err != nil {
				return errors.Wrapf(err, "failed to get relay list for user %v", userMasterKey)
			}
			otherUserNotifier := make(chan client.BroadcastStatus, 100)
			otherUserMasterKeyBytes, err := hex.DecodeString(otherUserMasterKey)
			if err != nil {
				return errors.Wrapf(err, "failed to transform user key to address")
			}
			otherUserAddr, err := client.PubKeyToAddress(string(otherUserMasterKeyBytes))
			if err != nil {
				return errors.Wrapf(err, "failed to transform user key to address")
			}
			c.client.BroadcastTx(broadcastCtx, otherUserAddr, c.convertRelaysToBroadcastEndpoints(otherUserRelays...), otherUserNotifier, otherUserTx...)
			otherUserEvents := []*model.Event{}
			for _, otherUserTransaction := range otherUserTx {
				var env nostr.EventEnvelope
				if jErr := env.UnmarshalJSON(otherUserTransaction.Data); jErr != nil {
					return errors.Wrapf(jErr, "failed to unmarshal event %v", string(otherUserTransaction.Data))
				}
				evs := make([]*model.Event, 0, len(env.Events))
				for _, e := range env.Events {
					evs = append(evs, &model.Event{*e})
				}
				otherUserEvents = append(otherUserEvents, evs...)
			}
			err = c.rollbackIfErr(ctx, otherUserMasterKey, notifier, otherUserEvents...)
		}
	}
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
	createdAt := int64(0)
	for ev, iErr := range evIt {
		if iErr != nil {
			return nil, errors.Wrapf(err, "failed to fetch user's relays for user %v", userMasterKey)
		}
		if ev.CreatedAt.Time().UnixNano() > createdAt {
			relays = collectRelaysFromRelayEvent(ev)
			createdAt = ev.CreatedAt.Time().UnixNano()
		}
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
	var env nostr.EventEnvelope
	env.Events = append(env.Events, &event.Event)
	jBytes, err := env.MarshalJSON()
	if err != nil {
		return "", nil, errors.Wrapf(err, "failed to serialize linked event")
	}
	return linkedEvent.GetMasterPublicKey(), &client.Transaction{
		Data:        jBytes,
		Fingerprint: mapEventKindToChainFingerprint(linkedEvent.Kind),
	}, nil
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

func mapEventsToTXs(ctx context.Context, events []*model.Event) (txs []client.Transaction, otherUserTxs map[string][]client.Transaction, err error) {
	txs = make([]client.Transaction, 0, len(events))
	otherUserTxs = make(map[string][]client.Transaction)
	encodedEvents := map[string]nostr.EventEnvelope{}
	for _, ev := range events {
		if ev.IsEphemeral() {
			continue
		}
		linkedMasterKey, otherUserTx, err := mapLinkedEventToTx(ctx, ev)
		if err == nil {
			otherUserTxs[linkedMasterKey] = append(otherUserTxs[linkedMasterKey], *otherUserTx)
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
			return nil, nil, errors.Wrapf(err, "failed to serialize events json")
		}
		txs = append(txs, client.Transaction{
			Data:        jBytes,
			Fingerprint: f,
		})
	}

	return txs, otherUserTxs, nil
}

func (c *consensus) convertRelaysToBroadcastEndpoints(relays ...string) []string {
	discoveryAddresses := make([]string, 0, len(relays))
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
		discoveryAddresses = append(discoveryAddresses, fmt.Sprintf("%v:%v", u.Hostname(), globalRPCPort))
	}
	return discoveryAddresses
}
