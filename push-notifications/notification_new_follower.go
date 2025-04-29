// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
)

func (pm *PushNotificationManager) handleNewFollowerEvent(ctx context.Context, event *model.Event, relatedEvents ...*model.Event) ([]*pn.Notification[*DeviceRegistrationEvent], error) {
	oldEvent, err := pm.getOldFollowListEvent(ctx, event.GetMasterPublicKey())
	if err != nil {
		return nil, errors.Wrap(err, "failed to get old follow list event")
	}
	recipientPubKey := pm.getLastFollowerPubkey(event, oldEvent)
	if recipientPubKey == "" {
		return nil, nil
	}

	return pm.createNewFollowerNotification(event, recipientPubKey, relatedEvents...), nil
}

func (pm *PushNotificationManager) getOldFollowListEvent(ctx context.Context, authorPubKey string) (*model.Event, error) {
	subscription := &model.Subscription{
		Filters: model.Filters{
			{
				Kinds:   []int{nostr.KindFollowList},
				Authors: []string{authorPubKey},
			},
		},
	}

	var oldEvent *model.Event
	it := query.GetStoredEvents(ctx, subscription)
	for ev, err := range it {
		if err != nil {
			return nil, errors.Wrap(err, "failed to get old follow list event")
		}
		oldEvent = ev

		break
	}

	return oldEvent, nil
}

func (pm *PushNotificationManager) getLastFollowerPubkey(event *model.Event, oldEvent *model.Event) string {
	currentPTags := event.GetTags("p")
	if len(currentPTags) == 0 {
		return ""
	}

	if oldEvent != nil {
		oldPTags := oldEvent.GetTags("p")
		if len(currentPTags) < len(oldPTags) {
			return ""
		}
	}

	return currentPTags[len(currentPTags)-1].Value()
}

func (pm *PushNotificationManager) createNewFollowerNotification(event *model.Event, recipientPubKey string, relatedEvents ...*model.Event) []*pn.Notification[*DeviceRegistrationEvent] {
	devices := pm.collectUserValidDevices(recipientPubKey, event)

	return pm.createNotifications(devices, NotificationTypeNewFollower, event, relatedEvents...)
}
