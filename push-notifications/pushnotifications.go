// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"slices"
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

	NotificationTranslation struct {
		Title    string
		Body     string
		ImageURL string
	}
	PushNotificationManager struct {
		userDevicesMap         map[PublicKey]map[DeviceID]DeviceInfo
		deviceMutex            sync.RWMutex
		pushNotificationClient *pn.Client
	}

	notificationTranslationFuncs struct {
		Title    func(events ...*model.Event) string
		Body     func(events ...*model.Event) string
		ImageURL func(events ...*model.Event) string
	}

	config struct {
		FCMCredentialsFile string   `yaml:"fcm-credentials-file"`
		FCMAndroidConfigs  []string `yaml:"fcm-android-configs"`
		FCMIOSConfigs      []string `yaml:"fcm-ios-configs"`
		FCMWebConfigs      []string `yaml:"fcm-web-configs"`
		PrivateKey         string   `yaml:"private-key"`
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

	CompressionMethodZlib = "zlib"
)

var (
	globalPushNotificationManager *PushNotificationManager
	DefaultTranslations           = map[NotificationType]notificationTranslationFuncs{
		NotificationTypeReaction: {
			Title: func(events ...*model.Event) string {
				return "New reaction"
			},
			Body: func(events ...*model.Event) string {
				return "Someone reacted to your post"
			},
			ImageURL: func(events ...*model.Event) string {
				return "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png"
			},
		},
		NotificationTypeRepost: {
			Title: func(events ...*model.Event) string {
				return "New repost"
			},
			Body: func(events ...*model.Event) string {
				return fmt.Sprintf("%s reposted your post", getDisplayNameFromRelevantEvents(events))
			},
			ImageURL: func(events ...*model.Event) string {
				return "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png"
			},
		},
		NotificationTypeMentionReply: {
			Title: func(events ...*model.Event) string {
				return "New mention/reply"
			},
			Body: func(events ...*model.Event) string {
				return fmt.Sprintf("%s mentioned/replied you", getDisplayNameFromRelevantEvents(events))
			},
			ImageURL: func(events ...*model.Event) string {
				return "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png"
			},
		},
		NotificationTypeDirectMessage: {
			Title: func(events ...*model.Event) string {
				return "New message"
			},
			Body: func(events ...*model.Event) string {
				return "You have a new message"
			},
			ImageURL: func(events ...*model.Event) string {
				return "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png"
			},
		},
		NotificationTypeGroupChatMessage: {
			Title: func(events ...*model.Event) string {
				return "New group message"
			},
			Body: func(events ...*model.Event) string {
				return "New message in group"
			},
			ImageURL: func(events ...*model.Event) string {
				return "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png"
			},
		},
		NotificationTypeChannelMessage: {
			Title: func(events ...*model.Event) string {
				return "New channel message"
			},
			Body: func(events ...*model.Event) string {
				return "New message in channel"
			},
			ImageURL: func(events ...*model.Event) string {
				return "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png"
			},
		},
		NotificationTypePaymentRequest: {
			Title: func(events ...*model.Event) string {
				return "Payment request"
			},
			Body: func(events ...*model.Event) string {
				return "Someone requested a payment"
			},
			ImageURL: func(events ...*model.Event) string {
				return "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png"
			},
		},
		NotificationTypePaymentReceived: {
			Title: func(events ...*model.Event) string {
				return "Payment received"
			},
			Body: func(events ...*model.Event) string {
				return "You received a payment"
			},
			ImageURL: func(events ...*model.Event) string {
				return "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png"
			},
		},
		NotificationTypeSystem: {
			Title: func(events ...*model.Event) string {
				return "System notification"
			},
			Body: func(events ...*model.Event) string {
				return "System notification"
			},
			ImageURL: func(events ...*model.Event) string {
				return "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png"
			},
		},
		NotificationTypeNewFollower: {
			Title: func(events ...*model.Event) string {
				return "New follower"
			},
			Body: func(events ...*model.Event) string {
				return fmt.Sprintf("%s is now following you", getDisplayNameFromRelevantEvents(events))
			},
			ImageURL: func(events ...*model.Event) string {
				return "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png"
			},
		},
	}
)

func MustInit() {
	userDevicesMap := make(map[PublicKey]map[DeviceID]DeviceInfo)

	var pnClient pn.Client
	var err error

	config := cfg.MustGet[config]()

	if config.FCMCredentialsFile == "" {
		panic("FCM credentials not provided")
	}
	if config.PrivateKey == "" {
		panic("private key is empty")
	}
	var opts []pn.Option
	if strings.HasPrefix(strings.TrimSpace(config.FCMCredentialsFile), "{") {
		opts = append(opts, pn.WithCredentialsJSON(config.FCMCredentialsFile))
	} else {
		if _, err := os.Stat(config.FCMCredentialsFile); err != nil {
			opts = append(opts, pn.WithCredentialsJSON(config.FCMCredentialsFile))
		} else {
			opts = append(opts, pn.WithCredentialsFile(config.FCMCredentialsFile))
		}
	}
	opts = append(opts, pn.WithPrivateKey(config.PrivateKey))

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
		var relevantEvents []*model.Event
		if !shouldSkipEphemeralEvent(event) {
			evs, ok := ephemeralEvents[event.ID]
			if !ok || len(evs) == 0 {
				continue
			}
			relevantEvents = evs
		}
		notifications, err := pm.processEvent(ctx, event, relevantEvents...)
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
	return event.Kind == nostr.KindGiftWrap || event.Kind == model.CustomIONSystemMessage
}

func (pm *PushNotificationManager) processEvent(ctx context.Context, event *model.Event, relevantEvents ...*model.Event) ([]*pn.Notification[*DeviceRegistrationEvent], error) {
	var notifications []*pn.Notification[*DeviceRegistrationEvent]
	var err error

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
			notifications, err = pm.handleCommunityMessageEvent(ctx, event, relevantEvents...)
			err = errors.Wrap(err, "failed to handle community message event")
		} else if (event.Kind == nostr.KindTextNote && event.GetTag("q") != nil) || (event.Kind == model.CustomIONKindEditableTextNote && event.GetTag(model.CustomIONTagAddressableQ) != nil) {
			notifications, err = pm.handleQuoteEvent(event, relevantEvents...)
			err = errors.Wrap(err, "failed to handle quote event")
		} else if event.Kind == nostr.KindGenericRepost {
			notifications, err = pm.handleEventWithPublicKey(event, NotificationTypeRepost, relevantEvents...)
			err = errors.Wrap(err, "failed to handle event for generic repost")
		} else {
			notifications, err = pm.handleMentionReplyEvent(event, relevantEvents...)
			err = errors.Wrap(err, "failed to handle mention reply/mention event")
		}
	case nostr.KindReaction:
		notifications, err = pm.handleEventWithPublicKey(event, NotificationTypeReaction, relevantEvents...)
		err = errors.Wrap(err, "failed to handle event for reaction")
	case nostr.KindGiftWrap:
		notifications, err = pm.handleGiftWrapEvent(event)
		err = errors.Wrap(err, "failed to handle gift wrap event")
	case nostr.KindFollowList:
		return pm.handleNewFollowerEvent(ctx, event)
	}
	if err != nil {
		return nil, err
	}

	return notifications, nil
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
	relevantEvents ...*model.Event,
) ([]*pn.Notification[*DeviceRegistrationEvent], error) {
	if len(deviceRegistrationEvents) == 0 {
		return nil, nil
	}

	notifications := make([]*pn.Notification[*DeviceRegistrationEvent], 0)
	defaultTranslation := pm.getTranslationWithRelevantInfo(notificationType, relevantEvents...)

	compressedEvent, err := compressData([]byte(incomingEvent.String()))
	if err != nil {
		return nil, errors.Wrap(err, "failed to compress event data")
	}
	var compressedRelevantEvents string
	if len(relevantEvents) > 0 {
		relevantEventsStrings := make([]string, 0, len(relevantEvents))
		for _, relevantEvent := range relevantEvents {
			relevantEventsStrings = append(relevantEventsStrings, relevantEvent.Content)
		}
		compressedRelevantEvents, err = compressData([]byte(strings.Join(relevantEventsStrings, ",")))
		if err != nil {
			return nil, errors.Wrap(err, "failed to compress relevant events data")
		}
	}
	for _, event := range deviceRegistrationEvents {
		data := map[string]interface{}{
			"compression": CompressionMethodZlib,
			"event":       compressedEvent,
		}
		if len(relevantEvents) > 0 {
			data["relevant_events"] = compressedRelevantEvents
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

	return notifications, nil
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

func (pm *PushNotificationManager) handleEventWithPublicKey(event *model.Event, notificationType NotificationType, relevantEvents ...*model.Event) ([]*pn.Notification[*DeviceRegistrationEvent], error) {
	referencePubkey := event.GetTag("p").Value()
	if referencePubkey == "" || referencePubkey == event.GetMasterPublicKey() {
		return nil, nil
	}
	deviceEvents := pm.collectUserValidDevices(referencePubkey, event)

	return pm.createNotifications(deviceEvents, notificationType, event, relevantEvents...)
}

func (pm *PushNotificationManager) handleQuoteEvent(event *model.Event, relevantEvents ...*model.Event) ([]*pn.Notification[*DeviceRegistrationEvent], error) {
	qLowerTag := event.GetTag("q")
	qUpperTag := event.GetTag("Q")
	var referencePubkey string
	if len(qLowerTag) >= 4 && qLowerTag[3] != "" {
		referencePubkey = qLowerTag[3]
	} else if len(qUpperTag) >= 4 && qUpperTag[3] != "" {
		referencePubkey = qUpperTag[3]
	} else {
		return nil, nil
	}
	devices := pm.collectUserValidDevices(referencePubkey, event)
	if len(devices) == 0 {
		return nil, nil
	}
	notifications, err := pm.createNotifications(devices, NotificationTypeRepost, event, relevantEvents...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create notifications")
	}

	return notifications, nil
}

func getDisplayNameFromRelevantEvents(events []*model.Event) string {
	const defaultDisplayName = "Someone"
	if len(events) == 0 {
		return defaultDisplayName
	}
	profileMetadataIndex := slices.IndexFunc(events, func(event *model.Event) bool {
		return event.Kind == nostr.KindProfileMetadata
	})
	if profileMetadataIndex == -1 {
		return defaultDisplayName
	}
	var profileData struct {
		Name        string `json:"name,omitempty"`
		DisplayName string `json:"display_name,omitempty"`
	}

	if err := json.Unmarshal([]byte(events[profileMetadataIndex].Content), &profileData); err != nil {
		log.Printf("failed to unmarshal profile metadata: %v", err)

		return defaultDisplayName
	}
	if profileData.DisplayName != "" {
		return "@" + profileData.DisplayName
	} else if profileData.Name != "" {
		return "@" + profileData.Name
	}

	return defaultDisplayName
}

func (pm *PushNotificationManager) getTranslationWithRelevantInfo(notificationType NotificationType, relevantEvents ...*model.Event) NotificationTranslation {
	translationTemplate, ok := DefaultTranslations[notificationType]
	if !ok {
		log.Printf("missing translation for notification type: %s", notificationType)

		return NotificationTranslation{
			Title:    "Notification",
			Body:     "You have a new notification",
			ImageURL: "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png",
		}
	}

	return NotificationTranslation{
		Title:    translationTemplate.Title(relevantEvents...),
		Body:     translationTemplate.Body(relevantEvents...),
		ImageURL: translationTemplate.ImageURL(relevantEvents...),
	}
}

func compressData(data []byte) (string, error) {
	var compressed bytes.Buffer
	zw, err := zlib.NewWriterLevel(&compressed, zlib.BestCompression)
	if err != nil {
		return "", errors.Wrap(err, "failed to create zlib writer")
	}
	defer zw.Close()
	if _, err := zw.Write(data); err != nil {
		return "", errors.Wrap(err, "failed to compress data")
	}
	if err := zw.Close(); err != nil {
		return "", errors.Wrap(err, "failed to close zlib writer")
	}

	return compressed.String(), nil
}
