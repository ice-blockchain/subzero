// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"context"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"
	"github.com/rs/zerolog/log"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
)

func (pm *PushNotificationManager) handleTokenizedCommunityAction(ctx context.Context, userDevice, event *model.Event) (NotificationType, error) {
	var isCreator bool
	var ownerMasterKey string // The master key of the token owner (author of the original 31175 event).

	targetAddressTag := event.GetTag("a").Value()
	if targetAddressTag == "" {
		eventID := event.GetTag("e").Value()
		if eventID == "" {
			log.Debug().Str("event", event.String()).Msg("no address or event tag found for tokenized community action")
			return "", nil
		}

		for ev, err := range query.GetStoredEvents(ctx, model.Filter{
			IDs:   []string{eventID},
			Kinds: []int{model.CustomIONKindTokenizedCommunityDefinition},
			Limit: 1,
		}) {
			if err != nil {
				return "", errors.Wrapf(err, "failed to get linked event %q for tokenized community action", eventID)
			}
			ownerMasterKey = ev.GetMasterPublicKey()
			isCreator = ev.GetTag("k").Value() == strconv.Itoa(nostr.KindProfileMetadata)
			break
		}
	} else {
		// Address tag, use master key part directly (kind:pubkey:).
		parts := strings.Split(targetAddressTag, ":")
		if len(parts) >= 2 {
			ownerMasterKey = parts[1]
			isCreator = parts[0] == strconv.Itoa(nostr.KindProfileMetadata)
		}
	}

	txType := event.GetTag("tx_type").Value()
	if txType != "buy" {
		return "", nil // Not a buy action, skipping.
	}

	userMasterKey := userDevice.GetMasterPublicKey()

	if userMasterKey == "" || ownerMasterKey == "" {
		log.Trace().
			Str("context", "PUSH_NOTIFICATION").
			Str("user_master_key", userMasterKey).
			Str("owner_master_key", ownerMasterKey).
			Str("event_id", event.ID).
			Msg("missing user or owner master key for tokenized community action, skipping notification")
		return "", nil
	} else if event.GetMasterPublicKey() == ownerMasterKey {
		log.Trace().
			Str("context", "PUSH_NOTIFICATION").
			Str("event_master_key", event.GetMasterPublicKey()).
			Str("owner_master_key", ownerMasterKey).
			Str("event_id", event.ID).
			Msg("event master key is the same as owner master key for tokenized community action, skipping self-notifications")
		return "", nil
	}

	if userMasterKey == ownerMasterKey {
		if isCreator {
			return NotificationTypeCreatorTokenSwapped, nil
		}
		return NotificationTypeContentTokenSwapped, nil
	}

	if isCreator {
		return NotificationTypeSomeoneCreatorTokenSwapped, nil
	}
	return NotificationTypeSomeoneContentTokenSwapped, nil
}

func (pm *PushNotificationManager) handleTokenizedCommunityCreation(_ context.Context, userDevice, event *model.Event) (NotificationType, error) {
	var isFirstBuy, isCreator bool
	var ownerMasterKey string

	for _, tag := range event.Tags {
		switch tag.Key() {
		case "t":
			isFirstBuy = isFirstBuy || tag.Value() == "community_token_action"
		case "k":
			isCreator = isCreator || tag.Value() == strconv.Itoa(nostr.KindProfileMetadata)
		case "p":
			if ownerMasterKey == "" {
				ownerMasterKey = tag.Value()
			}
		}
	}

	if !isFirstBuy {
		return "", nil // Not a first buy, skipping.
	}

	userMasterKey := userDevice.GetMasterPublicKey()

	if userMasterKey == "" || ownerMasterKey == "" {
		log.Trace().
			Str("context", "PUSH_NOTIFICATION").
			Str("user_master_key", userMasterKey).
			Str("owner_master_key", ownerMasterKey).
			Str("event_id", event.ID).
			Msg("missing user or owner master key for tokenized community definition, skipping notification")
		return "", nil
	} else if event.GetMasterPublicKey() == ownerMasterKey {
		log.Trace().
			Str("context", "PUSH_NOTIFICATION").
			Str("event_master_key", event.GetMasterPublicKey()).
			Str("owner_master_key", ownerMasterKey).
			Str("event_id", event.ID).
			Msg("event master key is the same as owner master key for tokenized community definition, skipping avoid self-notifications")
		return "", nil
	}

	if userMasterKey == ownerMasterKey {
		if isCreator {
			return NotificationTypeCreatorTokenCreated, nil
		}
		return NotificationTypeContentTokenCreated, nil
	}

	if isCreator {
		return NotificationTypeSomeoneCreatorTokenCreated, nil
	}
	return NotificationTypeSomeoneContentTokenCreated, nil
}

func (pm *PushNotificationManager) handleTokenizedCommunityEvent(ctx context.Context, event *model.Event, relevantEvents ...*model.Event) (notifications []*pn.Notification[*model.Event], err error) {
	// Collect devices without filtering by master key as we need all of them to determine type of notification to create.
	devices := pm.collectUserValidDevices("", event)
	if len(devices) == 0 {
		log.Trace().
			Str("context", "PUSH_NOTIFICATION").
			Str("event_id", event.ID).
			Msg("no active devices found for tokenized community event, skipping notification")
		return nil, nil
	}

	for _, device := range devices {
		var notifyType NotificationType

		switch event.Kind {
		case model.CustomIONKindTokenizedCommunityAction:
			notifyType, err = pm.handleTokenizedCommunityAction(ctx, device, event)

		case model.CustomIONKindTokenizedCommunityDefinition:
			notifyType, err = pm.handleTokenizedCommunityCreation(ctx, device, event)

		default:
			log.Warn().Int("kind", event.Kind).Msg("unsupported tokenized community event kind")
			return nil, nil
		}

		if err != nil {
			return nil, errors.Wrapf(err, "failed to handle tokenized community event %d for master key %s", event.Kind, device.GetMasterPublicKey())
		}

		if notifyType != "" {
			deviceNotifications, err := pm.createNotifications([]*model.Event{device}, notifyType, event, relevantEvents...)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to create tokenized community notification for device %s and type %q", device.ID, notifyType)
			}
			if len(deviceNotifications) > 0 {
				notifications = append(notifications, deviceNotifications...)
			}
		}
	}

	return notifications, nil
}
