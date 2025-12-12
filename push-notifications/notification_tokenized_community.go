// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"github.com/cockroachdb/errors"

	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
)

func (pm *PushNotificationManager) handleTokenizedCommunityEvent(event *model.Event, relevantEvents ...*model.Event) ([]*pn.Notification[*DeviceRegistrationEvent], error) {
	creator := event.GetTag("p").Value()
	if creator == "" || creator == event.GetMasterPublicKey() {
		return nil, nil
	}

	devices := pm.collectUserValidDevices(creator, event)
	notifications, err := pm.createNotifications(devices, NotificationTypeTokenizedCommunityBuy, event, relevantEvents...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create tokenized community notification")
	}

	return notifications, nil
}
