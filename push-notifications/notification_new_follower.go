// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"github.com/cockroachdb/errors"

	"github.com/ice-blockchain/subzero/model"
)

func (pm *PushNotificationManager) handleNewFollowerEvent(event *model.Event, relevantEvents ...*model.Event) (*notificationTargets, error) {
	newlyFollowedPubKeys := model.GetNewlyFollowedPubkeys(event, event.Previous)
	if len(newlyFollowedPubKeys) == 0 {
		return nil, nil
	}

	var allNotifications notificationTargets
	for _, recipientPubKey := range newlyFollowedPubKeys {
		notifications, err := pm.createNewFollowerNotification(event, recipientPubKey, relevantEvents...)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create new follower notification")
		}
		allNotifications.Append(notifications)
	}

	return &allNotifications, nil
}

func (pm *PushNotificationManager) createNewFollowerNotification(event *model.Event, recipientPubKey string, relevantEvents ...*model.Event) (*notificationTargets, error) {
	if recipientPubKey == event.GetMasterPublicKey() {
		return nil, nil
	}

	localDevices, remoteDevices := pm.collectNotificationDevices(recipientPubKey, event)
	notifications, err := pm.createNotifications(localDevices, remoteDevices, NotificationTypeNewFollower, event, relevantEvents...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create new follower notification")
	}
	return notifications, nil
}
