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

	errNotFound = errors.New("not found")
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
	return c.RollbackTx(ctx, userAddress, transactions...)
}

func (c *consensus) broadcastUserEvents(ctx context.Context, events ...*model.Event) error {
	userMasterKey, relays, isProfileDeletion, err := c.getUserAndRelaysForBroadcast(ctx, events...)
	if err != nil {
		return errors.Wrapf(err, "failed to detect user master key and relays")
	}
	broadcastCtx, broadcastCancel := context.WithTimeout(ctx, consensusTimeout)
	defer broadcastCancel()
	notifier := make(chan client.BroadcastStatus, 1)
	if isProfileDeletion {
		c.client.BroadcastTxRemoval(broadcastCtx, userMasterKey, c.convertRelaysToBroadcastEndpoints(relays...), notifier)
		res := <-notifier
		return errors.Wrapf(res.Error, "failed to delete chains for user deletion %v", userMasterKey)
	}
	txs, err := mapEventsToTXs(events)
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

	return errors.Wrapf(err, "failed to broadcast user events for %v to %#v", userMasterKey, relays)
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

func (c *consensus) parseEphemeralAckEvent(ev *model.Event) (*model.Event, error) {
	var ack model.Event
	err := ack.UnmarshalJSON([]byte(ev.Content))
	if err != nil {
		return nil, errors.Wrapf(err, "malformed ack")
	}
	return &ack, nil
}

func (c *consensus) getUserAndRelaysForBroadcast(ctx context.Context, events ...*model.Event) (masterKey string, relays []string, isProfileDeletion bool, err error) {
	userMasterKeys := map[string][]string{}
	var profileDeletion *model.Event
	matchingEphemeralAckEvents := map[string]*model.Event{}
	for _, ev := range events {
		if !ev.IsEphemeral() {
			continue
		}
		if ev.Kind != model.CustomIONKindEphemeralEmbeddding {
			continue
		}
		ackEvent, aErr := c.parseEphemeralAckEvent(ev)
		if aErr != nil {
			return "", nil, false, errors.Wrapf(aErr, "malformed 21750: %v", ev.Content)
		}
		matchingEphemeralAckEvents[ackEvent.GetMasterPublicKey()] = ackEvent
	}
	for _, ev := range events {
		if ev.IsEphemeral() {
			continue
		}
		if ev.Kind == nostr.KindRelayListMetadata {
			relays = collectRelaysFromRelayEvent(ev)
		}
		if ev.Kind == nostr.KindDeletion && (len(ev.Tags) == 0) && ev.PubKey == ev.GetMasterPublicKey() {
			profileDeletion = ev
		}
		masterKey, err = c.broadcastMasterKey(ctx, ev, matchingEphemeralAckEvents)
		if err != nil {
			if errors.Is(err, ErrUserIsNotPresentedOnRelay) {
				continue
			}
			return "", nil, false, errors.Wrapf(err, "failed to get master key to broadcast the event %+v", ev)
		}
		if userMasterKeys[masterKey] == nil {
			userMasterKeys[masterKey] = relays
		}
	}
	if len(userMasterKeys) > 1 {
		return "", nil, false, ErrMultipleMasterKeys
	}
	var userMasterKey string
	for userKey, _ := range userMasterKeys {
		userMasterKey = userKey
		break
	}
	if len(userMasterKeys[userMasterKey]) == 0 {
		if userMasterKeys[userMasterKey], err = c.fetchUserRelays(ctx, userMasterKey); err != nil {
			return "", nil, false, errors.Wrapf(err, "failed to get relay list for user %v", userMasterKey)
		}
	}
	return userMasterKey, userMasterKeys[userMasterKey], profileDeletion != nil, nil
}

func (c *consensus) broadcastMasterKey(ctx context.Context, ev *model.Event, ephemeralAckEvents map[string]*model.Event) (masterKey string, err error) {
	var ack *model.Event
	hasAck := false
	if ack, hasAck = ephemeralAckEvents[ev.GetMasterPublicKey()]; !hasAck {
		return ev.GetMasterPublicKey(), nil
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
			masterKey = pTag[1]
		}
	case nostr.KindGiftWrap:
		if pTag := ev.GetTag("p"); pTag != nil && len(pTag) > 2 {
			masterKey = pTag[1]
			linkedEvent = ev
		}
	case nostr.KindReaction, nostr.KindTextNote, model.CustomIONKindEditableTextNote, nostr.KindArticle:
		if eTag := ev.GetTag("e"); eTag != nil && eTag.Value() != "" {
			linkedEvent, err = c.getEvent(ctx, eTag.Value())
			if err != nil {
				if errors.Is(err, errNotFound) {
					linkedEvent = nil
					err = nil
				}
				if err != nil {
					return "", errors.Wrapf(err, "failed to fetch linked event for event %+v", ev)
				}
			}
			masterKey = linkedEvent.GetMasterPublicKey()
		}
		if pTag := ev.GetTag("p"); pTag != nil && pTag.Value() != "" {
			relays, rErr := c.fetchUserRelays(ctx, pTag.Value()) // Mentioned user is presented on relay
			if rErr == nil && len(relays) > 0 {
				masterKey = pTag.Value()
			}
		}
		if qTag := ev.GetTag("q"); qTag != nil && qTag.Value() != "" {
			linkedEvent, err = c.getEvent(ctx, qTag.Value())
			if err != nil {
				if errors.Is(err, errNotFound) {
					linkedEvent = nil
					err = nil
				}
				if err != nil {
					return "", errors.Wrapf(err, "failed to fetch linked event for event %+v", ev)
				}
			}
			if linkedEvent != nil {
				masterKey = linkedEvent.GetMasterPublicKey()
			}
		}
		if qTag := ev.GetTag("Q"); qTag != nil && qTag.Value() != "" {
			if splitted := strings.Split(qTag.Value(), ":"); len(splitted) >= 2 {
				linkedEvent, err = c.getEvent(ctx, splitted[1])
				if err != nil {
					if errors.Is(err, errNotFound) {
						linkedEvent = nil
						err = nil
					}
					if err != nil {
						return "", errors.Wrapf(err, "failed to fetch linked event for event %+v", ev)
					}
				}
				if linkedEvent != nil {
					masterKey = linkedEvent.GetMasterPublicKey()
				}
			}
		}

	default:
		masterKey = ev.GetMasterPublicKey()
	}
	if linkedEvent == nil && masterKey == "" {
		return "", ErrUserIsNotPresentedOnRelay
	}
	if ev.GetMasterPublicKey() != ack.GetMasterPublicKey() {
		return "", errors.Wrapf(ErrUserIsNotPresentedOnRelay, "21750 was not provided with kind %v or b tag mismatch (%v %v)", ev.Kind, linkedEvent.GetMasterPublicKey(), ack.GetMasterPublicKey())
	}
	return masterKey, nil
}

func (c *consensus) getEvent(ctx context.Context, eventID string) (event *model.Event, err error) {
	it := query.GetStoredEvents(ctx, &model.Subscription{Filters: model.Filters{
		model.Filter{IDs: []string{eventID}},
	}})
	for e, iErr := range it {
		if iErr != nil {
			return nil, errors.Wrapf(iErr, "failed to fetch linked event for by id %v", eventID)
		}
		event = e
		break
	}
	if event == nil {
		return nil, errNotFound
	}
	return event, nil
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

func mapEventKindToChainFingerprint(event *model.Event) (fingerprint string) {
	kTagValue := ""
	kind := event.Kind
	if event.Kind == nostr.KindRelayListMetadata {
		kind = model.CustomIONKindAttestation
	}
	if kTag := event.GetTag("k"); kTag != nil {
		kTagValue = "_" + kTag.Value()
	}

	return client.GetFingerprint(fmt.Sprintf("%v%v", kind, kTagValue))
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

func mapEventsToTXs(events []*model.Event) (txs []client.Transaction, err error) {
	txs = make([]client.Transaction, 0, len(events))
	encodedEvents := map[string]nostr.EventEnvelope{}
	for _, ev := range events {
		if ev.IsEphemeral() {
			continue
		}
		fingerprint := mapEventKindToChainFingerprint(ev)
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
