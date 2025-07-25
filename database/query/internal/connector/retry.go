// SPDX-License-Identifier: ice License 1.0

package connector

import (
	"context"
	"log"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/cockroachdb/errors"
)

func retryStop(err error) error {
	return backoff.Permanent(err)
}

func withRetry[T any](ctx context.Context, op func() (T, error)) (T, error) {
	return backoff.Retry(
		ctx,
		op,
		backoff.WithNotify(func(err error, d time.Duration) {
			log.Printf("[DATABASE]: ERROR: call failed: %v (%s). retrying in %v... ", err, errors.FlattenDetails(err), d)
		}),
		backoff.WithMaxElapsedTime(25*time.Second),
		backoff.WithBackOff(&backoff.ExponentialBackOff{
			InitialInterval:     100 * time.Millisecond,
			RandomizationFactor: 0.5,
			Multiplier:          2.5,
			MaxInterval:         time.Second,
		}),
	)
}
