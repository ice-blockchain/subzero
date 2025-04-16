// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/pushnotifications/internal"
)

func (pm *PushNotificationManager) handleDirectMessageNotification(event *model.Event, language Language) *NotificationBatch {
	var recipient PublicKey
	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag[0] == "p" {
			recipient = tag[1]
			break
		}
	}

	if recipient == "" || recipient == event.GetMasterPublicKey() {
		return &NotificationBatch{
			singleNotifications:    make([]*pn.Notification[pn.DeviceToken], 0),
			multicastNotifications: make([]*pn.Notification[pn.DeviceTokens], 0),
		}
	}

	title := pm.translationMgr.GetTranslation(NotificationTypeDirectMessage, language, "title", map[string]interface{}{"pubkey": recipient})
	body := pm.translationMgr.GetTranslation(NotificationTypeDirectMessage, language, "body", map[string]interface{}{"pubkey": recipient, "message": event.Content})
	imageURL := "" // TODO: add image URL.
	data := make(map[string]interface{})

	validDevices := pm.collectValidDevices(recipient, NotificationTypeDirectMessage, event)

	return pm.addNotificationsToDevices(validDevices, title, body, imageURL, data)
}
