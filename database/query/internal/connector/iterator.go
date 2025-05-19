// SPDX-License-Identifier: ice License 1.0

package connector

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/georgysavva/scany/v2/pgxscan"
)

func iteratorInternal[T any](ctx context.Context, db Querier, sql string, args ...any) (Iterator[*T], error) {
	rows, err := db.Query(ctx, sql, args...)
	if err != nil {
		return nil, errors.Wrap(parseError(err), "failed to execute query")
	}
	return func(yield func(*T, error) bool) {
		defer rows.Close()

		scanner := pgxscan.NewRowScanner(rows)
		for rows.Next() {
			var data T

			err := scanner.Scan(&data)
			if !yield(&data, errors.Wrap(parseError(err), "failed to scan row")) {
				return
			}
		}
		if err := rows.Err(); err != nil {
			yield(nil, errors.Wrap(parseError(err), "rows iteration error"))
		}
	}, nil
}

func SelectIterator[T any](ctx context.Context, db Querier, sql string, args ...any) (Iterator[*T], error) {
	if pool, ok := db.(*DB); ok {
		db = pool.replica()
	}
	return iteratorInternal[T](ctx, db, sql, args...)
}

func ExecIterator[T any](ctx context.Context, db Querier, sql string, args ...any) (Iterator[*T], error) {
	if pool, ok := db.(*DB); ok {
		db = pool.primary()
	}
	return iteratorInternal[T](ctx, db, sql, args...)
}
