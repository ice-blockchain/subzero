// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"
	"github.com/rs/zerolog/log"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

type (
	DeviceInfo struct {
		Event   *model.Event  // Original device registration event, used for extracting tags and content.
		Filters model.Filters // Parsed filters from the event content, defining what notifications the device is subscribed to.
		Remote  bool          // Indicates if the device is registered on a different relay (true) or the same relay (false).
	}
)

func (pm *PushNotificationManager) syncDevices(ctx context.Context) error {
	var totalDevices int
	for event, err := range query.CollectDeviceRegistrationEvents(ctx) {
		if err != nil {
			return fmt.Errorf("error getting device registration events: %w", err)
		}

		if err := pm.processDeviceRegistrationEvent(event); err != nil {
			log.Error().Str("context", "PUSH-NOTIFICATIONS").
				Err(err).
				Str("event_id", event.ID).
				Msg("error processing device registration event")
			continue
		}
	}

	for _, devices := range pm.userDevicesMap {
		totalDevices += len(devices)
	}

	log.Info().
		Str("context", "PUSH-NOTIFICATIONS").
		Int("total_users", len(pm.userDevicesMap)).
		Int("total_devices", totalDevices).
		Msg("device synchronization completed")

	return nil
}

func deviceNotificationTarget(e *model.Event) (masterPubKey string, deviceID string, remote bool) {
	dTag := e.Tags.GetD()
	masterPubKey, deviceID, remote = e.GetMasterPublicKey(), dTag, false

	// For remote devices, the d-tag format is: <masterPubKey>_<deviceID>.
	if strings.Contains(dTag, "_") {
		parts := strings.SplitN(dTag, "_", 2)
		masterPubKey, deviceID = parts[0], parts[1]
		remote = true
	}

	return masterPubKey, deviceID, remote
}

func (pm *PushNotificationManager) processDeviceRegistrationEvent(event *model.Event) error {
	masterPubKey, deviceID, remote := deviceNotificationTarget(event)

	if masterPubKey == "" || deviceID == "" {
		return fmt.Errorf("invalid device registration event: missing master public key %q or device ID %q in tags", masterPubKey, deviceID)
	}

	var filters model.Filters
	if err := json.Unmarshal([]byte(event.Content), &filters); err != nil {
		return errors.Wrap(err, "failed to unmarshal device filters")
	}

	deviceInfo := DeviceInfo{
		Filters: filters,
		Event:   event,
		Remote:  remote,
	}

	// If the relay URL in the event doesn't match the manager's relay URL for local device,
	// It means the device registration event was created on another relay
	// And we should just delete the device if it exists in the cache without adding the new one, since we won't be able to send notifications to it.
	shouldDeleteFromCache := !remote && !model.CompareRelaysURLs(event.GetTag("relay").Value(), pm.relayURL)

	pm.deviceMutex.Lock()
	defer pm.deviceMutex.Unlock()

	if shouldDeleteFromCache {
		devicesByUser, ok := pm.userDevicesMap[masterPubKey]
		if ok {
			if _, exists := devicesByUser[deviceID]; exists {
				delete(devicesByUser, deviceID)
				if len(devicesByUser) == 0 {
					delete(pm.userDevicesMap, masterPubKey)
				}
			}
		}
		return nil
	}

	if _, ok := pm.userDevicesMap[masterPubKey]; !ok {
		pm.userDevicesMap[masterPubKey] = make(map[string]DeviceInfo)
	}
	pm.userDevicesMap[masterPubKey][deviceID] = deviceInfo

	return nil
}

func (pm *PushNotificationManager) removeDeviceFromCache(event *model.Event) {
	masterPubKey, deviceID, _ := deviceNotificationTarget(event)

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
	var kCount int

	if event.Kind != nostr.KindDeletion {
		return false
	}

	expectedKValue := strconv.Itoa(model.CustomIONKindDeviceRegistration)
	for _, tag := range event.Tags {
		if tag.Key() != "k" {
			continue
		}
		kCount++

		if tag.Value() == expectedKValue {
			return true
		}
	}
	return kCount == 0 // If there are no 'k' tags, we treat it as a deletion of all kinds, including device registrations.
}

func (pm *PushNotificationManager) removeDevicesIfAny(ctx context.Context, events []*model.Event) error {
	var deletionEvents []*model.Event
	for _, event := range events {
		if pm.shouldProcessDeletionEvent(event) {
			deletionEvents = append(deletionEvents, event)
		}
	}

	if len(deletionEvents) == 0 {
		return nil
	}

	var eventIDs []string
	for _, event := range deletionEvents {
		for _, tag := range event.GetTags("e") {
			eventIDs = append(eventIDs, tag.Value())
		}
	}

	if len(eventIDs) == 0 {
		return nil
	}

	// TODO: rebuild devices index to include device registration events ids.
	for event, err := range query.GetStoredEvents(ctx, model.Filter{IDs: eventIDs}) {
		if err != nil {
			return errors.Wrap(err, "error getting event")
		}
		if event.Kind == model.CustomIONKindDeviceRegistration {
			pm.removeDeviceFromCache(event)
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

func (pm *PushNotificationManager) removeInvalidTokenDevicesFromCache(deviceEvents model.Events) {
	for _, deviceEvent := range deviceEvents {
		pm.removeDeviceFromCache(deviceEvent)
	}
}
