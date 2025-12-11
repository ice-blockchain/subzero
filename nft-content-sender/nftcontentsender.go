// SPDX-License-Identifier: ice License 1.0

package nftcontentsender

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/imroc/req/v3"
	"github.com/nbd-wtf/go-nostr"
	"github.com/rs/zerolog/log"

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

func getBSCWalletAddress(wallets map[string]string) string {
	for network, walletAddr := range wallets {
		if strings.EqualFold(network, "bsc") || strings.EqualFold(network, "bsctestnet") {
			return walletAddr
		}
	}

	return ""
}

func (p *sender) processEvents(ctx context.Context, events ...*model.Event) error {
	if len(events) == 0 {
		return nil
	}
	contentEvent := p.findContentEvent(events)
	if contentEvent == nil {
		log.Trace().Strs("event_ids", model.Events(events).IDs()).Msg("content event not found for events")

		return nil
	}
	if contentEvent.Previous != nil {
		switch contentEvent.Kind {
		case nostr.KindProfileMetadata:
			var parsedContent model.ProfileMetadataContent
			if err := json.Unmarshal([]byte(contentEvent.Content), &parsedContent); err != nil {
				return errors.Wrapf(err, "invalid profile metadata content for user %s", contentEvent.GetMasterPublicKey())
			}
			var previousContent model.ProfileMetadataContent
			if err := json.Unmarshal([]byte(contentEvent.Previous.Content), &previousContent); err != nil {
				return errors.Wrapf(err, "invalid previous profile metadata content for user %s", contentEvent.GetMasterPublicKey())
			}

			hadNoNFT := len(previousContent.IONContentNFTCollections) == 0
			hasNFT := len(parsedContent.IONContentNFTCollections) > 0
			nftCollectionsAdded := hadNoNFT && hasNFT

			currentBSC := getBSCWalletAddress(parsedContent.Wallets)
			previousBSC := getBSCWalletAddress(previousContent.Wallets)
			bscWalletChanged := currentBSC != previousBSC

			if !nftCollectionsAdded && !bscWalletChanged {
				return nil
			}
		default:
			log.Trace().Str("content_event_id", contentEvent.ID).Msg("content event was already processed previously, skipping")

			return nil
		}
	}
	profileMetadataEvent, attestationEvent, err := p.getRequiredEventsFromStorage(ctx, contentEvent.GetMasterPublicKey(), contentEvent)
	if err != nil {
		return errors.Wrapf(err, "failed to get required events from storage for contentEvent:%s", contentEvent.ID)
	}
	if attestationEvent == nil || (contentEvent.Kind != nostr.KindProfileMetadata && profileMetadataEvent == nil) {
		log.Trace().Str("content_event_id", contentEvent.ID).Msg("required events not found in the database for contentEvent")

		return nil
	}
	eventsToSend := p.buildEventsToSend(contentEvent, profileMetadataEvent, attestationEvent)

	return errors.Wrapf(p.sendEvents(ctx, eventsToSend), "failed to send events for contentEvent:%s", contentEvent.ID)
}

func (p *sender) findContentEvent(events []*model.Event) *model.Event {
	if idx := slices.IndexFunc(events, func(event *model.Event) bool {
		return event.Kind == model.CustomIONKindEphemeralEmbedding
	}); idx != -1 {
		return nil
	}
	for _, event := range events {
		if _, ok := contentEventKinds[event.Kind]; ok {
			if event.IsComment() || event.IsCommunityPost() || event.IsStory() {
				continue
			}

			return event
		}
	}

	return nil
}

func (p *sender) getRequiredEventsFromStorage(
	ctx context.Context, masterPubkey string, contentEvent *model.Event,
) (profileMetadataEvent, attestationEvent *model.Event, err error) {
	var kinds = []int{model.CustomIONKindAttestation}
	if contentEvent.Kind != nostr.KindProfileMetadata {
		kinds = append(kinds, nostr.KindProfileMetadata)
	}
	requiredEvents := query.GetStoredEvents(ctx, model.Filter{
		Kinds:   kinds,
		Authors: []string{masterPubkey},
	})
	for evt, err := range requiredEvents {
		if err != nil {
			return nil, nil, errors.Wrap(err, "failed to get required events")
		}
		switch evt.Kind {
		case nostr.KindProfileMetadata:
			profileMetadataEvent = evt
		case model.CustomIONKindAttestation:
			attestationEvent = evt
		default:
			continue
		}
	}

	return profileMetadataEvent, attestationEvent, nil
}

func (p *sender) buildEventsToSend(contentEvent, profileMetadataEvent, attestationEvent *model.Event) model.Events {
	eventsToSend := model.Events{contentEvent, attestationEvent}
	if contentEvent.Kind != nostr.KindProfileMetadata {
		eventsToSend = append(eventsToSend, profileMetadataEvent)
	}

	return eventsToSend
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
				log.Error().Err(err).Msg("failed to send nft content data, retrying")
			} else {
				log.Error().Int("status_code", resp.GetStatusCode()).Msg("failed to send nft content data with status code, retrying")
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
