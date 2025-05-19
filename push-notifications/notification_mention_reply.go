// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"github.com/cockroachdb/errors"

	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
)

func (pm *PushNotificationManager) handleMentionReplyEvent(event *model.Event, relevantEvents ...*model.Event) ([]*pn.Notification[*DeviceRegistrationEvent], error) {
	notifications := make([]*pn.Notification[*DeviceRegistrationEvent], 0)
	for _, pTag := range event.GetTags("p") {
		if pTag.Value() != "" {
			if pTag.Value() == event.GetMasterPublicKey() {
				continue
			}

			devices := pm.collectUserValidDevices(pTag.Value(), event)
			pubkeyNotifications, err := pm.createNotifications(devices, NotificationTypeMentionReply, event, relevantEvents...)
			if err != nil {
				return nil, errors.Wrap(err, "failed to create notifications")
			}

			notifications = append(notifications, pubkeyNotifications...)
		}
	}

	return notifications, nil
}
