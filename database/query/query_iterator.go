// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"iter"

	"github.com/cockroachdb/errors"
	"github.com/jmoiron/sqlx"

	"github.com/ice-blockchain/subzero/model"
)

type EventIterator iter.Seq2[*model.Event, error]

type (
	eventIterator struct {
		Fetch func() (*sqlx.Rows, error)
		Map   func(*databaseEvent) *databaseEvent
	}
)

func (it *eventIterator) decodeTags(jtags string) (tags model.Tags, err error) {
	if len(jtags) == 0 {
		return
	}

	if err = tags.Scan(jtags); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal tags")
	}

	return tags, nil
}

func (it *eventIterator) scanEvent(rows *sqlx.Rows) (_ *databaseEvent, err error) {
	var ev databaseEvent

	if err := rows.StructScan(&ev); err != nil {
		return nil, errors.Wrap(err, "failed to struct scan")
	}

	if ev.Tags, err = it.decodeTags(ev.Jtags); err != nil {
		return nil, errors.Wrap(err, "failed to decode tags")
	}

	return &ev, nil
}

func (it *eventIterator) Each(ctx context.Context, fn func(*databaseEvent) error) error {
	rows, err := it.Fetch()
	if err != nil {
		return errors.Wrap(err, "failed to get events")
	} else if rows == nil {
		return nil
	}
	defer rows.Close()

	for rows.Next() && ctx.Err() == nil {
		event, err := it.scanEvent(rows)
		if err != nil {
			return errors.Wrap(err, "failed to scan event")
		}

		if it.Map != nil {
			event = it.Map(event)
		}

		err = fn(event)
		if err != nil {
			return errors.Wrap(err, "failed to process event")
		}
	}

	if err := rows.Err(); err != nil {
		return errors.Wrap(err, "failed to iterate events")
	}

	return ctx.Err()
}

func (db *dbClient) newReadEventIterator(ctx context.Context, sqlQuery string, params map[string]any) EventIterator {
	it := &eventIterator{
		Fetch: func() (*sqlx.Rows, error) {
			stmt, err := db.prepare(ctx, sqlQuery, hashSQL(sqlQuery))
			if err != nil {
				return nil, errors.Wrapf(err, "failed to prepare query sql: %q with params %v", sqlQuery, params)
			}

			rows, err := stmt.QueryxContext(ctx, params)

			return rows, errors.Wrapf(err, "failed to query query events sql: %q", sqlQuery)
		}}

	return func(yield func(*model.Event, error) bool) {
		err := it.Each(ctx, func(dbEvent *databaseEvent) error {
			if !yield(&dbEvent.Event, nil) {
				return errEventIteratorInterrupted
			}

			return nil
		})

		if err != nil && !errors.Is(err, errEventIteratorInterrupted) {
			yield(nil, errors.Wrap(err, "failed to iterate events"))
		}
	}
}
