// SPDX-License-Identifier: ice License 1.0

//go:build test

package query

import (
	"context"

	"github.com/cockroachdb/errors"

	"github.com/ice-blockchain/subzero/database/query/internal/postgres/fixture"
)

func TriggerExpiredEventsCleanup(ctx context.Context) error {
	return globalDB.Client.deleteExpiredEvents(ctx)
}

func DeleteAllEvents(ctx context.Context) error {
	const stmt = `DELETE FROM events`

	_, err := globalDB.Client.ExecContext(context.WithoutCancel(ctx), stmt)

	return errors.Wrap(err, "failed to delete all events")
}

func NewTestDatabase(ctx context.Context) (string, func() error) {
	container := fixture.New(ctx)

	return container.ConnectionString(ctx, ""), func() error {
		return container.Close(context.Background())
	}
}
