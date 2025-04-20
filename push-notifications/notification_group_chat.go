// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
)

// TODO: specify the parameters of the event.
func (pm *PushNotificationManager) handleGroupChatMessageNotification(event *model.Event) []*pn.Notification[pn.DeviceToken] {
	groupID := ""
	replyToId := ""
	for _, tag := range event.GetTags("e") {
		if len(tag) < 4 {
			continue
		}
		if tag[3] == "root" {
			groupID = tag.Value()
		} else if tag[3] == "reply" {
			replyToId = tag.Value()
		}
	}

	if groupID == "" {
		return nil
	}

	iosDevices, otherDevices := pm.collectValidDevices(groupID, NotificationTypeGroupChatMessage, event)

	data := make(map[string]interface{})
	data["eventId"] = event.ID
	data["authorPubKey"] = event.GetMasterPublicKey()
	data["notificationType"] = string(NotificationTypeGroupChatMessage)
	data["groupId"] = groupID
	if replyToId != "" {
		data["replyToId"] = replyToId
	}
	data["content"] = event.Content

	return pm.createAndSendNotifications(iosDevices, otherDevices, NotificationTypeGroupChatMessage, data)
}
