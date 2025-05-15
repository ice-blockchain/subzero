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

	"github.com/ice-blockchain/cometbft/config"
	cmtlog "github.com/ice-blockchain/cometbft/libs/log"
	"github.com/ice-blockchain/cometbft/multiplex/client"
	"github.com/ice-blockchain/cometbft/multiplex/server"
	"github.com/ice-blockchain/subzero/model"
)

type (
	consensus struct {
		Server       server.Server
		Client       client.Client
		Logger       cmtlog.Logger
		ServerConfig *config.Config
		Config       *Config
		Query        QueryFunc
		ShutdownCh   chan chan<- error
	}
)

const consensusTimeout = time.Second * 25

var (
	ErrMultipleMasterKeys = errors.New("cannot broadcast single batch to multiple master keys")
)

func (c *consensus) waitForStop(ctx context.Context) {
	var ch chan<- error

	select {
	case <-ctx.Done():
		c.Logger.Debug("received shutdown signal from context")
	case ch = <-c.ShutdownCh:
		c.Logger.Debug("received shutdown signal from shutdown channel")
	}

	var err error
	if c.Server != nil {
		c.Logger.Debug("stopping consensus server")
		err = c.Server.Close()
		c.Logger.Debug("stopped consensus server")
	}
	if ch != nil {
		ch <- err
	}
	c.Server = nil
}

func (c *consensus) CommitBroadcastTx(ctx context.Context, transactions ...client.Transaction) error {
	events := make([]*model.Event, 0, len(transactions))
	for _, tx := range transactions {
		evs, err := mapTxToEvent(tx)
		if err != nil {
			return errors.Wrapf(err, "failed to transform tx into event: %v", tx.Data)
		}
		events = append(events, evs...)
	}
	ctx = context.WithValue(ctx, "consensusPort", c.Config.DiscoveryPort)
	if commitEventListener != nil && len(events) > 0 {
		err := commitEventListener(ctx, events...)
		if err != nil {
			return errors.Wrapf(err, "failed to commit broadcasted txs %v", func() string {
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

func (c *consensus) CommitBroadcastTxRemoval(ctx context.Context, transactions ...client.Transaction) error {
	return c.CommitBroadcastTx(ctx, transactions...)
}
func (c *consensus) AcceptBroadcastTx(ctx context.Context, transactions ...client.Transaction) error {
	events := make([]*model.Event, 0, len(transactions))
	for _, tx := range transactions {
		evs, err := mapTxToEvent(tx)
		if err != nil {
			return errors.Wrapf(err, "failed to transform tx into event: %v", tx.Data)
		}
		events = append(events, evs...)
	}
	ctx = context.WithValue(ctx, "consensusPort", c.Config.DiscoveryPort)
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

func (c *consensus) ReplayBroadcastTxBatch(ctx context.Context, transactions ...client.Transaction) error {
	events := make([]*model.Event, 0, len(transactions))
	for _, tx := range transactions {
		evs, err := mapTxToEvent(tx)
		if err != nil {
			return errors.Wrapf(err, "failed to transform tx into event: %v", tx.Data)
		}
		events = append(events, evs...)
	}
	ctx = context.WithValue(ctx, "consensusPort", c.Config.DiscoveryPort)
	ctx = context.WithValue(ctx, model.ConsensusReplayCtxKey, true)
	if consensusEventListener != nil && len(events) > 0 {
		err := consensusEventListener(ctx, events...)
		if err != nil {
			return errors.Wrapf(err, "failed to accept replayed txs (%v)", len(transactions))
		}
	}
	if commitEventListener != nil && len(events) > 0 {
		err := commitEventListener(ctx, events...)
		if err != nil {
			return errors.Wrapf(err, "failed to commit replayed txs (%v)", len(transactions))
		}
	}
	return nil
}

func (c *consensus) RollbackTx(ctx context.Context, transactions ...client.Transaction) error {
	events := make([]*model.Event, 0, len(transactions))
	for _, tx := range transactions {
		evs, err := mapTxToEvent(tx)
		if err != nil {
			return errors.Wrapf(err, "failed to transform tx into event")
		}
		events = append(events, evs...)
	}
	ctx = context.WithValue(ctx, "consensusPort", c.Config.DiscoveryPort)
	return errors.Wrapf(rollback(ctx, events...), "failed to rollback non-accepted txs")
}

func (c *consensus) AcceptBroadcastTxRemoval(ctx context.Context, transactions ...client.Transaction) error {
	return c.AcceptBroadcastTx(ctx, transactions...)
}
func (c *consensus) RollbackTxRemoval(ctx context.Context, transactions ...client.Transaction) error {
	return c.RollbackTx(ctx, transactions...)
}

func (c *consensus) broadcastUserEvents(ctx context.Context, events ...*model.Event) error {
	userMasterKey, relays, ackEvents, isProfileDeletion, err := c.getUserAndRelaysForBroadcast(ctx, events...)
	if err != nil {
		return errors.Wrapf(err, "failed to detect user master key and relays")
	}
	if userMasterKey == "" {
		return nil
	}
	if len(relays) == 0 {
		return errors.Wrapf(ErrUserIsNotPresentedOnRelay, "no relays found for user %v", userMasterKey)
	}
	broadcastCtx, broadcastCancel := context.WithTimeout(ctx, consensusTimeout)
	defer broadcastCancel()
	notifier := make(chan client.BroadcastStatus, 1)
	if isProfileDeletion {
		c.Client.BroadcastTxRemoval(broadcastCtx, userMasterKey, c.convertRelaysToBroadcastEndpoints(relays...), notifier)
		res := <-notifier
		return errors.Wrapf(res.Error, "failed to delete chains for user deletion %v", userMasterKey)
	}
	txs, err := mapEventsToTXs(events, ackEvents)
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
	c.Client.BroadcastTx(broadcastCtx, userAddr, c.convertRelaysToBroadcastEndpoints(relays...), notifier, txs...)
	err = c.rollbackIfErr(ctx, userMasterKey, userAddr, notifier, txs, events...)

	return errors.Wrapf(err, "failed to broadcast user events for %v to %#v", userMasterKey, relays)
}

func (c *consensus) rollbackIfErr(ctx context.Context, userMasterKey, userAddr string, notifier <-chan client.BroadcastStatus, txs []client.Transaction, events ...*model.Event) error {
	var err error
	rollbackContext, rollbackCancel := context.WithTimeout(context.Background(), 30*time.Second)
	rollbackContext = context.WithValue(rollbackContext, "consensusPort", c.Config.DiscoveryPort)
	defer rollbackCancel()
	select {
	case res := <-notifier:
		if res.Error != nil {
			err = errors.Wrapf(res.Error, "failed to broadcast txs for %v (%v)  (%v)", userMasterKey, userAddr, strings.Join(func() []string {
				logging := make([]string, 0, len(txs))
				for _, tx := range txs {
					logging = append(logging, fmt.Sprintf("%v = %X", string(tx.Data), client.TransactionToRawTx(tx).Hash()))
				}
				return logging
			}(), ", "))
			rErr := errors.Wrapf(rollback(rollbackContext, events...), "failed to rollback changes due to failed consensus")
			if rErr != nil {
				err = errors.Join(err, rErr)
			}
			return err
		}
	case <-ctx.Done():
		err = errors.Wrapf(context.Canceled, "failed to broadcast txs for %v (%v)  (%#v)", userMasterKey, userAddr, strings.Join(func() []string {
			logging := make([]string, 0, len(txs))
			for _, tx := range txs {
				logging = append(logging, fmt.Sprintf("%v = %X,", string(tx.Data), client.TransactionToRawTx(tx).Hash()))
			}
			return logging
		}(), ", "))
		rErr := errors.Wrapf(rollback(rollbackContext, events...), "failed to rollback changes due to failed consensus")
		if rErr != nil {
			err = errors.Join(err, rErr)
		}
	}
	return err
}

func (c *consensus) fetchUserRelays(ctx context.Context, userMasterKey string) (relays []string, err error) {
	evIt := c.Query(ctx,
		model.Filter{
			Authors: []string{userMasterKey},
			Kinds:   []int{nostr.KindRelayListMetadata},
		},
	)
	for ev, iErr := range evIt {
		if iErr != nil {
			return nil, errors.Wrapf(err, "failed to fetch user's relays for user %v", userMasterKey)
		}
		relays = collectRelaysFromRelayEvent(ev)
		break
	}
	return relays, nil
}

func (c *consensus) getUserAndRelaysForBroadcast(ctx context.Context, events ...*model.Event) (masterKey string, relays []string, matchingEphemeralAckEvents map[string][]*model.EphemeralEmbeddingEvent, isProfileDeletion bool, err error) {
	userMasterKeys := map[string][]string{}
	var profileDeletion *model.Event
	matchingEphemeralAckEvents, err = model.ParseEphemeralEmbeddingEvents(events...)

	for _, ev := range events {
		if ev.IsEphemeral() || ev.IsJobResponse() || ev.IsJobRequest() {
			continue
		}
		if ev.Kind == nostr.KindRelayListMetadata {
			relays = collectRelaysFromRelayEvent(ev)
		}
		if ev.Kind == nostr.KindDeletion && (len(ev.Tags) == 0) && ev.PubKey == ev.GetMasterPublicKey() {
			profileDeletion = ev
		}
		masterKey, err = c.broadcastMasterKey(ctx, ev, matchingEphemeralAckEvents, events)
		if err != nil {
			if errors.Is(err, ErrUserIsNotPresentedOnRelay) {
				continue
			}
			return "", nil, nil, false, errors.Wrapf(err, "failed to get master key to broadcast the event %+v", ev)
		}
		if userMasterKeys[masterKey] == nil {
			userMasterKeys[masterKey] = relays
		}
	}

	if len(userMasterKeys) > 1 {
		return "", nil, nil, false, ErrMultipleMasterKeys
	} else if len(userMasterKeys) == 0 {
		return "", nil, nil, false, nil
	}
	var userMasterKey string
	for userKey := range userMasterKeys {
		userMasterKey = userKey
		break
	}
	if len(userMasterKeys[userMasterKey]) == 0 {
		if userMasterKeys[userMasterKey], err = c.fetchUserRelays(ctx, userMasterKey); err != nil {
			return "", nil, nil, false, errors.Wrapf(err, "failed to get relay list for user %v", userMasterKey)
		}
	}

	return userMasterKey, userMasterKeys[userMasterKey], matchingEphemeralAckEvents, profileDeletion != nil, nil
}

func (c *consensus) broadcastMasterKey(ctx context.Context, ev *model.Event, ephemeralAckEvents map[string][]*model.EphemeralEmbeddingEvent, incomingEvents []*model.Event) (masterKey string, err error) {
	var acks []*model.EphemeralEmbeddingEvent
	hasAck := false
	if acks, hasAck = ephemeralAckEvents[ev.Address()]; !hasAck || len(acks) == 0 {
		switch ev.Kind {
		case nostr.KindGiftWrap:
			if pTag := ev.GetTag("p"); pTag != nil && len(pTag) > 2 {
				return pTag.Value(), nil
			}
		case nostr.KindBadgeAward:
			hasBadgeDefinition := false
			for _, evt := range incomingEvents {
				if evt.Kind == nostr.KindBadgeDefinition {
					hasBadgeDefinition = true

					break
				}
			}
			if pTag := ev.GetTag("p"); pTag != nil && pTag.Value() != "" {
				masterKey = pTag.Value()
			}
			if masterKey == "" || !hasBadgeDefinition {
				return "", errors.Wrapf(ErrUserIsNotPresentedOnRelay, "no badge definition found in events or no p tag %v", ev.ID)
			}

			return masterKey, nil
		case nostr.KindBadgeDefinition:
			for _, evt := range incomingEvents {
				if evt.Kind == nostr.KindBadgeAward {
					if pTag := evt.GetTag("p"); pTag != nil && pTag.Value() != "" {
						masterKey = pTag.Value()

						break
					}
				}
			}
			if masterKey == "" {
				return "", errors.Wrapf(ErrUserIsNotPresentedOnRelay, "no badge award found in events or no p tag %v", ev.ID)
			}

			return masterKey, nil
		default:
			return ev.GetMasterPublicKey(), nil
		}
	}
	var linkedEvent *model.Event
	switch ev.Kind {
	case nostr.KindRepost, nostr.KindGenericRepost:
		var repostedEvent model.Event
		err = repostedEvent.UnmarshalJSON([]byte(ev.Content))
		if err != nil {
			return "", errors.Wrapf(err, "malformed repost")
		}
		linkedEvent = &repostedEvent
		masterKey = repostedEvent.GetMasterPublicKey()
	case nostr.KindFollowList:
		if pTag := ev.GetTag("p"); pTag != nil && len(pTag) > 2 {
			masterKey = pTag.Value()
		}
	case nostr.KindReaction:
		if eTag := ev.GetTag("e"); eTag != nil && eTag.Value() != "" {
			linkedEvent, masterKey, err = c.getEvent(ctx, eTag.Value())
			if err != nil {
				return "", errors.Wrapf(err, "failed to get referenced event")
			}
		}
		if aTag := ev.GetTag("a"); aTag != nil && aTag.Value() != "" {
			linkedEvent, masterKey, err = c.getEvent(ctx, aTag.Value())
			if err != nil {
				return "", errors.Wrapf(err, "failed to get referenced event")
			}
		}
	case nostr.KindTextNote, model.CustomIONKindEditableTextNote, nostr.KindArticle:
		if pTag := ev.GetTag("p"); pTag != nil && pTag.Value() != "" {
			relays, rErr := c.fetchUserRelays(ctx, pTag.Value()) // Mentioned user is presented on relay
			if rErr == nil && len(relays) > 0 {
				masterKey = pTag.Value()
			}
		}
		if eTag := ev.GetTag("e"); eTag != nil && eTag.Value() != "" && len(eTag) >= 4 && eTag[3] == model.TagMarkerReply {
			linkedEvent, masterKey, err = c.getEvent(ctx, eTag.Value())
			if err != nil {
				return "", errors.Wrapf(err, "failed to get referenced event")
			}
		}
		refTags := []string{"q", "Q", "a"}
		for _, tagName := range refTags {
			if tag := ev.GetTag(tagName); tag != nil && tag.Value() != "" {
				linkedEvent, masterKey, err := c.getEvent(ctx, tag.Value())
				if err != nil {
					return "", errors.Wrapf(err, "failed to get referenced event")
				}
				if linkedEvent != nil && masterKey != "" {
					break
				}
			}
		}
	default:
		masterKey = ev.GetMasterPublicKey()
	}
	if linkedEvent == nil && masterKey == "" {
		return "", errors.Wrapf(ErrUserIsNotPresentedOnRelay, "no linked event and master key %v", ev.ID)
	}
	foundMatchingMasterKey := true
	mismatchedMasterKey := ""
	for _, e := range acks {
		if ev.GetMasterPublicKey() != e.GetMasterPublicKey() {
			foundMatchingMasterKey = false
			mismatchedMasterKey = e.GetMasterPublicKey()

			break
		}
	}
	if !foundMatchingMasterKey {
		return "", errors.Wrapf(ErrUserIsNotPresentedOnRelay, "21750 was not provided with kind %v or b tag mismatch (%v %v)", ev.Kind, linkedEvent.GetMasterPublicKey(), mismatchedMasterKey)
	}

	return masterKey, nil
}

func (c *consensus) getEvent(ctx context.Context, address string) (event *model.Event, masterKey string, err error) {
	it := c.Query(ctx, model.Filter{Addresses: []string{address}})
	for e, iErr := range it {
		if iErr != nil {
			return nil, "", errors.Wrapf(iErr, "failed to fetch linked event for by filter %v ", address)
		}
		event = e

		break
	}
	if event != nil {
		masterKey = event.GetMasterPublicKey()
	}

	return event, masterKey, nil
}

func collectRelaysFromRelayEvent(ev *model.Event) []string {
	relays := make([]string, 0, len(ev.Tags))
	for _, tag := range ev.Tags {
		if tag.Key() == "r" {
			relays = append(relays, tag.Value())
		}
	}
	return relays
}

func mapEventKindToChainFingerprint(event *model.Event) (fingerprint string, err error) {
	switch event.Kind {
	case nostr.KindRelayListMetadata,
		model.CustomIONKindAttestation,
		nostr.KindFileStorageServerList,
		nostr.KindDMRelayList:
		return client.GetFingerprint("metadata"), nil
	case nostr.KindGiftWrap:
		if kTag := event.GetTag("k"); kTag != nil {
			kTagValue, err := strconv.Atoi(kTag.Value())
			if err != nil {
				return "", errors.Wrapf(err, "malformed k tag:%v", kTagValue)
			}
			switch kTagValue {
			case nostr.KindDirectMessage, model.CustomIONDirectMessage, nostr.KindReaction, nostr.KindDeletion:
				return client.GetFingerprint("tmp"), nil
			default:
				return client.GetFingerprint(fmt.Sprintf("%v%v", event.Kind, kTagValue)), nil
			}
		}
		return "", errors.Errorf("malformed %v event, no k tag", nostr.KindGiftWrap)
	case nostr.KindBadgeDefinition, nostr.KindBadgeAward:
		return client.GetFingerprint(fmt.Sprintf("%v", nostr.KindBadgeAward)), nil
	default:
		kTagValue := ""
		if kTag := event.GetTag("k"); kTag != nil {
			kTagValue = "_" + kTag.Value()
		}

		return client.GetFingerprint(fmt.Sprintf("%v%v", event.Kind, kTagValue)), nil
	}
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

func mapEventsToTXs(events []*model.Event, ackEvents map[string][]*model.EphemeralEmbeddingEvent) (txs []client.Transaction, err error) {
	txs = make([]client.Transaction, 0, len(events))
	encodedEvents := map[string]nostr.EventEnvelope{}
	for _, ev := range events {
		if ev.IsEphemeral() && ev.Kind != model.CustomIONKindEphemeralEmbeddding {
			continue
		}
		fingerprint, err := mapEventKindToChainFingerprint(ev)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to detect event's fingerprint, probably malformed event %v", ev)
		}
		var env nostr.EventEnvelope
		ok := false
		if env, ok = encodedEvents[fingerprint]; !ok {
			env = nostr.EventEnvelope{}
		}
		env.Events = append(env.Events, &ev.Event)
		mappedEphemeralEvents := ackEvents[ev.Address()]
		ackEphepheralEvents := make([]*nostr.Event, 0, len(mappedEphemeralEvents))
		for _, e := range mappedEphemeralEvents {
			if e.ContentEvent != nil && (e.ContentEvent.Kind == model.CustomIONKindAttestation || e.ContentEvent.Kind == nostr.KindProfileMetadata) {
				ackEphepheralEvents = append(ackEphepheralEvents, &e.Event.Event)
			}
		}
		if len(ackEphepheralEvents) > 0 {
			env.Events = append(env.Events, ackEphepheralEvents...)
		}

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
			log.Printf("malformed relay %v: %v", relay, err)
			continue
		}
		port, err := strconv.ParseUint(u.Port(), 10, 64)
		if err != nil {
			log.Printf("malformed relay %v: %v", relay, err)
			continue
		}
		discoveryPort := (port + 10000)
		discoveryAddresses = append(discoveryAddresses, fmt.Sprintf("%v:%v", u.Hostname(), discoveryPort))
	}
	return discoveryAddresses
}
