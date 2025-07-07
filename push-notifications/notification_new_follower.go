// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"context"

	"github.com/cockroachdb/errors"

	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
)

func (pm *PushNotificationManager) handleNewFollowerEvent(ctx context.Context, event *model.Event, relevantEvents ...*model.Event) ([]*pn.Notification[*DeviceRegistrationEvent], error) {
	newlyFollowedPubKeys := pm.getNewlyFollowedPubkeys(event, event.Previous)
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

func (pm *PushNotificationManager) getNewlyFollowedPubkeys(event *model.Event, oldEvent *model.Event) []string {
	currentPTags := event.GetTags("p")
	if len(currentPTags) == 0 {
		return nil
	}

	currentPubKeys := make(map[string]struct{})
	for _, tag := range currentPTags {
		currentPubKeys[tag.Value()] = struct{}{}
	}

	if oldEvent == nil {
		result := make([]string, 0, len(currentPubKeys))
		for pubKey := range currentPubKeys {
			result = append(result, pubKey)
		}

		return result
	}
	oldPTags := oldEvent.GetTags("p")
	oldPubKeys := make(map[string]struct{})
	for _, tag := range oldPTags {
		oldPubKeys[tag.Value()] = struct{}{}
	}

	var newPubKeys []string
	for pubKey := range currentPubKeys {
		if _, exists := oldPubKeys[pubKey]; !exists {
			newPubKeys = append(newPubKeys, pubKey)
		}
	}

	return newPubKeys
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
