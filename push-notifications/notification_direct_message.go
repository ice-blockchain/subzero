// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"log"

	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
)

func (pm *PushNotificationManager) handleDirectMessageNotification(event *model.Event) []*pn.Notification[pn.DeviceToken] {
	recipient := event.GetTag("p").Value()
	if recipient == "" || recipient == event.GetMasterPublicKey() {
		return nil
	}
	iosDevices, otherDevices := pm.collectValidDevices(recipient, NotificationTypeDirectMessage, event)
	neventLinks := neventRegex.FindAllString(event.Content, -1)

	data := map[string]interface{}{
		"eventId":          event.ID,
		"authorPubKey":     event.GetMasterPublicKey(),
		"notificationType": string(NotificationTypeDirectMessage),
		"content":          event.Content,
	}

	for _, neventLink := range neventLinks {
		paymentInfo, err := extractPaymentInfoFromNeventLink(neventLink)
		if err != nil {
			log.Printf("error extracting payment info from nevent link: %s", err)

			continue
		}
		if paymentInfo != nil {
			data = populateNotificationDataFromPaymentInfo(paymentInfo, NotificationTypeDirectMessage)

			break
		}
	}

	return pm.createAndSendNotifications(iosDevices, otherDevices, NotificationTypeDirectMessage, data)
}
