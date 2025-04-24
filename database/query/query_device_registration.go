// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/model"
)

func (db *dbClient) collectDeviceRegistrationEvents(ctx context.Context) (events []*model.Event, err error) {
	const batchSize = 1000
	events = make([]*model.Event, 0)

	var lastCreatedAt time.Time
	var lastID string
	var hasCursor bool

	for ctx.Err() == nil {
		query := `
		SELECT 
			e.kind,
			e.created_at,
			e.id,
			e.pubkey,
			e.master_pubkey,
			e.sig,
			e.content,
			e.d_tag,
			e.tags,
			COALESCE(et.event_tag_value2, '') as token_status
		FROM events e
		LEFT JOIN event_tags et ON e.id = et.event_id AND et.event_tag_key = 'token'
		WHERE e.kind = $1
		`

		args := []interface{}{model.CustomIONKindDeviceRegistration}
		paramPos := 2
		if hasCursor {
			query += `AND (e.created_at, e.id) > ($2, $3) `
			args = append(args, lastCreatedAt, lastID)
			paramPos = 4
		}

		query += ` ORDER BY e.created_at, e.id LIMIT $` + strconv.Itoa(paramPos)
		args = append(args, batchSize)
		rows, err := db.DB.QueryxContext(ctx, query, args...)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to query device registration events")
		}
		var currentBatch []*model.Event
		for rows.Next() && ctx.Err() == nil {
			var dbEvent struct {
				Kind         int
				CreatedAt    time.Time
				ID           string
				PubKey       string
				MasterPubKey string
				Sig          string
				Content      string
				Dtag         string
				Tags         []byte
				TokenStatus  string
			}
			err := rows.Scan(&dbEvent.Kind, &dbEvent.CreatedAt, &dbEvent.ID, &dbEvent.PubKey, &dbEvent.MasterPubKey, &dbEvent.Sig, &dbEvent.Content, &dbEvent.Dtag, &dbEvent.Tags, &dbEvent.TokenStatus)
			if err != nil {
				rows.Close()

				return nil, errors.Wrap(err, "failed to scan device registration event")
			}
			event := &model.Event{
				Event: nostr.Event{
					Kind:      dbEvent.Kind,
					CreatedAt: nostr.Timestamp(dbEvent.CreatedAt.Unix()),
					ID:        dbEvent.ID,
					PubKey:    dbEvent.PubKey,
					Sig:       dbEvent.Sig,
					Content:   dbEvent.Content,
				},
			}
			if err := json.Unmarshal(dbEvent.Tags, &event.Tags); err != nil {
				rows.Close()

				return nil, errors.Wrap(err, "failed to unmarshal tags")
			}
			if dbEvent.TokenStatus == "invalid" {
				for i, tag := range event.Tags {
					if tag.Key() == "token" {
						if len(tag) <= 2 {
							event.Tags[i] = append(tag, "invalid")
						} else {
							event.Tags[i][2] = "invalid"
						}

						break
					}
				}
			}
			currentBatch = append(currentBatch, event)
			lastCreatedAt = dbEvent.CreatedAt
			lastID = dbEvent.ID
			hasCursor = true
		}
		if err := rows.Err(); err != nil {
			rows.Close()

			return nil, errors.Wrap(err, "error iterating device registration events")
		}
		rows.Close()
		if len(currentBatch) == 0 {
			break
		}
		events = append(events, currentBatch...)
		if len(currentBatch) < batchSize {
			break
		}
	}

	return events, nil
}

func (db *dbClient) markTokenAsInvalidInEventTags(ctx context.Context, events []*model.Event) error {
	if len(events) == 0 {
		return nil
	}

	eventIDs := make([]string, len(events))
	for i, event := range events {
		eventIDs[i] = event.ID
	}

	res, err := db.DB.ExecContext(ctx, `
		UPDATE event_tags
			SET event_tag_value2 = 'invalid'
		WHERE event_id = ANY($1) AND event_tag_key = 'token'
	`, eventIDs)

	if err != nil {
		return errors.Wrapf(err, "failed to update token status in event_tags table for events %s", strings.Join(eventIDs, ", "))
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return errors.Wrap(err, "failed to get rows affected for batch update")
	}

	if rowsAffected == 0 {
		return errors.New("failed to update token status in event_tags table: no event tags were updated")
	}

	return nil
}
