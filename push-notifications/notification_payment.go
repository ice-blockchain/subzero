// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"strconv"

	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
)

func (pm *PushNotificationManager) handlePaymentNotification(event *model.Event) []*pn.Notification[*model.Event] {
	pTag := event.GetTag("p")
	if pTag == nil {
		return nil
	}
	recipientPubKey := pTag.Value()
	if recipientPubKey == "" {
		return nil
	}
	if recipientPubKey == event.GetMasterPublicKey() {
		return nil
	}
	kTag := event.GetTag("k")
	if kTag == nil {
		return nil
	}

	kTagValue, err := strconv.Atoi(kTag.Value())
	if err != nil {
		return nil
	}

	tpe := NotificationTypePaymentRequest
	if kTagValue == model.CustomIONKindFundReceive {
		tpe = NotificationTypePaymentReceived
	}

	deviceEvents := pm.collectUserValidDevices(recipientPubKey, event)

	return pm.createNotifications(deviceEvents, tpe, map[string]interface{}{
		"event": event.String(),
	})
}
