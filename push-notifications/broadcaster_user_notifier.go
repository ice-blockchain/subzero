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
		MasterPublicKey string       `json:"user_master_public_key"`
		BatchID         string       `json:"batch_id"`
		SourceRelayURL  string       `json:"source_relay_url"`
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

func (w *broadcasterPushNotificationWorker) ProcessLocalEvents(ctx context.Context, events model.Events, relatedEvents model.Events) error {
	for _, ev := range events {
		batch := append(model.Events{ev}, relatedEvents...)
		notification, err := w.Manager.collectNotifications(ctx, batch, "")
		if err != nil {
			return errors.Wrap(err, "failed to collect notifications")
		}
		notification.Remote = nil // Local events should not have remote notifications.

		err = w.Manager.sendNotifications(ctx, notification)
		if err != nil {
			return errors.Wrap(err, "failed to send notifications")
		}
	}
	return nil
}

func (w *broadcasterPushNotificationWorker) ProcessBroadcastedEvents(ctx context.Context, events model.Events, targetUserMasterKey, sourceRelayURL string) error {
	return errors.Wrapf(
		w.Manager.AcceptEvents(ctx, events, targetUserMasterKey),
		"failed to accept broadcasted events from relay %s for %v", sourceRelayURL, targetUserMasterKey,
	)
}

func (w *broadcasterPushNotificationWorker) Work(ctx context.Context, job *rq.Job[broadcasterPushNotificationWorkerArgs]) error {
	log.Debug().Str("context", "PUSH_NOTIFICATIONS").
		Str("master_public_key", job.Args.MasterPublicKey).
		Str("source_relay_url", job.Args.SourceRelayURL).
		Int("events_count", len(job.Args.Events)).
		Str("batch", job.Args.BatchID).
		Msg("starting broadcaster push notification worker")

	var relatedEvents model.Events
	var originalEvents model.Events
	for _, ev := range job.Args.Events {
		if ev.Kind == model.CustomIONKindEphemeralEmbedding {
			relatedEvents = append(relatedEvents, ev)
		} else {
			originalEvents = append(originalEvents, ev)
		}
	}

	if job.Args.SourceRelayURL != "" && !model.CompareRelaysURLs(job.Args.SourceRelayURL, w.Manager.relayURL) {
		// This is a broadcasted event from different relay.
		return w.ProcessBroadcastedEvents(ctx, job.Args.Events, job.Args.MasterPublicKey, job.Args.SourceRelayURL)
	} else {
		// This is our own event.
		return w.ProcessLocalEvents(ctx, originalEvents, relatedEvents)
	}
}
