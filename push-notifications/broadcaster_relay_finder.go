// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"context"
	"strconv"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"
	"github.com/rs/zerolog/log"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/rq"
)

type (
	broadcasterRelayFinderWorkerArgs struct {
		BatchID string       `json:"batch_id"`
		Events  model.Events `json:"events"`
	}
	broadcasterRelayFinderWorker struct {
		rq.WorkerDefaults[broadcasterRelayFinderWorkerArgs]
		Manager *PushNotificationManager
	}
)

func (broadcasterRelayFinderWorkerArgs) Kind() string {
	return "pn_broadcaster_relay_finder_worker_args"
}

func (w *broadcasterRelayFinderWorker) Filter(in model.Events) (out model.Events) {
next:
	for _, event := range in {
		if _, ok := allowedBroadcastKinds[event.Kind]; !ok {
			continue
		}

		switch event.Kind {
		case model.CustomIONKindTokenizedCommunityDefinition:
			if event.GetTag("p").Value() == "" {
				// Want only `first buy` events.
				continue next
			}

		case nostr.KindGenericRepost:
			kValue, err := strconv.ParseInt(event.GetTag("k").Value(), 10, 16)
			if err != nil {
				continue next
			}
			switch int(kValue) {
			case model.CustomIONKindEditableTextNote, nostr.KindArticle:
				// Accept.
			default:
				continue next
			}

		case model.CustomIONKindEditableTextNote:
			if event.IsComment() { // Want only top-level posts.
				continue next
			}
		}

		out = append(out, event)
	}
	return out
}

func (w *broadcasterRelayFinderWorker) Work(ctx context.Context, job *rq.Job[broadcasterRelayFinderWorkerArgs]) error {
	const highBoundRelayListSize = 30

	log.Debug().Str("context", "PUSH_NOTIFICATIONS").Str("batch", job.Args.BatchID).Msg("starting relay finder and filter worker")

	events := w.Filter(job.Args.Events)
	if len(events) == 0 {
		log.Debug().Str("context", "PUSH_NOTIFICATIONS").Str("batch", job.Args.BatchID).Msg("no events to process after filtering, skipping relay finder worker")
		return nil
	}

	targets := make(map[string]struct{}) // Contains unique relay URLs.

	masterKeys := make([]string, 0, len(events))
	for _, event := range events {
		keys := w.Manager.collectTargetMasterKeys(event)
		if len(keys) == 0 {
			continue
		}
		masterKeys = append(masterKeys, keys...)
	}
	if len(masterKeys) == 0 {
		log.Debug().Str("context", "PUSH_NOTIFICATIONS").Str("batch", job.Args.BatchID).Msg("no target master keys found, skipping relay finder worker")
		return nil
	}

	for ev, err := range query.GetStoredEvents(ctx, model.Filter{
		Authors: masterKeys,
		Kinds:   []int{nostr.KindRelayListMetadata},
		Limit:   len(masterKeys),
	}) {
		if err != nil {
			log.Error().Err(err).Str("context", "PUSH_NOTIFICATIONS").Str("batch", job.Args.BatchID).Msg("failed to get stored events")
			return err
		}

		// TODO: should we use just a few relays from the list instead of all of them?
		relays := model.CollectRelaysFromRelayEvent(ev)
		if len(relays) > highBoundRelayListSize {
			log.Warn().Str("context", "PUSH_NOTIFICATIONS").
				Str("batch", job.Args.BatchID).
				Str("user", ev.GetMasterPublicKey()).
				Int("relays_count", len(relays)).
				Msg("relay list metadata event contains too many relays, shrinking to high bound")
			relays = relays[:highBoundRelayListSize]
		}
		log.Trace().
			Str("context", "PUSH_NOTIFICATIONS").
			Str("batch", job.Args.BatchID).
			Str("user", ev.GetMasterPublicKey()).
			Int("relays_count", len(relays)).
			Msg("collected relays from relay list metadata event")
		for _, r := range relays {
			targets[r] = struct{}{}
		}
	}

	log.Debug().Str("context", "PUSH_NOTIFICATIONS").
		Str("batch", job.Args.BatchID).
		Int("unique_relay_count", len(targets)).
		Int("events_count", len(events)).
		Msg("relay finder found unique relays for broadcasting")

	broadcastEvents := w.Manager.packEventsForBroadcast(ctx, events, job.Args.BatchID)
	broadcastArgs := make([]rq.JobArgs, 0, len(targets))
	for relayURL := range targets {
		broadcastArgs = append(broadcastArgs, &broadcasterBroadcastWorkerArgs{
			RelayURL:        relayURL,
			BatchID:         job.Args.BatchID,
			EphemeralEvents: broadcastEvents,
		})
	}

	return errors.Wrap(w.Manager.rq.Push(ctx, broadcastArgs...), "failed to push broadcaster broadcast workers")
}
