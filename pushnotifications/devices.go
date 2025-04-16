// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

type deviceToRemove struct {
	deviceID     DeviceID
	masterPubKey PublicKey
}

func (pm *PushNotificationManager) syncDevicesRoutine() {
	incrementalTicker := time.NewTicker(1 * time.Minute)
	defer incrementalTicker.Stop()

	fullSyncTicker := time.NewTicker(24 * time.Hour)
	defer fullSyncTicker.Stop()

	ctx := context.Background()
	if err := pm.fullSyncDevices(ctx); err != nil {
		log.Printf("Error performing full device synchronization at startup: %v", err)
	}

	if err := pm.syncInvalidTokens(ctx); err != nil {
		log.Printf("Error synchronizing invalid tokens at startup: %v", err)
	}

	for {
		select {
		case <-incrementalTicker.C:
			ctx := context.Background()
			if err := pm.syncDevices(ctx); err != nil {
				log.Printf("Error performing incremental device synchronization: %v", err)
			}
		case <-fullSyncTicker.C:
			ctx := context.Background()
			if err := pm.fullSyncDevices(ctx); err != nil {
				log.Printf("Error performing full device synchronization: %v", err)
			}
			if err := pm.syncInvalidTokens(ctx); err != nil {
				log.Printf("Error synchronizing invalid tokens: %v", err)
			}
		}
	}
}

func (pm *PushNotificationManager) syncDevices(ctx context.Context) error {
	pm.deviceMutex.RLock()
	lastSyncTime := pm.lastSyncTime
	pm.deviceMutex.RUnlock()

	timestamp := nostr.Timestamp(lastSyncTime.Unix())
	eventIterator := query.GetStoredEvents(ctx, &model.Subscription{
		Filters: []model.Filter{{
			Kinds: []int{model.CustomIONKindDeviceRegistration},
			Since: &timestamp,
		}},
	})

	pm.deviceMutex.RLock()
	newDevices := make(map[DeviceID]DeviceInfo)
	for k, v := range pm.devices {
		newDevices[k] = v
	}

	newUserDevices := make(map[string][]DeviceID)
	for k, v := range pm.userDevices {
		newUserDevices[k] = make([]DeviceID, len(v))
		copy(newUserDevices[k], v)
	}

	newFilterToDevices := make(map[NotificationType]map[DeviceID]bool)
	for filterType, deviceMap := range pm.filterToDevices {
		newFilterToDevices[filterType] = make(map[DeviceID]bool)
		for deviceID, exists := range deviceMap {
			newFilterToDevices[filterType][deviceID] = exists
		}
	}
	pm.deviceMutex.RUnlock()

	for event, err := range eventIterator {
		if err != nil {
			return fmt.Errorf("error getting device registration events: %w", err)
		}

		if err := pm.processDeviceRegistrationEvent(event); err != nil {
			log.Printf("Error processing device registration event %s: %v", event.ID, err)

			continue
		}
	}

	pm.deviceMutex.Lock()
	pm.devices = newDevices
	pm.userDevices = newUserDevices
	pm.filterToDevices = newFilterToDevices
	pm.lastSyncTime = time.Now()
	pm.deviceMutex.Unlock()

	return nil
}

func (pm *PushNotificationManager) processDeviceRegistrationEvent(event *model.Event) error {
	if event.Kind != model.CustomIONKindDeviceRegistration {
		return nil
	}

	var deviceID DeviceID
	var platform, relayURL, encryptedToken string
	for _, tag := range event.Tags {
		if len(tag) >= 2 {
			switch tag.Key() {
			case "d":
				deviceID = DeviceID(tag.Value())
			case "t":
				platform = tag.Value()
			case "relay":
				relayURL = tag.Value()
			case "token":
				encryptedToken = tag.Value()
			}
		}
	}

	if deviceID == "" || encryptedToken == "" {
		return nil
	}

	var filters nostr.Filters
	err := json.Unmarshal([]byte(event.Content), &filters)
	if err != nil {
		return err
	}

	pm.deviceMutex.Lock()
	defer pm.deviceMutex.Unlock()

	deviceInfo := DeviceInfo{
		DeviceID:    deviceID,
		Platform:    platform,
		RelayURL:    relayURL,
		Filters:     filters,
		FCMToken:    encryptedToken,
		LastUpdated: time.Now(),
		PubKey:      event.GetMasterPublicKey(),
	}

	pm.devices[DeviceID(deviceID)] = deviceInfo

	if _, ok := pm.userDevices[event.GetMasterPublicKey()]; !ok {
		pm.userDevices[event.GetMasterPublicKey()] = []DeviceID{}
	}

	deviceExists := false
	for _, id := range pm.userDevices[event.GetMasterPublicKey()] {
		if id == DeviceID(deviceID) {
			deviceExists = true

			break
		}
	}

	if !deviceExists {
		pm.userDevices[event.GetMasterPublicKey()] = append(pm.userDevices[event.GetMasterPublicKey()], DeviceID(deviceID))
	}

	pm.categorizeDeviceByFilters(deviceID, filters)

	return nil
}

func (pm *PushNotificationManager) categorizeDeviceByFilters(deviceID DeviceID, filters nostr.Filters) {
	for _, deviceMap := range pm.filterToDevices {
		delete(deviceMap, DeviceID(deviceID))
	}

	for _, filter := range filters {
		if containsAny(filter.Kinds, []int{nostr.KindTextNote, model.CustomIONKindEditableTextNote}) {
			pm.filterToDevices[NotificationTypePost][DeviceID(deviceID)] = true
		}
		if containsAny(filter.Kinds, []int{nostr.KindChannelMessage}) {
			pm.filterToDevices[NotificationTypeChannelMessage][DeviceID(deviceID)] = true
		}
		if containsAny(filter.Kinds, []int{nostr.KindReaction}) {
			pm.filterToDevices[NotificationTypeReaction][DeviceID(deviceID)] = true
		}
		if containsAny(filter.Kinds, []int{nostr.KindRepost, nostr.KindGenericRepost}) {
			pm.filterToDevices[NotificationTypeRepost][DeviceID(deviceID)] = true
		}
		if containsAny(filter.Kinds, []int{nostr.KindGiftWrap}) {
			pm.filterToDevices[NotificationTypeDirectMessage][DeviceID(deviceID)] = true
		}
		if containsAny(filter.Kinds, []int{model.CustomIONKindFundSendNotify}) {
			pm.filterToDevices[NotificationTypePaymentRequest][DeviceID(deviceID)] = true
		}
		if containsAny(filter.Kinds, []int{model.CustomIONKindFundReceive}) {
			pm.filterToDevices[NotificationTypePaymentReceived][DeviceID(deviceID)] = true
		}
		if containsAny(filter.Kinds, []int{model.CustomIONSystemMessage}) {
			pm.filterToDevices[NotificationTypeSystem][DeviceID(deviceID)] = true
		}
		if containsAny(filter.Kinds, []int{nostr.KindTextNote, model.CustomIONKindEditableTextNote}) && filter.Tags.HasValues("p") {
			pm.filterToDevices[NotificationTypeMention][DeviceID(deviceID)] = true
		}
		if containsAny(filter.Kinds, []int{nostr.KindTextNote, model.CustomIONKindEditableTextNote}) && (filter.Tags.HasValues("e") || filter.Tags.HasValues("a")) {
			pm.filterToDevices[NotificationTypeReply][DeviceID(deviceID)] = true
		}
		if len(filter.Authors) > 0 && containsAny(filter.Kinds, []int{nostr.KindTextNote, model.CustomIONKindEditableTextNote}) {
			pm.filterToDevices[NotificationTypePost][DeviceID(deviceID)] = true
		}
	}
}

func containsAny(slice []int, values []int) bool {
	for _, item := range slice {
		for _, value := range values {
			if item == value {
				return true
			}
		}
	}
	return false
}

func (pm *PushNotificationManager) eventMatchesDeviceFilters(event *model.Event, filters nostr.Filters) bool {
	if len(filters) == 0 {
		return false
	}

	for _, filter := range filters {
		matches := true
		if len(filter.Kinds) > 0 {
			kindMatches := false
			for _, kind := range filter.Kinds {
				if kind == event.Kind {
					kindMatches = true
					break
				}
			}
			if !kindMatches {
				matches = false

				continue
			}
		}

		if len(filter.Authors) > 0 {
			authorMatches := false
			eventAuthor := event.GetMasterPublicKey()

			for _, author := range filter.Authors {
				if author == eventAuthor {
					authorMatches = true
					break
				}
			}

			if !authorMatches && event.PubKey != eventAuthor {
				for _, author := range filter.Authors {
					if author == event.PubKey {
						authorMatches = true
						break
					}
				}
			}

			if !authorMatches {
				matches = false

				continue
			}
		}

		if len(filter.Tags) > 0 {
			tagsMatch := true
			for tagName, tagValueSets := range filter.Tags {
				if len(tagValueSets) == 0 {
					continue
				}

				tagFound := false
				for _, tag := range event.Tags {
					if len(tag) < 2 || tag[0] != tagName[1:] {
						continue
					}

					for _, valueSet := range tagValueSets {
						if matchesTagValueSet(tag, valueSet) {
							tagFound = true

							break
						}
					}
					if tagFound {
						break
					}
				}
				if !tagFound {
					tagsMatch = false

					break
				}
			}

			if !tagsMatch {
				matches = false

				continue
			}
		}

		if filter.Since != nil && event.CreatedAt < *filter.Since {
			matches = false

			continue
		}

		if filter.Until != nil && event.CreatedAt > *filter.Until {
			matches = false

			continue
		}

		if matches {
			return true
		}
	}

	return false
}

func matchesTagValueSet(tag nostr.Tag, valueSet nostr.TagValues) bool {
	if len(valueSet) == 0 {
		return true
	}
	for _, value := range valueSet {
		if value == nil {
			continue
		}
		valueFound := false
		for i := 1; i < len(tag); i++ {
			if tag[i] == *value {
				valueFound = true
				break
			}
		}
		if !valueFound {
			return false
		}
	}

	return true
}

func (pm *PushNotificationManager) fullSyncDevices(ctx context.Context) error {
	timestamp := nostr.Timestamp(pm.lastSyncTime.Unix())
	eventIterator := query.GetStoredEvents(ctx, &model.Subscription{
		Filters: []model.Filter{{
			Kinds: []int{model.CustomIONKindDeviceRegistration},
			Since: &timestamp,
		}},
	})

	newDevices := make(map[DeviceID]DeviceInfo)
	newUserDevices := make(map[string][]DeviceID)
	newFilterToDevices := make(map[NotificationType]map[DeviceID]bool)

	for filterType := range pm.filterToDevices {
		newFilterToDevices[filterType] = make(map[DeviceID]bool)
	}
	for event, err := range eventIterator {
		if err != nil {
			return fmt.Errorf("error getting device registration events: %w", err)
		}

		if err = pm.processDeviceRegistrationEvent(event); err != nil {
			log.Printf("Error processing device registration event: %v", err)

			continue
		}
	}

	pm.deviceMutex.Lock()
	pm.devices = newDevices
	pm.userDevices = newUserDevices
	pm.filterToDevices = newFilterToDevices
	pm.lastSyncTime = time.Now()
	pm.deviceMutex.Unlock()

	log.Printf("Full device synchronization completed: %d devices", len(newDevices))

	return nil
}

func (pm *PushNotificationManager) syncInvalidTokens(ctx context.Context) error {
	invalidTokens, err := query.GetInvalidTokens(ctx)
	if err != nil {
		return fmt.Errorf("error getting invalid tokens: %w", err)
	}
	newInvalidTokens := make(map[DeviceID]InvalidTokenInfo)
	for _, info := range invalidTokens {
		newInvalidTokens[DeviceID(info.DeviceID)] = InvalidTokenInfo{
			DeviceID:     DeviceID(info.DeviceID),
			MasterPubKey: info.MasterPubKey,
			Token:        info.Token,
			CreatedAt:    info.CreatedAt,
		}
	}

	pm.deviceMutex.Lock()
	pm.invalidTokens = newInvalidTokens
	pm.deviceMutex.Unlock()

	return nil
}

func (pm *PushNotificationManager) markTokenAsInvalid(ctx context.Context, deviceID DeviceID, masterPubKey, token string) error {
	if err := query.MarkTokenAsInvalid(ctx, string(deviceID), masterPubKey, token); err != nil {
		return fmt.Errorf("error saving invalid token: %w", err)
	}

	pm.deviceMutex.Lock()
	pm.invalidTokens[DeviceID(deviceID)] = InvalidTokenInfo{
		DeviceID:     deviceID,
		MasterPubKey: masterPubKey,
		Token:        token,
		CreatedAt:    time.Now(),
	}
	pm.deviceMutex.Unlock()

	return nil
}

func (pm *PushNotificationManager) isTokenInvalid(deviceID DeviceID, token string) bool {
	pm.deviceMutex.RLock()
	defer pm.deviceMutex.RUnlock()

	invalidTokenInfo, exists := pm.invalidTokens[deviceID]
	if !exists {
		return false
	}

	return invalidTokenInfo.Token == token
}

func (pm *PushNotificationManager) RemoveDevice(ctx context.Context, deviceID DeviceID, masterPubKey PublicKey) error {
	pm.deviceMutex.Lock()
	defer pm.deviceMutex.Unlock()

	deviceInfo, exists := pm.devices[deviceID]
	if !exists {
		return nil
	}

	if deviceInfo.PubKey != masterPubKey {
		return fmt.Errorf("device belongs to another user")
	}

	for _, deviceMap := range pm.filterToDevices {
		delete(deviceMap, deviceID)
	}

	if devices, ok := pm.userDevices[masterPubKey]; ok {
		updatedDevices := make([]DeviceID, 0, len(devices))
		for _, id := range devices {
			if id != deviceID {
				updatedDevices = append(updatedDevices, id)
			}
		}
		pm.userDevices[masterPubKey] = updatedDevices
	}

	delete(pm.devices, deviceID)

	return nil
}

func (pm *PushNotificationManager) shouldProcessDeletionEvent(event *model.Event) bool {
	if event.Kind != nostr.KindDeletion {
		return false
	}

	kTags := event.GetTags("k")
	if len(kTags) == 0 {
		return true
	}

	for _, kTag := range kTags {
		kindStr := kTag.Value()
		if kind, err := strconv.Atoi(kindStr); err == nil && kind == model.CustomIONKindDeviceRegistration {
			return true
		}
	}

	return false
}

func (pm *PushNotificationManager) collectDevicesToRemove(ctx context.Context, events []*model.Event) (map[DeviceID]deviceToRemove, error) {
	deviceToRemoveMap := make(map[DeviceID]deviceToRemove)

	var eventIDs []string

	var deletionEvents []*model.Event
	for _, event := range events {
		if pm.shouldProcessDeletionEvent(event) {
			deletionEvents = append(deletionEvents, event)
		}
	}
	if len(deletionEvents) == 0 {
		return deviceToRemoveMap, nil
	}
	for _, event := range deletionEvents {
		for _, tag := range event.GetTags("e") {
			eventIDs = append(eventIDs, tag.Value())
		}
	}
	if len(eventIDs) > 0 {
		it := query.GetStoredEvents(ctx, &model.Subscription{
			Filters: []model.Filter{
				{
					IDs: eventIDs,
				},
			},
		})
		for event, err := range it {
			if err != nil {
				log.Printf("Error getting event %v: %v", event.ID, err)

				continue
			}

			if event.Kind == model.CustomIONKindDeviceRegistration {
				if dTag := event.GetTag("d"); dTag != nil {
					deviceID := DeviceID(dTag.Value())
					masterPubKey := event.GetMasterPublicKey()
					deviceToRemoveMap[deviceID] = deviceToRemove{
						deviceID:     deviceID,
						masterPubKey: masterPubKey,
					}
				}
			}
		}
	}

	return deviceToRemoveMap, nil
}

func (pm *PushNotificationManager) removeDevices(ctx context.Context, deviceToRemoveMap map[DeviceID]deviceToRemove) error {
	var errors []error
	for _, device := range deviceToRemoveMap {
		if err := pm.RemoveDevice(ctx, device.deviceID, device.masterPubKey); err != nil {
			errors = append(errors, err)

			log.Printf("Error when deleting device %s for user %s: %v",
				device.deviceID, device.masterPubKey, err)
		}
	}
	if len(errors) > 0 {
		return fmt.Errorf("errors occurred while processing deletion events: %v", errors)
	}

	return nil
}

func (pm *PushNotificationManager) ProcessDeletionEvents(ctx context.Context, events []*model.Event) error {
	if len(events) == 0 {
		return nil
	}
	deviceToRemoveMap, err := pm.collectDevicesToRemove(ctx, events)
	if err != nil {
		return err
	}

	return pm.removeDevices(ctx, deviceToRemoveMap)
}
