// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"slices"
	"strconv"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
)

type (
	DeviceID   = pn.DeviceID
	DeviceInfo struct {
		Filters  model.Filters
		Event    *model.Event
		DeviceID DeviceID
	}
)

func (pm *PushNotificationManager) syncDevices(ctx context.Context) error {
	var totalDevices int
	for event, err := range query.CollectDeviceRegistrationEvents(ctx) {
		if err != nil {
			return fmt.Errorf("error getting device registration events: %w", err)
		}

		if err := pm.processDeviceRegistrationEvent(event); err != nil {
			log.Printf("Error processing device registration event %s: %v", event.ID, err)
			continue
		}
	}

	for _, devices := range pm.userDevicesMap {
		totalDevices += len(devices)
	}

	log.Printf("Device synchronization completed: %d devices", totalDevices)

	return nil
}

func (pm *PushNotificationManager) processDeviceRegistrationEvent(event *model.Event) error {
	if event.GetTag("relay").Value() != pm.relayURL {
		return nil
	}
	deviceID := DeviceID(event.Tags.GetD())

	var filters nostr.Filters
	if err := json.Unmarshal([]byte(event.Content), &filters); err != nil {
		return errors.Wrap(err, "failed to unmarshal device filters")
	}

	deviceInfo := DeviceInfo{
		DeviceID: deviceID,
		Filters:  filters,
		Event:    event,
	}

	pm.deviceMutex.Lock()
	defer pm.deviceMutex.Unlock()

	masterPubKey := event.GetMasterPublicKey()

	if _, ok := pm.userDevicesMap[masterPubKey]; !ok {
		pm.userDevicesMap[masterPubKey] = make(map[DeviceID]DeviceInfo)
	}

	pm.userDevicesMap[masterPubKey][deviceID] = deviceInfo

	return nil
}

func (pm *PushNotificationManager) removeDeviceFromCache(deviceID DeviceID, masterPubKey PublicKey) {
	pm.deviceMutex.Lock()
	defer pm.deviceMutex.Unlock()

	if _, ok := pm.userDevicesMap[masterPubKey]; ok {
		delete(pm.userDevicesMap[masterPubKey], deviceID)

		if len(pm.userDevicesMap[masterPubKey]) == 0 {
			delete(pm.userDevicesMap, masterPubKey)
		}
	}
}

func (pm *PushNotificationManager) shouldProcessDeletionEvent(event *model.Event) bool {
	if event.Kind != nostr.KindDeletion {
		return false
	}
	kTags := event.GetTags("k")
	if len(kTags) == 0 {
		return true
	}
	idx := slices.IndexFunc(kTags, func(kTag nostr.Tag) bool {
		kindStr := kTag.Value()
		kind, err := strconv.Atoi(kindStr)
		return err == nil && kind == model.CustomIONKindDeviceRegistration
	})
	if idx == -1 {
		return false
	}

	return true
}

func (pm *PushNotificationManager) removeDevicesIfAny(ctx context.Context, events []*model.Event) error {
	var eventIDs []string
	var deletionEvents []*model.Event
	for _, event := range events {
		if pm.shouldProcessDeletionEvent(event) {
			deletionEvents = append(deletionEvents, event)
		}
	}
	if len(deletionEvents) == 0 {
		return nil
	}
	for _, event := range deletionEvents {
		for _, tag := range event.GetTags("e") {
			eventIDs = append(eventIDs, tag.Value())
		}
	}
	if len(eventIDs) > 0 {
		for event, err := range query.GetStoredEvents(ctx, model.Filter{IDs: eventIDs}) {
			if err != nil {
				return errors.Wrap(err, "error getting event")
			}
			if event.Kind == model.CustomIONKindDeviceRegistration {
				pm.deviceMutex.RLock()
				deviceInfo, exists := pm.userDevicesMap[event.GetMasterPublicKey()][DeviceID(event.Tags.GetD())]
				pm.deviceMutex.RUnlock()
				if !exists {
					return nil
				}
				if deviceInfo.Event.GetMasterPublicKey() != event.GetMasterPublicKey() {
					return fmt.Errorf("device belongs to another user")
				}

				pm.removeDeviceFromCache(DeviceID(event.Tags.GetD()), event.GetMasterPublicKey())
			}
		}
	}

	return nil
}

func (pm *PushNotificationManager) ManageDeviceRegistrationEvents(ctx context.Context, events []*model.Event) error {
	if len(events) == 0 {
		return nil
	}
	if err := pm.removeDevicesIfAny(ctx, events); err != nil {
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

func (pm *PushNotificationManager) removeInvalidTokenDevicesFromCache(deviceEvents []*DeviceRegistrationEvent) {
	for _, deviceEvent := range deviceEvents {
		pm.removeDeviceFromCache(DeviceID(deviceEvent.Tags.GetD()), deviceEvent.GetMasterPublicKey())
	}
}
