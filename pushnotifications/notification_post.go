// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"context"
	"strings"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/pushnotifications/internal"
	"github.com/nbd-wtf/go-nostr"
)

func (pm *PushNotificationManager) handlePostNotification(ctx context.Context, event *model.Event, language Language) *NotificationBatch {
	notifications := &NotificationBatch{
		singleNotifications:    make([]*pn.Notification[pn.DeviceToken], 0),
		multicastNotifications: make([]*pn.Notification[pn.DeviceTokens], 0),
	}

	isReply, isMention, replyToPubkey, mentionedPubkeys := pm.classifyPostType(event)

	if isReply && replyToPubkey != "" && replyToPubkey != event.GetMasterPublicKey() {
		replyBatch := pm.handleReplyPost(event, replyToPubkey, language)
		if replyBatch != nil {
			notifications.singleNotifications = append(notifications.singleNotifications, replyBatch.singleNotifications...)
			notifications.multicastNotifications = append(notifications.multicastNotifications, replyBatch.multicastNotifications...)
		}
	}

	mentionBatch := pm.handleMentionPost(event, mentionedPubkeys, language)
	if mentionBatch != nil {
		notifications.singleNotifications = append(notifications.singleNotifications, mentionBatch.singleNotifications...)
		notifications.multicastNotifications = append(notifications.multicastNotifications, mentionBatch.multicastNotifications...)
	}

	if !isReply && !isMention {
		regularPostBatch := pm.handleRegularPost(ctx, event, language)
		if regularPostBatch != nil {
			notifications.singleNotifications = append(notifications.singleNotifications, regularPostBatch.singleNotifications...)
			notifications.multicastNotifications = append(notifications.multicastNotifications, regularPostBatch.multicastNotifications...)
		}
	}

	return notifications
}

func (pm *PushNotificationManager) classifyPostType(event *model.Event) (isReply bool, isMention bool, replyToPubkey string, mentionedPubkeys []string) {
	isReply = false
	isMention = false
	mentionedPubkeys = make([]string, 0)

	for _, tag := range event.Tags {
		if len(tag) >= 4 && tag.Key() == "e" {
			if tag[3] == model.TagMarkerReply || tag[3] == model.TagMarkerRoot {
				isReply = true
				for _, ptag := range event.Tags {
					if len(ptag) >= 2 && ptag.Key() == "p" {
						replyToPubkey = ptag.Value()

						break
					}
				}
			} else if tag[3] == model.TagMarkerMention {
				isMention = true
			}
		}
		if len(tag) >= 2 && tag.Key() == "p" {
			if tag.Value() != event.GetMasterPublicKey() {
				mentionedPubkeys = append(mentionedPubkeys, tag.Value())
			}
		}
	}

	contentWords := strings.Fields(event.Content)
	for _, word := range contentWords {
		if strings.HasPrefix(word, "@npub") {
			isMention = true
		}
	}

	if len(mentionedPubkeys) > 0 && !isReply {
		isMention = true
	}

	return
}

func (pm *PushNotificationManager) handleReplyPost(event *model.Event, replyToPubkey string, language Language) *NotificationBatch {
	title := pm.translationMgr.GetTranslation(NotificationTypeReply, language, "title", map[string]interface{}{"pubkey": event.GetMasterPublicKey()})
	body := pm.translationMgr.GetTranslation(NotificationTypeReply, language, "body", map[string]interface{}{"pubkey": event.GetMasterPublicKey(), "message": truncateContent(event.Content, 100)})
	imageURL := "" // TODO: add image URL.
	data := make(map[string]interface{})

	validDevices := pm.collectValidDevices(replyToPubkey, NotificationTypeReply, event)

	return pm.addNotificationsToDevices(validDevices, title, body, imageURL, data)
}

func (pm *PushNotificationManager) handleMentionPost(event *model.Event, mentionedPubkeys []string, language Language) *NotificationBatch {
	notifications := &NotificationBatch{
		singleNotifications:    make([]*pn.Notification[pn.DeviceToken], 0),
		multicastNotifications: make([]*pn.Notification[pn.DeviceTokens], 0),
	}

	if len(mentionedPubkeys) == 0 {
		return nil
	}

	title := pm.translationMgr.GetTranslation(NotificationTypeMention, language, "title", map[string]interface{}{"pubkey": event.GetMasterPublicKey()})
	body := pm.translationMgr.GetTranslation(NotificationTypeMention, language, "body", map[string]interface{}{"pubkey": event.GetMasterPublicKey(), "message": truncateContent(event.Content, 100)})
	imageURL := "" // TODO: add image URL.
	data := make(map[string]interface{})

	for _, pubkey := range mentionedPubkeys {
		if pubkey == event.GetMasterPublicKey() {
			continue
		}

		validDevices := pm.collectValidDevices(pubkey, NotificationTypeMention, event)

		mentionBatch := pm.addNotificationsToDevices(validDevices, title, body, imageURL, data)

		if mentionBatch != nil {
			notifications.singleNotifications = append(notifications.singleNotifications, mentionBatch.singleNotifications...)
			notifications.multicastNotifications = append(notifications.multicastNotifications, mentionBatch.multicastNotifications...)
		}
	}

	return notifications
}

func (pm *PushNotificationManager) handleRegularPost(ctx context.Context, event *model.Event, language Language) *NotificationBatch {
	notifications := &NotificationBatch{
		singleNotifications:    make([]*pn.Notification[pn.DeviceToken], 0),
		multicastNotifications: make([]*pn.Notification[pn.DeviceTokens], 0),
	}

	authorPubKey := event.GetMasterPublicKey()
	subscription := &model.Subscription{
		Filters: []model.Filter{
			{
				Kinds:   []int{nostr.KindFollowList},
				Authors: []string{authorPubKey},
			},
		},
	}

	followerPubkeys := make([]string, 0)
	eventIterator := query.GetStoredEvents(ctx, subscription)
	for ev, err := range eventIterator {
		if err != nil || ev == nil {
			continue
		}

		followerPubKey := ev.GetMasterPublicKey()
		followerPubkeys = append(followerPubkeys, followerPubKey)
	}

	if len(followerPubkeys) == 0 {
		return notifications
	}

	title := pm.translationMgr.GetTranslation(NotificationTypePost, language, "title", map[string]interface{}{"pubkey": event.GetMasterPublicKey()})
	body := pm.translationMgr.GetTranslation(NotificationTypePost, language, "body", map[string]interface{}{"pubkey": event.GetMasterPublicKey(), "message": truncateContent(event.Content, 100)})
	imageURL := "" // TODO: add image URL.
	data := make(map[string]interface{})

	for _, followerPubKey := range followerPubkeys {
		if followerPubKey == event.GetMasterPublicKey() {
			continue
		}

		validDevices := pm.collectValidDevices(followerPubKey, NotificationTypePost, event)

		followerBatch := pm.addNotificationsToDevices(validDevices, title, body, imageURL, data)

		if followerBatch != nil {
			notifications.singleNotifications = append(notifications.singleNotifications, followerBatch.singleNotifications...)
			notifications.multicastNotifications = append(notifications.multicastNotifications, followerBatch.multicastNotifications...)
		}
	}

	return notifications
}
