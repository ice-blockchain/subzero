// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"context"

	"github.com/cockroachdb/errors"

	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
	"github.com/ice-blockchain/subzero/validation"
)

func (pm *PushNotificationManager) handleCommunityMessageEvent(ctx context.Context, event *model.Event, relevantEvents ...*model.Event) ([]*pn.Notification[*DeviceRegistrationEvent], error) {
	referencePubkey := event.GetTag("p").Value()
	if referencePubkey == "" || referencePubkey == event.GetMasterPublicKey() {
		return nil, nil
	}
	notificationType, err := getCommunityNotificationType(ctx, event)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get community notification type")
	}
	deviceEvents := pm.collectUserValidDevices(referencePubkey, event)
	notifications, err := pm.createNotifications(deviceEvents, notificationType, event, relevantEvents...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create notifications")
	}

	return notifications, nil
}

func getCommunityNotificationType(ctx context.Context, event *model.Event) (NotificationType, error) {
	communityDefinition, err := validation.GetCommunityDefinition(ctx, event.GetHTag())
	if err != nil {
		return NotificationTypeChannelMessage, errors.Wrap(err, "failed to get community definition")
	}
	settings := communityDefinition.GetTags("settings")
	if len(settings) == 0 {
		return NotificationTypeChannelMessage, nil
	}
	for _, setting := range settings {
		if setting.Value() == model.CommentsEnabledSettings && len(setting) > 2 && setting[2] == "true" {
			return NotificationTypeGroupChatMessage, nil
		}
	}

	return NotificationTypeChannelMessage, nil
}
