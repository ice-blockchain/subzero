// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"context"

	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
)

func (pm *PushNotificationManager) handleNewFollowerNotification(event *model.Event) []*pn.Notification[*model.Event] {
	oldEvent := pm.getOldFollowListEvent(context.Background(), event.GetMasterPublicKey())

	shouldSend, recipientPubKey := pm.shouldSendNewFollowerNotification(event, oldEvent)
	if !shouldSend {
		return nil
	}

	return pm.createNewFollowerNotification(event, recipientPubKey)
}

func (pm *PushNotificationManager) getOldFollowListEvent(ctx context.Context, authorPubKey string) *model.Event {
	subscription := &model.Subscription{
		Filters: model.Filters{
			{
				Kinds:   []int{nostr.KindFollowList},
				Authors: []string{authorPubKey},
			},
		},
	}

	var oldEvent *model.Event
	query.GetStoredEvents(ctx, subscription)(func(e *model.Event, err error) bool {
		if err != nil {
			return false
		}
		oldEvent = e
		return true
	})

	return oldEvent
}

func (pm *PushNotificationManager) shouldSendNewFollowerNotification(event *model.Event, oldEvent *model.Event) (bool, string) {
	currentPTags := event.GetTags("p")
	if len(currentPTags) == 0 {
		return false, ""
	}

	if oldEvent != nil {
		oldPTags := oldEvent.GetTags("p")
		if len(currentPTags) < len(oldPTags) {
			return false, ""
		}
	}

	lastFollowedPubKey := currentPTags[len(currentPTags)-1].Value()
	if lastFollowedPubKey == "" || lastFollowedPubKey == event.GetMasterPublicKey() {
		return false, ""
	}

	return true, lastFollowedPubKey
}

func (pm *PushNotificationManager) createNewFollowerNotification(event *model.Event, recipientPubKey string) []*pn.Notification[*model.Event] {
	devices := pm.collectUserValidDevices(recipientPubKey, event)

	return pm.createNotifications(devices, NotificationTypeNewFollower, map[string]interface{}{
		"event": event.String(),
	})
}
