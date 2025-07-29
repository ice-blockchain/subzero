// SPDX-License-Identifier: ice License 1.0

package nftcontentsender

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/imroc/req/v3"
	"github.com/nbd-wtf/go-nostr"
	"golang.org/x/sync/errgroup"

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
	contentEventKinds = map[int]struct{}{
		nostr.KindProfileMetadata:           {},
		nostr.KindTextNote:                  {},
		model.CustomIONKindEditableTextNote: {},
		nostr.KindArticle:                   {},
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
		panic("nft content sender not initialized")
	}

	return errors.Wrap(globalSender.sender.processEvents(ctx, events...), "failed to process content events")
}

func (p *sender) processEvents(ctx context.Context, events ...*model.Event) error {
	if len(events) == 0 {
		return nil
	}
	embeddedEvents, err := model.ParseEphemeralEmbeddingEvents(events...)
	if err != nil {
		return errors.Wrap(err, "failed to parse ephemeral embedding events")
	}
	contentEvents := make(map[string]*model.Event)
	profileMetadataEvents := make(map[string]*model.Event)
	attestationEvents := make(map[string]*model.Event)
	for _, event := range events {
		if event.Kind == model.CustomIONKindEphemeralEmbeddding {
			continue
		}
		if _, ok := contentEventKinds[event.Kind]; ok {
			if event.IsComment() || event.IsCommunityPost() || event.IsStory() {
				continue
			}
			contentEvents[event.GetMasterPublicKey()] = event
		}
	}
	for _, embeddedEventsList := range embeddedEvents {
		for _, embeddedEvent := range embeddedEventsList {
			if embeddedEvent.ContentEvent != nil {
				switch embeddedEvent.ContentEvent.Kind {
				case nostr.KindProfileMetadata:
					profileMetadataEvents[embeddedEvent.ContentEvent.GetMasterPublicKey()] = embeddedEvent.ContentEvent
				case model.CustomIONKindAttestation:
					attestationEvents[embeddedEvent.ContentEvent.GetMasterPublicKey()] = embeddedEvent.ContentEvent
				}
			}
		}
	}
	var masterPubkeysToFetch []string
	for _, contentEvent := range contentEvents {
		masterPubkey := contentEvent.GetMasterPublicKey()
		if _, hasProfile := profileMetadataEvents[masterPubkey]; !hasProfile && contentEvent.Kind != nostr.KindProfileMetadata {
			masterPubkeysToFetch = append(masterPubkeysToFetch, masterPubkey)
		}
		if _, hasAttestation := attestationEvents[masterPubkey]; !hasAttestation {
			masterPubkeysToFetch = append(masterPubkeysToFetch, masterPubkey)
		}
	}
	if len(masterPubkeysToFetch) > 0 {
		requiredEvents := query.GetStoredEvents(ctx, model.Filter{
			Kinds:   []int{nostr.KindProfileMetadata, model.CustomIONKindAttestation},
			Authors: masterPubkeysToFetch,
		})
		for evt, err := range requiredEvents {
			if err != nil {
				return errors.Wrap(err, "failed to get required events")
			}
			switch evt.Kind {
			case nostr.KindProfileMetadata:
				profileMetadataEvents[evt.GetMasterPublicKey()] = evt
			case model.CustomIONKindAttestation:
				attestationEvents[evt.GetMasterPublicKey()] = evt
			}
		}
	}
	g, gCtx := errgroup.WithContext(ctx)
	for _, contentEvent := range contentEvents {
		event := contentEvent
		g.Go(func() error {
			masterPubkey := event.GetMasterPublicKey()
			profileEvent, hasProfile := profileMetadataEvents[masterPubkey]
			if !hasProfile && event.Kind != nostr.KindProfileMetadata {
				log.Printf("no profile metadata found for user %s, skipping content event %s", masterPubkey, event.ID)

				return nil
			}
			attestationEvent, hasAttestation := attestationEvents[masterPubkey]
			if !hasAttestation {
				log.Printf("no attestation found for master pubkey %s, skipping content event %s", masterPubkey, event.ID)

				return nil
			}
			eventsToSend := model.Events{event, attestationEvent}
			if event.Kind != nostr.KindProfileMetadata {
				eventsToSend = append(eventsToSend, profileEvent)
			}
			if err := p.sendEvents(gCtx, eventsToSend); err != nil {
				return errors.Wrapf(err, "failed to send events for content %s", event.ID)
			}

			return nil
		})
	}

	return errors.Wrap(g.Wait(), "failed to process content events")
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
				log.Printf("failed to send nft content data, retrying...: %v", err)
			} else {
				log.Printf("failed to send nft content data with status code:%v, retrying...", resp.GetStatusCode())
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
		Post("/v1/statistics/nft-content")

	if err != nil {
		return errors.Wrap(err, "failed to send nft content data")
	}
	if resp.GetStatusCode() != http.StatusAccepted {
		return fmt.Errorf("nft content service responded with status: %d", resp.GetStatusCode())
	}

	return nil
}
