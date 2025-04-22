// SPDX-License-Identifier: ice License 1.0


package pushnotifications

import (
	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
)

func (pm *PushNotificationManager) handleReactionNotification(event *model.Event) []*pn.Notification[*model.Event] {
	referencePubkey := event.GetTag("p").Value()
	if referencePubkey == "" || referencePubkey == event.GetMasterPublicKey() {
		return nil
	}
	deviceEvents := pm.collectUserValidDevices(referencePubkey, NotificationTypeReaction, event)

	return pm.createNotifications(deviceEvents, NotificationTypeReaction, map[string]interface{}{
		"event": event.String(),
	})
}
