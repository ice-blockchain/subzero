// SPDX-License-Identifier: ice License 1.0

package dvm

import (
	"context"
	"crypto/rsa"
	"crypto/tls"
	"encoding/json"
	"fmt"
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
		ServiceProviderPrivKey string
		OutputRelays           []*nostr.Relay
		devMode                bool
	}
)

const (
	NostrEventCountGroupContent = "content"
	NostrEventCountGroupPubkey  = "pubkey"
	NostrEventCountGroupReply   = "reply"
	NostrEventCountGroupRoot    = "root"
)

func newNostrEventCountJob(outputRelays []*nostr.Relay, tlsConfig *tls.Config, devMode bool) *nostrEventCountJob {
	var privKeyHex string
	if tlsConfig != nil && len(tlsConfig.Certificates) > 0 {
		privKeyHex = fmt.Sprintf("%x", tlsConfig.Certificates[0].PrivateKey.(*rsa.PrivateKey).D.Bytes())
	}

	return &nostrEventCountJob{
		ServiceProviderPrivKey: privKeyHex,
		OutputRelays:           outputRelays,
		devMode:                devMode,
	}
}

func (n *nostrEventCountJob) Process(ctx context.Context, e *model.Event) (payload string, err error) {
	const zeroResponse = "0"
	queryRelays, err := connectToRelays(ctx, collectRelayURLs(e), n.devMode)
	if err != nil {
		return "", errors.Wrapf(err, "failed to connect to job relays to publish response: %v", e)
	}
	defer closeRelays(queryRelays)
	filters, err := parseListOfFilters(e.Content)
	if err != nil {
		return zeroResponse, errors.Wrapf(err, "failed to call parseListOfFilters: %v", e)
	}
	var groups []string
	for _, tag := range e.Tags {
		if tag.Key() == "param" && tag.Value() == "group" {
			groups = append(groups, tag[2])
		}
	}
	if len(groups) == 0 {
		count, err := n.doCount(ctx, filters, queryRelays)
		if err != nil {
			return zeroResponse, errors.Wrapf(err, "failed to do offline job: %v", filters)
		}

		return strconv.FormatInt(count, 10), nil
	}
	//TODO: implement group based requests by relay.Count call.
	evList, err := n.doQuery(ctx, filters, queryRelays)
	if err != nil {
		return zeroResponse, errors.Wrapf(err, "failed to do query job: %v", filters)
	}
	if len(evList) == 0 {
		return zeroResponse, nil
	}
	groupped := countBasedOnGroups(evList, groups)
	if len(groupped) == 0 {
		return zeroResponse, nil
	}
	res, err := json.Marshal(groupped)
	if err != nil {
		return zeroResponse, errors.Wrap(err, "failed to serialize group counts")
	}

	return string(res), nil
}

func (n *nostrEventCountJob) doCount(ctx context.Context, filters []nostr.Filter, queryRelays []*nostr.Relay) (count int64, err error) {
	if len(queryRelays) == 0 {
		count, err = query.CountEvents(ctx, &model.Subscription{Filters: filters})
		if err != nil {
			return 0, errors.Wrapf(err, "offline, failed to count events for filters: %v", filters)
		}

		return count, nil
	}
	for _, relay := range queryRelays {
		queriedCount, err := relay.Count(ctx, filters)
		if err != nil {
			log.Printf("online, can't get events from relay %v: %v", relay.URL, err)

			continue
		}

		count += int64(queriedCount)
	}

	return count, nil
}

func (n *nostrEventCountJob) doQuery(ctx context.Context, filters []nostr.Filter, queryRelays []*nostr.Relay) (events []*nostr.Event, err error) {
	evList := make([]*nostr.Event, 0)
	if len(queryRelays) == 0 {
		evIt := query.GetStoredEvents(ctx, &model.Subscription{Filters: filters})
		for ev, err := range evIt {
			if err != nil {
				return nil, errors.Wrap(err, "offline, failed to get events")
			}
			evList = append(evList, &ev.Event)
		}
	}
	for _, relay := range queryRelays {
		for _, filter := range filters {
			evs, err := relay.QueryEvents(ctx, filter)
			if err != nil {
				log.Printf("online, can't get events from relay %v: %v", relay.URL, err)

				continue
			}
			for ev := range evs {
				evList = append(evList, ev)
			}
		}
	}

	return model.DeduplicateSlice(evList, func(ev *nostr.Event) string { return ev.ID }), nil
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
			log.Printf("failed to parse payment amount %v: err: %v", amount, err)

			return false
		}

		return amount >= n.RequiredPaymentAmount()
	}

	return true
}

func parseListOfFilters(content string) (result []nostr.Filter, err error) {
	var filter []nostr.Filter
	if err := json.Unmarshal([]byte(content), &filter); err != nil {
		return nil, errors.Wrapf(err, "failed to parse filters: %v", content)
	}

	return filter, nil
}

func countBasedOnGroups(evList []*nostr.Event, groups []string) map[string]uint64 {
	groupCounts := make(map[string]uint64, 0)
	for _, group := range groups {
		for _, ev := range evList {
			switch group {
			case NostrEventCountGroupContent:
				groupCounts[ev.Content]++
			case NostrEventCountGroupPubkey:
				groupCounts[ev.PubKey]++
			case NostrEventCountGroupReply:
				for _, tag := range ev.Tags {
					if tag.Key() == "e" {
						if tag[2] == model.TagMarkerReply {
							groupCounts[tag.Value()]++
						}
					}
				}
			case NostrEventCountGroupRoot:
				for _, tag := range ev.Tags {
					if tag.Key() == "e" {
						if tag[2] == model.TagMarkerRoot {
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

func collectRelayURLs(e *model.Event) []string {
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

func (n *nostrEventCountJob) OnErrorFeedback(ctx context.Context, event *model.Event, inErr error) error {
	return errors.Wrapf(
		publishJobFeedback(ctx, event, model.JobFeedbackStatusError, n.OutputRelays, n.ServiceProviderPrivKey, "error: %v"+inErr.Error(), n.RequiredPaymentAmount()),
		"failed to publish job payment required feedback: %v", event)
}

func (n *nostrEventCountJob) OnProcessingFeedback(ctx context.Context, event *model.Event) error {
	return nil
}

func (n *nostrEventCountJob) OnSuccessFeedback(ctx context.Context, event *model.Event) error {
	return nil
}

func (n *nostrEventCountJob) OnPartialFeedback(ctx context.Context, event *model.Event) error {
	return nil
}

func (n *nostrEventCountJob) OnPaymentRequiredFeedback(ctx context.Context, event *model.Event) error {
	return errors.Wrapf(
		publishJobFeedback(ctx, event, model.JobFeedbackStatusPaymentRequired, n.OutputRelays, n.ServiceProviderPrivKey, "not enough bid", n.RequiredPaymentAmount()),
		"failed to publish job payment required feedback: %v", event)
}
