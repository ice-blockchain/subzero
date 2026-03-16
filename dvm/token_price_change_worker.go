// SPDX-License-Identifier: ice License 1.0

package dvm

import (
	"context"
	"encoding/json"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"
	"github.com/rs/zerolog/log"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/rq"
)

type (
	tokenPriceChangeWorkerArgs struct {
		BatchNum int          `json:"batch_num"`
		Devices  []string     `json:"devices"`
		Event    *model.Event `json:"event"`
	}
	tokenPriceChangeWorker struct {
		rq.WorkerDefaults[tokenPriceChangeWorkerArgs]
		DVM *dvm
	}
	tokenPriceChangeWorkerJob = rq.Job[tokenPriceChangeWorkerArgs]
)

func (tokenPriceChangeWorkerArgs) Kind() string {
	return "dvm_token_price_change_worker_args"
}

func (w *tokenPriceChangeWorker) Work(ctx context.Context, job *tokenPriceChangeWorkerJob) error {
	log.Trace().
		Str("context", "DVM").
		Int("batch_num", job.Args.BatchNum).
		Int("num_devices", len(job.Args.Devices)).
		Str("event_id", job.Args.Event.ID).
		Msg("starting token price change worker")

	data, err := query.FetchAndUpdatePriceChangeNotification(ctx, job.Args.Event, job.Args.Devices)
	if err != nil {
		return errors.Wrapf(err, "batch %d: failed to fetch and update price change notification for event %s", job.Args.BatchNum, job.Args.Event.ID)
	}

	for _, entry := range data {
		var ev model.Event

		content, err := json.Marshal(model.Events{entry.PreviousEvent, job.Args.Event})
		if err != nil {
			log.Error().Str("context", "DVM").Err(err).Str("event_id", job.Args.Event.ID).Msg("failed to marshal events for price change notification")
			continue
		}

		ev.Kind = model.CustomIONKindDVMJobResponsePriceChange
		ev.Content = string(content)
		ev.CreatedAt = nostr.Now()
		ev.Tags = model.Tags{
			{"request", entry.Request},
			{"e", entry.RequestID},
			{"p", entry.MasterPubKey},
			{"a", job.Args.Event.GetTag("a").Value()},
		}
		if err := ev.SignWithAlg(w.DVM.Config.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519); err != nil {
			log.Panic().Err(err).Str("context", "DVM").Str("event_id", job.Args.Event.ID).Msg("failed to sign price change notification event")
			continue
		}

		log.Trace().
			Str("context", "DVM").
			Int("batch_num", job.Args.BatchNum).
			Str("event_id", job.Args.Event.ID).
			Str("notification_event_id", ev.ID).
			Stringer("notification_event", &ev).
			Msg("submitting price change notification event to DVM")

		w.DVM.SubmitResult(ctx, nil, &ev)
	}

	return nil
}
