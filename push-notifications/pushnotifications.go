// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip44"

	"github.com/ice-blockchain/subzero/cfg"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
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
		relayURL               string
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
		RelayURL           string   `yaml:"relay-url"`
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
	allowedPushEventKinds = map[int]struct{}{
		nostr.KindTextNote:                  {},
		model.CustomIONKindEditableTextNote: {},
		nostr.KindGenericRepost:             {},
		nostr.KindReaction:                  {},
		nostr.KindGiftWrap:                  {},
		nostr.KindFollowList:                {},
		model.CustomIONSystemMessage:        {},
	}
)

func MustInit(ctx context.Context) {
	userDevicesMap := make(map[PublicKey]map[DeviceID]DeviceInfo)

	var pnClient pn.Client
	var err error

	config := cfg.MustGet[config]()

	if config.FCMCredentialsFile == "" {
		panic("[push-notifications] FCM credentials not provided")
	}
	if config.PrivateKey == "" {
		panic("[push-notifications] private key is empty")
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

	pnClient, err = pn.New(ctx, opts...)
	if err != nil {
		log.Panicf("[push-notifications] failed to create push notification client: %v", err)
	}
	mustRunSelfTest(ctx, pnClient, config.PrivateKey)

	globalPushNotificationManager = &PushNotificationManager{
		userDevicesMap:         userDevicesMap,
		pushNotificationClient: &pnClient,
		relayURL:               config.RelayURL,
	}

	if err := globalPushNotificationManager.syncDevices(ctx); err != nil {
		log.Panicf("[push-notifications] failed to perform full device synchronization at startup: %v", err)
	}
}

func runSelfTest(ctx context.Context, pnClient pn.Client, privateKey string) error {
	devicePriv, devicePub := model.GenerateKeyPair()
	serverPrivX25519, err := nip44.ConvertEd25519PrivateKeyToX25519(privateKey)
	if err != nil {
		return errors.Wrap(err, "failed to convert server private key")
	}
	devicePubX25519, err := nip44.ConvertEd25519PublicKeyToX25519(devicePub)
	if err != nil {
		return errors.Wrap(err, "failed to convert device pubkey")
	}
	convKey, err := nip44.GenerateConversationKeyX25519(serverPrivX25519, devicePubX25519)
	if err != nil {
		return errors.Wrap(err, "failed to derive conversation key")
	}
	bogusToken := "self-test-invalid-token"
	encryptedToken, err := nip44.EncryptX25519(bogusToken, convKey, nil)
	if err != nil {
		return errors.Wrap(err, "failed to encrypt token")
	}
	incomingEvent := &model.Event{Event: nostr.Event{
		Kind:      model.CustomIONKindEditableTextNote,
		CreatedAt: nostr.Now(),
		Content:   "self-test",
		Tags:      nostr.Tags{{"test", "test"}},
	}}
	if err := incomingEvent.SignWithAlg(privateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519); err != nil {
		return errors.Wrap(err, "failed to sign event")
	}
	compressedEvent, err := compressAndEncodeBase64(incomingEvent.String())
	if err != nil {
		return errors.Wrap(err, "failed to compress event")
	}
	compressedRelevantEvents, err := compressAndEncodeBase64("[]")
	if err != nil {
		return errors.Wrap(err, "failed to compress relevant events")
	}
	deviceRegistrationEvent := &model.Event{Event: nostr.Event{
		Kind:      model.CustomIONKindDeviceRegistration,
		CreatedAt: nostr.Now(),
		Tags:      nostr.Tags{{"token", encryptedToken}},
	}}
	if err := deviceRegistrationEvent.SignWithAlg(devicePriv, model.SignAlgEDDSA, model.KeyAlgCurve25519); err != nil {
		return errors.Wrap(err, "failed to sign device registration event")
	}
	n := &pn.Notification[*DeviceRegistrationEvent]{
		Target: deviceRegistrationEvent,
		Data: map[string]interface{}{
			"compression":     CompressionMethodZlib,
			"event":           compressedEvent,
			"relevant_events": compressedRelevantEvents,
		},
		Kind:  model.CustomIONKindEditableTextNote,
		Title: "self-test",
		Body:  "self-test",
	}
	if err = pnClient.SendSingle(ctx, n); err != nil && !pn.IsInvalidDeviceToken(err) {
		return errors.Wrap(err, "unexpected error")
	}

	return nil
}

func mustRunSelfTest(ctx context.Context, pnClient pn.Client, privateKey string) {
	if err := runSelfTest(ctx, pnClient, privateKey); err != nil {
		log.Panicf("[push-notifications] self-test failed: %v", err)
	}
}

func GetFCMConfigs() (androidConfigs, iosConfigs, webConfigs []string) {
	config := cfg.MustGet[config]()

	return config.FCMAndroidConfigs, config.FCMIOSConfigs, config.FCMWebConfigs
}

func AcceptEvents(ctx context.Context, events ...*model.Event) error {
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
	ephemeralByRef, parseErr := model.ParseEphemeralEmbeddingEvents(events...)
	if parseErr != nil {
		log.Printf("[push-notifications] failed to parse ephemeral embedding events: %v", parseErr)
		ephemeralByRef = make(map[string][]*model.EphemeralEmbeddingEvent)
	}
	for _, event := range events {
		if event.Kind == model.CustomIONKindEphemeralEmbedding {
			continue
		}
		if _, ok := allowedPushEventKinds[event.Kind]; !ok {
			continue
		}
		if event.Kind == model.CustomIONSystemMessage {
			if notifications := pm.handleSystemEvent(event); notifications != nil {
				topicNotifications = append(topicNotifications, notifications...)
			}
		}
		var relevantEvents []*model.Event
		if !shouldSkipEphemeralEvent(event) {
			if embeddings, ok := ephemeralByRef[event.Address()]; ok && len(embeddings) > 0 {
				for _, emb := range embeddings {
					relevantEvents = append(relevantEvents, emb.Event)
				}
			}
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

func shouldSkipEphemeralEvent(event *model.Event) bool {
	return event.Kind == nostr.KindGiftWrap || event.Kind == model.CustomIONSystemMessage
}

func (pm *PushNotificationManager) processEvent(ctx context.Context, event *model.Event, relevantEvents ...*model.Event) ([]*pn.Notification[*DeviceRegistrationEvent], error) {
	var notifications []*pn.Notification[*DeviceRegistrationEvent]
	var err error

	if len(relevantEvents) == 0 && !shouldSkipEphemeralEvent(event) {
		isAuthoritative, profileMetadataEvent, attestationEvent, err := pm.getAuthoritativeEvents(ctx, event)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get authoritative events")
		}
		if isAuthoritative {
			if profileMetadataEvent == nil || attestationEvent == nil {
				return nil, fmt.Errorf("empty profile metadata or attestation event for event %s", event.ID)
			}
			relevantEvents = append(relevantEvents, pm.createEphemeralEmbeddingEvent(profileMetadataEvent), pm.createEphemeralEmbeddingEvent(attestationEvent))
		}
	}

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
		return pm.handleNewFollowerEvent(ctx, event, relevantEvents...)
	}
	if err != nil {
		return nil, errors.Wrapf(err, "failed to handle event %s", event.ID)
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
				errChan <- errors.Wrap(err, "failed to send notification")
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

	compressedEvent, err := compressAndEncodeBase64(incomingEvent.String())
	if err != nil {
		return nil, errors.Wrap(err, "failed to compress event data")
	}
	var compressedRelevantEvents string
	if len(relevantEvents) > 0 {
		relevantEventsStrings := make([]string, 0, len(relevantEvents))
		for _, relevantEvent := range relevantEvents {
			relevantEventsStrings = append(relevantEventsStrings, relevantEvent.Content)
		}
		compressedRelevantEvents, err = compressAndEncodeBase64(`[` + strings.Join(relevantEventsStrings, ",") + `]`)
		if err != nil {
			return nil, errors.Wrap(err, "failed to compress relevant events data")
		}
	}
	var deviceEventIDs []string
	for _, event := range deviceRegistrationEvents {
		deviceEventIDs = append(deviceEventIDs, event.ID)
		data := map[string]interface{}{
			"compression": CompressionMethodZlib,
			"event":       compressedEvent,
		}
		if len(relevantEvents) > 0 {
			data["relevant_events"] = compressedRelevantEvents
		}

		switch event.GetTag("t").Value() {
		case model.DeviceTokenOSAndroid:
			notifications = append(notifications, &pn.Notification[*DeviceRegistrationEvent]{
				Target: event,
				Data:   data,
				Kind:   incomingEvent.Kind,
			})
		default:
			notifications = append(notifications, &pn.Notification[*DeviceRegistrationEvent]{
				Target:   event,
				Title:    defaultTranslation.Title,
				Body:     defaultTranslation.Body,
				ImageURL: defaultTranslation.ImageURL,
				Data:     data,
				Kind:     incomingEvent.Kind,
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

func compressAndEncodeBase64(data string) (string, error) {
	var buf bytes.Buffer
	base64Encoder := base64.NewEncoder(base64.StdEncoding, &buf)
	defer base64Encoder.Close()
	zw, err := zlib.NewWriterLevel(base64Encoder, zlib.BestCompression)
	if err != nil {
		return "", errors.Wrap(err, "failed to create zlib writer")
	}
	defer zw.Close()

	if _, err := zw.Write([]byte(data)); err != nil {
		return "", errors.Wrap(err, "failed to compress data")
	}

	if err := zw.Close(); err != nil {
		return "", errors.Wrap(err, "failed to close zlib writer")
	}

	if err := base64Encoder.Close(); err != nil {
		return "", errors.Wrap(err, "failed to close base64 encoder")
	}

	return buf.String(), nil
}

func (pm *PushNotificationManager) createEphemeralEmbeddingEvent(contentEvent *model.Event) *model.Event {
	ephemeralEvent := &model.Event{
		Event: nostr.Event{
			Kind:      model.CustomIONKindEphemeralEmbedding,
			CreatedAt: nostr.Now(),
			Content:   contentEvent.String(),
		},
	}

	return ephemeralEvent
}

func (pm *PushNotificationManager) getAuthoritativeEvents(ctx context.Context, event *model.Event) (bool, *model.Event, *model.Event, error) {
	masterPubKey := event.GetMasterPublicKey()
	it := query.GetStoredEvents(ctx,
		model.Filter{
			Authors: []string{masterPubKey},
			Kinds:   []int{nostr.KindRelayListMetadata},
			Tags:    model.TagMap{}.Set("r", &pm.relayURL),
			Limit:   1,
		},
		model.Filter{
			Authors: []string{masterPubKey},
			Kinds:   []int{nostr.KindProfileMetadata},
			Limit:   1,
		},
		model.Filter{
			Authors: []string{masterPubKey},
			Kinds:   []int{model.CustomIONKindAttestation},
			Tags:    model.TagMap{}.Set("p", &event.PubKey),
			Limit:   1,
		},
	)
	var relayListMetadataEvent, profileMetadataEvent, attestationEvent *model.Event
	for ev, err := range it {
		if err != nil {
			return false, nil, nil, errors.Wrap(err, "failed to check relay list metadata")
		}
		switch ev.Kind {
		case nostr.KindRelayListMetadata:
			relayListMetadataEvent = ev
		case nostr.KindProfileMetadata:
			profileMetadataEvent = ev
		case model.CustomIONKindAttestation:
			attestationEvent = ev
		}
	}

	return relayListMetadataEvent != nil, profileMetadataEvent, attestationEvent, nil
}
