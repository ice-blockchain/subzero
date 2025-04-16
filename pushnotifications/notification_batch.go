// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"context"
	"fmt"
	"log"

	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/pushnotifications/internal"
	"github.com/nbd-wtf/go-nostr"
)

type NotificationBatch struct {
	singleNotifications    []*pn.Notification[pn.DeviceToken]
	multicastNotifications []*pn.Notification[pn.DeviceTokens]
	topicNotifications     []*pn.Notification[pn.SubscriptionTopic]
}

func (pm *PushNotificationManager) NotifyFCM(ctx context.Context, language Language, events []*model.Event) error {
	if len(events) == 0 {
		return nil
	}

	batch := &NotificationBatch{
		singleNotifications:    make([]*pn.Notification[pn.DeviceToken], 0),
		multicastNotifications: make([]*pn.Notification[pn.DeviceTokens], 0),
		topicNotifications:     make([]*pn.Notification[pn.SubscriptionTopic], 0),
	}

	eventsByKind := make(map[int][]*model.Event)
	for _, event := range events {
		eventsByKind[event.Kind] = append(eventsByKind[event.Kind], event)
	}

	for kind, kindEvents := range eventsByKind {
		for _, event := range kindEvents {
			switch kind {
			case nostr.KindTextNote:
				if notifications := pm.handlePostNotification(ctx, event, language); notifications != nil {
					batch.singleNotifications = append(batch.singleNotifications, notifications.singleNotifications...)
					batch.multicastNotifications = append(batch.multicastNotifications, notifications.multicastNotifications...)
				}
			case nostr.KindReaction:
				if notifications := pm.handleReactionNotification(event, language); notifications != nil {
					batch.singleNotifications = append(batch.singleNotifications, notifications.singleNotifications...)
					batch.multicastNotifications = append(batch.multicastNotifications, notifications.multicastNotifications...)
				}
			case nostr.KindGiftWrap:
				if notifications := pm.handleDirectMessageNotification(event, language); notifications != nil {
					batch.singleNotifications = append(batch.singleNotifications, notifications.singleNotifications...)
					batch.multicastNotifications = append(batch.multicastNotifications, notifications.multicastNotifications...)
				}
			case nostr.KindRepost, nostr.KindGenericRepost:
				if notifications := pm.handleRepostNotification(event, language); notifications != nil {
					batch.singleNotifications = append(batch.singleNotifications, notifications.singleNotifications...)
					batch.multicastNotifications = append(batch.multicastNotifications, notifications.multicastNotifications...)
				}
			case model.CustomIONKindFundSendNotify:
				if notifications := pm.handlePaymentRequestNotification(event, language); notifications != nil {
					batch.singleNotifications = append(batch.singleNotifications, notifications.singleNotifications...)
					batch.multicastNotifications = append(batch.multicastNotifications, notifications.multicastNotifications...)
				}
			case model.CustomIONKindFundReceive:
				if notifications := pm.handlePaymentReceiveNotification(event, language); notifications != nil {
					batch.singleNotifications = append(batch.singleNotifications, notifications.singleNotifications...)
					batch.multicastNotifications = append(batch.multicastNotifications, notifications.multicastNotifications...)
				}
			case nostr.KindFollowList:
				if notifications := pm.handleNewFollowerNotification(event, language); notifications != nil {
					batch.singleNotifications = append(batch.singleNotifications, notifications.singleNotifications...)
					batch.multicastNotifications = append(batch.multicastNotifications, notifications.multicastNotifications...)
				}
			case nostr.KindChannelMessage:
				if notifications := pm.handleChannelMessagesNotification(event, language); notifications != nil {
					batch.singleNotifications = append(batch.singleNotifications, notifications.singleNotifications...)
					batch.multicastNotifications = append(batch.multicastNotifications, notifications.multicastNotifications...)
				}
			case nostr.KindSimpleGroupChatMessage:
				if notifications := pm.handleGroupChatMessagesNotification(event, language); notifications != nil {
					batch.singleNotifications = append(batch.singleNotifications, notifications.singleNotifications...)
					batch.multicastNotifications = append(batch.multicastNotifications, notifications.multicastNotifications...)
				}
			case model.CustomIONSystemMessage:
				if notifications := pm.handleSystemNotification(event); notifications != nil {
					batch.topicNotifications = append(batch.topicNotifications, notifications.topicNotifications...)
				}
			}
		}
	}

	errChan := make(chan error, len(batch.singleNotifications)+len(batch.multicastNotifications)+len(batch.topicNotifications))

	for _, notification := range batch.singleNotifications {
		go func(n *pn.Notification[pn.DeviceToken]) {
			err := (*pm.pushNotificationClient).SendSingle(ctx, n)
			if err != nil {
				if pn.IsInvalidDeviceToken(err) {
					deviceID := n.Target.DeviceID
					pm.deviceMutex.RLock()
					deviceInfo, exists := pm.devices[deviceID]
					pm.deviceMutex.RUnlock()
					if exists {
						if err := pm.markTokenAsInvalid(ctx, deviceID, deviceInfo.PubKey, n.Target.Token); err != nil {
							errChan <- fmt.Errorf("error marking token as invalid: %w", err)
							return
						}
					}
				}
			}
			errChan <- err
		}(notification)
	}

	for _, notification := range batch.multicastNotifications {
		go func(n *pn.Notification[pn.DeviceTokens]) {
			err := (*pm.pushNotificationClient).SendMulticast(ctx, n)
			if err != nil {
				if multicastErr, ok := err.(*pn.MulticastError); ok {
					for deviceToken, tokenErr := range multicastErr.GetInvalidTokens() {
						if pn.IsInvalidDeviceToken(tokenErr) {
							deviceID := deviceToken.DeviceID
							pm.deviceMutex.RLock()
							deviceInfo, exists := pm.devices[deviceID]
							pm.deviceMutex.RUnlock()
							if exists {
								if markErr := pm.markTokenAsInvalid(ctx, deviceID, deviceInfo.PubKey, deviceToken.Token); markErr != nil {
									log.Printf("Error marking token as invalid: %v", markErr)
								}
							}
						}
					}
				}
			}
			errChan <- err
		}(notification)
	}

	for _, notification := range batch.topicNotifications {
		go func(n *pn.Notification[pn.SubscriptionTopic]) {
			errChan <- (*pm.pushNotificationClient).SendTopic(ctx, n)
		}(notification)
	}

	var errors []error
	for i := 0; i < len(batch.singleNotifications)+len(batch.multicastNotifications)+len(batch.topicNotifications); i++ {
		if err := <-errChan; err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to send notifications: %v", errors)
	}

	return nil
}
