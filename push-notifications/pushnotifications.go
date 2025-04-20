// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/cfg"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
	"github.com/ice-blockchain/subzero/validation"
)

type (
	DeviceID         = pn.DeviceID
	PublicKey        = string
	NotificationType string
	Language         string

	NotificationKind struct {
		Title       string
		Description string
		ImageURL    string
	}
	DeviceInfo struct {
		DeviceID                  DeviceID
		Platform                  string
		RelayURL                  string
		FCMToken                  string
		Filters                   nostr.Filters
		PubKey                    PublicKey
		Invalid                   bool
		DeviceRegistrationEventID string
	}

	PushNotificationManager struct {
		devices                map[DeviceID]DeviceInfo
		userDevices            map[PublicKey][]DeviceID
		filterToDevices        map[NotificationType]map[DeviceID]bool
		deviceMutex            sync.RWMutex
		pushNotificationClient *pn.Client
	}

	config struct {
		FCMCredentialsPath string `yaml:"fcm-credentials-path" validate:"required"`
	}
)

const (
	NotificationTypePost             NotificationType = "post"
	NotificationTypeReaction         NotificationType = "reaction"
	NotificationTypeRepost           NotificationType = "repost"
	NotificationTypeMention          NotificationType = "mention"
	NotificationTypeReply            NotificationType = "reply"
	NotificationTypeDirectMessage    NotificationType = "direct_message"
	NotificationTypeGroupChatMessage NotificationType = "group_chat_message"
	NotificationTypeChannelMessage   NotificationType = "channel_message"
	NotificationTypePaymentRequest   NotificationType = "payment_request"
	NotificationTypePaymentReceived  NotificationType = "payment_received"
	NotificationTypeSystem           NotificationType = "system"
	NotificationTypeNewFollower      NotificationType = "new_follower"
)

var DefaultTranslations = map[NotificationType]NotificationKind{
	NotificationTypePost: {
		Title:       "New post",
		Description: "You have a new post",
		ImageURL:    "",
	},
	NotificationTypeReaction: {
		Title:       "New reaction",
		Description: "Someone reacted to your post",
		ImageURL:    "",
	},
	NotificationTypeRepost: {
		Title:       "New repost",
		Description: "Someone reposted your post",
		ImageURL:    "",
	},
	NotificationTypeMention: {
		Title:       "New mention",
		Description: "Someone mentioned you",
		ImageURL:    "",
	},
	NotificationTypeReply: {
		Title:       "New reply",
		Description: "Someone replied to your post",
		ImageURL:    "",
	},
	NotificationTypeDirectMessage: {
		Title:       "New message",
		Description: "You have a new message",
		ImageURL:    "",
	},
	NotificationTypeGroupChatMessage: {
		Title:       "New group message",
		Description: "New message in group",
		ImageURL:    "",
	},
	NotificationTypeChannelMessage: {
		Title:       "New channel message",
		Description: "New message in channel",
		ImageURL:    "",
	},
	NotificationTypePaymentRequest: {
		Title:       "Payment request",
		Description: "Someone requested a payment",
		ImageURL:    "",
	},
	NotificationTypePaymentReceived: {
		Title:       "Payment received",
		Description: "You received a payment",
		ImageURL:    "",
	},
	NotificationTypeSystem: {
		Title:       "System notification",
		Description: "System notification",
		ImageURL:    "",
	},
	NotificationTypeNewFollower: {
		Title:       "New follower",
		Description: "Someone is now following you",
		ImageURL:    "",
	},
}

func NewPushNotificationManager() *PushNotificationManager {
	devices := make(map[DeviceID]DeviceInfo)
	userDevices := make(map[PublicKey][]DeviceID)
	filterToDevices := make(map[NotificationType]map[DeviceID]bool)

	filterToDevices[NotificationTypePost] = make(map[DeviceID]bool)
	filterToDevices[NotificationTypeMention] = make(map[DeviceID]bool)
	filterToDevices[NotificationTypeReply] = make(map[DeviceID]bool)
	filterToDevices[NotificationTypeReaction] = make(map[DeviceID]bool)
	filterToDevices[NotificationTypeRepost] = make(map[DeviceID]bool)
	filterToDevices[NotificationTypeNewFollower] = make(map[DeviceID]bool)
	filterToDevices[NotificationTypeDirectMessage] = make(map[DeviceID]bool)
	filterToDevices[NotificationTypeGroupChatMessage] = make(map[DeviceID]bool)
	filterToDevices[NotificationTypeChannelMessage] = make(map[DeviceID]bool)
	filterToDevices[NotificationTypePaymentRequest] = make(map[DeviceID]bool)
	filterToDevices[NotificationTypePaymentReceived] = make(map[DeviceID]bool)
	filterToDevices[NotificationTypeSystem] = make(map[DeviceID]bool)

	var pnClient pn.Client
	var err error

	cfg := cfg.MustGet[config]()

	if cfg.FCMCredentialsPath == "" {
		panic("FCM credentials file path is empty")
	}

	if _, err := os.Stat(cfg.FCMCredentialsPath); err != nil {
		panic("FCM credentials file not found, push notifications will be disabled")
	}
	pnClient, err = pn.New(context.Background(), cfg.FCMCredentialsPath, pn.WithDryRun(false))
	if err != nil {
		panic("Failed to create push notification client")
	}

	pm := &PushNotificationManager{
		devices:                devices,
		userDevices:            userDevices,
		filterToDevices:        filterToDevices,
		pushNotificationClient: &pnClient,
	}

	if err := pm.fullSyncDevices(context.Background()); err != nil {
		log.Printf("Error performing full device synchronization at startup: %v", err)
	}

	return pm
}

func (pm *PushNotificationManager) Notify(ctx context.Context, events []*model.Event) error {
	if len(events) == 0 {
		return nil
	}

	singleNotifications := make([]*pn.Notification[pn.DeviceToken], 0)
	topicNotifications := make([]*pn.Notification[pn.SubscriptionTopic], 0)

	eventsByKind := make(map[int][]*model.Event)
	for _, event := range events {
		eventsByKind[event.Kind] = append(eventsByKind[event.Kind], event)
	}

	for kind, kindEvents := range eventsByKind {
		notifications := pm.processEventsByKind(ctx, kind, kindEvents)

		for _, notification := range notifications {
			switch n := notification.(type) {
			case *pn.Notification[pn.DeviceToken]:
				singleNotifications = append(singleNotifications, n)
			case *pn.Notification[pn.SubscriptionTopic]:
				topicNotifications = append(topicNotifications, n)
			}
		}
	}

	return pm.sendNotifications(ctx, singleNotifications, topicNotifications)
}

func (pm *PushNotificationManager) collectValidDevices(pubKey PublicKey, notificationType NotificationType, event *model.Event) (iosDevices, otherDevices []struct {
	deviceID DeviceID
	token    string
}) {
	pm.deviceMutex.RLock()
	deviceIDs, ok := pm.userDevices[pubKey]
	pm.deviceMutex.RUnlock()

	if !ok || len(deviceIDs) == 0 {
		return nil, nil
	}

	pm.deviceMutex.RLock()
	defer pm.deviceMutex.RUnlock()

	for _, deviceID := range deviceIDs {
		deviceInfo, exists := pm.devices[deviceID]
		if !exists {
			continue
		}
		if deviceInfo.Invalid {
			continue
		}
		if _, hasFilter := pm.filterToDevices[notificationType][deviceID]; !hasFilter {
			continue
		}

		if deviceInfo.Filters.Match(&event.Event) {
			if deviceInfo.Platform == validation.DeviceTokenOSIOS {
				iosDevices = append(iosDevices, struct {
					deviceID DeviceID
					token    string
				}{
					deviceID: deviceID,
					token:    deviceInfo.FCMToken,
				})
			} else {
				otherDevices = append(otherDevices, struct {
					deviceID DeviceID
					token    string
				}{
					deviceID: deviceID,
					token:    deviceInfo.FCMToken,
				})
			}
		}
	}

	return iosDevices, otherDevices
}

func (pm *PushNotificationManager) addNotificationsToDevices(devices []struct {
	deviceID DeviceID
	token    string
}, title, body, imageURL string, data map[string]interface{}) []*pn.Notification[pn.DeviceToken] {
	if len(devices) == 0 {
		return nil
	}
	notifications := make([]*pn.Notification[pn.DeviceToken], 0)
	for _, device := range devices {
		pm.deviceMutex.RLock()
		deviceInfo, exists := pm.devices[device.deviceID]
		pm.deviceMutex.RUnlock()

		var deviceRegistrationEventID string
		if exists {
			deviceRegistrationEventID = deviceInfo.DeviceRegistrationEventID
		}

		notifications = append(notifications, &pn.Notification[pn.DeviceToken]{
			Title:    title,
			Body:     body,
			Data:     data,
			ImageURL: imageURL,
			Target: pn.DeviceToken{
				Token:                     device.token,
				DeviceID:                  device.deviceID,
				DeviceRegistrationEventID: deviceRegistrationEventID,
			},
		})
	}

	return notifications
}

func (pm *PushNotificationManager) processEventsByKind(ctx context.Context, kind int, events []*model.Event) []interface{} {
	var allNotifications []interface{}

	for _, event := range events {
		var notifications interface{}

		switch kind {
		case nostr.KindTextNote:
			notifications = pm.handlePostNotification(ctx, event)
		case nostr.KindReaction:
			notifications = pm.handleReactionNotification(event)
		case nostr.KindGiftWrap:
			notifications = pm.handleDirectMessageNotification(event)
		case nostr.KindRepost, nostr.KindGenericRepost:
			notifications = pm.handleRepostNotification(event)
		case model.CustomIONKindFundSendNotify:
			notifications = pm.handlePaymentRequestNotification(event)
		case model.CustomIONKindFundReceive:
			notifications = pm.handlePaymentReceivedNotification(event)
		case nostr.KindFollowList:
			notifications = pm.handleNewFollowerNotification(event)
		case nostr.KindChannelMessage:
			notifications = pm.handleChannelMessageNotification(event)
		case nostr.KindSimpleGroupChatMessage:
			notifications = pm.handleGroupChatMessageNotification(event)
		case model.CustomIONSystemMessage:
			notifications = pm.handleSystemNotification(event)
		}

		if notifications != nil {
			switch n := notifications.(type) {
			case []*pn.Notification[pn.DeviceToken]:
				for _, notification := range n {
					allNotifications = append(allNotifications, notification)
				}
			case []*pn.Notification[pn.SubscriptionTopic]:
				for _, notification := range n {
					allNotifications = append(allNotifications, notification)
				}
			}
		}
	}

	return allNotifications
}

func (pm *PushNotificationManager) sendNotifications(ctx context.Context,
	singleNotifications []*pn.Notification[pn.DeviceToken],
	topicNotifications []*pn.Notification[pn.SubscriptionTopic]) error {

	totalCount := len(singleNotifications) + len(topicNotifications)
	if totalCount == 0 {
		return nil
	}

	errChan := make(chan error, totalCount)

	for _, notification := range singleNotifications {
		go func(n *pn.Notification[pn.DeviceToken]) {
			err := (*pm.pushNotificationClient).SendSingle(ctx, n)
			if err != nil && pn.IsInvalidDeviceToken(err) {
				pm.handleInvalidDeviceToken(ctx, n.Target.DeviceID, errChan)
			} else {
				errChan <- err
			}
		}(notification)
	}

	for _, notification := range topicNotifications {
		go func(n *pn.Notification[pn.SubscriptionTopic]) {
			errChan <- (*pm.pushNotificationClient).SendTopic(ctx, n)
		}(notification)
	}

	var errors []error
	for i := 0; i < totalCount; i++ {
		if err := <-errChan; err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to send notifications: %v", errors)
	}

	return nil
}

func (pm *PushNotificationManager) handleInvalidDeviceToken(ctx context.Context, deviceID pn.DeviceID, errChan chan<- error) {
	pm.deviceMutex.RLock()
	deviceInfo, exists := pm.devices[deviceID]
	pm.deviceMutex.RUnlock()

	if exists {
		deviceRegistrationEventID := deviceInfo.DeviceRegistrationEventID

		if deviceRegistrationEventID == "" {
			errChan <- fmt.Errorf("cannot mark token as invalid: missing EventID for device %s", deviceID)

			return
		}

		if err := query.MarkTokenAsInvalid(ctx, deviceRegistrationEventID); err != nil {
			errChan <- fmt.Errorf("error marking token as invalid: %w", err)

			return
		}

		pm.deviceMutex.Lock()
		deviceInfo.Invalid = true
		pm.devices[deviceID] = deviceInfo
		pm.deviceMutex.Unlock()
	}

	errChan <- nil
}

func (pm *PushNotificationManager) createAndSendNotifications(
	iosDevices, otherDevices []struct {
		deviceID DeviceID
		token    string
	},
	notificationType NotificationType,
	data map[string]interface{},
) []*pn.Notification[pn.DeviceToken] {
	notifications := make([]*pn.Notification[pn.DeviceToken], 0)
	if len(iosDevices) > 0 {
		title := getDefaultTranslation(notificationType, "title")
		body := getDefaultTranslation(notificationType, "body")
		imageURL := DefaultTranslations[notificationType].ImageURL

		notifications = append(notifications, pm.addNotificationsToDevices(iosDevices, title, body, imageURL, data)...)
	}
	if len(otherDevices) > 0 {
		notifications = append(notifications, pm.addNotificationsToDevices(otherDevices, "", "", "", data)...)
	}

	return notifications
}

func getDefaultTranslation(notificationType NotificationType, key string) string {
	if defaultTranslation, exists := DefaultTranslations[notificationType]; exists {
		var result string
		switch key {
		case "title":
			result = defaultTranslation.Title
		case "body":
			result = defaultTranslation.Description
		default:
			return key
		}

		return result
	}

	return key
}
