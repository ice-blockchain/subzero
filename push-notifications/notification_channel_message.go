// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
)

// TODO: specify the parameters of the event.
func (pm *PushNotificationManager) handleChannelMessageNotification(event *model.Event) []*pn.Notification[pn.DeviceToken] {
	channelID := ""
	replyToId := ""
	for _, tag := range event.GetTags("e") {
		if len(tag) < 4 {
			continue
		}
		if tag[3] == "root" {
			channelID = tag.Value()
		} else if tag[3] == "reply" {
			replyToId = tag.Value()
		}
	}
	if channelID == "" {
		return nil
	}

	data := map[string]interface{}{
		"eventId":          event.ID,
		"authorPubKey":     event.GetMasterPublicKey(),
		"notificationType": string(NotificationTypeChannelMessage),
		"channelId":        channelID,
		"replyToId":        replyToId,
		"content":          event.Content,
	}
	iosDevices, otherDevices := pm.collectValidDevices(channelID, NotificationTypeChannelMessage, event)

	return pm.createAndSendNotifications(iosDevices, otherDevices, NotificationTypeChannelMessage, data)
}
