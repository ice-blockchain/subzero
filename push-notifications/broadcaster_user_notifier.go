// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/rs/zerolog/log"

	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
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

func (broadcasterPushNotificationWorker) deviceKey(ev *model.Event) string {
	if ev == nil {
		return ""
	}
	return ev.PubKey
}

func (w *broadcasterPushNotificationWorker) Work(ctx context.Context, job *rq.Job[broadcasterPushNotificationWorkerArgs]) error {
	log.Debug().Str("context", "PUSH_NOTIFICATIONS").
		Str("master_public_key", job.Args.MasterPublicKey).
		Str("device_public_key", w.deviceKey(job.Args.Device)).
		Int("events_count", len(job.Args.Events)).
		Str("batch", job.Args.BatchID).
		Msg("starting broadcaster push notification worker")

	if len(job.Args.Events) == 0 {
		return nil
	}

	singleNotifications, topicNotifications, err := w.Manager.collectNotifications(ctx, job.Args.Events)
	if err != nil {
		return errors.Wrap(err, "failed to collect notifications")
	}

	var singleNotificationsFiltered []*pn.Notification[*model.Event]
	var filteredCount int
	if job.Args.Device != nil {
		for _, n := range singleNotifications {
			if n.Target.PubKey != job.Args.Device.PubKey {
				filteredCount++
				continue
			}

			singleNotificationsFiltered = append(singleNotificationsFiltered, n)
		}
	} else {
		singleNotificationsFiltered = singleNotifications
	}

	if len(singleNotificationsFiltered) == 0 {
		log.Trace().
			Str("context", "PUSH_NOTIFICATIONS").
			Str("master_public_key", job.Args.MasterPublicKey).
			Str("device_public_key", w.deviceKey(job.Args.Device)).
			Int("events_count", len(job.Args.Events)).
			Int("filtered_count", filteredCount).
			Str("batch", job.Args.BatchID).
			Msg("no notifications to send for this device and user")
		return nil
	}

	return errors.Wrap(w.Manager.sendNotifications(ctx, singleNotificationsFiltered, topicNotifications), "failed to send notifications from broadcaster push notification worker")
}
