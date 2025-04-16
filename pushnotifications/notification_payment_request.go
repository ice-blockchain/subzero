// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"github.com/ice-blockchain/subzero/model"
)

func (pm *PushNotificationManager) handlePaymentRequestNotification(event *model.Event, language Language) *NotificationBatch {
	recipientPubkey := event.GetTag("p").Value()
	address := event.GetTag("l").Value()
	amount := event.GetTag("amount").Value()
	network := event.GetTag("network").Value()

	if recipientPubkey == "" || recipientPubkey == event.GetMasterPublicKey() {
		return nil
	}

	title := pm.translationMgr.GetTranslation(NotificationTypePaymentRequest, language, "title", map[string]interface{}{"amount": amount, "address": address})
	body := pm.translationMgr.GetTranslation(NotificationTypePaymentRequest, language, "body", map[string]interface{}{"amount": amount, "address": address})
	imageURL := "" // TODO: add image URL.

	data := map[string]interface{}{
		"event_id": event.ID,
		"amount":   amount,
		"pubkey":   event.GetMasterPublicKey(),
		"network":  network,
	}

	validDevices := pm.collectValidDevices(recipientPubkey, NotificationTypePaymentRequest, event)

	return pm.addNotificationsToDevices(validDevices, title, body, imageURL, data)
}
