// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"github.com/ice-blockchain/subzero/model"
)

func (pm *PushNotificationManager) handlePaymentReceiveNotification(event *model.Event, language Language) *NotificationBatch {
	senderPubkey := event.GetTag("p").Value()
	amount := event.GetTag("amount").Value()
	network := event.GetTag("network").Value()
	address := event.GetTag("l").Value()

	if senderPubkey == "" || senderPubkey == event.GetMasterPublicKey() {
		return nil
	}

	title := pm.translationMgr.GetTranslation(NotificationTypePaymentReceived, language, "title", map[string]interface{}{"amount": amount, "address": address})
	body := pm.translationMgr.GetTranslation(NotificationTypePaymentReceived, language, "body", map[string]interface{}{"amount": amount, "address": address})
	imageURL := "" // TODO: add image URL.

	data := map[string]interface{}{
		"event_id": event.ID,
		"amount":   amount,
		"pubkey":   event.GetMasterPublicKey(),
		"network":  network,
	}

	validDevices := pm.collectValidDevices(senderPubkey, NotificationTypePaymentReceived, event)

	return pm.addNotificationsToDevices(validDevices, title, body, imageURL, data)
}
