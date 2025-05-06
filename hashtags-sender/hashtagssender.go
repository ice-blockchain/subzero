// SPDX-License-Identifier: ice License 1.0

package hashtagssender

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/imroc/req/v3"
	"github.com/nbd-wtf/go-nostr"
	"golang.org/x/sync/errgroup"

	"github.com/ice-blockchain/subzero/cfg"
	"github.com/ice-blockchain/subzero/model"
)

type (
	Config struct {
		BaseURL        string        `yaml:"base-url"`
		RequestTimeout time.Duration `yaml:"request-timeout"`
		BatchSize      int           `yaml:"batch-size"`
		SendInterval   time.Duration `yaml:"send-interval"`
	}
	eventsData struct {
		Events []*model.Event `json:"events"`
	}
	hashtagsSender struct {
		events   []*model.Event
		mu       sync.Mutex
		lastSent time.Time
		config   *Config
	}
)

var (
	globalSender *hashtagsSender
)

func init() {
	req.DefaultClient().SetJsonMarshal(json.Marshal)
	req.DefaultClient().SetJsonUnmarshal(json.Unmarshal)
	req.DefaultClient().GetClient().Timeout = 30 * time.Second
}

func MustInit() {
	config := cfg.MustGet[Config]()
	if config.BaseURL == "" {
		panic("Base URL not provided, hashtags processor will not be initialized")
	}
	if config.RequestTimeout <= 0 {
		config.RequestTimeout = 30 * time.Second
	}
	if config.BatchSize <= 0 {
		config.BatchSize = 1000
	}
	if config.SendInterval <= 0 {
		config.SendInterval = time.Hour
	}

	globalSender = &hashtagsSender{
		events:   make([]*model.Event, 0, config.BatchSize),
		lastSent: time.Now(),
		config:   config,
	}
}

func AcceptEvents(ctx context.Context, events ...*model.Event) error {
	if globalSender == nil {
		return nil
	}

	return globalSender.processEvents(ctx, events...)
}

func (p *hashtagsSender) processEvents(ctx context.Context, events ...*model.Event) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, event := range events {
		if event.Kind == nostr.KindTextNote || event.Kind == model.CustomIONKindEditableTextNote || event.Kind == nostr.KindArticle {
			p.events = append(p.events, event)
		}
	}
	if len(p.events) < p.config.BatchSize && time.Since(p.lastSent) < p.config.SendInterval {
		return nil
	}

	var batches [][]*model.Event
	eventsCount := len(p.events)
	for i := 0; i < eventsCount; i += p.config.BatchSize {
		end := i + p.config.BatchSize
		if end > eventsCount {
			end = eventsCount
		}

		batch := make([]*model.Event, end-i)
		copy(batch, p.events[i:end])
		batches = append(batches, batch)
	}
	p.events = make([]*model.Event, 0, p.config.BatchSize)
	p.lastSent = time.Now()
	g, ctx := errgroup.WithContext(ctx)
	for ix, batch := range batches {
		batchIndex := ix
		batchData := batch
		g.Go(func() error {
			return errors.Wrapf(p.sendEvents(ctx, batchData), "failed to send hashtags events batch %d", batchIndex)
		})
	}

	return errors.Wrap(g.Wait(), "failed to send hashtags events")
}

func (p *hashtagsSender) sendEvents(ctx context.Context, events []*model.Event) error {
	if len(events) == 0 {
		return nil
	}
	data := eventsData{
		Events: events,
	}
	client := req.C()
	resp, err := client.R().
		SetContext(ctx).
		SetRetryCount(25).
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
				log.Printf("failed to send hashtags data, retrying...: %v", err)
			} else {
				log.Printf("failed to send hashtags data with status code:%v, retrying...", resp.GetStatusCode())
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
		Post(p.config.BaseURL + "/v1/statistics/hashtags")

	if err != nil {
		return errors.Wrap(err, "failed to send hashtags data")
	}
	if resp.GetStatusCode() != http.StatusAccepted {
		return errors.Newf("hashtags stats service responded with status: %d", resp.GetStatusCode())
	}

	return nil
}
