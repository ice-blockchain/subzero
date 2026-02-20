// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"github.com/cockroachdb/errors"

	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
)

func (pm *PushNotificationManager) handleMentionReplyEvent(event *model.Event, relevantEvents ...*model.Event) ([]*pn.Notification[*model.Event], error) {
	notifications := make([]*pn.Notification[*model.Event], 0)
	mentionedPubkeys, err := model.ExtractMentionedPubkeys(event)
	if err != nil {
		return nil, errors.Wrap(err, "failed to extract mentioned pubkeys")
	}
	processedPubkeys := make(map[string]bool)
	for _, pubkey := range mentionedPubkeys { // mentions in content/rich_text
		if pubkey == event.GetMasterPublicKey() {
			continue
		}
		if processedPubkeys[pubkey] {
			continue
		}
		processedPubkeys[pubkey] = true
		devices := pm.collectLocalDevices(pubkey, event)
		pubkeyNotifications, err := pm.createNotifications(devices, NotificationTypeMentionReply, event, relevantEvents...)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create notifications")
		}
		notifications = append(notifications, pubkeyNotifications...)
	}
	for _, pTag := range event.GetTags("p") { // replies in p tag
		if pTag.Value() == event.GetMasterPublicKey() {
			continue
		}
		if processedPubkeys[pTag.Value()] {
			continue
		}
		processedPubkeys[pTag.Value()] = true
		devices := pm.collectLocalDevices(pTag.Value(), event)
		pubkeyNotifications, err := pm.createNotifications(devices, NotificationTypeMentionReply, event, relevantEvents...)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create notifications")
		}
		notifications = append(notifications, pubkeyNotifications...)
	}

	return notifications, nil
}
