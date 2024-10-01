// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/subzero/model"
)

type eventIterator struct {
	fetch   func(pivot int64) (*sqlx.Rows, error)
	oneShot bool
}

func (*eventIterator) decodeTags(jtags string) (tags model.Tags, err error) {
	if len(jtags) == 0 {
		return
	}
	if err = tags.Scan(jtags); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal tags")
	}

	// Remove empty values from tags.
	// tag e: ["e", ""] -> ["e"].
	// tag p: [""] -> [].
	for i := range tags {
		for j := range tags[i] {
			if tags[i][j] == "" {
				if j == 0 {
					tags[i] = nil
				} else {
					tags[i] = tags[i][:j]
				}
				break
			}
		}
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
	rows, err := it.fetch(pivot)
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

		if pivot == newPivot || it.oneShot {
			return nil
		}

		pivot = newPivot
	}

	return ctx.Err()
}
