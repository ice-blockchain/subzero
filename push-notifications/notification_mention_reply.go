// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"github.com/cockroachdb/errors"

	"github.com/ice-blockchain/subzero/model"
)

func (pm *PushNotificationManager) handleMentionReplyEvent(event *model.Event, relevantEvents ...*model.Event) (*notificationTargets, error) {
	var targets notificationTargets

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
		localDevices, remoteDevices := pm.collectNotificationDevices(pubkey, event)
		pubkeyNotifications, err := pm.createNotifications(localDevices, remoteDevices, NotificationTypeMentionReply, event, relevantEvents...)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create notifications")
		}
		targets.Local = append(targets.Local, pubkeyNotifications.Local...)
		targets.Remote = append(targets.Remote, pubkeyNotifications.Remote...)
	}
	for _, pTag := range event.GetTags("p") { // replies in p tag
		if pTag.Value() == event.GetMasterPublicKey() {
			continue
		}
		if processedPubkeys[pTag.Value()] {
			continue
		}
		processedPubkeys[pTag.Value()] = true
		localDevices, remoteDevices := pm.collectNotificationDevices(pTag.Value(), event)
		pubkeyNotifications, err := pm.createNotifications(localDevices, remoteDevices, NotificationTypeMentionReply, event, relevantEvents...)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create notifications")
		}
		targets.Append(pubkeyNotifications)
	}

	return &targets, nil
}
