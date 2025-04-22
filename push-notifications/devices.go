// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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
		Filters  nostr.Filters
		Event    *model.Event
		DeviceID DeviceID
	}

	deviceToRemove struct {
		deviceID     DeviceID
		masterPubKey PublicKey
	}
)

func (pm *PushNotificationManager) syncDevices(ctx context.Context) error {
	eventIterator := query.GetStoredEvents(ctx, &model.Subscription{
		Filters: []model.Filter{{
			Kinds: []int{model.CustomIONKindDeviceRegistration},
		}},
	})
	for event, err := range eventIterator {
		if err != nil {
			return fmt.Errorf("error getting device registration events: %w", err)
		}
		if err := pm.processDeviceRegistrationEvent(event); err != nil {
			log.Printf("Error processing device registration event %s: %v", event.ID, err)

			continue
		}
	}
	log.Printf("Device synchronization completed: %d devices", len(pm.devices))

	return nil
}

func (pm *PushNotificationManager) processDeviceRegistrationEvent(event *model.Event) error {
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

	pm.devices[deviceID] = deviceInfo

	masterPubKey := event.GetMasterPublicKey()
	if _, ok := pm.userDevices[masterPubKey]; !ok {
		pm.userDevices[masterPubKey] = []DeviceID{}
	}
	deviceExists := false
	for _, id := range pm.userDevices[masterPubKey] {
		if id == deviceID {
			deviceExists = true

			break
		}
	}
	if !deviceExists {
		pm.userDevices[masterPubKey] = append(pm.userDevices[masterPubKey], deviceID)
	}

	return nil
}

func (pm *PushNotificationManager) RemoveDevice(ctx context.Context, deviceID DeviceID, masterPubKey PublicKey) error {
	pm.deviceMutex.Lock()
	defer pm.deviceMutex.Unlock()

	deviceInfo, exists := pm.devices[deviceID]
	if !exists {
		return nil
	}
	if deviceInfo.Event.GetMasterPublicKey() != masterPubKey {
		return fmt.Errorf("device belongs to another user")
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
