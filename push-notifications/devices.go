// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"cmp"
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"
	"github.com/rs/zerolog/log"
	"github.com/zeebo/xxh3"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

type (
	deviceInfo struct {
		Event   *model.Event
		Filters model.FiltersWithEvents
		Remote  bool
	}
)

func calcDeviceKey(event *model.Event) string {
	return cmp.Or(event.Tags.GetD(), event.ID)
}

func (d *deviceInfo) Hash() uint64 {
	return xxh3.HashString(calcDeviceKey(d.Event))
}

func (pm *PushNotificationManager) syncDevices(ctx context.Context) error {
	for event, err := range query.CollectDeviceRegistrationEvents(ctx) {
		if err != nil {
			return fmt.Errorf("error getting device registration events: %w", err)
		}

		if err := pm.processDeviceRegistrationEvent(ctx, event); err != nil {
			log.Error().Str("context", "PUSH_NOTIFICATIONS").
				Err(err).
				Str("event_id", event.ID).
				Msg("error processing device registration event")
			continue
		}
	}

	log.Info().
		Str("context", "PUSH_NOTIFICATIONS").
		Int("total_devices", pm.devicesFilterIndex.Size()).
		Msg("device synchronization completed")

	return nil
}

func notificationTarget(e *model.Event) (masterPubKey string, deviceID string, remote bool) {
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

func filterDevices(devices model.Events, event *model.Event, fn func(deviceEvent, event *model.Event) (keep bool)) (matchedDevices model.Events) {
	for _, deviceEvent := range devices {
		if fn(deviceEvent, event) {
			matchedDevices = append(matchedDevices, deviceEvent)
		}
	}
	return matchedDevices
}

func (pm *PushNotificationManager) processDeviceRegistrationEvent(ctx context.Context, event *model.Event) error {
	masterPubKey, deviceID, remote := notificationTarget(event)

	if masterPubKey == "" || deviceID == "" {
		return fmt.Errorf("invalid device registration event: missing master public key %q or device ID %q in tags", masterPubKey, deviceID)
	}

	var filters model.FiltersWithEvents
	if err := filters.UnmarshalJSON([]byte(event.Content)); err != nil {
		return errors.Wrap(err, "failed to unmarshal device filters")
	}

	deviceInfo := deviceInfo{
		Filters: filters,
		Event:   event,
		Remote:  remote,
	}

	// Remove old device info from cache if exists to avoid duplicates and stale data.
	pm.removeDeviceFromCache(ctx, event)

	// If the relay URL in the event doesn't match the manager's relay URL for local device,
	// It means the device registration event was created on another relay
	// And we should just delete the device if it exists in the cache without adding the new one, since we won't be able to send notifications to it.
	if !remote && !model.CompareRelaysURLs(event.GetTag("relay").Value(), pm.relayURL) {
		return nil
	}

	deviceKey := calcDeviceKey(event)
	if !remote {
		for _, ev := range filters.Data {
			switch ev.Kind {
			case model.CustomIONKindDVMJobRequestPriceChange:
				log.Trace().Str("context", "PUSH_NOTIFICATIONS").
					Str("event_id", event.ID).
					Str("job_request_event_id", ev.ID).
					Msg("device registration event is linked to a price change job request")

				err := query.RegisterPriceChangeSubscriber(ctx, deviceKey, ev)
				if err != nil {
					return errors.Wrapf(err, "%v: failed to register price change subscriber", ev.ID)
				}

			default:
				log.Debug().Str("context", "PUSH_NOTIFICATIONS").
					Str("event_id", event.ID).
					Int("linked_event_kind", ev.Kind).
					Msg("unexpected event kind in device registration filters")
			}
		}
	}

	indexKeys := []deviceIndexKey{
		{Key: deviceIndexKeyTypeEventID, Value: event.ID},
		{Key: deviceIndexKeyTypeDeviceKey, Value: deviceKey},
	}
	for _, key := range indexKeys {
		pm.devicesEventMap.Store(key, event)
	}

	pm.devicesFilterIndex.Index(filters.Filters, &deviceInfo)

	return nil
}

// removeDeviceFromCache removes the device registration event from the cache.
// `event` could be either the new device registration event that is being processed or the old one that is being removed, since both events will have the same device key (d-tag or event ID).
func (pm *PushNotificationManager) removeDeviceFromCache(ctx context.Context, event *model.Event) {
	deviceKey := calcDeviceKey(event)
	pm.devicesEventMap.Delete(deviceIndexKey{Key: deviceIndexKeyTypeEventID, Value: event.ID})
	oldEvent, ok := pm.devicesEventMap.LoadAndDelete(deviceIndexKey{Key: deviceIndexKeyTypeDeviceKey, Value: deviceKey})
	if !ok || oldEvent == nil {
		// No old event found for the device key, nothing to remove from cache.
		return
	}

	pm.devicesEventMap.Delete(deviceIndexKey{Key: deviceIndexKeyTypeEventID, Value: oldEvent.ID})
	d := deviceInfo{Event: oldEvent}
	di, ok := pm.devicesFilterIndex.RemoveByHash(d.Hash())
	if ok && di != nil && !di.Remote {
		for _, ev := range di.Filters.Data {
			var err error
			switch ev.Kind {
			case model.CustomIONKindDVMJobRequestPriceChange:
				err = query.DeletePriceChangeSubscriber(ctx, di.Event.PubKey, deviceKey, "", ev.ID)
			}
			if err != nil {
				log.Error().
					Str("context", "PUSH_NOTIFICATIONS").
					Err(err).
					Str("event_id", event.ID).
					Str("linked_event_id", ev.ID).
					Msg("failed to delete price change subscriber linked to removed device")
			}
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

func (pm *PushNotificationManager) removeDevicesIfAny(ctx context.Context, events []*model.Event) {
	var deletionEvents model.Events

	for _, event := range events {
		if pm.shouldProcessDeletionEvent(event) {
			deletionEvents = append(deletionEvents, event)
		}
	}

	var eventIDs []string
	for _, event := range deletionEvents {
		for _, tag := range event.Tags {
			if tag.Key() == "e" {
				eventIDs = append(eventIDs, tag.Value())
			}
		}
	}

	for _, eventID := range eventIDs {
		regEvent, ok := pm.devicesEventMap.Load(deviceIndexKey{Key: deviceIndexKeyTypeEventID, Value: eventID})
		if ok && regEvent != nil {
			log.Trace().
				Str("context", "PUSH_NOTIFICATIONS").
				Str("event_id", eventID).
				Str("device_pubkey", regEvent.PubKey).
				Msg("processing deletion event for device registration")
			pm.removeDeviceFromCache(ctx, regEvent)
		}
	}
}

func (pm *PushNotificationManager) ManageDeviceRegistrationEvents(ctx context.Context, events model.Events) error {
	if len(events) == 0 {
		return nil
	}

	pm.removeDevicesIfAny(ctx, events)

	var errs error
	for _, event := range events {
		if event.Kind == model.CustomIONKindDeviceRegistration {
			errs = errors.Join(errs, errors.Wrapf(pm.processDeviceRegistrationEvent(ctx, event), "error on processing device registration event: %v", event.ID))
		}
	}

	return errors.Wrap(errs, "error on processing device registration events")
}

func (pm *PushNotificationManager) removeInvalidTokenDevicesFromCache(ctx context.Context, deviceEvents model.Events) {
	for _, deviceEvent := range deviceEvents {
		pm.removeDeviceFromCache(ctx, deviceEvent)
	}
}
