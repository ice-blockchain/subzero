// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/pushnotifications/internal"
)

func (pm *PushNotificationManager) handleSystemNotification(event *model.Event) *NotificationBatch {
	notifications := &NotificationBatch{
		topicNotifications: make([]*pn.Notification[pn.SubscriptionTopic], 0),
	}

	imageURL := "" // TODO: add image URL.
	data := map[string]interface{}{
		"event_id": event.ID,
	}

	allLanguages := pm.translationMgr.GetAvailableLanguages()
	for _, lang := range allLanguages {
		localizedTitle := pm.translationMgr.GetTranslation(NotificationTypeSystem, lang, "title", map[string]interface{}{"title": truncateContent(event.Content, 100)})
		localizedBody := pm.translationMgr.GetTranslation(NotificationTypeSystem, lang, "body", map[string]interface{}{"message": truncateContent(event.Content, 100)})

		notification := &pn.Notification[pn.SubscriptionTopic]{
			Title:    localizedTitle,
			Body:     localizedBody,
			ImageURL: imageURL,
			Data:     data,
			Target:   pn.SubscriptionTopic("system_" + string(lang)),
		}

		notifications.topicNotifications = append(notifications.topicNotifications, notification)
	}

	return notifications
}
