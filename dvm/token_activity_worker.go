// SPDX-License-Identifier: ice License 1.0

package dvm

import (
	"context"
	"encoding/json"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"
	"github.com/rs/zerolog/log"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/rq"
)

type (
	tokenActivityWorkerArgs struct {
		BatchNum int          `json:"batch_num"`
		Tokens   []string     `json:"tokens"`
		Request  *model.Event `json:"request"`
	}
	tokenActivityWorker struct {
		rq.WorkerDefaults[tokenActivityWorkerArgs]
		DVM *dvm
	}
	tokenActivityWorkerJob = rq.Job[tokenActivityWorkerArgs]
)

func (tokenActivityWorkerArgs) Kind() string {
	return "dvm_token_activity_worker_args"
}

func (w *tokenActivityWorker) Work(ctx context.Context, job *tokenActivityWorkerJob) error {
	log.Trace().
		Str("context", "DVM").
		Int("batch_num", job.Args.BatchNum).
		Int("num_tokens", len(job.Args.Tokens)).
		Msg("starting token activity worker")

	data, err := query.FetchAndUpdateTokenActivityNotification(ctx, time.Now(), job.Args.Tokens)
	if err != nil {
		return errors.Wrapf(err, "batch %d: failed to fetch and update token activity notification for tokens %v", job.Args.BatchNum, job.Args.Tokens)
	}

	for _, entry := range data {
		var action, def model.Event

		if err := json.Unmarshal([]byte(entry.Definition), &def); err != nil {
			log.Error().Str("context", "DVM").Err(err).Str("definition", entry.Definition).Msg("failed to unmarshal token activity definition event")
			continue
		}

		if err := json.Unmarshal([]byte(entry.FirstBuy), &action); err != nil {
			log.Error().Str("context", "DVM").Err(err).Str("first_buy", entry.FirstBuy).Msg("failed to unmarshal token activity first buy event")
			continue
		}

		content, err := json.Marshal(model.Events{&action, &def})
		if err != nil {
			log.Error().Str("context", "DVM").Err(err).Str("definition", entry.Definition).Str("first_buy", entry.FirstBuy).Msg("failed to marshal events for token activity notification")
			continue
		}

		var ev model.Event
		ev.Kind = model.CustomIONKindDVMJobResponseTrendingTokens
		ev.Content = string(content)
		ev.CreatedAt = nostr.Now()
		ev.Tags = model.Tags{
			{"p", def.GetMasterPublicKey()},
			{"a", def.Address()},
		}

		if job.Args.Request != nil {
			ev.Tags = append(ev.Tags, model.Tag{"e", job.Args.Request.ID})
			ev.Tags = append(ev.Tags, model.Tag{"request", job.Args.Request.String()})
		}

		if err := ev.SignWithAlg(w.DVM.Config.PrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519); err != nil {
			log.Panic().Err(err).Str("context", "DVM").Msg("failed to sign token activity notification event")
		}

		log.Trace().
			Str("context", "DVM").
			Int("batch_num", job.Args.BatchNum).
			Str("notification_event_id", ev.ID).
			Stringer("notification_event", &ev).
			Msg("submitting token activity notification event to DVM")

		w.DVM.SubmitResult(ctx, nil, &ev)
	}

	return nil
}
