// SPDX-License-Identifier: ice License 1.0

package dvm

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"log"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

type (
	nostrEventCountJob struct {
		RelayConnectTLS *tls.Config
	}
)

func newNostrEventCountJob(relayConnectTLS *tls.Config) *nostrEventCountJob {
	return &nostrEventCountJob{
		RelayConnectTLS: relayConnectTLS,
	}
}

func collectValuesFromTagMap(values []model.TagValues) (data []string) {
	for _, val := range values {
		for _, v := range val {
			if v != nil {
				data = append(data, *v)
			}
		}
	}
	return data
}

func (n *nostrEventCountJob) Process(ctx context.Context, e *model.Event) (payload string, err error) {
	var filters model.Filters

	err = json.Unmarshal([]byte(e.Content), &filters)
	if err != nil {
		return "0", errors.Wrapf(err, "failed to parse filters: %v", e)
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
	if len(queryRelays) == 0 || (len(queryRelays) == 1 && globalConfig != nil && queryRelays[0].URL == globalConfig.RelayURL) {
		var groupBy string
		for _, tag := range e.Tags {
			if tag.Key() == "param" && tag.Value() == "group" {
				groupBy = tag[2]
				break
			}
		}
		if groupBy != "" {
			for idx := range filters {
				if len(filters[idx].IDs) == 0 {
					if filters[idx].Tags.HasValues("e") {
						filters[idx].IDs = collectValuesFromTagMap(filters[idx].Tags["e"])
					} else if filters[idx].Tags.HasValues("q") {
						filters[idx].IDs = collectValuesFromTagMap(filters[idx].Tags["q"])
					}
				} else if len(filters[idx].Authors) == 0 && filters[idx].Tags.HasValues("p") {
					filters[idx].IDs = collectValuesFromTagMap(filters[idx].Tags["p"])
				}
				if groupBy == "root" || groupBy == "reply" {
					filters[idx].Tags.Append("e", nil, nil, &groupBy)
				}
			}
		}
		if len(filters) == 1 && len(filters[0].Kinds) == 1 && filters[0].Kinds[0] == nostr.KindReaction {
			result, err = query.CountEventReactions(ctx, &model.Subscription{Filters: filters})
		} else {
			var count int64
			count, err = query.CountEvents(ctx, &model.Subscription{Filters: filters})
			result = strconv.FormatInt(count, 10)
		}
		if err != nil {
			return "", errors.Wrapf(err, "failed to count events for filters in local DB: %v", filters)
		}
		return result, nil
	}

	for _, relay := range queryRelays {
		queriedCount, err := relay.Count(ctx, filters)
		if err == nil {
			// Use result from the first relay that returns a valid count.
			if len(filters) == 1 && len(filters[0].Kinds) == 1 && filters[0].Kinds[0] == nostr.KindReaction {
				return `{"total": ` + strconv.FormatInt(queriedCount, 10) + `}`, nil
			} else {
				return strconv.FormatInt(queriedCount, 10), nil
			}
		}
	}

	return "", errors.Errorf("remote relays are not available: %v", queryRelays)
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
