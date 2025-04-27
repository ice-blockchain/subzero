// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"strings"
	"time"

	"github.com/cockroachdb/errors"

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
			e.tags
		FROM event_tags et
		RIGHT JOIN events e ON et.event_id = e.id AND et.event_tag_key = 'token' AND et.event_tag_value2 != 'invalid'
		WHERE e.kind = :kind
			AND et.event_id IS NOT NULL
			AND (e.created_at > :last_created_at OR (e.created_at = :last_created_at AND e.id > :last_id))
		ORDER BY e.created_at ASC, e.id
		LIMIT :batch_size
	`

	return func(yield func(*model.Event, error) bool) {
		var lastCreatedAt time.Time
		var lastID string

		for ctx.Err() == nil {
			params := map[string]any{
				"kind":            model.CustomIONKindDeviceRegistration,
				"last_created_at": lastCreatedAt.UTC(),
				"last_id":         lastID,
				"batch_size":      batchSize,
			}

			var eventsProcessed int
			it := db.newReadEventIterator(ctx, sqlQuery, params)
			for event, iterErr := range it {
				if iterErr != nil {
					if !yield(nil, errors.Wrap(iterErr, "failed to iterate device registration events")) {
						return
					}
					break
				}
				if !yield(event, nil) {
					return
				}

				lastCreatedAt = time.Unix(int64(event.CreatedAt), 0)
				lastID = event.ID
				eventsProcessed++
			}
			if eventsProcessed < batchSize {
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
