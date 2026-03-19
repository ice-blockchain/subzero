// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"github.com/cockroachdb/errors"

	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
)

func (pm *PushNotificationManager) handleAnonymousFundSendEvent(event *model.Event, relevantEvents ...*model.Event) ([]*pn.Notification[*model.Event], error) {
	recipientMasterPubKey := event.GetTag("p").Value()
	if recipientMasterPubKey == "" || recipientMasterPubKey == event.GetMasterPublicKey() {
		return nil, nil
	}

	devices := pm.collectLocalDevices(recipientMasterPubKey, event)
	notifications, err := pm.createNotifications(devices, NotificationTypeAnonymousPaymentReceived, event, relevantEvents...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create anonymous payment notification")
	}
	return notifications, nil
}
