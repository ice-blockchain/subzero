// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"slices"
	"strconv"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

type deviceToRemove struct {
	deviceID     DeviceID
	masterPubKey PublicKey
}

func (pm *PushNotificationManager) syncDevices(ctx context.Context) error {
	eventIterator := query.GetStoredEvents(ctx, &model.Subscription{
		Filters: []model.Filter{{
			Kinds: []int{model.CustomIONKindDeviceRegistration},
		}},
	})

	pm.deviceMutex.RLock()

	newDevices := maps.Clone(pm.devices)

	newUserDevices := make(map[string][]DeviceID)
	for k, v := range pm.userDevices {
		newUserDevices[k] = make([]DeviceID, len(v))
		copy(newUserDevices[k], v)
	}

	newFilterToDevices := make(map[NotificationType]map[DeviceID]bool)
	for filterType, deviceMap := range pm.filterToDevices {
		newFilterToDevices[filterType] = maps.Clone(deviceMap)
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
	pm.deviceMutex.Unlock()

	return nil
}

func (pm *PushNotificationManager) processDeviceRegistrationEvent(event *model.Event) error {
	if event.Kind != model.CustomIONKindDeviceRegistration {
		return nil
	}
	var deviceID DeviceID
	var platform, relayURL, encryptedToken string
	var invalid bool

	for _, tag := range event.Tags {
		switch tag.Key() {
		case "d":
			deviceID = DeviceID(tag.Value())
		case "t":
			platform = tag.Value()
		case "relay":
			relayURL = tag.Value()
		case "token":
			encryptedToken = tag.Value()
			if len(tag) > 2 && tag[2] == "invalid" {
				invalid = true
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
		DeviceID:                  deviceID,
		Platform:                  platform,
		RelayURL:                  relayURL,
		Filters:                   filters,
		FCMToken:                  encryptedToken,
		PubKey:                    event.GetMasterPublicKey(),
		Invalid:                   invalid,
		DeviceRegistrationEventID: event.ID,
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
		if slices.ContainsFunc(filter.Kinds, func(kind int) bool { return kind == nostr.KindTextNote || kind == model.CustomIONKindEditableTextNote }) {
			pm.filterToDevices[NotificationTypePost][DeviceID(deviceID)] = true
		}
		if slices.ContainsFunc(filter.Kinds, func(kind int) bool { return kind == nostr.KindChannelMessage }) {
			pm.filterToDevices[NotificationTypeChannelMessage][DeviceID(deviceID)] = true
		}
		if slices.ContainsFunc(filter.Kinds, func(kind int) bool { return kind == nostr.KindReaction }) {
			pm.filterToDevices[NotificationTypeReaction][DeviceID(deviceID)] = true
		}
		if slices.ContainsFunc(filter.Kinds, func(kind int) bool { return kind == nostr.KindRepost || kind == nostr.KindGenericRepost }) {
			pm.filterToDevices[NotificationTypeRepost][DeviceID(deviceID)] = true
		}
		if slices.ContainsFunc(filter.Kinds, func(kind int) bool { return kind == nostr.KindGiftWrap }) {
			pm.filterToDevices[NotificationTypeDirectMessage][DeviceID(deviceID)] = true
		}
		if slices.ContainsFunc(filter.Kinds, func(kind int) bool { return kind == model.CustomIONKindFundSendNotify }) {
			pm.filterToDevices[NotificationTypePaymentRequest][DeviceID(deviceID)] = true
		}
		if slices.ContainsFunc(filter.Kinds, func(kind int) bool { return kind == model.CustomIONKindFundReceive }) {
			pm.filterToDevices[NotificationTypePaymentReceived][DeviceID(deviceID)] = true
		}
		if slices.ContainsFunc(filter.Kinds, func(kind int) bool { return kind == model.CustomIONSystemMessage }) {
			pm.filterToDevices[NotificationTypeSystem][DeviceID(deviceID)] = true
		}
		if slices.ContainsFunc(filter.Kinds, func(kind int) bool {
			return kind == nostr.KindTextNote || kind == model.CustomIONKindEditableTextNote
		}) && filter.Tags.HasValues("p") {
			pm.filterToDevices[NotificationTypeMention][DeviceID(deviceID)] = true
		}
		if (slices.ContainsFunc(filter.Kinds, func(kind int) bool { return kind == nostr.KindTextNote || kind == model.CustomIONKindEditableTextNote }) && filter.Tags.HasValues("e") || filter.Tags.HasValues("a")) ||
			(slices.ContainsFunc(filter.Kinds, func(kind int) bool { return kind == model.CustomIONKindEditableTextNote }) && filter.Tags.HasValues("q") && filter.Tags.HasValues("Q")) {
			pm.filterToDevices[NotificationTypeReply][DeviceID(deviceID)] = true
		}
		if len(filter.Authors) > 0 && slices.ContainsFunc(filter.Kinds, func(kind int) bool { return kind == nostr.KindTextNote || kind == model.CustomIONKindEditableTextNote }) {
			pm.filterToDevices[NotificationTypePost][DeviceID(deviceID)] = true
		}
	}
}

func (pm *PushNotificationManager) fullSyncDevices(ctx context.Context) error {
	eventIterator := query.GetStoredEvents(ctx, &model.Subscription{
		Filters: []model.Filter{{
			Kinds: []int{model.CustomIONKindDeviceRegistration},
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
	pm.deviceMutex.Unlock()

	log.Printf("Full device synchronization completed: %d devices", len(newDevices))

	return nil
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
	var eventIDs []string
	var deletionEvents []*model.Event
	for _, event := range events {
		if pm.shouldProcessDeletionEvent(event) {
			deletionEvents = append(deletionEvents, event)
		}
	}
	if len(deletionEvents) == 0 {
		return nil, nil
	}
	for _, event := range deletionEvents {
		for _, tag := range event.GetTags("e") {
			eventIDs = append(eventIDs, tag.Value())
		}
	}
	deviceToRemoveMap := make(map[DeviceID]deviceToRemove)
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

				return nil, errors.Wrap(err, "error getting event")
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
	var errs error
	for _, device := range deviceToRemoveMap {
		if err := pm.RemoveDevice(ctx, device.deviceID, device.masterPubKey); err != nil {
			errs = errors.Join(errs, errors.Wrapf(err, "error when deleting device: %v", device.deviceID))
		}
	}

	return errs
}

func (pm *PushNotificationManager) ProcessDeviceRegistrationEvents(ctx context.Context, events []*model.Event) error {
	if len(events) == 0 {
		return nil
	}
	deviceToRemoveMap, err := pm.collectDevicesToRemove(ctx, events)
	if err != nil {
		return err
	}
	if err := pm.removeDevices(ctx, deviceToRemoveMap); err != nil {
		return err
	}
	var errs error
	for _, event := range events {
		if event.Kind == model.CustomIONKindDeviceRegistration {
			errs = errors.Join(errs, errors.Wrapf(pm.processDeviceRegistrationEvent(event), "error on processing device registration event: %v", event.ID))
		}
	}

	return errors.Wrap(errs, "error on processing device registration events")
}
