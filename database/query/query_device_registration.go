// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"strings"

	"github.com/cockroachdb/errors"

	"github.com/ice-blockchain/subzero/database/query/internal/connector"
	"github.com/ice-blockchain/subzero/model"
)

func (db *dbClient) collectDeviceRegistrationEvents(ctx context.Context) EventIterator {
	const batchSize = 1000
	sqlQuery := `
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
			et.id as tag_id
		FROM event_tags et
		JOIN events e ON et.event_id = e.id AND e.kind = :kind
		JOIN event_tags et_relay ON et_relay.event_id = e.id 
			AND et_relay.event_tag_key = 'relay' 
			AND et_relay.event_tag_value1 = :relay_url
		WHERE et.event_tag_key = 'token' AND et.event_tag_value2 != 'invalid' AND et.id > :last_tag_id
		ORDER BY et.id ASC
		LIMIT :batch_size
	`

	return func(yield func(*model.Event, error) bool) {
		var lastTagID int64

		for ctx.Err() == nil {
			params := map[string]any{
				"kind":        model.CustomIONKindDeviceRegistration,
				"relay_url":   db.relayURL,
				"last_tag_id": lastTagID,
				"batch_size":  batchSize,
			}

			events, err := connector.SelectNamed[databaseEvent](ctx, db.db, sqlQuery, params)
			if err != nil {
				yield(nil, errors.Wrap(err, "failed to collect device registration events"))
				return
			}

			for _, event := range events {
				if yield(event.Event, nil) {
					lastTagID = event.TagID
				} else {
					return
				}
			}

			if len(events) < batchSize {
				break
			}
		}
	}
}

func (db *dbClient) markTokenAsInvalidInEventTags(ctx context.Context, events []*model.Event) error {
	if len(events) == 0 {
		return nil
	}

	eventIDs := make([]string, len(events))
	for i, event := range events {
		eventIDs[i] = event.ID
	}

	rowsAffected, err := connector.Exec(ctx, db.db, `UPDATE event_tags SET event_tag_value2 = 'invalid' WHERE event_id = ANY($1) AND event_tag_key = 'token'`, eventIDs)
	if err != nil {
		return errors.Wrapf(err, "failed to update token status in event_tags table for events %s", strings.Join(eventIDs, ", "))
	}

	if rowsAffected == 0 {
		return errors.New("failed to update token status in event_tags table: no event tags were updated")
	}

	return nil
}
