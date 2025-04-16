// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"github.com/ice-blockchain/subzero/model"
)

func (pm *PushNotificationManager) handleChannelMessagesNotification(event *model.Event, language Language) *NotificationBatch {
	var channelPubkey string
	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag[0] == "p" {
			channelPubkey = tag[1]
			break
		}
	}
	if channelPubkey == "" {
		return nil
	}

	title := pm.translationMgr.GetTranslation(NotificationTypeChannelMessage, language, "title", map[string]interface{}{"channel_name": channelPubkey})
	body := pm.translationMgr.GetTranslation(NotificationTypeChannelMessage, language, "body", map[string]interface{}{"message": truncateContent(event.Content, 100)})
	imageURL := "" // TODO: add image URL.
	data := map[string]interface{}{
		"event_id": event.ID,
	}
	validDevices := pm.collectValidDevices(channelPubkey, NotificationTypeChannelMessage, event)

	return pm.addNotificationsToDevices(validDevices, title, body, imageURL, data)
}
