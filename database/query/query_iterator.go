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

	internalEventIterator = connector.Iterator[*databaseEvent]
	eventIterator         struct {
		Fetch func() (internalEventIterator, error)
		Map   func(*databaseEvent) *databaseEvent
	}
)

func (it *eventIterator) Each(ctx context.Context, fn func(*databaseEvent) error) error {
	reader, err := it.Fetch()
	if err != nil {
		return errors.Wrap(err, "failed to get events")
	} else if reader == nil {
		return nil
	}

	for ev, err := range reader {
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

func (db *dbClient) newReadEventIterator(ctx context.Context, sqlQuery string, params map[string]any) EventIterator {
	it := &eventIterator{
		Fetch: func() (internalEventIterator, error) {
			return connector.SelectNamedIterator[databaseEvent](ctx, db.db, sqlQuery, params)
		},
	}

	return func(yield func(*model.Event, error) bool) {
		err := it.Each(ctx, func(event *databaseEvent) error {
			if !yield(&event.Event, nil) {
				return errEventIteratorInterrupted
			}
			return nil
		})

		if err != nil && !errors.Is(err, errEventIteratorInterrupted) {
			yield(nil, errors.Wrap(err, "failed to iterate events"))
		}
	}
}
