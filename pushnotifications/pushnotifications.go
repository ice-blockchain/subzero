// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/pushnotifications/internal"
)

type (
	DeviceID         = pn.DeviceID
	PublicKey        = string
	NotificationType string

	DeviceInfo struct {
		DeviceID    DeviceID
		Platform    string
		RelayURL    string
		FCMToken    string
		Filters     nostr.Filters
		LastUpdated time.Time
		PubKey      PublicKey
	}

	InvalidTokenInfo struct {
		DeviceID     DeviceID
		MasterPubKey PublicKey
		Token        string
		CreatedAt    time.Time
	}

	PushNotificationManager struct {
		devices                map[DeviceID]DeviceInfo
		userDevices            map[PublicKey][]DeviceID
		filterToDevices        map[NotificationType]map[DeviceID]bool
		invalidTokens          map[DeviceID]InvalidTokenInfo
		deviceMutex            sync.RWMutex
		invalidTokensMutex     sync.RWMutex
		lastSyncTime           time.Time
		translationMgr         *TranslationManager
		pushNotificationClient *pn.Client
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

func NewPushNotificationManager(fcmCredentialsPath string, translationsDir string) *PushNotificationManager {
	devices := make(map[DeviceID]DeviceInfo)
	userDevices := make(map[PublicKey][]DeviceID)
	filterToDevices := make(map[NotificationType]map[DeviceID]bool)
	invalidTokens := make(map[DeviceID]InvalidTokenInfo)

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

	translationMgr := NewTranslationManager(translationsDir)

	var pnClient pn.Client
	var err error
	if fcmCredentialsPath == "" {
		panic("FCM credentials file path is empty")
	}

	if _, err := os.Stat(fcmCredentialsPath); err != nil {
		panic("FCM credentials file not found, push notifications will be disabled")
	}
	pnClient, err = pn.New(context.Background(), fcmCredentialsPath, false)
	if err != nil {
		panic("Failed to create push notification client")
	}

	pm := &PushNotificationManager{
		devices:                devices,
		userDevices:            userDevices,
		filterToDevices:        filterToDevices,
		invalidTokens:          invalidTokens,
		lastSyncTime:           time.Now(),
		translationMgr:         translationMgr,
		pushNotificationClient: &pnClient,
	}

	go pm.syncDevicesRoutine()
	go pm.startCleanupRoutine()

	return pm
}

func truncateContent(content string, maxLength int) string {
	if len(content) <= maxLength {
		return content
	}
	return content[:maxLength] + "..."
}

func (pm *PushNotificationManager) startCleanupRoutine() {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		ctx := context.Background()
		if err := pm.cleanupOldInvalidTokens(ctx); err != nil {
			log.Println("Error cleaning up outdated invalid tokens", err)
		}
	}
}

func (pm *PushNotificationManager) cleanupOldInvalidTokens(ctx context.Context) error {
	if err := query.CleanupOldInvalidTokens(ctx); err != nil {
		return fmt.Errorf("error cleaning up outdated invalid tokens: %w", err)
	}

	return pm.syncInvalidTokens(ctx)
}

func (pm *PushNotificationManager) collectValidDevices(pubKey PublicKey, notificationType NotificationType, event *model.Event) []struct {
	deviceID DeviceID
	token    string
} {
	pm.deviceMutex.RLock()
	deviceIDs, ok := pm.userDevices[pubKey]
	pm.deviceMutex.RUnlock()

	if !ok || len(deviceIDs) == 0 {
		return nil
	}

	var validDevices []struct {
		deviceID DeviceID
		token    string
	}

	pm.deviceMutex.RLock()
	defer pm.deviceMutex.RUnlock()

	for _, deviceID := range deviceIDs {
		deviceInfo, exists := pm.devices[deviceID]
		if !exists {
			continue
		}

		if pm.isTokenInvalid(deviceID, deviceInfo.FCMToken) {
			continue
		}

		if _, hasFilter := pm.filterToDevices[notificationType][deviceID]; !hasFilter {
			continue
		}

		if pm.eventMatchesDeviceFilters(event, deviceInfo.Filters) {
			validDevices = append(validDevices, struct {
				deviceID DeviceID
				token    string
			}{
				deviceID: deviceID,
				token:    deviceInfo.FCMToken,
			})
		}
	}

	return validDevices
}

func (pm *PushNotificationManager) addNotificationsToDevices(devices []struct {
	deviceID DeviceID
	token    string
}, title, body, imageURL string, data map[string]interface{}) *NotificationBatch {
	if len(devices) == 0 {
		return nil
	}

	notifications := &NotificationBatch{
		singleNotifications:    make([]*pn.Notification[pn.DeviceToken], 0),
		multicastNotifications: make([]*pn.Notification[pn.DeviceTokens], 0),
	}

	if len(devices) == 1 {
		notifications.singleNotifications = append(notifications.singleNotifications, &pn.Notification[pn.DeviceToken]{
			Title:    title,
			Body:     body,
			Data:     data,
			ImageURL: imageURL,
			Target: pn.DeviceToken{
				Token:    devices[0].token,
				DeviceID: devices[0].deviceID,
			},
		})
	} else {
		tokens := make([]pn.DeviceToken, 0, len(devices))
		for _, device := range devices {
			tokens = append(tokens, pn.DeviceToken{
				Token:    device.token,
				DeviceID: device.deviceID,
			})
		}

		notifications.multicastNotifications = append(notifications.multicastNotifications, &pn.Notification[pn.DeviceTokens]{
			Title:    title,
			Body:     body,
			Data:     data,
			ImageURL: imageURL,
			Target:   tokens,
		})
	}

	return notifications
}
