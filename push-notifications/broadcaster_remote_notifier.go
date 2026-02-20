// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"context"
	"net/url"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/rs/zerolog/log"

	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/rq"
)

type (
	broadcasterPushNotificationRemoteWorkerArgs struct {
		BatchID string       `json:"batch_id"`
		Events  model.Events `json:"events"`
	}
	broadcasterPushNotificationRemoteWorker struct {
		rq.WorkerDefaults[broadcasterPushNotificationRemoteWorkerArgs]
		Manager *PushNotificationManager
	}
	broadcasterPushNotificationRemoteWorkerJob = rq.Job[broadcasterPushNotificationRemoteWorkerArgs]
)

func (broadcasterPushNotificationRemoteWorkerArgs) Kind() string {
	return "pn_broadcaster_push_notification_remote_worker_args"
}

func (w *broadcasterPushNotificationRemoteWorker) CollectTargets(ctx context.Context, job *broadcasterPushNotificationRemoteWorkerJob) (targets map[string]model.Events) {
	var relays []string
	var devicesCollected uint

	for _, event := range job.Args.Events {
		if event.IsEphemeral() {
			continue
		}

		devices := w.Manager.collectRemoteDevices("", event)
		for _, device := range devices {
			devicesCollected++
			if v := collectRelaysFromDevice(device); len(v) > 0 {
				relays = append(relays, v...)
			}
		}
	}
	uniqueRelays := compactRelays(relays)

	if len(uniqueRelays) == 0 || devicesCollected == 0 {
		// No relays found, nothing to do.
		return nil
	}

	log.Trace().
		Str("context", "PUSH_NOTIFICATIONS").
		Int("events_count", len(job.Args.Events)).
		Uint("devices_collected", devicesCollected).
		Int("total_relays_collected", len(relays)).
		Int("unique_relays_collected", len(uniqueRelays)).
		Str("batch", job.Args.BatchID).
		Msg("collected relays for remote push notification")

	packedEvents := w.Manager.packEventsForBroadcast(ctx, job.Args.Events, job.Args.BatchID)
	targets = make(map[string]model.Events, len(uniqueRelays))
	for _, relayURL := range uniqueRelays {
		targets[relayURL] = packedEvents
	}
	return targets
}

func (w *broadcasterPushNotificationRemoteWorker) Work(ctx context.Context, job *broadcasterPushNotificationRemoteWorkerJob) error {
	log.Trace().Str("context", "PUSH_NOTIFICATIONS").Str("batch", job.Args.BatchID).Msg("starting remote push notification worker")

	targets := w.CollectTargets(ctx, job)
	if len(targets) == 0 {
		return nil
	}

	var nextJobArgs []rq.JobArgs
	for relayURL, events := range targets {
		nextJobArgs = append(nextJobArgs, &broadcasterBroadcastWorkerArgs{
			RelayURL: relayURL,
			BatchID:  job.Args.BatchID,
			Events:   events,
		})
	}

	err := w.Manager.rq.Push(ctx, nextJobArgs...)

	return errors.Wrapf(err, "failed to push broadcaster broadcast worker jobs for remote push notification for batch %s", job.Args.BatchID)
}

func collectRelaysFromDevice(ev *model.Event) []string {
	relays := make([]string, 0, len(ev.Tags))
	for _, tag := range ev.Tags {
		if tag.Key() == "relay" && tag.Value() != "" {
			relays = append(relays, tag.Value())
		}
	}
	return relays
}

func compactRelays(relays []string) []string {
	var compacted []string

	seen := make(map[string][]string)
	for _, relay := range relays {
		key := strings.ToLower(relay)
		if parsed, err := url.Parse(relay); err == nil {
			key = strings.ToLower(parsed.Hostname())
		}

		var isDuplicate bool
		for _, seenRelay := range seen[key] {
			if model.CompareRelaysURLs(relay, seenRelay) {
				isDuplicate = true
				break
			}
		}

		if !isDuplicate {
			seen[key] = append(seen[key], relay)
			compacted = append(compacted, relay)
		}
	}

	return compacted
}
