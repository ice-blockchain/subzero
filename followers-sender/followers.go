// SPDX-License-Identifier: ice License 1.0

package followerssender

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/imroc/req/v3"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/cfg"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

type (
	Config struct {
		BaseURL        string        `yaml:"base-url" validate:"required,url"`
		RequestTimeout time.Duration `yaml:"request-timeout"`
	}
	eventsData struct {
		Events model.Events `json:"events"`
	}
	sender struct {
		config *Config
		client *req.Client
	}
)

var (
	globalSender struct {
		*sender
		Once sync.Once
	}
)

func MustInit(ctx context.Context) {
	globalSender.Once.Do(func() {
		config := cfg.MustGet[Config]()
		if config.RequestTimeout <= 0 {
			config.RequestTimeout = 30 * time.Second
		}
		globalSender.sender = &sender{
			config: config,
			client: req.C().SetBaseURL(config.BaseURL).SetTimeout(config.RequestTimeout),
		}
	})
}

func AcceptEvents(ctx context.Context, events ...*model.Event) error {
	if globalSender.sender == nil {
		panic("followers sender not initialized")
	}

	return errors.Wrap(globalSender.sender.processEvents(ctx, events...), "failed to process followers events")
}

func (p *sender) processEvents(ctx context.Context, events ...*model.Event) error {
	if len(events) == 0 {
		return nil
	}
	followersEvent := p.findFollowerListEvent(events)
	if followersEvent == nil {
		log.Printf("followers event not found for events: %v", model.Events(events).IDs())

		return nil
	}
	if len(model.GetNewlyFollowedPubkeys(followersEvent, followersEvent.Previous)) == 0 {
		log.Printf("no new followers in the followers event, skipping: %s", followersEvent.ID)

		return nil
	}
	attestationEvent, err := p.getAttestationEventFromStorage(ctx, followersEvent.GetMasterPublicKey())
	if err != nil {
		return errors.Wrapf(err, "failed to get required events from storage for followersEvent:%s", followersEvent.ID)
	}
	if attestationEvent == nil {
		log.Printf("required events not found in the database for followersEvent:%s", followersEvent.ID)

		return nil
	}
	eventsToSend := model.Events{followersEvent, attestationEvent}

	return errors.Wrapf(p.sendEvents(ctx, eventsToSend), "failed to send events for followersEvent:%s", followersEvent.ID)
}

func (p *sender) findFollowerListEvent(events []*model.Event) *model.Event {
	idx := slices.IndexFunc(events, func(event *model.Event) bool {
		return event.Kind == nostr.KindFollowList
	})
	if idx == -1 {
		return nil
	}

	return events[idx]
}

func (p *sender) getAttestationEventFromStorage(ctx context.Context, masterPubkey string) (attestationEvent *model.Event, err error) {
	requiredEvents := query.GetStoredEvents(ctx, model.Filter{
		Kinds:   []int{model.CustomIONKindAttestation},
		Authors: []string{masterPubkey},
		Limit:   1,
	})
	for evt, err := range requiredEvents {
		if err != nil {
			return nil, errors.Wrap(err, "failed to get attestation event")
		}

		return evt, nil
	}

	return nil, nil
}

func (p *sender) sendEvents(ctx context.Context, events model.Events) error {
	if len(events) == 0 {
		return nil
	}
	data := eventsData{
		Events: events,
	}

	cCtx, cancel := context.WithTimeout(ctx, p.config.RequestTimeout)
	defer cancel()
	resp, err := p.client.R().
		SetContext(cCtx).
		SetRetryCount(5).
		SetRetryInterval(func(resp *req.Response, attempt int) time.Duration {
			switch {
			case attempt <= 1:
				return 100 * time.Millisecond
			case attempt == 2:
				return 1 * time.Second
			default:
				return 10 * time.Second
			}
		}).
		SetRetryHook(func(resp *req.Response, err error) {
			if err != nil {
				log.Printf("failed to send followers data, retrying...: %v", err)
			} else {
				log.Printf("failed to send followers data with status code:%v, retrying...", resp.GetStatusCode())
			}
		}).
		SetRetryCondition(func(resp *req.Response, err error) bool {
			return err != nil || resp.GetStatusCode() >= http.StatusInternalServerError
		}).
		AddQueryParam("caller", "subzero").
		SetHeader("Accept", "application/json").
		SetHeader("Cache-Control", "no-cache, no-store, must-revalidate").
		SetHeader("Pragma", "no-cache").
		SetHeader("Expires", "0").
		SetBody(data).
		Post("/v1/statistics/followers")

	if err != nil {
		return errors.Wrap(err, "failed to send followers data")
	}
	if resp.GetStatusCode() != http.StatusAccepted {
		return fmt.Errorf("followers service responded with status: %d", resp.GetStatusCode())
	}

	return nil
}
