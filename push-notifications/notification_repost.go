// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
)

func (pm *PushNotificationManager) handleRepostNotification(event *model.Event) []*pn.Notification[pn.DeviceToken] {
	originalPostAuthor := event.GetTag("p").Value()
	originalPostID := event.GetTag("e").Value()
	if originalPostAuthor == "" || originalPostAuthor == event.GetMasterPublicKey() {
		return nil
	}

	iosDevices, otherDevices := pm.collectValidDevices(originalPostAuthor, NotificationTypeRepost, event)

	data := map[string]interface{}{
		"eventId":          event.ID,
		"authorPubKey":     event.GetMasterPublicKey(),
		"notificationType": string(NotificationTypeRepost),
		"content":          event.Content,
		"repostedEventId":  originalPostID,
	}

	return pm.createAndSendNotifications(iosDevices, otherDevices, NotificationTypeRepost, data)
}
