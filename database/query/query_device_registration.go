// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/jmoiron/sqlx"

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
		WHERE et.event_tag_key = 'token' AND et.event_tag_value2 != 'invalid' AND et.id > :last_tag_id
		ORDER BY et.id ASC
		LIMIT :batch_size
	`

	return func(yield func(*model.Event, error) bool) {
		var lastTagID int64

		for ctx.Err() == nil {
			params := map[string]any{
				"kind":        model.CustomIONKindDeviceRegistration,
				"last_tag_id": lastTagID,
				"batch_size":  batchSize,
			}

			it := &eventIterator{
				Fetch: func() (*sqlx.Rows, error) {
					stmt, err := db.prepare(ctx, sqlQuery, hashSQL(sqlQuery))
					if err != nil {
						return nil, errors.Wrapf(err, "failed to prepare device registration events sql: %q with params %v", sqlQuery, params)
					}

					rows, err := stmt.QueryxContext(ctx, params)

					return rows, errors.Wrapf(err, "failed to query device registration events sql: %q", sqlQuery)
				},
			}

			var eventsProcessed int
			var lastErr error

			err := it.Each(ctx, func(event *databaseEvent) error {
				if !yield(&event.Event, nil) {
					return errEventIteratorInterrupted
				}

				lastTagID = event.TagID
				eventsProcessed++
				return nil
			})

			if err != nil && !errors.Is(err, errEventIteratorInterrupted) {
				if !yield(nil, errors.Wrap(err, "failed to iterate device registration events")) {
					return
				}
				break
			}

			if lastErr != nil {
				if !yield(nil, lastErr) {
					return
				}
				break
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
