// SPDX-License-Identifier: ice License 1.0

package connector

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/jmoiron/sqlx"
)

func bindNamed(stmt string, params map[string]any) (string, []any, error) {
	query, argList, err := sqlx.BindNamed(sqlx.DOLLAR, stmt, params)
	if err != nil {
		return "", nil, errors.Wrap(err, "failed to bind named parameters")
	}
	return query, argList, nil
}

func SelectNamedIterator[T any](ctx context.Context, db Querier, stmt string, params map[string]any) (Iterator[*T], error) {
	query, argList, err := bindNamed(stmt, params)
	if err != nil {
		return nil, err
	}
	return SelectIterator[T](ctx, db, query, argList...)
}

func GetNamed[T any](ctx context.Context, db Querier, stmt string, params map[string]any) (*T, error) {
	query, argList, err := bindNamed(stmt, params)
	if err != nil {
		return nil, err
	}

	return Get[T](ctx, db, query, argList...)
}

func SelectNamed[T any](ctx context.Context, db Querier, stmt string, params map[string]any) ([]*T, error) {
	query, argList, err := bindNamed(stmt, params)
	if err != nil {
		return nil, err
	}

	return Select[T](ctx, db, query, argList...)
}

func ExecNamedIterator[T any](ctx context.Context, db Querier, stmt string, params map[string]any) (Iterator[*T], error) {
	query, argList, err := bindNamed(stmt, params)
	if err != nil {
		return nil, err
	}
	return ExecIterator[T](ctx, db, query, argList...)
}

func ExecNamed[T any](ctx context.Context, db Querier, stmt string, params map[string]any) ([]*T, error) {
	query, argList, err := bindNamed(stmt, params)
	if err != nil {
		return nil, err
	}

	return ExecMany[T](ctx, db, query, argList...)
}
