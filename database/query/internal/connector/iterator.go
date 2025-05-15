// SPDX-License-Identifier: ice License 1.0

package connector

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/georgysavva/scany/v2/pgxscan"
)

func SelectIterator[T any](ctx context.Context, db Querier, sql string, args ...any) (Iterator[*T], error) {
	rows, err := db.Query(ctx, sql, args...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute query")
	}
	return func(yield func(*T, error) bool) {
		defer rows.Close()

		scanner := pgxscan.NewRowScanner(rows)
		for rows.Next() {
			var data T

			err := scanner.Scan(&data)
			if !yield(&data, errors.Wrap(err, "failed to scan row")) {
				return
			}
		}
		if err := rows.Err(); err != nil {
			yield(nil, errors.Wrap(err, "rows iteration error"))
		}
	}, nil
}
