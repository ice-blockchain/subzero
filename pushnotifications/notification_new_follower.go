// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/pushnotifications/internal"
)

func (pm *PushNotificationManager) handleNewFollowerNotification(event *model.Event, language Language) *NotificationBatch {
	notifications := &NotificationBatch{
		singleNotifications:    make([]*pn.Notification[pn.DeviceToken], 0),
		multicastNotifications: make([]*pn.Notification[pn.DeviceTokens], 0),
	}

	var followers []string
	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag[0] == "p" {
			followers = append(followers, tag[1])
		}
	}

	if len(followers) == 0 {
		return nil
	}
	// TODO: take previous followers list from db to check if there was unfollowing?

	if len(followers) > 0 {
		lastFollowedPubKey := followers[len(followers)-1]
		if lastFollowedPubKey == "" || lastFollowedPubKey == event.GetMasterPublicKey() {
			return nil
		}

		title := pm.translationMgr.GetTranslation(NotificationTypeNewFollower, language, "title", map[string]interface{}{"pubkey": lastFollowedPubKey})
		body := pm.translationMgr.GetTranslation(NotificationTypeNewFollower, language, "body", map[string]interface{}{"pubkey": lastFollowedPubKey})
		imageURL := "" // TODO: add image URL.

		validDevices := pm.collectValidDevices(lastFollowedPubKey, NotificationTypeNewFollower, event)

		if followBatch := pm.addNotificationsToDevices(validDevices, title, body, imageURL, nil); followBatch != nil {
			notifications.singleNotifications = append(notifications.singleNotifications, followBatch.singleNotifications...)
			notifications.multicastNotifications = append(notifications.multicastNotifications, followBatch.multicastNotifications...)
		}
	}

	return notifications
}
