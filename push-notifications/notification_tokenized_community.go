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

func (pm *PushNotificationManager) handleTokenizedCommunityEvent(ctx context.Context, event *model.Event, relevantEvents ...*model.Event) ([]*pn.Notification[*model.Event], error) {
	var targetMasterKey string
	var notifyType NotificationType

	switch event.Kind {
	case model.CustomIONKindTokenizedCommunityAction:
		var isBuy, isCreator bool
		targetAddressTag := event.GetTag("a").Value()
		if targetAddressTag == "" {
			eventID := event.GetTag("e").Value()
			if eventID == "" {
				log.Debug().Str("event", event.String()).Msg("no address or event tag found for tokenized community action")
				return nil, nil
			}
			for ev, err := range query.GetStoredEvents(ctx, model.Filter{
				IDs:   []string{eventID},
				Kinds: []int{model.CustomIONKindTokenizedCommunityDefinition},
				Limit: 1,
			}) {
				if err != nil {
					return nil, errors.Wrapf(err, "failed to get linked event %q for tokenized community action", eventID)
				}
				targetMasterKey = ev.GetMasterPublicKey() // Use the master key of the parent 31175.
				isCreator = ev.GetTag("k").Value() == strconv.Itoa(nostr.KindProfileMetadata)
				break
			}
		} else {
			// Address tag, use master key part directly.
			parts := strings.Split(targetAddressTag, ":")
			if len(parts) >= 2 {
				targetMasterKey = parts[1]
				isCreator = parts[0] == strconv.Itoa(nostr.KindProfileMetadata)
			}
		}

		isBuy = event.GetTag("tx_type").Value() == "buy"
		if isBuy {
			notifyType = NotificationTypeContentTokenSwapped
			if isCreator {
				notifyType = NotificationTypeCreatorTokenSwapped
			}
		} else {
			// Not a buy action, skipping.
			log.Trace().
				Str("context", "PUSH_NOTIFICATION").
				Str("event_id", event.ID).
				Msg("tokenized community action is not a buy, skipping notification")
			return nil, nil
		}

	case model.CustomIONKindTokenizedCommunityDefinition:
		var isFirstBuy, isCreator bool
		for _, tag := range event.Tags {
			switch tag.Key() {
			case "t":
				isFirstBuy = isFirstBuy || tag.Value() == "community_token_action"
			case "k":
				isCreator = isCreator || tag.Value() == strconv.Itoa(nostr.KindProfileMetadata)
			case "p":
				if targetMasterKey == "" {
					targetMasterKey = tag.Value()
				}
			}
		}

		if isFirstBuy {
			notifyType = NotificationTypeContentTokenCreated
			if isCreator {
				notifyType = NotificationTypeCreatorTokenCreated
			}
		} else {
			// Not a first buy.
			log.Trace().
				Str("context", "PUSH_NOTIFICATION").
				Str("event_id", event.ID).
				Msg("tokenized community definition is not a first buy, skipping notification")
			return nil, nil
		}
	default:
		log.Warn().Int("kind", event.Kind).Msg("unsupported tokenized community event kind")
		return nil, nil
	}

	if targetMasterKey == "" || targetMasterKey == event.GetMasterPublicKey() {
		log.Trace().
			Str("context", "PUSH_NOTIFICATION").
			Str("target_master_key", targetMasterKey).
			Str("event_master_key", event.GetMasterPublicKey()).
			Msg("no valid target master key found for tokenized community event, skipping notification")
		return nil, nil
	} else if notifyType == "" {
		log.Warn().
			Str("context", "PUSH_NOTIFICATION").
			Str("event", event.String()).
			Msg("unable to determine notification type for tokenized community event")
		return nil, nil
	}

	devices := pm.collectUserValidDevices(targetMasterKey, event)
	notifications, err := pm.createNotifications(devices, notifyType, event, relevantEvents...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create tokenized community notification for type %q", notifyType)
	}

	return notifications, nil
}
