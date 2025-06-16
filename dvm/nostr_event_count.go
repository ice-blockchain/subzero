// SPDX-License-Identifier: ice License 1.0

package dvm

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"log"
	"strconv"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

const (
	NostrEventCountGroupContent = "content"
	NostrEventCountGroupPubkey  = "pubkey"
	NostrEventCountGroupReply   = "reply"
	NostrEventCountGroupRoot    = "root"
)

type nostrEventCountJob struct {
	RelayConnectTLS *tls.Config
}

func newNostrEventCountJob(relayConnectTLS *tls.Config) *nostrEventCountJob {
	return &nostrEventCountJob{
		RelayConnectTLS: relayConnectTLS,
	}
}

func (n *nostrEventCountJob) Process(ctx context.Context, e *model.Event) (payload string, err error) {
	var filters model.Filters

	err = json.Unmarshal([]byte(e.Content), &filters)
	if err != nil {
		return "0", errors.Wrapf(err, "failed to parse filters: %v", e.Content)
	}

	queryRelays := connectToRelays(ctx, e.ID, collectRelayURLsFromEvent(e), n.RelayConnectTLS)
	defer closeRelays(queryRelays)

	countString, err := n.doCount(ctx, e, filters, queryRelays)
	if err != nil {
		return "0", errors.Wrap(err, "count failed")
	}

	return countString, nil
}

func (n *nostrEventCountJob) doCount(ctx context.Context, e *model.Event, filters model.Filters, queryRelays []*nostr.Relay) (result string, err error) {
	var groupBy string
	for _, tag := range e.Tags {
		if tag.Key() == "param" && tag.Value() == "group" && len(tag) > 2 {
			groupBy = tag[2]
			break
		}
	}

	if len(queryRelays) == 0 || (len(queryRelays) == 1 && globalConfig != nil && queryRelays[0].URL == globalConfig.RelayURL) {
		return n.doCountLocal(ctx, filters, groupBy)
	}

	return n.doCountRemote(ctx, filters, queryRelays, groupBy)
}

func (n *nostrEventCountJob) doCountMRF(ctx context.Context, filter model.Filter) (string, error) {
	m, pk, authenticated, kinds := model.GetUserDataFromContext(ctx)
	if !authenticated {
		return "", model.ErrNotAuthorized
	} else if _, ok := kinds[nostr.KindFollowList]; len(kinds) > 0 && !ok {
		return "", model.ErrNotAuthorized
	}

	var total int64
	for ev, err := range query.GetStoredEvents(ctx, model.Filter{
		Kinds:   []int{nostr.KindFollowList},
		Authors: []string{m, pk},
		Search:  "include:dependencies:kind3>kind0+p+|" + strings.Join(filter.Tags.All("p"), ",") + "|",
		Limit:   1,
	}) {
		if err != nil {
			return "", errors.Wrap(err, "failed to get events")
		} else if ev.Kind != nostr.KindProfileMetadata {
			continue
		}
		total++
	}
	return strconv.FormatInt(total, 10), nil
}

func (n *nostrEventCountJob) doCountLocal(ctx context.Context, filters model.Filters, groupBy string) (string, error) {
	for _, filter := range filters {
		if strings.EqualFold(filter.Search, model.ExtensionTextMRF) {
			return n.doCountMRF(ctx, filter)
		}
	}

	if groupBy == "" {
		count, err := query.CountEvents(ctx, filters...)

		return strconv.FormatInt(count, 10), err
	}

	if len(filters) == 1 && len(filters[0].Kinds) == 1 && filters[0].Kinds[0] == nostr.KindReaction && groupBy == NostrEventCountGroupContent {
		return query.CountGroupedEventReactions(ctx, filters...)
	}

	var events []*nostr.Event
	for ev, err := range query.GetStoredEvents(ctx, filters...) {
		if err != nil {
			return "", errors.Wrap(err, "failed to get events")
		}
		events = append(events, &ev.Event)
	}

	return countBasedOnGroupsFromEvents(events, groupBy)
}

func (n *nostrEventCountJob) doCountRemote(ctx context.Context, filters model.Filters, queryRelays []*nostr.Relay, groupBy string) (string, error) {
	var wg sync.WaitGroup

	if groupBy == "" {
		for _, relay := range queryRelays {
			queriedCount, err := relay.Count(ctx, filters)
			if err == nil {
				return strconv.FormatInt(queriedCount, 10), nil
			}
		}
		return "", errors.Errorf("remote relays are not available: %v", queryRelays)
	}

	wg.Add(len(queryRelays))
	events := make([]*nostr.Event, 0, 10)
	output := make(chan *nostr.Event, 10)
	go func() {
		for ev := range output {
			events = append(events, ev)
		}
	}()
	for _, relay := range queryRelays {
		go func() {
			defer wg.Done()

			eventCh, err := relay.QueryEventsMany(ctx, filters...)
			if err != nil {
				log.Printf("cannot get events from relay %v: %v", relay.URL, err)

				return
			}
			for ev := range eventCh {
				output <- ev
			}
		}()
	}
	wg.Wait()
	close(output)

	return countBasedOnGroupsFromEvents(events, groupBy)
}

func countBasedOnGroupsFromEvents(events []*nostr.Event, groups ...string) (string, error) {
	result := countBasedOnGroups(model.DeduplicateSlice(events, func(ev *nostr.Event) string { return ev.ID }), groups...)
	if len(result) == 1 {
		for _, count := range result {
			return strconv.FormatUint(count, 10), nil
		}
	}

	data, err := json.Marshal(result)

	return string(data), errors.Wrap(err, "failed to marshal group counts")
}

func getMasterPublicKey(ev *nostr.Event) string {
	for _, tag := range ev.Tags {
		if tag.Key() == model.CustomIONTagOnBehalfOf && tag.Value() != "" {
			return tag.Value()
		}
	}
	return ev.PubKey
}

func countBasedOnGroups(evList []*nostr.Event, groups ...string) map[string]uint64 {
	groupCounts := make(map[string]uint64, 0)
	for _, group := range groups {
		for _, ev := range evList {
			switch group {
			case NostrEventCountGroupContent:
				groupCounts[ev.Content]++

			case NostrEventCountGroupPubkey:
				groupCounts[getMasterPublicKey(ev)]++

			case NostrEventCountGroupRoot, NostrEventCountGroupReply:
				for _, tag := range ev.Tags {
					if tag.Key() == "e" && len(tag) > 3 {
						if tag[3] == group {
							groupCounts[tag.Value()]++
						}
					}
				}

			default:
				for _, tag := range ev.Tags {
					if tag.Key() == group {
						groupCounts[tag.Value()]++
					}
				}
			}
		}
	}

	return groupCounts
}

func (n *nostrEventCountJob) RequiredPaymentAmount() float64 {
	return 0.0
}

func (n *nostrEventCountJob) IsBidAmountEnough(amount string) bool {
	if n.RequiredPaymentAmount() > 0 {
		if strings.TrimSpace(amount) == "" {
			return false
		}
		amount, err := strconv.ParseFloat(amount, 64)
		if err != nil {
			log.Printf("DVM: failed to parse payment amount %v: err: %v", amount, err)

			return false
		}

		return amount >= n.RequiredPaymentAmount()
	}

	return true
}

func collectRelayURLsFromEvent(e *model.Event) []string {
	var relayList []string
	for _, tag := range e.Tags {
		if tag.Key() == "param" && tag.Value() == "relay" {
			for _, relayURL := range tag[2:] {
				relayList = append(relayList, relayURL)
			}
		}
	}

	return relayList
}
