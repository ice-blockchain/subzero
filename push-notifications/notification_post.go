// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"context"
	"strings"

	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
)

func (pm *PushNotificationManager) handlePostNotification(ctx context.Context, event *model.Event) []*pn.Notification[pn.DeviceToken] {
	notifications := make([]*pn.Notification[pn.DeviceToken], 0)
	isReply, isMention, replyToPubkey, mentionedPubkeys := pm.classifyPostType(event)

	if isReply && replyToPubkey != "" && replyToPubkey != event.GetMasterPublicKey() {
		notifications = append(notifications, pm.handleReplyPost(event, replyToPubkey)...)
	}
	if isMention {
		notifications = append(notifications, pm.handleMentionPost(event, mentionedPubkeys)...)
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
				for _, ptag := range event.GetTags("p") {
					replyToPubkey = ptag.Value()

					break
				}
			} else if tag[3] == model.TagMarkerMention {
				isMention = true
				for _, ptag := range event.GetTags("p") {
					if ptag.Value() != "" {
						mentionedPubkeys = append(mentionedPubkeys, ptag.Value())
					}
				}
			}
		}
	}

	contentWords := strings.Fields(event.Content)
	for _, word := range contentWords {
		if strings.HasPrefix(word, "nprofile") {
			prefix, data, err := nip19.Decode(word)
			if err == nil && prefix == "nprofile" {
				if profileData, ok := data.(nostr.ProfilePointer); ok {
					mentionedPubkeys = append(mentionedPubkeys, profileData.PublicKey)
					isMention = true
				} else if profileMap, ok := data.(map[string]interface{}); ok {
					if pubkey, ok := profileMap["pubkey"].(string); ok {
						mentionedPubkeys = append(mentionedPubkeys, pubkey)
						isMention = true
					}
				}
			}
		}
	}

	return
}

func (pm *PushNotificationManager) handleReplyPost(event *model.Event, replyToPubkey string) []*pn.Notification[pn.DeviceToken] {
	iosDevices, otherDevices := pm.collectValidDevices(replyToPubkey, NotificationTypeReply, event)

	data := map[string]interface{}{
		"eventID":          event.ID,
		"authorPubKey":     event.GetMasterPublicKey(),
		"notificationType": string(NotificationTypeReply),
		"content":          event.Content,
	}

	return pm.createAndSendNotifications(iosDevices, otherDevices, NotificationTypeReply, data)
}

func (pm *PushNotificationManager) handleMentionPost(event *model.Event, mentionedPubkeys []string) []*pn.Notification[pn.DeviceToken] {
	notifications := make([]*pn.Notification[pn.DeviceToken], 0)

	if len(mentionedPubkeys) == 0 {
		return nil
	}

	for _, pubkey := range mentionedPubkeys {
		if pubkey == event.GetMasterPublicKey() {
			continue
		}

		iosDevices, otherDevices := pm.collectValidDevices(pubkey, NotificationTypeMention, event)

		data := map[string]interface{}{
			"eventId":          event.ID,
			"authorPubKey":     event.GetMasterPublicKey(),
			"notificationType": string(NotificationTypeMention),
			"content":          event.Content,
		}

		mentions := pm.createAndSendNotifications(iosDevices, otherDevices, NotificationTypeMention, data)
		notifications = append(notifications, mentions...)
	}

	return notifications
}
