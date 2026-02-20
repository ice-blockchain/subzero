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
	broadcasterUserNotificationWorkerArgs struct {
		BatchID         string       `json:"batch_id"`
		EphemeralEvents model.Events `json:"events"`
	}
	broadcasterUserNotificationWorker struct {
		rq.WorkerDefaults[broadcasterUserNotificationWorkerArgs]
		Manager *PushNotificationManager
	}
)

func (broadcasterUserNotificationWorkerArgs) Kind() string {
	return "pn_broadcaster_user_notification_worker_args"
}

func (w *broadcasterUserNotificationWorker) Work(ctx context.Context, job *rq.Job[broadcasterUserNotificationWorkerArgs]) error {
	log.Debug().Str("context", "PUSH_NOTIFICATIONS").
		Int("events_count", len(job.Args.EphemeralEvents)).
		Str("batch", job.Args.BatchID).
		Msg("starting broadcaster user notification worker for collecting target devices")

	var decodedEvents model.Events
	for _, ev := range job.Args.EphemeralEvents {
		var decodedEvent model.Event

		err := decodedEvent.UnmarshalJSON([]byte(ev.Content))
		if err != nil {
			log.Error().Str("context", "PUSH_NOTIFICATIONS").
				Err(err).
				Str("event_id", ev.ID).
				Msg("failed to unmarshal ephemeral event content")
			continue
		}
		// We assume that the event is valid here as it was already validated by `validator` layer BEFORE forwarding it there.
		decodedEvents = append(decodedEvents, &decodedEvent)
	}

	if len(decodedEvents) == 0 {
		return nil
	}

	targets := make(map[string]*broadcasterPushNotificationWorkerArgs) // Device key -> Events.
	for _, event := range decodedEvents {
		devices := w.Manager.collectLocalDevices("", event)
		for _, device := range devices {
			args, exists := targets[device.PubKey]
			if !exists {
				args = &broadcasterPushNotificationWorkerArgs{
					MasterPublicKey: device.GetMasterPublicKey(),
					Device:          device,
					BatchID:         job.Args.BatchID,
				}
				targets[device.PubKey] = args
			}
			args.Events = append(args.Events, event)
		}
	}
	log.Debug().
		Str("context", "PUSH_NOTIFICATIONS").
		Str("batch", job.Args.BatchID).
		Int("unique_devices_count", len(targets)).
		Msg("collected target devices for push notifications")

	tasks := make([]rq.JobArgs, 0, len(targets))
	for _, args := range targets {
		tasks = append(tasks, args)
	}

	return errors.Wrap(w.Manager.rq.Push(ctx, tasks...), "failed to push broadcaster push notification worker jobs")
}
