// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/pushnotifications/internal"
)

func (pm *PushNotificationManager) handleReactionNotification(event *model.Event, language Language) *NotificationBatch {
	var reactedToPubkey PublicKey
	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag.Key() == "e" {
			for _, ptag := range event.Tags {
				if len(ptag) >= 2 && ptag.Key() == "p" {
					reactedToPubkey = ptag.Value()
				}
			}
			break
		}
	}

	if reactedToPubkey == "" || reactedToPubkey == event.GetMasterPublicKey() {
		return &NotificationBatch{
			singleNotifications:    make([]*pn.Notification[pn.DeviceToken], 0),
			multicastNotifications: make([]*pn.Notification[pn.DeviceTokens], 0),
		}
	}

	title := pm.translationMgr.GetTranslation(NotificationTypeReaction, language, "title", nil)
	body := pm.translationMgr.GetTranslation(NotificationTypeReaction, language, "body", map[string]interface{}{"reaction": event.Content, "title": event.Content})
	imageURL := "" // TODO: add image URL.
	data := make(map[string]interface{})

	validDevices := pm.collectValidDevices(reactedToPubkey, NotificationTypeReaction, event)

	return pm.addNotificationsToDevices(validDevices, title, body, imageURL, data)
}
