// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"github.com/cockroachdb/errors"

	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
)

func (pm *PushNotificationManager) handleNewFollowerEvent(event *model.Event, relevantEvents ...*model.Event) ([]*pn.Notification[*DeviceRegistrationEvent], error) {
	newlyFollowedPubKeys := model.GetNewlyFollowedPubkeys(event, event.Previous)
	if len(newlyFollowedPubKeys) == 0 {
		return nil, nil
	}

	var allNotifications []*pn.Notification[*DeviceRegistrationEvent]
	for _, recipientPubKey := range newlyFollowedPubKeys {
		notifications, err := pm.createNewFollowerNotification(event, recipientPubKey, relevantEvents...)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create new follower notification")
		}
		allNotifications = append(allNotifications, notifications...)
	}

	return allNotifications, nil
}

func (pm *PushNotificationManager) createNewFollowerNotification(event *model.Event, recipientPubKey string, relevantEvents ...*model.Event) ([]*pn.Notification[*DeviceRegistrationEvent], error) {
	if recipientPubKey == event.GetMasterPublicKey() {
		return nil, nil
	}

	devices := pm.collectUserValidDevices(recipientPubKey, event)
	notifications, err := pm.createNotifications(devices, NotificationTypeNewFollower, event, relevantEvents...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create new follower notification")
	}

	return notifications, nil
}
