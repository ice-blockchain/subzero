// SPDX-License-Identifier: ice License 1.0

//go:build test

package query

import (
	"context"

	"github.com/cockroachdb/errors"
)

func TriggerExpiredEventsCleanup(ctx context.Context) error {
	return globalDB.Client.deleteExpiredEvents(ctx)
}

func DeleteAllEvents(ctx context.Context) error {
	const stmt = `DELETE FROM events`

	_, err := globalDB.Client.ExecContext(context.WithoutCancel(ctx), stmt)

	return errors.Wrap(err, "failed to delete all events")
}
