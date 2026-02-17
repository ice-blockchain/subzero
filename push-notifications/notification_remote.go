// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"context"

	"github.com/ice-blockchain/subzero/model"
	"github.com/nbd-wtf/go-nostr"
)

const (
	NotificationTypeRemotePost             NotificationType = "remote_post"
	NotificationTypeRemoteVideo            NotificationType = "remote_video"
	NotificationTypeRemoteArticle          NotificationType = "remote_article"
	NotificationTypeRemoteStory            NotificationType = "remote_story"
	NotificationTypeRemoteNewCreatorToken  NotificationType = "remote_new_creator_token"
	NotificationTypeRemoteNewContentToken  NotificationType = "remote_new_content_token"
	NotificationTypeRemoteSwapCreatorToken NotificationType = "remote_swap_creator_token"
	NotificationTypeRemoteSwapContentToken NotificationType = "remote_swap_content_token"
)

var (
	defaultRemoteTranslations = map[NotificationType]notificationTranslation{
		NotificationTypeRemotePost: {
			Title:    "New post added",
			Body:     "New post added from someone you activated account notifications for",
			ImageURL: "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png",
		},
		NotificationTypeRemoteVideo: {
			Title:    "New video added",
			Body:     "New video added from someone you activated account notifications for",
			ImageURL: "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png",
		},
		NotificationTypeRemoteArticle: {
			Title:    "New article is out",
			Body:     "New article added from someone you activated account notifications for",
			ImageURL: "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png",
		},
		NotificationTypeRemoteStory: {
			Title:    "Quick update",
			Body:     "New story added from someone you activated account notifications for",
			ImageURL: "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png",
		},
		NotificationTypeRemoteNewCreatorToken: {
			Title:    "New Creator Token",
			Body:     "Someone Else Creator Token Created",
			ImageURL: "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png",
		},
		NotificationTypeRemoteNewContentToken: {
			Title:    "New Content Token",
			Body:     "Someone Else Content Token Created",
			ImageURL: "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png",
		},
		NotificationTypeRemoteSwapCreatorToken: {
			Title:    "New Buy",
			Body:     "Someone Bought Another Persons Creator Token",
			ImageURL: "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png",
		},
		NotificationTypeRemoteSwapContentToken: {
			Title:    "New Buy",
			Body:     "Someone Bought Another Persons Content Token",
			ImageURL: "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png",
		},
	}
)

func (pm *PushNotificationManager) getRemoteNotificationType(event *model.Event, relevantEvents ...*model.Event) NotificationType {
	switch event.Kind {
	case nostr.KindArticle:
		return NotificationTypeRemoteArticle

	case nostr.KindTextNote, model.CustomIONKindEditableTextNote:
		if event.GetTag("expiration").Value() != "" {
			return NotificationTypeRemoteStory
		}
		return NotificationTypeRemotePost

	case model.CustomIONKindTokenizedCommunityDefinition:
		isCreator := event.GetTag("k").Value() == "0"
		var isNew bool
		for _, tag := range event.Tags {
			if tag.Key() == "t" && tag.Value() == "community_token_action" {
				isNew = true
				break
			}
		}
		if isCreator && isNew {
			return NotificationTypeRemoteNewCreatorToken
		} else if !isCreator && isNew {
			return NotificationTypeRemoteNewContentToken
		}

	case model.CustomIONKindTokenizedCommunityAction:
		isCreator := event.GetTag("k").Value() == "0"
		if isCreator {
			return NotificationTypeRemoteSwapCreatorToken
		} else {
			return NotificationTypeRemoteSwapContentToken
		}

	}
	return ""
}

func (pm *PushNotificationManager) handleRemoteEvent(ctx context.Context, targetUserMasterKey string, event *model.Event, relevantEvents ...*model.Event) (*notificationTargets, error) {
	notificationType := pm.getRemoteNotificationType(event, relevantEvents...)
	if notificationType == "" {
		return nil, nil
	}

	localDevices, _ := pm.collectNotificationDevices(targetUserMasterKey, event)
	return pm.createNotifications(localDevices, nil, notificationType, event, relevantEvents...)
}
