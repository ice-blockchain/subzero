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
		Filters  model.Filters
		Event    *model.Event
		DeviceID DeviceID
	}

	deviceToRemove struct {
		deviceID     DeviceID
		masterPubKey PublicKey
	}
)

func (pm *PushNotificationManager) syncDevices(ctx context.Context) error {
	events, err := query.CollectDeviceRegistrationEvents(ctx)
	if err != nil {
		return fmt.Errorf("error getting device registration events: %w", err)
	}

	for _, event := range events {
		if err := pm.processDeviceRegistrationEvent(event); err != nil {
			log.Printf("Error processing device registration event %s: %v", event.ID, err)

			continue
		}
	}

	var totalDevices int
	for _, devices := range pm.userDevicesMap {
		totalDevices += len(devices)
	}

	log.Printf("Device synchronization completed: %d devices", totalDevices)

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

	masterPubKey := event.GetMasterPublicKey()

	if _, ok := pm.userDevicesMap[masterPubKey]; !ok {
		pm.userDevicesMap[masterPubKey] = make(map[DeviceID]DeviceInfo)
	}

	pm.userDevicesMap[masterPubKey][deviceID] = deviceInfo

	return nil
}

func (pm *PushNotificationManager) RemoveDevice(ctx context.Context, deviceID DeviceID, masterPubKey PublicKey) error {
	pm.deviceMutex.Lock()
	defer pm.deviceMutex.Unlock()

	if devices, ok := pm.userDevicesMap[masterPubKey]; ok {
		deviceInfo, exists := devices[deviceID]
		if !exists {
			return nil
		}

		if deviceInfo.Event.GetMasterPublicKey() != masterPubKey {
			return fmt.Errorf("device belongs to another user")
		}

		delete(pm.userDevicesMap[masterPubKey], deviceID)

		if len(pm.userDevicesMap[masterPubKey]) == 0 {
			delete(pm.userDevicesMap, masterPubKey)
		}
	}

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
	var errs error
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
				return errors.Wrap(err, "error getting event")
			}
			if event.Kind == model.CustomIONKindDeviceRegistration {
				if err := pm.RemoveDevice(ctx, DeviceID(event.Tags.GetD()), event.GetMasterPublicKey()); err != nil {
					errs = errors.Join(errs, errors.Wrapf(err, "error when deleting device: %v", event.Tags.GetD()))
				}
			}
		}
	}

	return errors.Wrap(errs, "error on removing devices")
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
