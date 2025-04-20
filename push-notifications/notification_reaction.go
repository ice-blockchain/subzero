// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
)

func (pm *PushNotificationManager) handleReactionNotification(event *model.Event) []*pn.Notification[pn.DeviceToken] {
	referencePubkey := event.GetTag("p").Value()
	referenceEventID := event.GetTag("e").Value()
	reactionContent := ""
	if event.Content != "" {
		reactionContent = event.Content
	}
	if referencePubkey == "" || referencePubkey == event.GetMasterPublicKey() {
		return nil
	}

	data := map[string]interface{}{
		"eventId":          event.ID,
		"authorPubKey":     event.GetMasterPublicKey(),
		"notificationType": string(NotificationTypeReaction),
		"referenceEventId": referenceEventID,
		"reaction":         reactionContent,
	}

	iosDevices, otherDevices := pm.collectValidDevices(referencePubkey, NotificationTypeReaction, event)

	return pm.createAndSendNotifications(iosDevices, otherDevices, NotificationTypeReaction, data)
}
