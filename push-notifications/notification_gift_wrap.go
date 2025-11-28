// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"slices"
	"strconv"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
)

var (
	mapGiftWrapToNotificationType = map[int]NotificationType{
		nostr.KindDirectMessage:           NotificationTypeDirectMessage,
		model.CustomIONKindDirectMessage:  NotificationTypeDirectMessage,
		model.CustomIONKindFundReceive:    NotificationTypePaymentReceived,
		model.CustomIONKindFundSendNotify: NotificationTypePaymentRequest,
		nostr.KindReaction:                NotificationTypeReaction,
	}
)

func (pm *PushNotificationManager) handleGiftWrapEvent(event *model.Event) ([]*pn.Notification[*DeviceRegistrationEvent], error) {
	var (
		deviceEvents []*model.Event
		kTag, pTag   model.Tag
	)
	kTag = event.GetTag("k")
	if kTag == nil {
		return nil, nil
	}
	kind, err := strconv.Atoi(kTag.Value())
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert k tag to int")
	}
	pTag = event.GetTag("p")
	if pTag == nil {
		return nil, nil
	}
	recipientMasterPubKey := pTag.Value()
	if recipientMasterPubKey == "" || recipientMasterPubKey == event.GetMasterPublicKey() {
		return nil, nil
	}
	if len(pTag) < 4 {
		return nil, nil
	}
	devicePubKey := pTag[3]
	if devicePubKey == "" {
		return nil, nil
	}
	deviceRegistrationEvents := pm.collectUserValidDevices(recipientMasterPubKey, event)
	evIdx := slices.IndexFunc(deviceRegistrationEvents, func(ev *model.Event) bool {
		return ev.PubKey == devicePubKey
	})
	if evIdx == -1 {
		return nil, nil
	}
	deviceEvents = append(deviceEvents, deviceRegistrationEvents[evIdx])
	notifications, err := pm.createNotifications(deviceEvents, mapGiftWrapToNotificationType[kind], event)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create notifications")
	}

	return notifications, nil
}
