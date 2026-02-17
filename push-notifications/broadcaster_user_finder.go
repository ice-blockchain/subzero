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
	var sourceRelayURL string

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

		if v := ev.GetTag("relay").Value(); v != "" && sourceRelayURL == "" {
			sourceRelayURL = v
		} else if v != "" && v != sourceRelayURL {
			log.Warn().Str("context", "PUSH_NOTIFICATIONS").
				Str("event_id", ev.ID).
				Str("source_relay_url", sourceRelayURL).
				Str("found_relay_url", v).
				Msg("multiple source relay URLs found in tags, using the first one")
		}
	}

	if len(decodedEvents) == 0 {
		return nil
	}

	targets := make(map[string]*broadcasterPushNotificationWorkerArgs) // Device key -> Events.

	if sourceRelayURL == "" {
		log.Trace().
			Str("context", "PUSH_NOTIFICATIONS").
			Str("batch", job.Args.BatchID).
			Msg("using local database to find target devices for push notifications")
		for _, event := range decodedEvents {
			devices := w.Manager.collectTargetDevices(event)
			for _, device := range devices {
				args, exists := targets[device.PubKey]
				if !exists {
					args = &broadcasterPushNotificationWorkerArgs{
						MasterPublicKey: device.GetMasterPublicKey(),
						BatchID:         job.Args.BatchID,
						SourceRelayURL:  sourceRelayURL,
					}
					targets[device.PubKey] = args
				}
				args.Events = append(args.Events, event)
			}
		}
	} else {
		log.Trace().
			Str("context", "PUSH_NOTIFICATIONS").
			Str("batch", job.Args.BatchID).
			Str("source_relay_url", sourceRelayURL).
			Msg("using event tags to find target devices for push notifications")
		for i := range job.Args.EphemeralEvents {
			for _, user := range extractTargetUsers(job.Args.EphemeralEvents[i]) {
				args := &broadcasterPushNotificationWorkerArgs{
					MasterPublicKey: user,
					BatchID:         job.Args.BatchID,
					SourceRelayURL:  sourceRelayURL,
					Events:          decodedEvents,
				}
				targets[user] = args
			}
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

func extractTargetUsers(event *model.Event) []string {
	for _, tag := range event.Tags {
		if tag.Key() == "l" && tag.Value() == "users" {
			return tag[2:]
		}
	}
	return nil
}
