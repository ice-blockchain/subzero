// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"github.com/ice-blockchain/subzero/model"
)

func (pm *PushNotificationManager) handleGroupChatMessagesNotification(event *model.Event, language Language) *NotificationBatch {
	var groupPubkey string

	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag.Key() == "p" {
			groupPubkey = tag.Value()

			break
		}
	}

	if groupPubkey == "" {
		return nil
	}

	title := pm.translationMgr.GetTranslation(NotificationTypeGroupChatMessage, language, "title", map[string]interface{}{"group_name": groupPubkey})
	body := pm.translationMgr.GetTranslation(NotificationTypeGroupChatMessage, language, "body", map[string]interface{}{"message": truncateContent(event.Content, 100)})
	imageURL := "" // TODO: add image URL.
	data := make(map[string]interface{})

	validDevices := pm.collectValidDevices(groupPubkey, NotificationTypeGroupChatMessage, event)

	return pm.addNotificationsToDevices(validDevices, title, body, imageURL, data)
}
