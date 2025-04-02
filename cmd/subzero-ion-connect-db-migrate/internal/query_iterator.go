// SPDX-License-Identifier: ice License 1.0

package internal

import (
	"context"
	"iter"

	"github.com/cockroachdb/errors"
	"github.com/jmoiron/sqlx"

	"github.com/ice-blockchain/subzero/model"
)

type EventIterator = iter.Seq2[*model.Event, error]

type (
	eventIterator struct {
		Fetch   func(pivot int64) (*sqlx.Rows, error)
		Map     func(*databaseEvent) *databaseEvent
		OneShot bool
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

func (it *eventIterator) scanBatch(ctx context.Context, fn func(*model.Event) error, pivot int64) (int64, error) {
	rows, err := it.Fetch(pivot)
	if err != nil {
		return -1, errors.Wrap(err, "failed to get events")
	} else if rows == nil {
		return pivot, nil
	}
	defer rows.Close()

	for rows.Next() && ctx.Err() == nil {
		event, err := it.scanEvent(rows)
		if err != nil {
			return -1, errors.Wrap(err, "failed to scan event")
		}

		if pivot == 0 || event.SystemCreatedAt < pivot {
			pivot = event.SystemCreatedAt
		}

		if it.Map != nil {
			event = it.Map(event)
		}

		err = fn(&event.Event)
		if err != nil {
			return -1, errors.Wrap(err, "failed to process event")
		}
	}

	return pivot, nil
}

func (it *eventIterator) Each(ctx context.Context, fn func(*model.Event) error) error {
	var pivot int64

	for ctx.Err() == nil {
		newPivot, err := it.scanBatch(ctx, fn, pivot)
		if err != nil {
			return err
		}

		if pivot == newPivot || it.OneShot {
			return nil
		}

		pivot = newPivot
	}

	return ctx.Err()
}

func (db *dbClient) newReadEventIterator(ctx context.Context, sqlQuery string, params map[string]any) EventIterator {
	it := &eventIterator{
		OneShot: true,
		Fetch: func(int64) (*sqlx.Rows, error) {
			stmt, err := db.PrepareNamedContext(ctx, sqlQuery)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to prepare query sql: %q with params %v", sqlQuery, params)
			}

			rows, err := stmt.QueryxContext(ctx, params)

			return rows, errors.Wrapf(err, "failed to query query events sql: %q", sqlQuery)
		}}

	return func(yield func(*model.Event, error) bool) {
		err := it.Each(ctx, func(event *model.Event) error {
			if !yield(event, nil) {
				return errEventIteratorInterrupted
			}

			return nil
		})

		if err != nil && !errors.Is(err, errEventIteratorInterrupted) {
			yield(nil, errors.Wrap(err, "failed to iterate events"))
		}
	}
}
