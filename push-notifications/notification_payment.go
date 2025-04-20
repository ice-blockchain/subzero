// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"encoding/json"
	"log"
	"regexp"

	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
)

var neventRegex = regexp.MustCompile(`nostr:nevent1[a-zA-Z0-9]+`)

type (
	paymentRequestContent struct {
		Amount    string `json:"amount"`
		AmountUSD string `json:"amount_usd"`
		AssetID   string `json:"asset_id,omitempty"`
		From      string `json:"from"`
		To        string `json:"to"`
	}
	paymentReceivedContent struct {
		Amount    string `json:"amount"`
		AmountUSD string `json:"amount_usd"`
		TxHash    string `json:"tx_hash"`
		AssetID   string `json:"asset_id,omitempty"`
		TxURL     string `json:"tx_url"`
		From      string `json:"from"`
		To        string `json:"to"`
	}
	paymentInfo struct {
		EventID      string
		Network      string
		AssetClass   string
		AssetAddress string
		Amount       string
		AmountUSD    string
		AssetID      string
		From         string
		To           string
		TxHash       string
		TxURL        string
		RequestEvent string
		RecipientKey string
		AuthorPubKey string
		EventKind    int
	}
	neventData struct {
		ID     string
		Kind   int
		PubKey string
	}
)

func (pm *PushNotificationManager) handlePaymentRequestNotification(event *model.Event) []*pn.Notification[pn.DeviceToken] {
	paymentInfo, err := extractPaymentInfoFromEvent(event)
	if err != nil {
		log.Printf("error extracting payment info from payment request: %s for event %s", err, event.ID)

		return nil
	}
	if paymentInfo.Network == "" || paymentInfo.AssetClass == "" || paymentInfo.RecipientKey == "" {
		log.Printf("missing required fields for payment request: network, asset_class or recipient for event %s", event.ID)

		return nil
	}
	if paymentInfo.RecipientKey == event.GetMasterPublicKey() {
		return nil
	}
	iosDevices, otherDevices := pm.collectValidDevices(paymentInfo.RecipientKey, NotificationTypePaymentRequest, event)
	data := populateNotificationDataFromPaymentInfo(paymentInfo, NotificationTypePaymentRequest)

	return pm.createAndSendNotifications(iosDevices, otherDevices, NotificationTypePaymentRequest, data)
}

func (pm *PushNotificationManager) handlePaymentReceivedNotification(event *model.Event) []*pn.Notification[pn.DeviceToken] {
	paymentInfo, err := extractPaymentInfoFromEvent(event)
	if err != nil {
		log.Printf("error extracting payment info from payment received: %s for event %s", err, event.ID)

		return nil
	}
	if paymentInfo.Network == "" || paymentInfo.AssetClass == "" || paymentInfo.RecipientKey == "" {
		log.Printf("missing required fields for payment received: network, asset_class or recipient for event %s", event.ID)

		return nil
	}
	if paymentInfo.RecipientKey == event.GetMasterPublicKey() {
		return nil
	}

	iosDevices, otherDevices := pm.collectValidDevices(paymentInfo.RecipientKey, NotificationTypePaymentReceived, event)
	data := populateNotificationDataFromPaymentInfo(paymentInfo, NotificationTypePaymentReceived)

	return pm.createAndSendNotifications(iosDevices, otherDevices, NotificationTypePaymentReceived, data)
}

func extractPaymentInfoFromEvent(event *model.Event) (*paymentInfo, error) {
	info := &paymentInfo{
		EventID:      event.ID,
		EventKind:    event.Kind,
		AuthorPubKey: event.GetMasterPublicKey(),
	}

	for _, tag := range event.Tags {
		if tag.Key() == "p" {
			info.RecipientKey = tag.Value()
		} else if tag.Key() == "network" {
			info.Network = tag.Value()
		} else if tag.Key() == "asset_class" {
			info.AssetClass = tag.Value()
		} else if tag.Key() == "asset_address" {
			info.AssetAddress = tag.Value()
		} else if tag.Key() == "request" {
			info.RequestEvent = tag.Value()
		}
	}

	switch event.Kind {
	case model.CustomIONKindFundReceive:
		var content paymentRequestContent
		if err := json.Unmarshal([]byte(event.Content), &content); err != nil {
			log.Printf("error unmarshalling payment request content: %s for event %s", err, event.ID)

			return nil, err
		}
		info.Amount = content.Amount
		info.AmountUSD = content.AmountUSD
		info.AssetID = content.AssetID
		info.From = content.From
		info.To = content.To

		if info.RecipientKey == "" {
			lTag := event.GetTag("L")
			if lTag != nil && len(lTag) >= 2 && lTag[1] == "wallet.address" {
				llTag := event.GetTag("l")
				if llTag != nil && len(llTag) >= 3 && llTag[2] == "wallet.address" {
					if llTag.Value() != content.From {
						return nil, nil
					}
				}
			}
		}

	case model.CustomIONKindFundSendNotify:
		var content paymentReceivedContent
		if err := json.Unmarshal([]byte(event.Content), &content); err != nil {
			log.Printf("error unmarshalling payment received content: %s for event %s", err, event.ID)

			return nil, err
		}
		info.Amount = content.Amount
		info.AmountUSD = content.AmountUSD
		info.AssetID = content.AssetID
		info.From = content.From
		info.To = content.To
		info.TxHash = content.TxHash
		info.TxURL = content.TxURL

		if info.RecipientKey == "" {
			lTag := event.GetTag("L")
			if lTag != nil && len(lTag) >= 2 && lTag[1] == "wallet.address" {
				llTag := event.GetTag("l")
				if llTag != nil && len(llTag) >= 3 && llTag[2] == "wallet.address" {
					if llTag.Value() != content.To {
						return nil, nil
					}
				}
			}
		}
	}

	return info, nil
}

func populateNotificationDataFromPaymentInfo(info *paymentInfo, notificationType NotificationType) map[string]interface{} {
	if info == nil {
		return nil
	}
	data := map[string]interface{}{
		"eventId":          info.EventID,
		"authorPubKey":     info.AuthorPubKey,
		"notificationType": string(notificationType),
		"amount":           info.Amount,
		"amountUsd":        info.AmountUSD,
		"assetId":          info.AssetID,
		"from":             info.From,
		"to":               info.To,
		"network":          info.Network,
		"assetClass":       info.AssetClass,
		"assetAddress":     info.AssetAddress,
	}
	if info.EventKind == model.CustomIONKindFundSendNotify {
		data["txHash"] = info.TxHash
		data["txUrl"] = info.TxURL
		data["requestEvent"] = info.RequestEvent
	}

	return data
}

func extractPaymentInfoFromNeventLink(neventLink string) (*paymentInfo, error) {
	linkToDecode := neventLink
	if len(neventLink) > 6 && neventLink[:6] == "nostr:" {
		linkToDecode = neventLink[6:]
	}

	prefix, eventData, err := nip19.Decode(linkToDecode)
	if err != nil {
		log.Printf("error decoding nevent link: %s", err)

		return nil, err
	}

	if prefix != "nevent" {
		return nil, nil
	}

	if eventPointer, ok := eventData.(nostr.EventPointer); ok {
		if eventPointer.Kind != model.CustomIONKindFundReceive && eventPointer.Kind != model.CustomIONKindFundSendNotify {
			return nil, nil
		}

		return &paymentInfo{
			EventID:      eventPointer.ID,
			EventKind:    eventPointer.Kind,
			AuthorPubKey: eventPointer.Author,
		}, nil
	}

	eventDataMap, ok := eventData.(map[string]interface{})
	if !ok {
		return nil, nil
	}

	paymentInfo := &paymentInfo{}
	if id, ok := eventDataMap["id"].(string); ok {
		paymentInfo.EventID = id
	}
	if author, ok := eventDataMap["pubkey"].(string); ok {
		paymentInfo.AuthorPubKey = author
	}
	if kind, ok := eventDataMap["kind"].(int); ok {
		paymentInfo.EventKind = kind
	}
	if paymentInfo.EventKind != model.CustomIONKindFundReceive && paymentInfo.EventKind != model.CustomIONKindFundSendNotify {
		return nil, nil
	}

	return paymentInfo, nil
}
