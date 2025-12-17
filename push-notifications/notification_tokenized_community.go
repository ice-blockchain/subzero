// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"context"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/rs/zerolog/log"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
)

func (pm *PushNotificationManager) handleTokenizedCommunityEvent(ctx context.Context, event *model.Event, relevantEvents ...*model.Event) ([]*pn.Notification[*DeviceRegistrationEvent], error) {
	var targetMasterKey string
	var notifyType NotificationType

	switch event.Kind {
	case model.CustomIONKindTokenizedCommunityAction:
		notifyType = NotificationTypeTokenizedCommunityAction
		targetAddressTag := event.GetTag("a").Value()
		if targetAddressTag == "" {
			eventID := event.GetTag("e").Value()
			if eventID == "" {
				log.Debug().Str("event", event.String()).Msg("no address or event tag found for tokenized community action")
				return nil, nil
			}
			for ev, err := range query.GetStoredEvents(ctx, model.Filter{
				IDs:   []string{eventID},
				Kinds: []int{model.CustomIONKindTokenizedCommunityDefinition},
				Limit: 1,
			}) {
				if err != nil {
					return nil, errors.Wrapf(err, "failed to get linked event %q for tokenized community action", eventID)
				}
				targetMasterKey = ev.GetMasterPublicKey() // Use the master key of the parent 31175.
				break
			}
		} else {
			// Address tag, use master key part directly.
			parts := strings.Split(targetAddressTag, ":")
			if len(parts) >= 2 {
				targetMasterKey = parts[1]
			}
		}

	case model.CustomIONKindTokenizedCommunityDefinition:
		notifyType = NotificationTypeTokenizedCommunityCreated
		targetMasterKey = event.GetTag("p").Value()

	default:
		log.Warn().Int("kind", event.Kind).Msg("unsupported tokenized community event kind")
		return nil, nil
	}

	if targetMasterKey == "" || targetMasterKey == event.GetMasterPublicKey() {
		return nil, nil
	}

	devices := pm.collectUserValidDevices(targetMasterKey, event)
	notifications, err := pm.createNotifications(devices, notifyType, event, relevantEvents...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create tokenized community notification for type %q", notifyType)
	}

	return notifications, nil
}
