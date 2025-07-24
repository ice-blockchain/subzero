// SPDX-License-Identifier: ice License 1.0

package hashtagssender

import (
	"context"
	"log"
	"net/http"
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
		Events []*model.Event `json:"events"`
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
			client: req.C().SetBaseURL(config.BaseURL),
		}
	})
}

func AcceptEvents(ctx context.Context, events ...*model.Event) error {
	if globalSender.sender == nil {
		panic("nft content sender not initialized")
	}

	return globalSender.sender.processEvents(ctx, events...)
}

func (p *sender) processEvents(ctx context.Context, events ...*model.Event) error {
	if len(events) == 0 {
		return nil
	}
	var contentEvents []*model.Event
	for _, event := range events {
		if event.Kind != nostr.KindTextNote && event.Kind != model.CustomIONKindEditableTextNote && event.Kind != nostr.KindArticle {
			continue
		}
		if event.IsComment() || event.IsCommunityPost() || event.IsStory() {
			continue
		}
		contentEvents = append(contentEvents, event)
	}
	if len(contentEvents) == 0 {
		return nil
	}
	var wg sync.WaitGroup
	for _, contentEvent := range contentEvents {
		wg.Add(1)
		go func(event *model.Event) {
			defer wg.Done()
			if err := p.processContentEvent(ctx, event); err != nil {
				log.Printf("failed to process content event %s: %v", event.ID, err)
			}
		}(contentEvent)
	}
	wg.Wait()

	return nil
}

func (p *sender) processContentEvent(ctx context.Context, contentEvent *model.Event) error {
	masterPubkey := contentEvent.GetMasterPublicKey()
	requiredEvents := query.GetStoredEvents(ctx, model.Filter{
		Kinds:   []int{nostr.KindProfileMetadata, model.CustomIONKindAttestation},
		Authors: []string{masterPubkey},
	})
	var profileEvent, attestationEvent *model.Event
	for evt, err := range requiredEvents {
		if err != nil {
			return errors.Wrap(err, "failed to get required events")
		}
		if evt.GetMasterPublicKey() != masterPubkey {
			continue
		}
		switch evt.Kind {
		case nostr.KindProfileMetadata:
			profileEvent = evt
		case model.CustomIONKindAttestation:
			attestationEvent = evt
		default:
			continue
		}
		if profileEvent != nil && attestationEvent != nil {
			break
		}
	}
	if profileEvent == nil {
		log.Printf("no profile metadata found for user %s, skipping content event %s", masterPubkey, contentEvent.ID)

		return nil
	}
	if attestationEvent == nil {
		log.Printf("no attestation found for master pubkey %s, skipping content event %s", masterPubkey, contentEvent.ID)

		return nil
	}
	eventsToSend := []*model.Event{contentEvent, profileEvent, attestationEvent}
	if err := p.sendEvents(ctx, eventsToSend); err != nil {
		return errors.Wrapf(err, "failed to send events for content %s", contentEvent.ID)
	}

	return nil
}

func (p *sender) sendEvents(ctx context.Context, events []*model.Event) error {
	if len(events) == 0 {
		return nil
	}
	data := eventsData{
		Events: events,
	}

	resp, err := p.client.R().
		SetContext(ctx).
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
				log.Printf("failed to send nft content data, retrying...: %v", err)
			} else {
				log.Printf("failed to send nft content data with status code:%v, retrying...", resp.GetStatusCode())
			}
		}).
		SetRetryCondition(func(resp *req.Response, err error) bool {
			return err != nil ||
				(resp.GetStatusCode() != http.StatusAccepted && resp.GetStatusCode() != http.StatusBadRequest && resp.GetStatusCode() != http.StatusUnprocessableEntity)
		}).
		AddQueryParam("caller", "subzero").
		SetHeader("Accept", "application/json").
		SetHeader("Cache-Control", "no-cache, no-store, must-revalidate").
		SetHeader("Pragma", "no-cache").
		SetHeader("Expires", "0").
		SetBody(data).
		Post("/v1/statistics/nft-content")

	if err != nil {
		return errors.Wrap(err, "failed to send hashtags data")
	}
	if resp.GetStatusCode() != http.StatusAccepted {
		return errors.Newf("hashtags stats service responded with status: %d", resp.GetStatusCode())
	}

	return nil
}
