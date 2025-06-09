// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"cmp"
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

func validatePollTag(tag model.Tag) error {
	var rules = map[string]int{
		"type":    0,
		"ttl":     0,
		"options": 0,
	}
	for _, part := range tag[1:] {
		parts := strings.SplitN(part, " ", 2)
		if len(parts) != 2 {
			return errors.Wrapf(ErrWrongEventParams, "poll: invalid tag value: %q: want key value", part)
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		switch key {
		case "type":
			if value != "single" && value != "multi" {
				return errors.Wrapf(ErrWrongEventParams, "poll: invalid type value: %q, want single or multi", value)
			}
		case "ttl":
			v, err := nostr.ParseTimestamp(value)
			if err != nil {
				return errors.Wrapf(ErrWrongEventParams, "poll: invalid ttl value: %q, want unix time: %v", value, err)
			} else if v < 0 {
				return errors.Wrapf(ErrWrongEventParams, "poll: invalid ttl value: %q, want unix time", value)
			} else if v > 0 && v.Time().Before(time.Now()) {
				return errors.Wrapf(ErrWrongEventParams, "poll: invalid ttl value: %q, want unix time in the future", value)
			}
		case "title":
		case "options":
			var options []string

			if err := json.Unmarshal([]byte(value), &options); err != nil {
				return errors.Wrapf(ErrWrongEventParams, "poll: invalid options value: %q: %v", value, err)
			} else if len(options) == 0 {
				return errors.Wrap(ErrWrongEventParams, "poll: options are empty")
			}
		default:
			return errors.Wrapf(ErrWrongEventParams, "poll: unknown key: %q", key)
		}
		rules[key]++
	}
	for key, count := range rules {
		if count == 0 {
			return errors.Wrapf(ErrWrongEventParams, "poll: missing required key: %q", key)
		}
	}
	return nil
}

func validatePollVote(ctx context.Context, e *model.Event) error {
	pollAddress := cmp.Or(e.GetTag("e").Value(), e.GetTag("a").Value())
	if pollAddress == "" {
		return errors.Wrapf(ErrWrongEventParams, "vote: missing poll address")
	}

	var poll *model.Event
	for ev, err := range query.GetStoredEvents(ctx,
		&model.Subscription{
			Filters: model.Filters{
				model.Filter{
					Addresses: []string{pollAddress},
					Limit:     1,
				},
			},
		}) {
		if err != nil {
			return errors.Wrapf(err, "vote: failed to get poll event %s", pollAddress)
		}
		poll = ev
	}
	if poll == nil {
		return errors.Wrapf(ErrWrongEventParams, "vote: poll event not found: %s", pollAddress)
	}

	pollTag := poll.GetTag(model.CustomIONTagPoll)
	if pollTag == nil {
		return errors.Wrap(ErrWrongEventParams, "vote: poll event does not have poll tag")
	}

	deadlineStr, _ := extractTagValueFromPairs(pollTag, "ttl")
	deadline, err := nostr.ParseTimestamp(deadlineStr)
	if err != nil {
		return errors.Wrapf(ErrWrongEventParams, "vote: invalid ttl value: %q: %v", deadlineStr, err)
	} else if deadline.Time().Before(time.Now()) {
		return errors.Wrapf(ErrWrongEventParams, "vote: poll is expired")
	}

	var options []int
	err = json.Unmarshal([]byte(e.Content), &options)
	if err != nil {
		return errors.Wrapf(ErrWrongEventParams, "vote: invalid options value: %q: %v", e.Content, err)
	} else if len(options) == 0 {
		return errors.Wrap(ErrWrongEventParams, "vote: options are empty")
	}

	pollType, _ := extractTagValueFromPairs(pollTag, "type")
	pollOptionsStr, _ := extractTagValueFromPairs(pollTag, "options")

	var pollOptions []string
	err = json.Unmarshal([]byte(pollOptionsStr), &pollOptions)
	if err != nil {
		return errors.Wrapf(ErrWrongEventParams, "vote: invalid poll options value: %q: %v", pollOptionsStr, err)
	}

	for _, option := range options {
		if option < 0 || option >= len(pollOptions) {
			return errors.Wrapf(ErrWrongEventParams, "vote: invalid option index: %d", option)
		}
	}

	if pollType == "single" && len(options) > 1 {
		return errors.Wrapf(ErrWrongEventParams, "vote: single poll can have only one option")
	}

	choises := make(map[int]int)
	for _, option := range options {
		choises[option]++
		if choises[option] > 1 {
			return errors.Wrapf(ErrWrongEventParams, "vote: duplicate option: %d", option)
		}
	}

	return nil
}
