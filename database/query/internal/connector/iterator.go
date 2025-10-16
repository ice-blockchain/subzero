// SPDX-License-Identifier: ice License 1.0

package connector

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/georgysavva/scany/v2/pgxscan"
)

func iteratorInternal[T any](ctx context.Context, scanner *pgxscan.API, db Querier, sql string, args ...any) (Iterator[*T], error) {
	rows, err := db.Query(ctx, sql, args...)
	if err != nil {
		return nil, errors.Wrap(parseError(err), "failed to execute query")
	}
	return func(yield func(*T, error) bool) {
		defer rows.Close()

		rowScanner := scanner.NewRowScanner(rows)
		for rows.Next() && ctx.Err() == nil {
			var data T

			err := rowScanner.Scan(&data)
			if !yield(&data, errors.Wrap(parseError(err), "failed to scan row")) {
				return
			}
		}
		if err := errors.Join(rows.Err(), ctx.Err()); err != nil {
			yield(nil, errors.Wrap(parseError(err), "rows iteration error"))
		}
	}, nil
}

func SelectIterator[T any](ctx context.Context, db Querier, sql string, args ...any) (Iterator[*T], error) {
	scanner := getScanner(db)
	if pool, ok := db.(*DB); ok {
		db = pool.replica()
	}
	return iteratorInternal[T](ctx, scanner, db, sql, args...)
}

func ExecIterator[T any](ctx context.Context, db Querier, sql string, args ...any) (Iterator[*T], error) {
	scanner := getScanner(db)
	if pool, ok := db.(*DB); ok {
		db = pool.primary()
	}
	return iteratorInternal[T](ctx, scanner, db, sql, args...)
}
