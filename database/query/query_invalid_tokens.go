// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"

	"github.com/cockroachdb/errors"
)

func (db *dbClient) markTokenAsInvalid(ctx context.Context, deviceRegistrationEventID string) error {
	if err := db.markTokenAsInvalidInEventTags(ctx, deviceRegistrationEventID); err != nil {
		return err
	}

	if err := db.markTokenAsInvalidInEvents(ctx, deviceRegistrationEventID); err != nil {
		if rollbackErr := db.rollbackTokenInEventTags(ctx, deviceRegistrationEventID); rollbackErr != nil {
			return errors.Wrapf(rollbackErr, "failed to update events and rollback failed: update error: %v", err)
		}

		return errors.Wrapf(err, "failed to update events for event %s (rollback completed)", deviceRegistrationEventID)
	}

	return nil
}

func (db *dbClient) markTokenAsInvalidInEventTags(ctx context.Context, eventID string) error {
	_, err := db.DB.ExecContext(ctx, `
		UPDATE event_tags 
		SET event_tag_value3 = 'invalid'
		WHERE 
			event_id = $1 AND 
			event_tag_key = 'token'
	`, eventID)

	if err != nil {
		return errors.Wrapf(err, "failed to update event_tags for event %s", eventID)
	}
	return nil
}

func (db *dbClient) markTokenAsInvalidInEvents(ctx context.Context, eventID string) error {
	res, err := db.DB.ExecContext(ctx, `
		UPDATE events
		SET tags = (
			SELECT json_agg(
				CASE
					WHEN elem->0 ? 'token' THEN
						jsonb_build_array(elem->0, elem->1, 'invalid')
					ELSE
						elem
				END
			)
			FROM jsonb_array_elements(tags) AS elem
		)
		WHERE id = $1
	`, eventID)

	if err != nil {
		return errors.Wrapf(err, "failed to update tags in events table for event %s", eventID)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return errors.Wrapf(err, "failed to get rows affected for event %s", eventID)
	}

	if rowsAffected == 0 {
		return errors.Errorf("failed to update tags in events table: event %s does not exist", eventID)
	}

	return nil
}

func (db *dbClient) rollbackTokenInEventTags(ctx context.Context, eventID string) error {
	res, err := db.DB.ExecContext(ctx, `
		UPDATE event_tags 
		SET event_tag_value3 = ''
		WHERE 
			event_id = $1 AND 
			event_tag_key = 'token'
	`, eventID)
	if err != nil {
		return errors.Wrapf(err, "failed to rollback event_tags for event %s", eventID)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return errors.Wrapf(err, "failed to get rows affected for rollback of event %s", eventID)
	}
	if rowsAffected == 0 {
		return errors.Errorf("failed to rollback event_tags: event %s does not exist", eventID)
	}

	return nil
}
