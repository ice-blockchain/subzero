// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"slices"
	"strconv"
	"strings"
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
		FCMCredentialsFile string   `yaml:"fcm-credentials-file"`
		FCMAndroidConfigs  []string `yaml:"fcm-android-configs"`
		FCMIOSConfigs      []string `yaml:"fcm-ios-configs"`
		FCMWebConfigs      []string `yaml:"fcm-web-configs"`
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
			Body:     "%v reacted to your post",
			ImageURL: "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png",
		},
		NotificationTypeRepost: {
			Title:    "New repost",
			Body:     "%v reposted your post",
			ImageURL: "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png",
		},
		NotificationTypeMentionReply: {
			Title:    "New mention/reply",
			Body:     "%v mentioned/replied you",
			ImageURL: "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png",
		},
		NotificationTypeDirectMessage: {
			Title:    "New message",
			Body:     "You have a new message",
			ImageURL: "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png",
		},
		NotificationTypeGroupChatMessage: {
			Title:    "New group message",
			Body:     "New message in group",
			ImageURL: "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png",
		},
		NotificationTypeChannelMessage: {
			Title:    "New channel message",
			Body:     "New message in channel",
			ImageURL: "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png",
		},
		NotificationTypePaymentRequest: {
			Title:    "Payment request",
			Body:     "Someone requested a payment",
			ImageURL: "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png",
		},
		NotificationTypePaymentReceived: {
			Title:    "Payment received",
			Body:     "You received a payment",
			ImageURL: "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png",
		},
		NotificationTypeSystem: {
			Title:    "System notification",
			Body:     "System notification",
			ImageURL: "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png",
		},
		NotificationTypeNewFollower: {
			Title:    "New follower",
			Body:     "%v is now following you",
			ImageURL: "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png",
		},
	}
)

func MustInit() {
	userDevicesMap := make(map[PublicKey]map[DeviceID]DeviceInfo)

	var pnClient pn.Client
	var err error

	cfg := cfg.MustGet[config]()

	if cfg.FCMCredentialsFile == "" {
		panic("FCM credentials not provided")
	}

	var opts []pn.Option
	if strings.HasPrefix(strings.TrimSpace(cfg.FCMCredentialsFile), "{") {
		opts = append(opts, pn.WithCredentialsJSON(cfg.FCMCredentialsFile))
	} else {
		if _, err := os.Stat(cfg.FCMCredentialsFile); err != nil {
			opts = append(opts, pn.WithCredentialsJSON(cfg.FCMCredentialsFile))
		} else {
			opts = append(opts, pn.WithCredentialsFile(cfg.FCMCredentialsFile))
		}
	}

	pnClient, err = pn.New(context.Background(), opts...)
	if err != nil {
		panic(fmt.Sprintf("Failed to create push notification client: %v", err))
	}

	globalPushNotificationManager = &PushNotificationManager{
		userDevicesMap:         userDevicesMap,
		pushNotificationClient: &pnClient,
	}

	if err := globalPushNotificationManager.syncDevices(context.Background()); err != nil {
		panic(errors.Wrap(err, "failed to perform full device synchronization at startup"))
	}
}

func GetFCMConfigs() (androidConfigs, iosConfigs, webConfigs []string) {
	config := cfg.MustGet[config]()

	return config.FCMAndroidConfigs, config.FCMIOSConfigs, config.FCMWebConfigs
}

func AcceptEvents(ctx context.Context, events []*model.Event) error {
	var errs error
	errs = errors.Join(errs,
		globalPushNotificationManager.AcceptEvents(ctx, events),
		globalPushNotificationManager.ManageDeviceRegistrationEvents(ctx, events),
	)
	if errs != nil {
		return errors.Wrap(errs, "failed to process events")
	}

	return nil
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
	ephemeralEvents, nonEphemeralEvents := pm.sortEphemeralEvents(events)

	for _, event := range nonEphemeralEvents {
		if event.Kind == model.CustomIONSystemMessage {
			if notifications := pm.handleSystemEvent(event); notifications != nil {
				topicNotifications = append(topicNotifications, notifications...)
			}
		}
		var relatedEvents []*model.Event
		if !shouldSkipEphemeralEvent(event) {
			if evs, ok := ephemeralEvents[event.ID]; ok {
				relatedEvents = evs
			}
		}

		notifications, err := pm.processEvent(ctx, event, relatedEvents...)
		if err != nil {
			return nil, nil, errors.Wrap(err, "failed to process event")
		}
		if notifications != nil {
			singleNotifications = append(singleNotifications, notifications...)
		}
	}

	return singleNotifications, topicNotifications, nil
}

func (pm *PushNotificationManager) sortEphemeralEvents(events []*model.Event) (map[string][]*model.Event, []*model.Event) {
	ephemeralEvents := make(map[string][]*model.Event)
	nonEphemeralEvents := make([]*model.Event, 0)

	for _, event := range events {
		if event.Kind == model.CustomIONKindEphemeralEmbeddding {
			var refID string
			if eTag := event.GetTag("e"); eTag != nil {
				refID = eTag.Value()
			} else if aTag := event.GetTag("a"); aTag != nil {
				var pubKey string
				parts := strings.Split(aTag.Value(), ":")
				if len(parts) >= 2 {
					pubKey = parts[1]
				}
				for _, e := range events {
					if e.Kind != model.CustomIONKindEphemeralEmbeddding && e.GetMasterPublicKey() == pubKey {
						refID = e.ID

						break
					}
				}
			}

			if refID != "" {
				if _, exists := ephemeralEvents[refID]; !exists {
					ephemeralEvents[refID] = make([]*model.Event, 0)
				}
				ephemeralEvents[refID] = append(ephemeralEvents[refID], event)
			}
		} else {
			nonEphemeralEvents = append(nonEphemeralEvents, event)
		}
	}

	return ephemeralEvents, nonEphemeralEvents
}

func shouldSkipEphemeralEvent(event *model.Event) bool {
	if event.Kind == nostr.KindGiftWrap {
		if kTag := event.GetTag("k"); kTag != nil {
			kindStr := kTag.Value()
			if kind, err := strconv.Atoi(kindStr); err == nil {
				return kind == nostr.KindDirectMessage ||
					kind == model.CustomIONDirectMessage ||
					kind == model.CustomIONKindFundReceive ||
					kind == model.CustomIONKindFundSendNotify
			}
		}

		return false
	}

	return event.Kind == model.CustomIONSystemMessage
}

func (pm *PushNotificationManager) processEvent(ctx context.Context, event *model.Event, relatedEvents ...*model.Event) ([]*pn.Notification[*DeviceRegistrationEvent], error) {
	if event.Kind == nostr.KindGenericRepost {
		shouldProcess, err := shouldProcessGenericRepostEvent(event)
		if err != nil {
			return nil, errors.Wrap(err, "failed to check if generic repost event should be processed")
		}
		if !shouldProcess {
			return nil, nil
		}
	}

	switch event.Kind {
	case nostr.KindTextNote, model.CustomIONKindEditableTextNote, nostr.KindGenericRepost:
		if hTag := event.GetHTag(); hTag != "" && hTag != event.ID {
			notifications, err := pm.handleCommunityMessageEvent(ctx, event, relatedEvents...)
			if err != nil {
				return nil, errors.Wrap(err, "failed to handle community message event")
			}

			return notifications, nil
		}
		if (event.Kind == nostr.KindTextNote && event.GetTag("q") != nil) || (event.Kind == model.CustomIONKindEditableTextNote && event.GetTag(model.CustomIONTagAddressableQ) != nil) ||
			event.Kind == nostr.KindGenericRepost {
			return pm.handleEventWithPublicKey(event, relatedEvents...), nil
		}

		return pm.handleMentionReplyEvent(event, relatedEvents...), nil
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

func shouldProcessGenericRepostEvent(event *model.Event) (bool, error) {
	var repostedEvent *model.Event
	if err := json.Unmarshal([]byte(event.Content), &repostedEvent); err != nil {
		return false, errors.Wrap(err, "failed to unmarshal repost event")
	}
	if repostedEvent.Kind != model.CustomIONKindEditableTextNote {
		return false, nil
	}

	return true, nil
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

	return errors.Wrap(pm.collectErrorsAndProcessInvalidDevices(ctx, totalCount, errChan, invalidDevices), "failed to collect errors and process invalid devices")
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
	if err := query.MarkTokenAsInvalidInEventTags(ctx, deviceEvents); err != nil {
		return errors.Wrap(err, "failed to mark devices as invalid on query level")
	}

	pm.removeInvalidTokenDevicesFromCache(deviceEvents)

	return nil
}

func (pm *PushNotificationManager) createNotifications(
	deviceRegistrationEvents []*DeviceRegistrationEvent,
	notificationType NotificationType,
	incomingEvent *model.Event,
	relatedEvents ...*model.Event,
) []*pn.Notification[*DeviceRegistrationEvent] {
	if len(deviceRegistrationEvents) == 0 {
		return nil
	}

	notifications := make([]*pn.Notification[*DeviceRegistrationEvent], 0)
	defaultTranslation := pm.getTranslationWithRelatedInfo(notificationType, relatedEvents...)

	for _, event := range deviceRegistrationEvents {
		data := map[string]interface{}{
			"event": incomingEvent.String(),
		}

		if len(relatedEvents) > 0 {
			relatedEventsStrings := make([]string, 0, len(relatedEvents))
			for _, relatedEvent := range relatedEvents {
				relatedEventsStrings = append(relatedEventsStrings, relatedEvent.String())
			}
			data["related_events"] = relatedEventsStrings
		}

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
		if deviceInfo.Filters == nil || deviceInfo.Filters.Match(&event.Event) {
			devices = append(devices, deviceInfo.Event)
		}
	}

	return devices
}

func (pm *PushNotificationManager) handleEventWithPublicKey(event *model.Event, relatedEvents ...*model.Event) []*pn.Notification[*DeviceRegistrationEvent] {
	referencePubkey := event.GetTag("p").Value()
	if referencePubkey == "" || referencePubkey == event.GetMasterPublicKey() {
		return nil
	}
	deviceEvents := pm.collectUserValidDevices(referencePubkey, event)

	return pm.createNotifications(deviceEvents, NotificationTypeRepost, event, relatedEvents...)
}

func (pm *PushNotificationManager) getTranslationWithRelatedInfo(notificationType NotificationType, relatedEvents ...*model.Event) NotificationKind {
	translation := DefaultTranslations[notificationType]
	if len(relatedEvents) == 0 {
		translation.Body = strings.Replace(translation.Body, "%v", "Someone", 1)

		return translation
	}
	profileMetadataIndex := slices.IndexFunc(relatedEvents, func(event *model.Event) bool {
		return event.Kind == nostr.KindProfileMetadata
	})
	if profileMetadataIndex != -1 {
		var profileData struct {
			Name        string `json:"name,omitempty"`
			DisplayName string `json:"display_name,omitempty"`
		}
		if err := json.Unmarshal([]byte(relatedEvents[profileMetadataIndex].Content), &profileData); err != nil {
			log.Printf("failed to unmarshal profile metadata: %v", err)

			return translation
		}
		if profileData.DisplayName != "" {
			translation.Body = fmt.Sprintf(translation.Body, "@"+profileData.DisplayName)
		} else if profileData.Name != "" {
			translation.Body = fmt.Sprintf(translation.Body, "@"+profileData.Name)
		} else {
			translation.Body = fmt.Sprintf(translation.Body, "Someone")
		}
	}

	return translation
}
