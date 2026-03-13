// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"

	"github.com/ice-blockchain/subzero/database/query/internal/connector"
	"github.com/ice-blockchain/subzero/model"
)

type (
	PriceChangeData struct {
		PreviousEvent *model.Event
		Request       string
		RequestID     string
		DeviceUUID    string
		DevicePubKey  string
		MasterPubKey  string
	}
)

var (
	pnFieldMap = map[string]string{
		"devicepubkey":          "user_device_pubkey",
		"deviceuuid":            "user_device_uuid",
		"oldevent":              "old_event",
		"lasttcactioneventid":   "last_tc_action_event_id",
		"lasttcactiontimestamp": "last_tc_action_timestamp",
	}
)

func (client *dbClient) RegisterPriceChangeSubscriber(ctx context.Context, deviceUUID string, ev *model.Event) error {
	var timeWindow, deltaPercentage int64
	var tokenAddress string

	for _, tag := range ev.Tags {
		if tag.Key() != "param" || len(tag) < 3 {
			continue
		}
		switch strings.ToLower(tag.Value()) {
		case "timewindow":
			n, err := strconv.ParseInt(tag[2], 10, 32)
			if err != nil {
				return err
			}
			timeWindow = n
		case "deltapercentage":
			n, err := strconv.ParseInt(tag[2], 10, 32)
			if err != nil {
				return err
			}
			deltaPercentage = n
		case "token":
			tokenAddress = tag[2]
		}
	}

	if timeWindow == 0 || deltaPercentage == 0 {
		return fmt.Errorf("one or more required parameters are missing: timeWindow=%d, deltaPercentage=%d", timeWindow, deltaPercentage)
	}

	const insertQuery = `
WITH latest_event AS (
	SELECT e.id, to_timestamp_seconds(e.lookup_created_at) AS ts
	FROM events e
	WHERE
		:tokenAddress != ''
		AND e.kind = 1175
		AND e.hidden = false
		AND subzero_get_first_a_tag_value(e.tags, 31175) = :tokenAddress
	ORDER BY e.lookup_created_at DESC
	LIMIT 1
)
INSERT INTO pn_price_changes (
	tc_definition_address,
	user_master_pubkey, user_device_pubkey, user_device_uuid,
	request, request_event_id,
	last_tc_action_event_id, last_tc_action_timestamp,
	cfg_time_window, cfg_delta_percentage
)
SELECT
	:tokenAddress,
	:userMasterPubkey, :userDevicePubkey, :deviceUUID,
	CAST(:request AS JSONB), :requestID,
	(SELECT id FROM latest_event),
	(SELECT ts FROM latest_event),
	CAST(:timeWindow AS INTEGER),
	CAST(:deltaPercentage AS INTEGER)
WHERE
	-- Allow entries with specific token address OR generic/wildcard entries without token address, but not both for the same device.
	:tokenAddress = ''
	OR EXISTS (SELECT 1 FROM events where address = :tokenAddress AND hidden = false)
ON CONFLICT (tc_definition_address, user_device_pubkey)
DO UPDATE SET
	cfg_time_window          = EXCLUDED.cfg_time_window,
	user_device_uuid         = EXCLUDED.user_device_uuid,
	cfg_delta_percentage     = EXCLUDED.cfg_delta_percentage,
	request                  = EXCLUDED.request,
	request_event_id         = EXCLUDED.request_event_id,
	last_notified_at         = NULL,
	last_tc_action_event_id  = EXCLUDED.last_tc_action_event_id,
	last_tc_action_timestamp = EXCLUDED.last_tc_action_timestamp
RETURNING id
`
	vals, err := connector.ExecNamed[int64](ctx, client.db, insertQuery, map[string]any{
		"tokenAddress":     tokenAddress,
		"userMasterPubkey": ev.GetMasterPublicKey(),
		"userDevicePubkey": ev.PubKey,
		"request":          ev.String(),
		"requestID":        ev.ID,
		"timeWindow":       timeWindow,
		"deltaPercentage":  deltaPercentage,
		"deviceUUID":       deviceUUID,
	})

	if err == nil && len(vals) != 1 {
		err = connector.ErrNotFound
	}
	return errors.Wrapf(err, "failed to register price change subscriber %v for token %s", ev.PubKey, tokenAddress)
}

func (client *dbClient) CollectPriceChangeSubscribersCandidates(ctx context.Context, ev *model.Event, startID, limit uint64) ([]string, uint64, error) {
	var currentPrice decimal.Decimal
	var tokenAddress string
	var tokenMasterPubKey string

	for _, tag := range ev.Tags {
		switch tag.Key() {
		case "tx_amount":
			if len(tag) >= 3 && tag[2] == "USD" && currentPrice.IsZero() {
				n, err := decimal.NewFromString(tag.Value())
				if err != nil {
					return nil, 0, errors.Wrapf(err, "%s: invalid price amount", ev.ID)
				}
				currentPrice = n
			}
		case "a":
			tokenAddress = tag.Value()
		}
	}

	if currentPrice.IsZero() || tokenAddress == "" {
		log.Trace().
			Str("context", "DB").
			Str("event_id", ev.ID).
			Str("current_price", currentPrice.String()).
			Str("token_address", tokenAddress).
			Msg("event does not contain price or token information, skipping price change notification")
		return nil, 0, nil // No price data, so nothing to do.
	}

	parts := strings.SplitN(tokenAddress, ":", 3)
	if len(parts) < 3 || parts[1] == "" {
		log.Trace().
			Str("context", "DB").
			Str("event_id", ev.ID).
			Str("token_address", tokenAddress).
			Msg("token address has invalid format, skipping price change notification")
		return nil, 0, nil
	}
	tokenMasterPubKey = parts[1]

	const query = `
WITH latest_trade_event AS (
	SELECT
		id,
		subzero_get_tx_amount(tags, 'USD') as usd_amount
	FROM events
	WHERE
		kind = 1175
		AND hidden = false
		AND subzero_get_first_a_tag_value(tags, 31175) = :tokenAddress
		AND id <> :priceEventID
	ORDER BY lookup_created_at DESC
	LIMIT 1
)
SELECT
	l.id,
	l.user_device_pubkey
FROM pn_price_changes l
JOIN latest_trade_event lte ON true
WHERE
	(
		l.tc_definition_address = :tokenAddress
		OR (l.tc_definition_address = '' AND l.user_master_pubkey = :tokenMasterPubkey)
	)
	AND l.id > :startID
	AND (l.last_tc_action_timestamp IS NULL OR :priceEventTs >= l.last_tc_action_timestamp)
	AND COALESCE(l.last_tc_action_event_id, '') <> :priceEventID
	AND (l.last_notified_at IS NULL OR :priceEventTs >= l.last_notified_at + l.cfg_time_window)
	AND lte.usd_amount > 0
	AND ABS((:currentPrice - lte.usd_amount) / lte.usd_amount) >= ABS(CAST(l.cfg_delta_percentage AS NUMERIC) / 100.0)
ORDER BY l.id
LIMIT :limit
FOR UPDATE SKIP LOCKED
`

	it, err := connector.SelectNamedIterator[struct {
		ID           uint64
		DevicePubKey string
	}](ctx, client.db, query, map[string]any{
		"tokenAddress":      tokenAddress,
		"tokenMasterPubkey": tokenMasterPubKey,
		"currentPrice":      currentPrice.String(),
		"priceEventID":      ev.ID,
		"priceEventTs":      ev.CreatedAt.Time().Unix(),
		"limit":             limit,
		"startID":           startID,
	})
	if err != nil {
		return nil, 0, errors.Wrapf(err, "failed to collect price change subscriber candidates for token %s", tokenAddress)
	}

	var candidates []string
	var lastID = startID
	for entry, err := range it {
		if err != nil {
			return nil, 0, errors.Wrapf(err, "error iterating price change subscriber candidates for token %s", tokenAddress)
		}
		candidates = append(candidates, entry.DevicePubKey)
		lastID = entry.ID
	}

	return candidates, lastID, nil
}

func (client *dbClient) FetchAndUpdatePriceChangeNotification(ctx context.Context, ev *model.Event, targetDevices []string) ([]PriceChangeData, error) {
	type subscriptionInfo struct {
		DevicePubKey string
		OldEvent     string
		Request      string
		RequestID    string
		DeviceUUID   string
		MasterPubKey string
	}
	var currentPrice decimal.Decimal
	var tokenAddress string
	var tokenMasterPubKey string

	for _, tag := range ev.Tags {
		switch tag.Key() {
		case "tx_amount":
			if len(tag) >= 3 && tag[2] == "USD" && currentPrice.IsZero() {
				n, err := decimal.NewFromString(tag.Value())
				if err != nil {
					return nil, errors.Wrapf(err, "%s: invalid price amount", ev.ID)
				}
				currentPrice = n
			}
		case "a":
			tokenAddress = tag.Value()
		}
	}

	if currentPrice.IsZero() || tokenAddress == "" {
		return nil, nil
	}

	parts := strings.SplitN(tokenAddress, ":", 3)
	if len(parts) < 3 || parts[1] == "" {
		log.Trace().
			Str("context", "DB").
			Str("event_id", ev.ID).
			Str("token_address", tokenAddress).
			Msg("token address has invalid format, skipping price change notification update")
		return nil, nil
	}
	tokenMasterPubKey = parts[1]

	const query = `
WITH latest_trade_event AS (
	SELECT
		e.id,
		subzero_get_tx_amount(e.tags, 'USD') AS usd_amount
	FROM events e
	WHERE
		e.kind = 1175
		AND e.hidden = false
		AND subzero_get_first_a_tag_value(e.tags, 31175) = :tokenAddress
		AND e.id <> :priceEventID
	ORDER BY e.lookup_created_at DESC
	LIMIT 1
),
to_notify AS (
	SELECT
		l.id,
		lte.id AS latest_trade_event_id,
		l.user_device_pubkey,
		l.user_device_uuid,
		l.request,
		l.last_notified_at,
		l.cfg_time_window
	FROM pn_price_changes l
	JOIN latest_trade_event lte ON true
	WHERE
		l.user_device_pubkey = ANY(:targetDevices)
		AND (
			l.tc_definition_address = :tokenAddress
			OR (l.tc_definition_address = '' AND l.user_master_pubkey = :tokenMasterPubkey)
		)
		AND COALESCE(l.last_tc_action_event_id, '') <> :priceEventID
		AND (l.last_tc_action_timestamp IS NULL OR :priceEventTs >= l.last_tc_action_timestamp)
		AND lte.usd_amount > 0
		AND (
			(l.cfg_delta_percentage > 0 AND (:currentPrice - lte.usd_amount) / lte.usd_amount >= CAST(l.cfg_delta_percentage AS NUMERIC) / 100.0)
			OR
			(l.cfg_delta_percentage < 0 AND (:currentPrice - lte.usd_amount) / lte.usd_amount <= CAST(l.cfg_delta_percentage AS NUMERIC) / 100.0)
		)
		AND (l.last_notified_at IS NULL OR :priceEventTs >= l.last_notified_at + l.cfg_time_window)
	ORDER BY l.id
	FOR UPDATE SKIP LOCKED
),
update_actions AS (
	UPDATE pn_price_changes l
	SET
		last_tc_action_event_id = :priceEventID,
		last_tc_action_timestamp = :priceEventTs,
		last_notified_at = :priceEventTs
	FROM to_notify tn
	WHERE l.id = tn.id
	RETURNING
		l.id,
		l.user_device_pubkey,
		l.user_master_pubkey,
		l.user_device_uuid,
		l.request,
		l.request_event_id,
		tn.latest_trade_event_id
)
SELECT
	ua.user_device_pubkey,
	ua.user_device_uuid,
	ua.user_master_pubkey as master_pubkey,
	ua.request,
	ua.request_event_id as requestid,
	jsonb_build_object(
		'id', e.id,
		'pubkey', e.pubkey,
		'created_at', e.created_at,
		'kind', e.kind,
		'tags', e.tags,
		'content', e.content,
		'sig', e.sig
	) AS old_event
FROM update_actions ua
JOIN events e ON e.id = ua.latest_trade_event_id;
	`

	data, err := connector.ExecNamed[subscriptionInfo](
		ctx,
		client.db,
		query,
		map[string]any{
			"tokenAddress":      tokenAddress,
			"tokenMasterPubkey": tokenMasterPubKey,
			"currentPrice":      currentPrice.String(),
			"priceEventID":      ev.ID,
			"priceEventTs":      ev.CreatedAt.Time().Unix(),
			"targetDevices":     targetDevices,
		},
	)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to execute price change notification query for token %s", tokenAddress)
	}

	notifications := make([]PriceChangeData, 0, len(data))
	for _, info := range data {
		if info.DevicePubKey == "" {
			continue
		}

		var previousPriceEvent model.Event
		err := previousPriceEvent.UnmarshalJSON([]byte(info.OldEvent))
		if err != nil {
			log.Error().
				Str("context", "DB").
				Err(err).
				Str("data", info.OldEvent).
				Str("device_pubkey", info.DevicePubKey).
				Msg("failed to unmarshal old event content for price change notification")
			continue
		}

		notifications = append(notifications, PriceChangeData{
			PreviousEvent: &previousPriceEvent,
			Request:       info.Request,
			DeviceUUID:    info.DeviceUUID,
			DevicePubKey:  info.DevicePubKey,
			MasterPubKey:  info.MasterPubKey,
			RequestID:     info.RequestID,
		})
	}

	return notifications, nil
}

func (client *dbClient) DeletePriceChangeSubscriber(ctx context.Context, devicePubKey, deviceUUID, token, eventID string) error {
	var deleteQuery = `DELETE FROM pn_price_changes WHERE user_device_pubkey = :devicePubKey`

	if token != "" {
		deleteQuery += ` AND tc_definition_address = :token`
	}

	if eventID != "" {
		deleteQuery += ` AND request_event_id = :eventID`
	}

	if deviceUUID != "" {
		deleteQuery += ` AND user_device_uuid = :deviceUUID`
	}

	_, err := connector.ExecNamed[int64](ctx, client.db, deleteQuery, map[string]any{
		"token":        token,
		"eventID":      eventID,
		"deviceUUID":   deviceUUID,
		"devicePubKey": devicePubKey,
	})
	if errors.Is(err, connector.ErrNotFound) {
		err = nil
	}
	return errors.Wrapf(err, "failed to delete price change subscriber for token %s and device %s", token, devicePubKey)
}
