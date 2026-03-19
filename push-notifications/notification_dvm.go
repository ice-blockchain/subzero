// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"context"

	"github.com/rs/zerolog/log"

	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
)

func (pm *PushNotificationManager) handleNewDVMEvent(_ context.Context, event *model.Event, relevantEvents ...*model.Event) ([]*pn.Notification[*model.Event], error) {
	devices := pm.collectLocalDevices("", event)
	if len(devices) == 0 {
		log.Trace().
			Str("context", "PUSH_NOTIFICATION").
			Str("event_id", event.ID).
			Int("event_kind", event.Kind).
			Msg("no active devices found for DVM event, skipping notification")
		return nil, nil
	}

	var notificationType NotificationType
	switch event.Kind {
	case model.CustomIONKindDVMJobResponsePriceChange:
		notificationType = NotificationTypeTokenPriceChange
	case model.CustomIONKindDVMJobResponseTrendingTokens:
		notificationType = NotificationTypeTokenActivity
	default:
		log.Warn().
			Str("context", "PUSH_NOTIFICATION").
			Str("event_id", event.ID).
			Int("event_kind", event.Kind).
			Msg("unrecognized DVM event kind, skipping notification")
		return nil, nil
	}

	return pm.createNotifications(devices, notificationType, event, relevantEvents...)
}
