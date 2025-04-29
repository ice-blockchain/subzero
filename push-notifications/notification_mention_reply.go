// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
)

func (pm *PushNotificationManager) handleMentionReplyEvent(event *model.Event, relatedEvents ...*model.Event) []*pn.Notification[*DeviceRegistrationEvent] {
	notifications := make([]*pn.Notification[*DeviceRegistrationEvent], 0)
	for _, pTag := range event.GetTags("p") {
		if pTag.Value() != "" {
			if pTag.Value() == event.GetMasterPublicKey() {
				continue
			}

			devices := pm.collectUserValidDevices(pTag.Value(), event)
			pubkeyNotifications := pm.createNotifications(devices, NotificationTypeMentionReply, event, relatedEvents...)

			notifications = append(notifications, pubkeyNotifications...)
		}
	}

	return notifications
}
