// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"context"

	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
)

func (pm *PushNotificationManager) handleNewPostEvent(_ context.Context, event *model.Event, relevantEvents ...*model.Event) (notifications []*pn.Notification[*model.Event], err error) {
	authorMasterKey := event.GetMasterPublicKey()
	devices := filterDevices(pm.collectLocalDevices("", event), event, func(deviceEvent, _ *model.Event) bool {
		return deviceEvent.GetMasterPublicKey() != authorMasterKey // Don't notify the author of the article.
	})

	if len(devices) == 0 {
		return nil, nil
	}

	var notificationType NotificationType
	switch event.Kind {
	case nostr.KindArticle:
		notificationType = NotificationTypeSomeoneArticle

	case model.CustomIONKindEditableTextNote:
		switch {
		case event.GetTag("expiration").Value() != "":
			notificationType = NotificationTypeSomeoneStory
		case event.HasVideoIMeta():
			notificationType = NotificationTypeSomeoneVideo
		default:
			notificationType = NotificationTypeSomeonePost
		}

	default:
		return nil, nil
	}

	return pm.createNotifications(devices, notificationType, event, relevantEvents...)
}
