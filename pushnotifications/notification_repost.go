// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"encoding/json"

	"github.com/ice-blockchain/subzero/model"
)

func (pm *PushNotificationManager) handleRepostNotification(event *model.Event, language Language) *NotificationBatch {
	var repostedEvent model.Event
	if err := json.Unmarshal([]byte(event.Content), &repostedEvent); err != nil {
		return nil
	}

	title := pm.translationMgr.GetTranslation(NotificationTypeRepost, language, "title", map[string]interface{}{"pubkey": event.GetMasterPublicKey(), "title": truncateContent(repostedEvent.Content, 100)})
	body := pm.translationMgr.GetTranslation(NotificationTypeRepost, language, "body", map[string]interface{}{"pubkey": event.GetMasterPublicKey(), "title": truncateContent(repostedEvent.Content, 100)})
	imageURL := "" // TODO: add image URL.
	data := make(map[string]interface{})

	validDevices := pm.collectValidDevices(repostedEvent.GetMasterPublicKey(), NotificationTypeRepost, event)

	return pm.addNotificationsToDevices(validDevices, title, body, imageURL, data)
}
