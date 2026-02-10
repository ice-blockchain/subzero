// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/rs/zerolog/log"

	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/rq"
)

type (
	broadcasterBroadcastWorkerArgs struct {
		RelayURL string       `json:"relay_url"`
		BatchID  string       `json:"batch_id"`
		Events   model.Events `json:"events"`
	}
	broadcasterBroadcastWorker struct {
		rq.WorkerDefaults[broadcasterBroadcastWorkerArgs]
		Manager *PushNotificationManager
	}
)

func (broadcasterBroadcastWorkerArgs) Kind() string {
	return "pn_broadcaster_broadcast_worker_args"
}

func (w *broadcasterBroadcastWorker) Work(ctx context.Context, job *rq.Job[broadcasterBroadcastWorkerArgs]) error {
	log.Debug().Str("context", "PUSH_NOTIFICATIONS").
		Str("batch", job.Args.BatchID).
		Str("relay_url", job.Args.RelayURL).
		Int("events_count", len(job.Args.Events)).
		Msg("starting broadcaster broadcast worker")

	start := time.Now()
	err := w.Manager.broadcaster.BroadcastTo(ctx, job.Args.RelayURL, job.Args.Events)
	spent := time.Since(start)
	if err != nil {
		return errors.Wrapf(err, "failed to broadcast %d events to relay %s", len(job.Args.Events), job.Args.RelayURL)
	}

	log.Debug().Str("context", "PUSH_NOTIFICATIONS").
		Str("batch", job.Args.BatchID).
		Str("relay_url", job.Args.RelayURL).
		Int("events_count", len(job.Args.Events)).
		Dur("time_spent", spent).
		Msg("finished broadcaster broadcast worker")

	return nil
}
