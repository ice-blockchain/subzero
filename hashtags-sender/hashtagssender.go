// SPDX-License-Identifier: ice License 1.0

package hashtagssender

import (
	"context"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/imroc/req/v3"
	"github.com/nbd-wtf/go-nostr"
	"github.com/rs/zerolog/log"

	"github.com/ice-blockchain/subzero/cfg"
	"github.com/ice-blockchain/subzero/model"
)

type (
	Config struct {
		BaseURL            string        `yaml:"base-url" validate:"required,url"`
		RequestTimeout     time.Duration `yaml:"request-timeout"`
		MaxEventsQueueSize int           `yaml:"max-events-queue-size"`
		SendInterval       time.Duration `yaml:"send-interval"`
	}
	eventsData struct {
		Events []*model.Event `json:"events"`
	}
	sender struct {
		lastSent     time.Time
		eventsToSend chan []*model.Event
		config       *Config
		client       *req.Client
		eventsQueue  []*model.Event
		mu           sync.Mutex
	}
)

var (
	globalSender struct {
		*sender
		Once sync.Once
	}

	hashtagRegex = regexp.MustCompile(`#[a-zA-Z0-9_]+`)
)

func MustInit(ctx context.Context) {
	globalSender.Once.Do(func() {
		config := cfg.MustGet[Config]()
		if config.RequestTimeout <= 0 {
			config.RequestTimeout = 30 * time.Second
		}
		if config.MaxEventsQueueSize <= 0 {
			config.MaxEventsQueueSize = 1000
		}
		if config.SendInterval <= 0 {
			config.SendInterval = time.Hour
		}
		globalSender.sender = &sender{
			eventsToSend: make(chan []*model.Event, config.MaxEventsQueueSize),
			mu:           sync.Mutex{},
			lastSent:     time.Now(),
			config:       config,
			client:       req.C().SetBaseURL(config.BaseURL),
			eventsQueue:  make([]*model.Event, 0, config.MaxEventsQueueSize),
		}
		go globalSender.sender.startSender(ctx)
	})
}

func AcceptEvents(ctx context.Context, events ...*model.Event) error {
	if globalSender.sender == nil {
		panic("hashtags sender not initialized")
	}

	return globalSender.sender.processEvents(events...)
}

func hasHashtagInRichText(e *model.Event) bool {
	richTextTag := e.GetTag(model.CustomIONTagRichText)
	if richTextTag == nil || len(richTextTag) < 3 || richTextTag.Value() != model.QuillDeltaProtocol {
		return false
	}

	return hashtagRegex.MatchString(richTextTag[2])
}

func (p *sender) processEvents(events ...*model.Event) error {
	validEvents := make([]*model.Event, 0, len(events))
	for _, event := range events {
		if event.Kind != nostr.KindTextNote && event.Kind != model.CustomIONKindEditableTextNote && event.Kind != nostr.KindArticle {
			continue
		}
		var hasHashtag bool
		if event.Content != "" {
			hasHashtag = hashtagRegex.MatchString(event.Content)
		} else {
			hasHashtag = hasHashtagInRichText(event)
		}
		if !hasHashtag {
			continue
		}
		validEvents = append(validEvents, event)
	}

	if len(validEvents) == 0 {
		return nil
	}

	p.mu.Lock()
	p.eventsQueue = append(p.eventsQueue, validEvents...)
	if len(p.eventsQueue) < p.config.MaxEventsQueueSize && time.Since(p.lastSent) < p.config.SendInterval {
		p.mu.Unlock()

		return nil
	}

	eventsToSend := make([]*model.Event, len(p.eventsQueue))
	copy(eventsToSend, p.eventsQueue)
	p.eventsQueue = make([]*model.Event, 0, p.config.MaxEventsQueueSize)
	p.mu.Unlock()

	select {
	case p.eventsToSend <- eventsToSend:
		p.mu.Lock()
		p.lastSent = time.Now()
		p.mu.Unlock()

		return nil
	default:
		return nil
	}
}

func (p *sender) startSender(ctx context.Context) {
	for {
		select {
		case events, ok := <-p.eventsToSend:
			if !ok {
				return
			}
			if err := p.sendEvents(ctx, events); err != nil {
				log.Error().Str("context", "HASHTAGS-SENDER").Err(err).Msg("failed to send events")
			}
		case <-ctx.Done():
			return
		}
	}
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
				log.Error().Str("context", "HASHTAGS-SENDER").Err(err).Msg("failed to send hashtags data, retrying")
			} else {
				log.Error().
					Str("context", "HASHTAGS-SENDER").
					Int("status_code", resp.GetStatusCode()).
					Msg("failed to send hashtags data with status code, retrying")
			}
		}).
		SetRetryCondition(func(resp *req.Response, err error) bool {
			return err != nil || resp.GetStatusCode() != http.StatusAccepted
		}).
		AddQueryParam("caller", "subzero").
		SetHeader("Accept", "application/json").
		SetHeader("Cache-Control", "no-cache, no-store, must-revalidate").
		SetHeader("Pragma", "no-cache").
		SetHeader("Expires", "0").
		SetBody(data).
		Post("/v1/statistics/hashtags")

	if err != nil {
		return errors.Wrap(err, "failed to send hashtags data")
	}
	if resp.GetStatusCode() != http.StatusAccepted {
		return errors.Newf("hashtags stats service responded with status: %d", resp.GetStatusCode())
	}

	return nil
}
