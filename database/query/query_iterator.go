// SPDX-License-Identifier: ice License 1.0

package query

import (
	"cmp"
	"context"

	"github.com/cockroachdb/errors"

	"github.com/ice-blockchain/subzero/database/query/internal/connector"
	"github.com/ice-blockchain/subzero/model"
)

type (
	EventIterator = connector.Iterator[*model.Event]

	eventIterator struct {
		Fetch func() ([]*databaseEvent, error)
		Map   func(*databaseEvent) *databaseEvent
	}
)

func (it *eventIterator) Each(ctx context.Context, fn func(*databaseEvent) error) error {
	events, err := it.Fetch()
	if err != nil {
		return errors.Wrap(err, "failed to get events")
	} else if len(events) == 0 {
		return nil
	}

	for _, ev := range events {
		if err != nil || ctx.Err() != nil {
			return errors.Wrap(cmp.Or(err, ctx.Err()), "failed to scan event")
		}

		if it.Map != nil {
			ev = it.Map(ev)
		}

		err = fn(ev)
		if err != nil {
			return errors.Wrap(err, "failed to process event")
		}
	}
	return ctx.Err()
}
