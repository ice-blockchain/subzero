// SPDX-License-Identifier: ice License 1.0

//go:build test

package query

import "context"

func TriggerExpiredEventsCleanup(ctx context.Context) error {
	return globalDB.Client.deleteExpiredEvents(ctx)
}
