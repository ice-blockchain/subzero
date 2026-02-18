// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/rs/zerolog/log"

	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/rq"
)

type (
	broadcasterPushNotificationWorkerArgs struct {
		Device          *model.Event `json:"device"`
		MasterPublicKey string       `json:"user_master_public_key"`
		BatchID         string       `json:"batch_id"`
		Events          model.Events `json:"events"`
	}
	broadcasterPushNotificationWorker struct {
		rq.WorkerDefaults[broadcasterPushNotificationWorkerArgs]
		Manager *PushNotificationManager
	}
)

func (broadcasterPushNotificationWorkerArgs) Kind() string {
	return "pn_broadcaster_push_notification_worker_args"
}

func (w *broadcasterPushNotificationWorker) Work(ctx context.Context, job *rq.Job[broadcasterPushNotificationWorkerArgs]) error {
	log.Debug().Str("context", "PUSH_NOTIFICATIONS").
		Str("master_public_key", job.Args.MasterPublicKey).
		Str("device_public_key", job.Args.Device.PubKey).
		Int("events_count", len(job.Args.Events)).
		Str("batch", job.Args.BatchID).
		Msg("starting broadcaster push notification worker")

	err := w.Manager.AcceptEvents(ctx, job.Args.Events)

	return errors.Wrap(err, "failed to accept events in broadcaster push notification worker")
}
