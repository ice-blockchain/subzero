// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/cfg"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
	"github.com/ice-blockchain/subzero/validation"
)

type (
	PublicKey        = string
	NotificationType string
	Language         string

	NotificationKind struct {
		Title    string
		Body     string
		ImageURL string
	}

	PushNotificationManager struct {
		devices                map[DeviceID]DeviceInfo
		userDevices            map[PublicKey][]DeviceID
		deviceMutex            sync.RWMutex
		pushNotificationClient *pn.Client
	}

	config struct {
		FCMCredentialsPath string `yaml:"fcm-credentials-path" validate:"required"`
	}
	notificationCollections struct {
		single []*pn.Notification[*model.Event]
		topic  []*pn.Notification[pn.SubscriptionTopic]
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

var (
	globalPushNotificationManager *PushNotificationManager
	DefaultTranslations           = map[NotificationType]NotificationKind{
		NotificationTypePost: {
			Title:    "New post",
			Body:     "You have a new post",
			ImageURL: "",
		},
		NotificationTypeReaction: {
			Title:    "New reaction",
			Body:     "Someone reacted to your post",
			ImageURL: "",
		},
		NotificationTypeRepost: {
			Title:    "New repost",
			Body:     "Someone reposted your post",
			ImageURL: "",
		},
		NotificationTypeMention: {
			Title:    "New mention",
			Body:     "Someone mentioned you",
			ImageURL: "",
		},
		NotificationTypeReply: {
			Title:    "New reply",
			Body:     "Someone replied to your post",
			ImageURL: "",
		},
		NotificationTypeDirectMessage: {
			Title:    "New message",
			Body:     "You have a new message",
			ImageURL: "",
		},
		NotificationTypeGroupChatMessage: {
			Title:    "New group message",
			Body:     "New message in group",
			ImageURL: "",
		},
		NotificationTypeChannelMessage: {
			Title:    "New channel message",
			Body:     "New message in channel",
			ImageURL: "",
		},
		NotificationTypePaymentRequest: {
			Title:    "Payment request",
			Body:     "Someone requested a payment",
			ImageURL: "",
		},
		NotificationTypePaymentReceived: {
			Title:    "Payment received",
			Body:     "You received a payment",
			ImageURL: "",
		},
		NotificationTypeSystem: {
			Title:    "System notification",
			Body:     "System notification",
			ImageURL: "",
		},
		NotificationTypeNewFollower: {
			Title:    "New follower",
			Body:     "Someone is now following you",
			ImageURL: "",
		},
	}
)

func MustInit() {
	devices := make(map[DeviceID]DeviceInfo)
	userDevices := make(map[PublicKey][]DeviceID)

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

	globalPushNotificationManager = &PushNotificationManager{
		devices:                devices,
		userDevices:            userDevices,
		pushNotificationClient: &pnClient,
	}

	if err := globalPushNotificationManager.syncDevices(context.Background()); err != nil {
		panic(errors.Wrap(err, "failed to perform full device synchronization at startup"))
	}
}

func AcceptEvents(ctx context.Context, events []*model.Event) error {
	if err := globalPushNotificationManager.ProcessDeviceRegistrationEvents(ctx, events); err != nil {
		return err
	}

	return globalPushNotificationManager.AcceptEvents(ctx, events)
}

func (pm *PushNotificationManager) AcceptEvents(ctx context.Context, events []*model.Event) error {
	if len(events) == 0 {
		return nil
	}

	notifications := pm.collectNotifications(ctx, events)

	return pm.sendNotifications(ctx, notifications.single, notifications.topic)
}

func (pm *PushNotificationManager) collectNotifications(ctx context.Context, events []*model.Event) notificationCollections {
	result := notificationCollections{
		single: make([]*pn.Notification[*model.Event], 0),
		topic:  make([]*pn.Notification[pn.SubscriptionTopic], 0),
	}

	eventsByKind := groupEventsByKind(events)

	for kind, kindEvents := range eventsByKind {
		notifications := pm.processEventsByKind(ctx, kind, kindEvents)
		result.single = append(result.single, notifications.single...)
		result.topic = append(result.topic, notifications.topic...)
	}

	return result
}

func groupEventsByKind(events []*model.Event) map[int][]*model.Event {
	result := make(map[int][]*model.Event)
	for _, event := range events {
		result[event.Kind] = append(result[event.Kind], event)
	}

	return result
}

func (pm *PushNotificationManager) processEventsByKind(ctx context.Context, kind int, events []*model.Event) notificationCollections {
	result := notificationCollections{
		single: make([]*pn.Notification[*model.Event], 0),
		topic:  make([]*pn.Notification[pn.SubscriptionTopic], 0),
	}

	for _, event := range events {
		if event.Kind == model.CustomIONSystemMessage {
			if notifications := pm.handleSystemNotification(event); notifications != nil {
				result.topic = append(result.topic, notifications...)
			}
		}
		if notifications := pm.processEvent(ctx, kind, event); notifications != nil {
			result.single = append(result.single, notifications...)
		}
	}

	return result
}

func (pm *PushNotificationManager) processEvent(ctx context.Context, kind int, event *model.Event) []*pn.Notification[*model.Event] {
	switch kind {
	case nostr.KindTextNote, model.CustomIONKindEditableTextNote, nostr.KindRepost, nostr.KindGenericRepost:
		return pm.processTextOrRepostEvent(ctx, kind, event)
	case nostr.KindGiftWrap:
		return pm.processGiftWrapEvent(event)
	case nostr.KindFollowList:
		return pm.handleNewFollowerNotification(event)
	}

	return nil
}

func (pm *PushNotificationManager) processTextOrRepostEvent(ctx context.Context, kind int, event *model.Event) []*pn.Notification[*model.Event] {
	hTag := event.GetHTag()
	if hTag != "" && hTag != event.ID {
		return pm.handleCommunityMessageNotification(event)
	}

	if kind == nostr.KindRepost || kind == nostr.KindGenericRepost {
		return pm.handleRepostNotification(event)
	}

	return pm.handlePostNotification(ctx, event)
}

func (pm *PushNotificationManager) processGiftWrapEvent(event *model.Event) []*pn.Notification[*model.Event] {
	kTag := event.GetTag("k")
	if kTag == nil {
		return nil
	}

	kind, err := strconv.Atoi(kTag.Value())
	if err != nil {
		log.Printf("failed to convert k tag to int: %v", err)
		return nil
	}

	switch kind {
	case model.CustomIONKindFundReceive, model.CustomIONKindFundSendNotify:
		return pm.handlePaymentNotification(event)
	case nostr.KindDirectMessage, model.CustomIONDirectMessage:
		return pm.handleDirectMessageNotification(event)
	case nostr.KindReaction:
		return pm.handleReactionNotification(event)
	default:
		return nil
	}
}

func (pm *PushNotificationManager) sendNotifications(ctx context.Context,
	singleNotifications []*pn.Notification[*model.Event],
	topicNotifications []*pn.Notification[pn.SubscriptionTopic]) error {

	totalCount := len(singleNotifications) + len(topicNotifications)
	if totalCount == 0 {
		return nil
	}

	errChan := make(chan error, totalCount)
	invalidDevices := pm.sendNotificationsAsync(ctx, singleNotifications, topicNotifications, errChan)

	return pm.collectErrorsAndProcessInvalidDevices(ctx, totalCount, errChan, invalidDevices)
}

func (pm *PushNotificationManager) sendNotificationsAsync(
	ctx context.Context,
	singleNotifications []*pn.Notification[*model.Event],
	topicNotifications []*pn.Notification[pn.SubscriptionTopic],
	errChan chan error) []*model.Event {

	var invalidDevicesMutex sync.Mutex
	invalidDevices := make([]*model.Event, 0)

	for _, notification := range singleNotifications {
		go func(n *pn.Notification[*model.Event]) {
			err := (*pm.pushNotificationClient).SendSingle(ctx, n)
			if err != nil && pn.IsInvalidDeviceToken(err) {
				invalidDevicesMutex.Lock()
				invalidDevices = append(invalidDevices, n.Target)
				invalidDevicesMutex.Unlock()
				errChan <- nil
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

	return invalidDevices
}

func (pm *PushNotificationManager) collectErrorsAndProcessInvalidDevices(ctx context.Context, totalCount int, errChan chan error, invalidDevices []*model.Event) error {
	var errors []error
	for i := 0; i < totalCount; i++ {
		if err := <-errChan; err != nil {
			errors = append(errors, err)
		}
	}

	if len(invalidDevices) > 0 {
		if err := pm.handleInvalidDeviceTokens(ctx, invalidDevices); err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to send notifications: %v", errors)
	}

	return nil
}

func (pm *PushNotificationManager) handleInvalidDeviceTokens(ctx context.Context, deviceEvents []*model.Event) error {
	if len(deviceEvents) == 0 {
		return nil
	}

	if err := pm.markDevicesAsInvalidInCache(deviceEvents); err != nil {
		return err
	}

	return query.MarkTokenAsInvalidInEvents(ctx, deviceEvents)
}

func (pm *PushNotificationManager) markDevicesAsInvalidInCache(deviceEvents []*model.Event) error {
	for _, deviceEvent := range deviceEvents {
		deviceID := DeviceID(deviceEvent.Tags.GetD())
		if deviceID == "" {
			return errors.New("device ID not found in event tags")
		}

		pm.deviceMutex.Lock()
		deviceInfo, ok := pm.devices[deviceID]
		if ok {
			deviceInfo.Event.NotificationTokenInvalid = true
			pm.devices[deviceID] = deviceInfo
		}
		pm.deviceMutex.Unlock()
	}

	return nil
}

func (pm *PushNotificationManager) createNotifications(deviceEvents []*model.Event, notificationType NotificationType, data map[string]interface{}) []*pn.Notification[*model.Event] {
	if len(deviceEvents) == 0 {
		return nil
	}
	data["notificationType"] = string(notificationType)

	notifications := make([]*pn.Notification[*model.Event], 0)
	defaultTranslation := DefaultTranslations[notificationType]
	for _, deviceEvent := range deviceEvents {
		platform := deviceEvent.GetTag("t").Value()
		if platform == validation.DeviceTokenOSAndroid {
			data["title"] = defaultTranslation.Title
			data["body"] = defaultTranslation.Body
			data["imageURL"] = defaultTranslation.ImageURL
			notifications = append(notifications, &pn.Notification[*model.Event]{
				Target: deviceEvent,
				Data:   data,
			})
		} else {
			notifications = append(notifications, &pn.Notification[*model.Event]{
				Target:   deviceEvent,
				Title:    defaultTranslation.Title,
				Body:     defaultTranslation.Body,
				ImageURL: defaultTranslation.ImageURL,
				Data:     data,
			})
		}
	}

	return notifications
}

func (pm *PushNotificationManager) collectUserValidDevices(pubKey PublicKey, event *model.Event) (devices []*model.Event) {
	pm.deviceMutex.RLock()
	deviceIDs, ok := pm.userDevices[pubKey]
	pm.deviceMutex.RUnlock()

	if !ok || len(deviceIDs) == 0 {
		return nil
	}

	pm.deviceMutex.RLock()
	defer pm.deviceMutex.RUnlock()

	for _, deviceID := range deviceIDs {
		deviceInfo, exists := pm.devices[deviceID]
		if !exists {
			continue
		}
		if deviceInfo.Event.NotificationTokenInvalid {
			continue
		}
		if deviceInfo.Filters == nil || deviceInfo.Filters.Match(&event.Event) {
			devices = append(devices, deviceInfo.Event)
		}
	}

	return devices
}
