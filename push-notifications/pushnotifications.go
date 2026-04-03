// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip44"
	"github.com/panjf2000/ants/v2"
	"github.com/rs/zerolog/log"

	"github.com/ice-blockchain/subzero/cfg"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
)

type (
	DeviceRegistrationEvent = pn.DeviceRegistrationEvent
	PublicKey               = string
	NotificationType        string

	PushNotificationManager struct {
		userDevicesMap         map[PublicKey]map[DeviceID]DeviceInfo
		pushNotificationClient *pn.Client
		relayURL               string
		deviceMutex            sync.RWMutex
		compressorPool         *sync.Pool
		stats                  *PushStats
		antsPool               *ants.Pool
	}

	notificationTranslation struct {
		Title    string
		Body     string
		ImageURL string
	}

	config struct {
		FCMCredentialsFile string   `yaml:"fcm-credentials-file"`
		PrivateKey         string   `yaml:"private-key"`
		RelayURL           string   `yaml:"relay-url"`
		FCMAndroidConfigs  []string `yaml:"fcm-android-configs"`
		FCMIOSConfigs      []string `yaml:"fcm-ios-configs"`
		FCMWebConfigs      []string `yaml:"fcm-web-configs"`
	}

	compressorPoolItem struct {
		buf           *bytes.Buffer
		base64Encoder io.WriteCloser
		zlibWriter    *zlib.Writer
	}
)

const (
	NotificationTypeReaction                  NotificationType = "reaction"
	NotificationTypeRepost                    NotificationType = "repost"
	NotificationTypeMentionReply              NotificationType = "mention_reply"
	NotificationTypeDirectMessage             NotificationType = "direct_message"
	NotificationTypeGroupChatMessage          NotificationType = "group_chat_message"
	NotificationTypeChannelMessage            NotificationType = "channel_message"
	NotificationTypePaymentRequest            NotificationType = "payment_request"
	NotificationTypePaymentReceived           NotificationType = "payment_received"
	NotificationTypeSystem                    NotificationType = "system"
	NotificationTypeNewFollower               NotificationType = "new_follower"
	NotificationTypeTokenizedCommunityCreated NotificationType = "community_token_created"
	NotificationTypeTokenizedCommunityAction  NotificationType = "community_token_swapped"

	CompressionMethodZlib = "zlib"
)

var (
	globalPushNotificationManager *PushNotificationManager
	DefaultTranslations           = map[NotificationType]notificationTranslation{
		NotificationTypeReaction: {
			Title:    "New reaction",
			Body:     "Someone reacted to your post",
			ImageURL: "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png",
		},
		NotificationTypeRepost: {
			Title:    "New repost",
			Body:     "Someone reposted your post",
			ImageURL: "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png",
		},
		NotificationTypeMentionReply: {
			Title:    "New mention/reply",
			Body:     "Someone mentioned/replied you",
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
			Body:     "Someone is now following you",
			ImageURL: "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png",
		},
		NotificationTypeTokenizedCommunityCreated: {
			Title:    "Someone created a token based on your post or a profile",
			Body:     "Token created",
			ImageURL: "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png",
		},
		NotificationTypeTokenizedCommunityAction: {
			Title:    "Someone swapped a token from your tokenized community",
			Body:     "Token swapped",
			ImageURL: "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png",
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
		// Disable for now.
		// model.CustomIONKindTokenizedCommunityDefinition: {},
		// model.CustomIONKindTokenizedCommunityAction:     {},
	}
)

func MustInit(ctx context.Context, antsPool *ants.Pool) {
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
		log.Fatal().Err(err).Msg("[push-notifications] failed to create push notification client")
	}
	globalPushNotificationManager = &PushNotificationManager{
		userDevicesMap:         userDevicesMap,
		pushNotificationClient: &pnClient,
		relayURL:               config.RelayURL,
		stats:                  newPushStats(),
		antsPool:               antsPool,
		compressorPool: &sync.Pool{
			New: func() any {
				buf := &bytes.Buffer{}
				base64Encoder := base64.NewEncoder(base64.StdEncoding, buf)
				zlibWriter, _ := zlib.NewWriterLevel(base64Encoder, zlib.BestCompression)

				return &compressorPoolItem{
					buf:           buf,
					base64Encoder: base64Encoder,
					zlibWriter:    zlibWriter,
				}
			},
		},
	}

	globalPushNotificationManager.mustRunSelfTest(ctx, pnClient, config.PrivateKey)

	globalPushNotificationManager.stats.StartPeriodicLogging(ctx)

	if err := globalPushNotificationManager.syncDevices(ctx); err != nil {
		log.Fatal().Err(err).Msg("[push-notifications] failed to perform full device synchronization at startup")
	}
}

func (pnm *PushNotificationManager) runSelfTest(ctx context.Context, pnClient pn.Client, privateKey string) error {
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
	compressedEvent, err := pnm.compressAndEncodeBase64(incomingEvent.String())
	if err != nil {
		return errors.Wrap(err, "failed to compress event")
	}
	compressedRelevantEvents, err := pnm.compressAndEncodeBase64("[]")
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
		SourceEvent: incomingEvent,
		Title:       "self-test",
		Body:        "self-test",
	}
	if err = pnClient.SendSingle(ctx, n); err != nil && !pn.IsInvalidDeviceTokenError(err) {
		return errors.Wrap(err, "unexpected error")
	}

	return nil
}

func (pm *PushNotificationManager) mustRunSelfTest(ctx context.Context, pnClient pn.Client, privateKey string) {
	if err := pm.runSelfTest(ctx, pnClient, privateKey); err != nil {
		log.Fatal().Err(err).Msg("[push-notifications] self-test failed")
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
		log.Error().Str("context", "PUSH_NOTIFICATIONS").
			Err(parseErr).
			Msg("failed to parse ephemeral embedding events")
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
		} else {
			log.Info().
				Str("context", "PUSH_NOTIFICATIONS").
				Str("event_id", event.ID).
				Int("event_kind", int(event.Kind)).
				Str("master_pubkey", event.GetMasterPublicKey()).
				Msg("non-authoritative event processed without relevant events")
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
	case model.CustomIONKindTokenizedCommunityDefinition, model.CustomIONKindTokenizedCommunityAction:
		notifications, err = pm.handleTokenizedCommunityEvent(ctx, event, relevantEvents...)
		err = errors.Wrap(err, "failed to handle tokenized community definition event")
	case nostr.KindFollowList:
		return pm.handleNewFollowerEvent(event, relevantEvents...)
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
	if repostedEvent.Kind != model.CustomIONKindEditableTextNote && repostedEvent.Kind != nostr.KindArticle {
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
	var wg sync.WaitGroup
	invalidDevices := make([]*model.Event, 0)

	for _, notification := range singleNotifications {
		wg.Add(1)
		if err := pm.antsPool.Submit(func() {
			defer wg.Done()
			err := (*pm.pushNotificationClient).SendSingle(ctx, notification)

			if err != nil {
				pm.stats.RecordError(notification.SourceEvent, err)

				if pn.IsInvalidDeviceTokenError(err) {
					invalidDevicesMutex.Lock()
					invalidDevices = append(invalidDevices, notification.Target)
					invalidDevicesMutex.Unlock()
					errChan <- nil
				} else {
					errChan <- errors.Wrap(err, "failed to send notification")
				}
			} else {
				pm.stats.RecordSuccess(notification.SourceEvent)
				errChan <- nil
			}
		}); err != nil {
			wg.Done()
			errChan <- errors.Wrap(err, "failed to submit notification task to pool")
		}
	}

	for _, notification := range topicNotifications {
		wg.Add(1)
		if err := pm.antsPool.Submit(func() {
			defer wg.Done()
			err := (*pm.pushNotificationClient).SendTopic(ctx, notification)
			if err != nil {
				pm.stats.RecordError(notification.SourceEvent, err)
			} else {
				pm.stats.RecordSuccess(notification.SourceEvent)
			}
			errChan <- err
		}); err != nil {
			wg.Done()
			errChan <- errors.Wrap(err, "failed to submit topic notification task to pool")
		}
	}

	wg.Wait()
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
	defaultTranslation := pm.getTranslation(notificationType)

	compressedEvent, err := pm.compressAndEncodeBase64(incomingEvent.String())
	if err != nil {
		return nil, errors.Wrap(err, "failed to compress event data")
	}
	var compressedRelevantEvents string
	if len(relevantEvents) > 0 {
		relevantEventsStrings := make([]string, 0, len(relevantEvents))
		for _, relevantEvent := range relevantEvents {
			relevantEventsStrings = append(relevantEventsStrings, relevantEvent.Content)
		}
		compressedRelevantEvents, err = pm.compressAndEncodeBase64(`[` + strings.Join(relevantEventsStrings, ",") + `]`)
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
				Target:      event,
				Data:        data,
				SourceEvent: incomingEvent,
			})
		default:
			notifications = append(notifications, &pn.Notification[*DeviceRegistrationEvent]{
				Target:      event,
				Title:       defaultTranslation.Title,
				Body:        defaultTranslation.Body,
				ImageURL:    defaultTranslation.ImageURL,
				Data:        data,
				SourceEvent: incomingEvent,
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
		log.Trace().Str("context", "PUSH-NOTIFICATIONS").
			Str("pubkey", pubKey).
			Str("event_id", event.ID).
			Msg("no devices found for user")
		return nil
	}

	for _, deviceInfo := range userDevices {
		if deviceInfo.Filters == nil || deviceInfo.Filters.Match(&event.Event) {
			devices = append(devices, deviceInfo.Event)
		}
	}

	log.Trace().Str("context", "PUSH-NOTIFICATIONS").
		Str("pubkey", pubKey).
		Int("target_num_devices", len(devices)).
		Int("total_num_devices", len(userDevices)).
		Str("event_id", event.ID).
		Msg("collected valid devices for user")

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

func (pm *PushNotificationManager) getTranslation(notificationType NotificationType) notificationTranslation {
	translation, ok := DefaultTranslations[notificationType]
	if !ok {
		log.Error().Str("context", "PUSH_NOTIFICATIONS").
			Str("notification_type", string(notificationType)).
			Msg("missing translation for notification type")

		return notificationTranslation{
			Title:    "Notification",
			Body:     "You have a new notification",
			ImageURL: "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png",
		}
	}

	return translation
}

func (pm *PushNotificationManager) compressAndEncodeBase64(data string) (string, error) {
	compressor := pm.compressorPool.Get().(*compressorPoolItem)
	defer pm.compressorPool.Put(compressor)
	compressor.reset()
	if _, err := compressor.zlibWriter.Write([]byte(data)); err != nil {
		return "", errors.Wrap(err, "failed to compress data")
	}
	if err := compressor.zlibWriter.Close(); err != nil {
		return "", errors.Wrap(err, "failed to close zlib writer")
	}
	if err := compressor.base64Encoder.Close(); err != nil {
		return "", errors.Wrap(err, "failed to close base64 encoder")
	}

	return compressor.buf.String(), nil
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

	relayTag := model.TagMap{}.Set("r", &pm.relayURL)
	if u, err := url.Parse(pm.relayURL); err == nil && u.Port() != "" {
		u.Host = u.Hostname()
		relayTag = relayTag.Append("r", model.PointerOf(u.String()))
	}

	it := query.GetStoredEvents(ctx,
		model.Filter{
			Authors: []string{masterPubKey},
			Kinds:   []int{nostr.KindRelayListMetadata},
			Tags:    relayTag,
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

func (c *compressorPoolItem) reset() {
	c.buf.Reset()
	c.zlibWriter.Reset(c.base64Encoder)
}
