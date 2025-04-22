// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
)

func (pm *PushNotificationManager) handleDirectMessageNotification(event *model.Event) []*pn.Notification[*model.Event] {
	pTag := event.GetTag("p")
	if pTag == nil {
		return nil
	}
	recipientPubKey := pTag.Value()
	if recipientPubKey == "" || recipientPubKey == event.GetMasterPublicKey() {
		return nil
	}
	deviceEvents := pm.collectUserValidDevices(recipientPubKey, event)

	return pm.createNotifications(deviceEvents, NotificationTypeDirectMessage, map[string]interface{}{
		"event": event.String(),
	})
}
