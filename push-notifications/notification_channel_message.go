// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
)

func (pm *PushNotificationManager) handleCommunityMessageNotification(event *model.Event) []*pn.Notification[*model.Event] {
	communityID := event.GetHTag()
	if communityID == "" {
		return nil
	}
	allDevices := make([]*model.Event, 0)
	for _, tag := range event.GetTags("e") {
		if len(tag) < 4 {
			continue
		}

		recipientPubKey := tag.Value()
		if recipientPubKey == event.GetMasterPublicKey() {
			continue
		}

		if tag[3] == "root" || tag[3] == "reply" {
			allDevices = append(allDevices, pm.collectUserValidDevices(recipientPubKey, event)...)
		}
	}
	data := map[string]interface{}{
		"event": event.String(),
	}

	return pm.createNotifications(allDevices, NotificationTypeChannelMessage, data)
}
