// SPDX-License-Identifier: ice License 1.0

package connector

import (
	"context"
	"net"

	"github.com/cockroachdb/errors"
	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type (
	Querier interface {
		pgxscan.Querier
	}
	Execer interface {
		Exec(ctx context.Context, sql string, arguments ...any) (commandTag pgconn.CommandTag, err error)
	}
	QueryExecer interface {
		Querier
		Execer
	}
)

func DoInTransaction(ctx context.Context, db *DB, fn func(conn QueryExecer) error) error {
	txOptions := pgx.TxOptions{
		IsoLevel:       pgx.Serializable,
		AccessMode:     pgx.ReadWrite,
		DeferrableMode: pgx.NotDeferrable,
	}

	_, err := withRetry(ctx, func() (any, error) {
		txErr := parseError(pgx.BeginTxFunc(ctx, db.primary(), txOptions, func(tx pgx.Tx) error {
			return fn(tx)
		}))

		switch {
		case errors.IsAny(txErr, ErrReadOnly):
			return nil, retryStop(txErr)
		case IsUnexpected(txErr) || errors.IsAny(txErr, ErrSerializationFailure, ErrTxAborted):
			return nil, txErr
		}

		return nil, retryStop(txErr)
	})

	return err
}

func Get[T any](ctx context.Context, db Querier, sql string, args ...any) (*T, error) {
	return withRetry(ctx, func() (*T, error) {
		if resp, err := get[T](ctx, db, sql, args...); err != nil && IsUnexpected(err) {
			return nil, err
		} else {
			return resp, retryStop(err)
		}
	})
}

func get[T any](ctx context.Context, db Querier, sql string, args ...any) (*T, error) {
	if pool, ok := db.(*DB); ok {
		db = pool.replica()
	}
	resp := new(T)
	if err := pgxscan.Get(ctx, db, resp, sql, args...); err != nil {
		return nil, parseError(err)
	}

	return resp, nil
}

func Select[T any](ctx context.Context, db Querier, sql string, args ...any) ([]*T, error) {
	return withRetry(ctx, func() ([]*T, error) {
		if resp, err := selectInternal[T](ctx, db, sql, args...); err != nil && IsUnexpected(err) {
			return nil, err
		} else {
			return resp, retryStop(err)
		}
	})
}

func selectInternal[T any](ctx context.Context, db Querier, sql string, args ...any) ([]*T, error) {
	if pool, ok := db.(*DB); ok {
		db = pool.replica()
	}
	var resp []*T
	if err := pgxscan.Select(ctx, db, &resp, sql, args...); err != nil {
		return nil, parseError(err)
	}

	return resp, nil
}

func Exec(ctx context.Context, db Execer, sql string, args ...any) (uint64, error) {
	return withRetry(ctx, func() (uint64, error) {
		if resp, err := exec(ctx, db, sql, args...); err != nil && IsUnexpected(err) {
			return 0, err
		} else {
			return resp, retryStop(err)
		}
	})
}

func exec(ctx context.Context, db Execer, sql string, args ...any) (uint64, error) {
	if pool, ok := db.(*DB); ok {
		db = pool.primary()
	}
	resp, err := db.Exec(ctx, sql, args...)
	if err != nil {
		return 0, parseError(err)
	}

	return uint64(resp.RowsAffected()), nil
}

func ExecOne[T any](ctx context.Context, db Querier, sql string, args ...any) (*T, error) {
	return withRetry(ctx, func() (*T, error) {
		if resp, err := execOne[T](ctx, db, sql, args...); err != nil && IsUnexpected(err) {
			return nil, err
		} else {
			return resp, retryStop(err)
		}
	})
}

func execOne[T any](ctx context.Context, db Querier, sql string, args ...any) (*T, error) {
	if pool, ok := db.(*DB); ok {
		db = pool.primary()
	}
	resp := new(T)
	if err := pgxscan.Get(ctx, db, resp, sql, args...); err != nil {
		return nil, parseError(err)
	}

	return resp, nil
}

func ExecMany[T any](ctx context.Context, db Querier, sql string, args ...any) ([]*T, error) {
	return withRetry(ctx, func() ([]*T, error) {
		if resp, err := execMany[T](ctx, db, sql, args...); err != nil && IsUnexpected(err) {
			return nil, err
		} else {
			return resp, retryStop(err)
		}
	})
}

func ExecManyWithCustomRetry[T any](ctx context.Context, db Querier, retryIf func(err error) bool, sql string, args ...any) ([]*T, error) {
	return withRetry(ctx, func() ([]*T, error) {
		resp, err := execMany[T](ctx, db, sql, args...)
		if err == nil {
			return resp, nil
		} else if retryIf(err) {
			return nil, err
		}
		return resp, retryStop(err)
	})
}

func execMany[T any](ctx context.Context, db Querier, sql string, args ...any) ([]*T, error) {
	if pool, ok := db.(*DB); ok {
		db = pool.primary()
	}
	var resp []*T
	if err := pgxscan.Select(ctx, db, &resp, sql, args...); err != nil {
		return nil, parseError(err)
	}

	return resp, nil
}

func IsUnexpected(err error) bool {
	var pgConnErr *pgconn.PgError
	var netOpErr *net.OpError

	if errors.As(err, &pgConnErr) {
		return pgConnErr.SQLState() != pgerrcode.SyntaxError
	}

	return errors.As(err, &netOpErr)
}
