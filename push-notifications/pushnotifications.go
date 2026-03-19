// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip44"
	"github.com/panjf2000/ants/v2"
	"github.com/puzpuzpuz/xsync/v4"
	"github.com/rs/zerolog/log"

	"github.com/ice-blockchain/subzero/cfg"
	"github.com/ice-blockchain/subzero/database/query"
	em "github.com/ice-blockchain/subzero/event-matcher"
	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
	"github.com/ice-blockchain/subzero/rq"
	"github.com/ice-blockchain/subzero/server/broadcaster"
)

type (
	NotificationType string

	PushNotificationManager struct {
		pushNotificationClient pn.Client
		rq                     rq.Client
		broadcaster            eventBroadcaster
		devicesFilterIndex     *em.Storage[*deviceInfo]
		devicesEventMap        *xsync.Map[deviceIndexKey, *model.Event] // Index key -> Device Registration Event.
		compressorPool         *sync.Pool
		stats                  *PushStats
		antsPool               *ants.Pool
		relayURL               string
		privateKey             string
	}

	deviceIndexKeyType string

	deviceIndexKey struct {
		Key   deviceIndexKeyType
		Value string
	}

	notificationTranslation struct {
		Title    string
		Body     string
		ImageURL string
	}

	config struct {
		FCMCredentialsFile  string   `yaml:"fcm-credentials-file"`
		PrivateKey          string   `yaml:"private-key"`
		RelayURL            string   `yaml:"relay-url"`
		BroadcastPrivateKey string   `yaml:"broadcast-private-key" validate:"required"`
		FCMAndroidConfigs   []string `yaml:"fcm-android-configs"`
		FCMIOSConfigs       []string `yaml:"fcm-ios-configs"`
		FCMWebConfigs       []string `yaml:"fcm-web-configs"`
	}

	compressorPoolItem struct {
		buf           *bytes.Buffer
		base64Encoder io.WriteCloser
		zlibWriter    *zlib.Writer
	}

	eventBroadcaster interface {
		BroadcastTo(ctx context.Context, target string, events model.Events) (err error)
		Close()
	}
)

const (
	NotificationTypeReaction                 NotificationType = "reaction"
	NotificationTypeRepost                   NotificationType = "repost"
	NotificationTypeMentionReply             NotificationType = "mention_reply"
	NotificationTypeDirectMessage            NotificationType = "direct_message"
	NotificationTypeGroupChatMessage         NotificationType = "group_chat_message"
	NotificationTypeChannelMessage           NotificationType = "channel_message"
	NotificationTypePaymentRequest           NotificationType = "payment_request"
	NotificationTypePaymentReceived          NotificationType = "payment_received"
	NotificationTypeAnonymousPaymentReceived NotificationType = "anonymous_payment_received"
	NotificationTypeSystem                   NotificationType = "system"
	NotificationTypeNewFollower              NotificationType = "new_follower"

	NotificationTypeCreatorTokenCreated NotificationType = "creator_token_created"
	NotificationTypeCreatorTokenSwapped NotificationType = "creator_token_swapped"

	NotificationTypeContentTokenCreated NotificationType = "content_token_created"
	NotificationTypeContentTokenSwapped NotificationType = "content_token_swapped"

	NotificationTypeTokenPriceChange NotificationType = "token_price_change"
	NotificationTypeTokenActivity    NotificationType = "token_activity"

	CompressionMethodZlib = "zlib"
)

const (
	NotificationTypeSomeoneCreatorTokenCreated NotificationType = "someone_creator_token_created"
	NotificationTypeSomeoneCreatorTokenSwapped NotificationType = "someone_creator_token_swapped"

	NotificationTypeSomeoneContentTokenCreated NotificationType = "someone_content_token_created"
	NotificationTypeSomeoneContentTokenSwapped NotificationType = "someone_content_token_swapped"

	NotificationTypeSomeonePost    NotificationType = "someone_post"
	NotificationTypeSomeoneVideo   NotificationType = "someone_video"
	NotificationTypeSomeoneArticle NotificationType = "someone_article"
	NotificationTypeSomeoneStory   NotificationType = "someone_story"
)

var (
	defaultTranslations = map[NotificationType]notificationTranslation{
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
		NotificationTypeAnonymousPaymentReceived: {
			Title:    "Anonymous payment received",
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
		NotificationTypeCreatorTokenCreated: {
			Title:    "Creator Token Is Live",
			Body:     "Your token is now available for trading",
			ImageURL: "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png",
		},
		NotificationTypeCreatorTokenSwapped: {
			Title:    "Someone Bought Your Creator Token",
			Body:     "Someone Bought Your Creator Token",
			ImageURL: "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png",
		},
		NotificationTypeContentTokenCreated: {
			Title:    "Content Token Is Live",
			Body:     "Community launched a token for your post",
			ImageURL: "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png",
		},
		NotificationTypeContentTokenSwapped: {
			Title:    "Someone Bought Your Content Token",
			Body:     "Someone Bought Your Content Token",
			ImageURL: "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png",
		},
		NotificationTypeSomeoneCreatorTokenCreated: {
			Title:    "New Creator Token",
			Body:     "Someone just launched their token. Trade now!",
			ImageURL: "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png",
		},
		NotificationTypeSomeoneContentTokenCreated: {
			Title:    "New Content Token",
			Body:     `Community launched a token for someone's post`,
			ImageURL: "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png",
		},
		NotificationTypeSomeoneCreatorTokenSwapped: {
			Title:    `Someone Bought Another Person's Creator Token`,
			Body:     `Someone Bought Another Person's Creator Token`,
			ImageURL: "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png",
		},
		NotificationTypeSomeoneContentTokenSwapped: {
			Title:    `Someone Bought Another Person's Content Token`,
			Body:     `Someone Bought Another Person's Content Token`,
			ImageURL: "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png",
		},
		NotificationTypeSomeonePost: {
			Title:    "New post",
			Body:     "New post from someone you enabled account notifications for",
			ImageURL: "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png",
		},
		NotificationTypeSomeoneVideo: {
			Title:    "New video",
			Body:     "New video from someone you enabled account notifications for",
			ImageURL: "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png",
		},
		NotificationTypeSomeoneArticle: {
			Title:    "New article is out",
			Body:     "New article from someone you enabled account notifications for",
			ImageURL: "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png",
		},
		NotificationTypeSomeoneStory: {
			Title:    "Quick update",
			Body:     "New story from someone you enabled account notifications for",
			ImageURL: "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png",
		},
		NotificationTypeTokenPriceChange: {
			Title:    "Token Price Changed",
			Body:     "Your token price changed",
			ImageURL: "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png",
		},
		NotificationTypeTokenActivity: {
			Title:    "Buying activity increased",
			Body:     "More buyers joined",
			ImageURL: "https://ice.io/wp-content/uploads/2024/04/ion-logo-2.png",
		},
	}
	allowedPushEventKinds = map[int]struct{}{
		nostr.KindArticle:                               {},
		nostr.KindTextNote:                              {},
		model.CustomIONKindEditableTextNote:             {},
		nostr.KindGenericRepost:                         {},
		nostr.KindReaction:                              {},
		nostr.KindGiftWrap:                              {},
		nostr.KindFollowList:                            {},
		model.CustomIONSystemMessage:                    {},
		model.CustomIONKindTokenizedCommunityDefinition: {},
		model.CustomIONKindTokenizedCommunityAction:     {},
		model.CustomIONKindDVMJobResponsePriceChange:    {},
		model.CustomIONKindDVMJobResponseTrendingTokens: {},
		model.CustomIONKindFundSendNotify:               {},
	}
	allowedBroadcastKinds = map[int]struct{}{
		model.CustomIONKindTokenizedCommunityAction:     {},
		model.CustomIONKindTokenizedCommunityDefinition: {},
		model.CustomIONKindEditableTextNote:             {},
		model.CustomIONKindDVMJobResponseTrendingTokens: {},
		nostr.KindArticle:       {},
		nostr.KindTextNote:      {},
		nostr.KindGenericRepost: {},
	}

	globalPushNotificationManager *PushNotificationManager
)

const (
	deviceIndexKeyTypeEventID   deviceIndexKeyType = "event_id"
	deviceIndexKeyTypeDeviceKey deviceIndexKeyType = "device_key"
)

func newManager(ctx context.Context, config *config, antsPool *ants.Pool, rqClient rq.Client, selfTest bool) (*PushNotificationManager, error) {
	var pnClient pn.Client
	var err error

	if config.FCMCredentialsFile == "" {
		return nil, errors.Errorf("FCM credentials not provided")
	}
	if config.PrivateKey == "" {
		return nil, errors.Errorf("private key is empty")
	}

	var opts []pn.Option
	if strings.HasPrefix(strings.TrimSpace(config.FCMCredentialsFile), "{") {
		opts = append(opts, pn.WithCredentialsJSON(config.FCMCredentialsFile))
	} else {
		if s, err := os.Stat(config.FCMCredentialsFile); err == nil && !s.IsDir() {
			log.Debug().Str("context", "PUSH_NOTIFICATIONS").Str("path", config.FCMCredentialsFile).Msg("using FCM credentials from file")
			opts = append(opts, pn.WithCredentialsFile(config.FCMCredentialsFile))
		} else {
			log.Debug().Str("context", "PUSH_NOTIFICATIONS").Str("path", config.FCMCredentialsFile).Msg("using FCM credentials from string")
			opts = append(opts, pn.WithCredentialsJSON(config.FCMCredentialsFile))
		}
	}
	opts = append(opts, pn.WithPrivateKey(config.PrivateKey))

	pnClient, err = pn.New(ctx, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create push notification client")
	}

	manager := &PushNotificationManager{
		devicesFilterIndex:     em.NewMatcherStorage[*deviceInfo](0),
		devicesEventMap:        xsync.NewMap[deviceIndexKey, *model.Event](),
		pushNotificationClient: pnClient,
		relayURL:               config.RelayURL,
		stats:                  newPushStats(),
		antsPool:               antsPool,
		rq:                     rqClient,
		privateKey:             config.PrivateKey,
		broadcaster: broadcaster.New(broadcaster.Config{
			RelayURL:   config.RelayURL,
			PrivateKey: config.BroadcastPrivateKey,
		}),
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

	if selfTest {
		manager.mustRunSelfTest(ctx, config.PrivateKey)
	}

	if err := manager.syncDevices(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to perform full device synchronization at startup")
	}

	manager.registerWorkers()

	return manager, nil
}

func (pnm *PushNotificationManager) registerWorkers() {
	if reg := pnm.rq.Register(); reg != nil {
		rq.RegisterWorker(reg, &broadcasterRelayFinderWorker{Manager: pnm})
		rq.RegisterWorker(reg, &broadcasterBroadcastWorker{Manager: pnm})
		rq.RegisterWorker(reg, &broadcasterUserNotificationWorker{Manager: pnm})
		rq.RegisterWorker(reg, &broadcasterPushNotificationWorker{Manager: pnm})
	}
}

func MustInit(ctx context.Context, antsPool *ants.Pool, rqClient rq.Client) {
	m, err := newManager(ctx, cfg.MustGet[config](), antsPool, rqClient, true)
	if err != nil {
		log.Panic().Err(err).Msg("[push-notifications] failed to initialize push notification manager")
	}

	m.stats.StartPeriodicLogging(ctx)
	globalPushNotificationManager = m
}

func (pnm *PushNotificationManager) runSelfTest(ctx context.Context, privateKey string) error {
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
	n := &pn.Notification[*model.Event]{
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
	if err = pnm.pushNotificationClient.SendSingle(ctx, n); err != nil && !pn.IsInvalidDeviceTokenError(err) {
		return errors.Wrap(err, "unexpected error")
	}

	return nil
}

func (pm *PushNotificationManager) mustRunSelfTest(ctx context.Context, privateKey string) {
	if err := pm.runSelfTest(ctx, privateKey); err != nil {
		log.Fatal().Err(err).Msg("[push-notifications] self-test failed")
	}
}

func GetFCMConfigs() (androidConfigs, iosConfigs, webConfigs []string) {
	config := cfg.MustGet[config]()

	return config.FCMAndroidConfigs, config.FCMIOSConfigs, config.FCMWebConfigs
}

func AcceptEventsFromBroadcast(ctx context.Context, events ...*model.Event) error {
	return globalPushNotificationManager.AcceptEventsFromBroadcast(ctx, events)
}

func AcceptEvents(ctx context.Context, events ...*model.Event) error {
	var err error

	hasNonEphemeralEvent := slices.ContainsFunc(events, func(event *model.Event) bool {
		return event.Kind != model.CustomIONKindEphemeralEmbedding
	})

	// Ephemeral embedding batch may come only from broadcaster, so we handle it separately.
	if !hasNonEphemeralEvent && len(events) > 0 {
		return errors.Wrap(globalPushNotificationManager.AcceptEventsFromBroadcast(ctx, events), "failed to accept ephemeral embedding events from broadcast")
	}

	err = errors.Join(err,
		errors.Wrap(globalPushNotificationManager.AcceptEvents(ctx, events), "pn.AcceptEvents failed"),
		errors.Wrap(globalPushNotificationManager.AcceptEventsForBroadcast(ctx, events), "pn.AcceptEventsForBroadcast failed"),
		errors.Wrap(globalPushNotificationManager.AcceptEventsForRemotePush(ctx, events), "pn.AcceptEventsForRemotePush failed"),
		errors.Wrap(globalPushNotificationManager.ManageDeviceRegistrationEvents(ctx, events), "pn.ManageDeviceRegistrationEvents failed"),
	)
	return errors.Wrap(err, "failed to process events")

}

func (pm *PushNotificationManager) AcceptEventsFromBroadcast(ctx context.Context, events []*model.Event) error {
	var batchID string

	if len(events) == 0 {
		return nil
	}

	// Syntax: l, batch, <batch_id>.
	if lTag := events[0].GetTag("l"); len(lTag) >= 3 && lTag[2] == "push-notification.broadcasting.tracing.id" {
		batchID = lTag.Value()
	}

	log.Trace().
		Str("context", "PUSH_NOTIFICATIONS").
		Int("events_count", len(events)).
		Str("batch", batchID).
		Strs("event_ids", model.Events(events).IDs()).
		Msg("received events from broadcaster for processing")

	return errors.Wrapf(
		pm.rq.Push(ctx,
			&broadcasterUserNotificationWorkerArgs{
				EphemeralEvents: events,
				BatchID:         batchID,
			},
		),
		"failed to push a job for processing %d ephemeral events from broadcaster",
		len(events),
	)
}

func (pm *PushNotificationManager) AcceptEventsForRemotePush(ctx context.Context, events model.Events) error {
	submitErr := pm.antsPool.Submit(func() {
		var relays []string
		var devicesCollected uint

		for _, event := range events {
			if event.IsEphemeral() {
				continue
			}

			devices := pm.collectRemoteDevices("", event)
			for _, device := range devices {
				devicesCollected++
				if v := collectRelaysFromDevice(device); len(v) > 0 {
					relays = append(relays, v...)
				} else if len(v) == 0 {
					log.Debug().
						Str("context", "PUSH_NOTIFICATIONS").
						Str("device_pubkey", device.PubKey).
						Str("event_id", event.ID).
						Msg("no relays found for device, skipping for remote push notification")
				}
			}
		}
		uniqueRelays := compactRelays(relays, pm.relayURL)

		batchID := events.Hash()
		log.Trace().
			Str("context", "PUSH_NOTIFICATIONS").
			Int("events_count", len(events)).
			Strs("event_ids", events.IDs()).
			Uint("devices_collected", devicesCollected).
			Int("total_relays_collected", len(relays)).
			Int("unique_relays_collected", len(uniqueRelays)).
			Str("batch", batchID).
			Msg("collected relays for remote push notification")

		if len(uniqueRelays) == 0 || devicesCollected == 0 {
			// No relays found, nothing to do.
			return
		}

		packedEvents := pm.packEventsForBroadcast(ctx, events, batchID)
		broadcastArgs := make([]rq.JobArgs, 0, len(uniqueRelays))
		for _, relayURL := range uniqueRelays {
			broadcastArgs = append(broadcastArgs, &broadcasterBroadcastWorkerArgs{
				RelayURL:        relayURL,
				BatchID:         batchID,
				EphemeralEvents: packedEvents,
			})
		}

		if err := pm.rq.Push(ctx, broadcastArgs...); err != nil {
			log.Error().Str("context", "PUSH_NOTIFICATIONS").
				Err(err).
				Str("batch", batchID).
				Strs("event_ids", events.IDs()).
				Msg("failed to push broadcaster broadcast worker jobs for remote push notification")
		}
	})
	return errors.Wrap(submitErr, "failed to submit task to ants pool for processing events for remote push")
}

func (pm *PushNotificationManager) AcceptEventsForBroadcast(ctx context.Context, events model.Events) error {
	if len(events) == 0 {
		return nil
	}

	batchID := events.Hash()

	return errors.Wrapf(
		pm.rq.Push(
			ctx,
			&broadcasterRelayFinderWorkerArgs{
				Events:  events,
				BatchID: batchID,
			},
		),
		"failed to push a job for broadcasting %d events",
		len(events),
	)
}

func (pm *PushNotificationManager) createEphemeralEmbeddingEvent(contentEvent *model.Event, source ...string) *model.Event {
	var ev model.Event

	ev.CreatedAt = contentEvent.CreatedAt
	ev.Kind = model.CustomIONKindEphemeralEmbedding
	ev.Content = contentEvent.String()
	ev.Tags = model.Tags{
		{"e", contentEvent.ID},
		{"p", contentEvent.GetMasterPublicKey()},
		{"k", strconv.Itoa(int(contentEvent.Kind))},
	}
	if len(source) > 0 {
		ev.Tags = append(ev.Tags, model.Tag{"L", "push-notification.broadcasting.tracing.id", source[0]})
		ev.Tags = append(ev.Tags, model.Tag{"l", source[0], "push-notification.broadcasting.tracing.id", source[0]})
	}
	if err := ev.SignWithAlg(pm.privateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519); err != nil {
		log.Panic().Err(err).Str("context", "PUSH_NOTIFICATIONS").Str("event_id", contentEvent.ID).Msg("failed to sign ephemeral embedding event for broadcasting")
	}

	return &ev
}

func (pm *PushNotificationManager) packEventsForBroadcast(_ context.Context, in model.Events, batch string) (out model.Events) {
	for i := range in {
		out = append(out, pm.createEphemeralEmbeddingEvent(in[i], batch))
	}
	return out
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
	singleNotifications []*pn.Notification[*model.Event],
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

func (pm *PushNotificationManager) processEvent(ctx context.Context, event *model.Event, relevantEvents ...*model.Event) ([]*pn.Notification[*model.Event], error) {
	var notifications []*pn.Notification[*model.Event]
	var err error

	if !event.IsJobResponse() && len(relevantEvents) == 0 && !shouldSkipEphemeralEvent(event) {
		isAuthoritative, profileMetadataEvent, attestationEvent, err := pm.getAuthoritativeEvents(ctx, event)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get authoritative events")
		}
		if isAuthoritative {
			if profileMetadataEvent == nil {
				return nil, errors.Errorf("%v: no profile metadata event found", event.ID)
			}
			if attestationEvent == nil {
				return nil, errors.Errorf("%v: no attestation event found", event.ID)
			}
			log.Trace().
				Str("context", "PUSH_NOTIFICATIONS").
				Str("event_id", event.ID).
				Msg("adding authoritative events as relevant events for push notification processing")
			relevantEvents = append(relevantEvents, pm.createEphemeralEmbeddingEvent(profileMetadataEvent), pm.createEphemeralEmbeddingEvent(attestationEvent))
		} else {
			log.Debug().
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
		} else if event.Kind == model.CustomIONKindEditableTextNote && (event.GetTag("expiration").Value() != "" || event.GetTag("p").Value() == "") {
			notifications, err = pm.handleNewPostEvent(ctx, event, relevantEvents...)
			err = errors.Wrap(err, "failed to handle new story/post event")
		} else {
			notifications, err = pm.handleMentionReplyEvent(event, relevantEvents...)
			err = errors.Wrap(err, "failed to handle mention reply/mention event")
		}
	case nostr.KindArticle:
		notifications, err = pm.handleNewPostEvent(ctx, event, relevantEvents...)
		err = errors.Wrap(err, "failed to handle article event")
	case model.CustomIONKindDVMJobResponsePriceChange, model.CustomIONKindDVMJobResponseTrendingTokens:
		notifications, err = pm.handleNewDVMEvent(ctx, event, relevantEvents...)
		err = errors.Wrap(err, "failed to handle DVM job response event")
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
	case model.CustomIONKindFundSendNotify: // Anonymous send from external wallet, in-app transfers are wrapped into gift wrap
		return pm.handleAnonymousFundSendEvent(event, relevantEvents...)
	}
	if err != nil {
		return nil, errors.Wrapf(err, "failed to handle event %s", event.ID)
	}

	log.Trace().
		Str("context", "PUSH_NOTIFICATIONS").
		Str("event_id", event.ID).
		Int("notifications_created", len(notifications)).
		Msg("processed event for notifications")

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
	singleNotifications []*pn.Notification[*model.Event],
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
	singleNotifications []*pn.Notification[*model.Event],
	topicNotifications []*pn.Notification[pn.SubscriptionTopic],
	errChan chan error,
) []*model.Event {
	var invalidDevicesMutex sync.Mutex
	var wg sync.WaitGroup
	invalidDevices := make([]*model.Event, 0)

	for _, notification := range singleNotifications {
		wg.Add(1)
		if err := pm.antsPool.Submit(func() {
			defer wg.Done()
			err := pm.pushNotificationClient.SendSingle(ctx, notification)

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
			err := pm.pushNotificationClient.SendTopic(ctx, notification)
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

func (pm *PushNotificationManager) collectErrorsAndProcessInvalidDevices(ctx context.Context, totalCount int, errChan chan error, invalidDevices []*model.Event) error {
	var receivedErrors []error
	for i := 0; i < totalCount; i++ {
		if err := <-errChan; err != nil {
			receivedErrors = append(receivedErrors, err)
		}
	}
	if len(invalidDevices) > 0 {
		if err := pm.handleInvalidDeviceTokens(ctx, invalidDevices); err != nil {
			receivedErrors = append(receivedErrors, err)
		}
	}
	if len(receivedErrors) > 0 {
		return errors.Wrap(errors.Join(receivedErrors...), "errors occurred while sending notifications")
	}

	return nil
}

func (pm *PushNotificationManager) handleInvalidDeviceTokens(ctx context.Context, deviceEvents []*model.Event) error {
	if len(deviceEvents) == 0 {
		return nil
	}
	if err := query.MarkTokenAsInvalidInEventTags(ctx, deviceEvents); err != nil {
		return errors.Wrap(err, "failed to mark devices as invalid on query level")
	}

	pm.removeInvalidTokenDevicesFromCache(ctx, deviceEvents)

	return nil
}

func (pm *PushNotificationManager) createNotifications(
	deviceRegistrationEvents model.Events,
	notificationType NotificationType,
	incomingEvent *model.Event,
	relevantEvents ...*model.Event,
) ([]*pn.Notification[*model.Event], error) {
	if len(deviceRegistrationEvents) == 0 {
		return nil, nil
	}

	notifications := make([]*pn.Notification[*model.Event], 0, len(deviceRegistrationEvents))
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
		log.Trace().
			Str("context", "PUSH_NOTIFICATIONS").
			Str("device_pubkey", event.PubKey).
			Str("event_id", incomingEvent.ID).
			Int("relevant_events_count", len(relevantEvents)).
			Str("notification_type", string(notificationType)).
			Msg("creating notification for device")

		deviceEventIDs = append(deviceEventIDs, event.ID)
		data := map[string]any{
			"compression": CompressionMethodZlib,
			"event":       compressedEvent,
		}
		if len(relevantEvents) > 0 {
			data["relevant_events"] = compressedRelevantEvents
		}

		switch event.GetTag("t").Value() {
		case model.DeviceTokenOSAndroid:
			notifications = append(notifications, &pn.Notification[*model.Event]{
				Target:      event,
				Data:        data,
				SourceEvent: incomingEvent,
			})
		default:
			notifications = append(notifications, &pn.Notification[*model.Event]{
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

func (pm *PushNotificationManager) collectUserDevices(pubKey string, remote bool, event *model.Event) (devices model.Events) {
	var candidates uint
	for deviceInfo := range pm.devicesFilterIndex.Lookup(event) {
		candidates++
		// If set, pubKey indicates that we should only consider devices registered with this public key.
		if pubKey != "" && deviceInfo.Event.GetMasterPublicKey() != pubKey && deviceInfo.Event.PubKey != pubKey {
			continue
		}

		if deviceInfo.Remote != remote {
			continue
		}

		if model.FiltersMatch(deviceInfo.Filters.Filters, event, "", "") {
			devices = append(devices, deviceInfo.Event)
		}
	}

	log.Trace().Str("context", "PUSH_NOTIFICATIONS").
		Str("pubkey", pubKey).
		Int("target_num_devices", len(devices)).
		Uint("candidate_num_devices", candidates).
		Str("event_id", event.ID).
		Bool("remote", remote).
		Msg("collected valid devices for user")

	return devices
}

func (pm *PushNotificationManager) collectRemoteDevices(pubKey string, event *model.Event) (devices model.Events) {
	return pm.collectUserDevices(pubKey, true, event)
}

func (pm *PushNotificationManager) collectLocalDevices(pubKey string, event *model.Event) (devices model.Events) {
	return pm.collectUserDevices(pubKey, false, event)
}

// collectTargetMasterKeys iterates through all registered user devices and identifies
// which master keys (users) have at least one device with filters matching the provided event.
// It returns a slice of master keys for users who should receive a notification for the event.
func (pm *PushNotificationManager) collectTargetMasterKeys(event *model.Event) (keys []string) {
	for _, device := range pm.collectLocalDevices("", event) {
		keys = append(keys, device.GetMasterPublicKey())
	}
	return model.DeduplicateStringSlice(keys)
}

func (pm *PushNotificationManager) handleEventWithPublicKey(event *model.Event, notificationType NotificationType, relevantEvents ...*model.Event) ([]*pn.Notification[*model.Event], error) {
	referencePubkey := event.GetTag("p").Value()
	if referencePubkey == "" || referencePubkey == event.GetMasterPublicKey() {
		return nil, nil
	}
	deviceEvents := pm.collectLocalDevices(referencePubkey, event)

	return pm.createNotifications(deviceEvents, notificationType, event, relevantEvents...)
}

func (pm *PushNotificationManager) handleQuoteEvent(event *model.Event, relevantEvents ...*model.Event) ([]*pn.Notification[*model.Event], error) {
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
	devices := pm.collectLocalDevices(referencePubkey, event)
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
	translation, ok := defaultTranslations[notificationType]
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

func (pm *PushNotificationManager) getAuthoritativeEvents(ctx context.Context, event *model.Event) (bool, *model.Event, *model.Event, error) {
	masterPubKey := event.GetMasterPublicKey()

	relayTag := model.TagMap{}.Set("r", &pm.relayURL)
	if u, err := url.Parse(pm.relayURL); err == nil && u.Port() != "" {
		u.Host = u.Hostname()
		relayTag = relayTag.Append("r", new(u.String()))
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
			return false, nil, nil, errors.Wrapf(err, "%v: failed to fetch authoritative events from storage", masterPubKey)
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

func collectRelaysFromDevice(ev *model.Event) []string {
	relays := make([]string, 0, len(ev.Tags))
	for _, tag := range ev.Tags {
		if tag.Key() == "relay" && tag.Value() != "" {
			relays = append(relays, tag.Value())
		}
	}
	return relays
}

func compactRelays(relays []string, filterOut ...string) []string {
	var compacted []string

	seen := make(map[string][]string)
	for _, relay := range relays {
		key := strings.ToLower(relay)
		if parsed, err := url.Parse(relay); err == nil {
			key = strings.ToLower(parsed.Hostname())
		}

		var isDuplicate bool
		for _, seenRelay := range seen[key] {
			if model.CompareRelaysURLs(relay, seenRelay) {
				isDuplicate = true
				break
			}
		}

		if !isDuplicate {
			var shouldFilterOut bool
			seen[key] = append(seen[key], relay)
			for _, v := range filterOut {
				if model.CompareRelaysURLs(relay, v) {
					shouldFilterOut = true
					break
				}
			}
			if !shouldFilterOut {
				compacted = append(compacted, relay)
			}
		}
	}

	return compacted
}
