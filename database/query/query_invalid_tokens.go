// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"strings"

	"github.com/cockroachdb/errors"

	"github.com/ice-blockchain/subzero/model"
)

func (db *dbClient) markTokenAsInvalidInEvents(ctx context.Context, events []*model.Event) error {
	if len(events) == 0 {
		return nil
	}

	eventIDs := make([]string, len(events))
	for i, event := range events {
		eventIDs[i] = event.ID
	}

	res, err := db.DB.ExecContext(ctx, `
		UPDATE events
			SET invalid_token = TRUE
		WHERE id = ANY($1) AND kind = 31751
	`, eventIDs)

	if err != nil {
		return errors.Wrapf(err, "failed to update invalid_token in events table for events %s", strings.Join(eventIDs, ", "))
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return errors.Wrap(err, "failed to get rows affected for batch update")
	}

	if rowsAffected == 0 {
		return errors.New("failed to update invalid_token in events table: no events were updated")
	}

	return nil
}
