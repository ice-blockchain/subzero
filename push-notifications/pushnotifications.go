// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"context"
	"fmt"
	"os"
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
	DeviceRegistrationEvent = pn.DeviceRegistrationEvent
	PublicKey               = string
	NotificationType        string

	NotificationKind struct {
		Title    string
		Body     string
		ImageURL string
	}
	PushNotificationManager struct {
		userDevicesMap         map[PublicKey]map[DeviceID]DeviceInfo
		deviceMutex            sync.RWMutex
		pushNotificationClient *pn.Client
	}

	config struct {
		FCMCredentialsPath string `yaml:"fcm-credentials-path" validate:"required"`
	}
	notificationCollections struct {
		single []*pn.Notification[*DeviceRegistrationEvent]
		topic  []*pn.Notification[pn.SubscriptionTopic]
	}
)

const (
	NotificationTypeReaction         NotificationType = "reaction"
	NotificationTypeRepost           NotificationType = "repost"
	NotificationTypeMentionReply     NotificationType = "mention_reply"
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
		NotificationTypeMentionReply: {
			Title:    "New mention/reply",
			Body:     "Someone mentioned/replied you",
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
	userDevicesMap := make(map[PublicKey]map[DeviceID]DeviceInfo)

	var pnClient pn.Client
	var err error

	cfg := cfg.MustGet[config]()

	if cfg.FCMCredentialsPath == "" {
		panic("FCM credentials file path is empty")
	}

	if _, err := os.Stat(cfg.FCMCredentialsPath); err != nil {
		panic("FCM credentials file not found, push notifications will be disabled")
	}
	pnClient, err = pn.New(context.Background(), cfg.FCMCredentialsPath)
	if err != nil {
		panic("Failed to create push notification client")
	}

	globalPushNotificationManager = &PushNotificationManager{
		userDevicesMap:         userDevicesMap,
		pushNotificationClient: &pnClient,
	}

	if err := globalPushNotificationManager.syncDevices(context.Background()); err != nil {
		panic(errors.Wrap(err, "failed to perform full device synchronization at startup"))
	}
}

func AcceptEvents(ctx context.Context, events []*model.Event) error {
	if err := globalPushNotificationManager.ManageDeviceRegistrationEvents(ctx, events); err != nil {
		return err
	}

	return globalPushNotificationManager.AcceptEvents(ctx, events)
}

func (pm *PushNotificationManager) AcceptEvents(ctx context.Context, events []*model.Event) error {
	if len(events) == 0 {
		return nil
	}
	singleNotifications, topicNotifications, err := pm.collectNotifications(ctx, events)
	if err != nil {
		return errors.Wrap(err, "failed to collect notifications")
	}

	return errors.Wrap(pm.sendNotifications(ctx, singleNotifications, topicNotifications), "failed to send notifications")
}

func (pm *PushNotificationManager) collectNotifications(ctx context.Context, events []*model.Event) (
	singleNotifications []*pn.Notification[*DeviceRegistrationEvent],
	topicNotifications []*pn.Notification[pn.SubscriptionTopic],
	err error,
) {
	for _, event := range events {
		if event.Kind == model.CustomIONSystemMessage {
			if notifications := pm.handleSystemEvent(event); notifications != nil {
				topicNotifications = append(topicNotifications, notifications...)
			}
		}
		notifications, err := pm.processEvent(ctx, event.Kind, event)
		if err != nil {
			return nil, nil, errors.Wrap(err, "failed to process event")
		}
		if notifications != nil {
			singleNotifications = append(singleNotifications, notifications...)
		}
	}

	return singleNotifications, topicNotifications, nil
}

func (pm *PushNotificationManager) processEvent(ctx context.Context, kind int, event *model.Event) ([]*pn.Notification[*DeviceRegistrationEvent], error) {
	switch kind {
	case nostr.KindTextNote, model.CustomIONKindEditableTextNote, nostr.KindRepost, nostr.KindGenericRepost:
		hTag := event.GetHTag()
		if hTag != "" && hTag != event.ID {
			notifications, err := pm.handleCommunityMessageEvent(ctx, event)
			if err != nil {
				return nil, errors.Wrap(err, "failed to handle community message event")
			}

			return notifications, nil
		}
		if (kind == nostr.KindTextNote && event.GetTag("q") != nil) || (kind == model.CustomIONKindEditableTextNote && event.GetTag(model.CustomIONTagAddressableQ) != nil) ||
			kind == nostr.KindRepost || kind == nostr.KindGenericRepost {
			return pm.handleEventWithPublicKey(event), nil
		}

		return pm.handleMentionReplyEvent(ctx, event), nil
	case nostr.KindGiftWrap:
		notifications, err := pm.handleGiftWrapEvent(event)
		if err != nil {
			return nil, errors.Wrap(err, "failed to handle gift wrap event")
		}

		return notifications, nil
	case nostr.KindFollowList:
		return pm.handleNewFollowerEvent(ctx, event)
	}

	return nil, nil
}

func (pm *PushNotificationManager) sendNotifications(ctx context.Context,
	singleNotifications []*pn.Notification[*DeviceRegistrationEvent],
	topicNotifications []*pn.Notification[pn.SubscriptionTopic],
) error {
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
	singleNotifications []*pn.Notification[*DeviceRegistrationEvent],
	topicNotifications []*pn.Notification[pn.SubscriptionTopic],
	errChan chan error,
) []*DeviceRegistrationEvent {
	var invalidDevicesMutex sync.Mutex
	invalidDevices := make([]*model.Event, 0)

	for _, notification := range singleNotifications {
		go func(n *pn.Notification[*DeviceRegistrationEvent]) {
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

func (pm *PushNotificationManager) collectErrorsAndProcessInvalidDevices(ctx context.Context, totalCount int, errChan chan error, invalidDevices []*DeviceRegistrationEvent) error {
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

func (pm *PushNotificationManager) handleInvalidDeviceTokens(ctx context.Context, deviceEvents []*DeviceRegistrationEvent) error {
	if len(deviceEvents) == 0 {
		return nil
	}
	if err := pm.markDevicesAsInvalidInCache(deviceEvents); err != nil {
		return err
	}

	return query.MarkTokenAsInvalidInEventTags(ctx, deviceEvents)
}

func (pm *PushNotificationManager) markDevicesAsInvalidInCache(deviceEvents []*DeviceRegistrationEvent) error {
	for _, deviceEvent := range deviceEvents {
		deviceID := DeviceID(deviceEvent.Tags.GetD())

		pm.deviceMutex.Lock()
		deviceInfo, ok := pm.userDevicesMap[deviceEvent.GetMasterPublicKey()][deviceID]
		if ok {
			for i, tag := range deviceInfo.Event.Tags {
				if tag.Key() == "token" {
					if len(tag) <= 2 {
						deviceInfo.Event.Tags[i] = append(tag, "invalid")
					} else {
						deviceInfo.Event.Tags[i][2] = "invalid"
					}
					break
				}
			}
			pm.userDevicesMap[deviceEvent.GetMasterPublicKey()][deviceID] = deviceInfo
		}
		pm.deviceMutex.Unlock()
	}

	return nil
}

func (pm *PushNotificationManager) createNotifications(
	deviceRegistrationEvents []*DeviceRegistrationEvent,
	notificationType NotificationType,
	incomingEvent *model.Event,
) []*pn.Notification[*DeviceRegistrationEvent] {
	if len(deviceRegistrationEvents) == 0 {
		return nil
	}
	data := map[string]interface{}{
		"event": incomingEvent.String(),
	}

	notifications := make([]*pn.Notification[*DeviceRegistrationEvent], 0)
	defaultTranslation := DefaultTranslations[notificationType]
	for _, event := range deviceRegistrationEvents {
		switch event.GetTag("t").Value() {
		case validation.DeviceTokenOSAndroid:
			data["title"] = defaultTranslation.Title
			data["body"] = defaultTranslation.Body
			data["imageUrl"] = defaultTranslation.ImageURL
			notifications = append(notifications, &pn.Notification[*DeviceRegistrationEvent]{
				Target: event,
				Data:   data,
			})
		default:
			notifications = append(notifications, &pn.Notification[*DeviceRegistrationEvent]{
				Target:   event,
				Title:    defaultTranslation.Title,
				Body:     defaultTranslation.Body,
				ImageURL: defaultTranslation.ImageURL,
				Data:     data,
			})
		}
	}

	return notifications
}

func (pm *PushNotificationManager) collectUserValidDevices(pubKey PublicKey, event *model.Event) (devices []*DeviceRegistrationEvent) {
	pm.deviceMutex.RLock()
	userDevices, ok := pm.userDevicesMap[pubKey]
	pm.deviceMutex.RUnlock()
	if !ok || len(userDevices) == 0 {
		return nil
	}

	pm.deviceMutex.RLock()
	defer pm.deviceMutex.RUnlock()

	for _, deviceInfo := range userDevices {
		tokenTag := deviceInfo.Event.GetTag("token")
		isTokenInvalid := tokenTag != nil && len(tokenTag) > 2 && tokenTag[2] == "invalid"
		if isTokenInvalid {
			continue
		}
		if deviceInfo.Filters == nil || deviceInfo.Filters.Match(&event.Event) {
			devices = append(devices, deviceInfo.Event)
		}
	}

	return devices
}

func (pm *PushNotificationManager) handleEventWithPublicKey(event *model.Event) []*pn.Notification[*DeviceRegistrationEvent] {
	referencePubkey := event.GetTag("p").Value()
	if referencePubkey == "" || referencePubkey == event.GetMasterPublicKey() {
		return nil
	}
	deviceEvents := pm.collectUserValidDevices(referencePubkey, event)

	return pm.createNotifications(deviceEvents, NotificationTypeRepost, event)
}
