// SPDX-License-Identifier: ice License 1.0

package dvm

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/model"
)

type (
	nostrEventCountJob struct {
		ServiceProviderPubkey  string
		ServiceProviderPrivKey string
		MinLeadingZeroBits     int
		OutputRelays           []*nostr.Relay
	}
)

const (
	NostrEventCountGroupContent = "content"
	NostrEventCountGroupPubkey  = "pubkey"
	NostrEventCountGroupReply   = "reply"
	NostrEventCountGroupRoot    = "root"
)

func newNostrEventCountJob(outputRelays []*nostr.Relay, serviceProviderPubKey string, serviceProviderPrivKey string, min int) *nostrEventCountJob {
	return &nostrEventCountJob{
		ServiceProviderPubkey:  serviceProviderPubKey,
		ServiceProviderPrivKey: serviceProviderPrivKey,
		MinLeadingZeroBits:     min,
		OutputRelays:           outputRelays,
	}
}

func (n *nostrEventCountJob) Process(ctx context.Context, e *model.Event) (payload string, err error) {
	queryRelays, err := connectToRelays(ctx, collectRelayURLs(e))
	if err != nil {
		return "", errors.Wrapf(err, "failed to connect to job relays to publish response: %v", e)
	}
	defer closeRelays(queryRelays)
	filters, err := parseListOfFilters(e.Content)
	if err != nil {
		return "", errors.Wrapf(err, "failed to parse filters: %v", e)
	}
	evList := make([]*nostr.Event, 0)
	for _, relay := range queryRelays {
		for _, filter := range filters {
			evs, err := relay.QueryEvents(ctx, filter)
			if err != nil {
				log.Printf("can't get events from relay %v: %v", relay.URL, err)

				continue
			}
			for ev := range evs {
				evList = append(evList, ev)

			}
		}
	}
	deduplEvents := deduplicateEvents(evList)
	var groups []string
	for _, tag := range e.Tags {
		if tag.Key() == "param" && tag.Value() == "group" {
			groups = append(groups, tag[2])
		}
	}
	if len(groups) == 0 {
		return fmt.Sprint(len(deduplEvents)), nil
	}
	groupped := countBasedOnGroups(deduplEvents, groups)
	if len(groupped) == 0 {
		return fmt.Sprint(0), nil
	}
	resultPayload, err := json.Marshal(groupped)
	if err != nil {
		return "", errors.Wrap(err, "failed to serialize group counts")
	}

	return string(resultPayload), nil
}

func (n *nostrEventCountJob) RequiredPaymentAmount() float64 {
	return 0.0
}

func (n *nostrEventCountJob) IsBidAmountEnough(amount string) bool {
	if n.RequiredPaymentAmount() > 0 {
		if strings.Trim(amount, " ") == "" {
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
				if tags := ev.Tags.GetAll([]string{"e"}); tags != nil {
					for _, tag := range tags {
						if tag[2] == model.TagMarkerReply {
							groupCounts[tag.Value()]++
						}
					}
				}
			case NostrEventCountGroupRoot:
				if tags := ev.Tags.GetAll([]string{"e"}); tags != nil {
					for _, tag := range tags {
						if tag[2] == model.TagMarkerRoot {
							groupCounts[tag.Value()]++
						}
					}
				}
			default:
				if tags := ev.Tags.GetAll([]string{group}); tags != nil {
					for _, tag := range tags {
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
			for ix, relayURL := range tag {
				if ix <= 1 {
					continue
				}
				relayList = append(relayList, relayURL)
			}
		}
	}

	return relayList
}

func (n *nostrEventCountJob) OnErrorFeedback(ctx context.Context, event *model.Event, inErr error) error {
	return errors.Wrapf(
		publishJobFeedback(ctx, event, model.JobFeedbackStatusError, n.OutputRelays, n.ServiceProviderPubkey, n.ServiceProviderPrivKey, fmt.Sprintf("error: %v", inErr.Error()), n.RequiredPaymentAmount(), n.MinLeadingZeroBits),
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
		publishJobFeedback(ctx, event, model.JobFeedbackStatusPaymentRequired, n.OutputRelays, n.ServiceProviderPubkey, n.ServiceProviderPrivKey, "not enough bid", n.RequiredPaymentAmount(), n.MinLeadingZeroBits),
		"failed to publish job payment required feedback: %v", event)
}
