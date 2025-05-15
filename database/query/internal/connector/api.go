// SPDX-License-Identifier: ice License 1.0

package connector

import (
	"context"
	"net"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/georgysavva/scany/v2/pgxscan"
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

	for ctx.Err() == nil {
		_, err := withRetry(ctx, func() (any, error) {
			if err := parseDBError(pgx.BeginTxFunc(ctx, db.primary(), txOptions, func(tx pgx.Tx) error { return fn(tx) })); err != nil && IsUnexpected(err) {
				return nil, err
			} else {
				return nil, retryStop(err)
			}
		})

		if errors.IsAny(err, ErrSerializationFailure, ErrTxAborted) {
			time.Sleep(10 * time.Millisecond)
			// Try again.
			continue
		}

		return err
	}

	return ctx.Err()
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
		return nil, parseDBError(err)
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
		return nil, parseDBError(err)
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
		return 0, parseDBError(err)
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
		return nil, parseDBError(err)
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

func execMany[T any](ctx context.Context, db Querier, sql string, args ...any) ([]*T, error) {
	if pool, ok := db.(*DB); ok {
		db = pool.primary()
	}
	var resp []*T
	if err := pgxscan.Select(ctx, db, &resp, sql, args...); err != nil {
		return nil, parseDBError(err)
	}

	return resp, nil
}

func IsUnexpected(err error) bool {
	var pgConnErr *pgconn.PgError
	var netOpErr *net.OpError

	return errors.As(err, &pgConnErr) || errors.As(err, &netOpErr)
}

func parseDBError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}

	var dbErr *pgconn.PgError
	if errors.As(err, &dbErr) {
		switch dbErr.SQLState() {
		case "23505":
			if strings.HasSuffix(dbErr.ConstraintName, "_pkey") {
				return errors.Wrap(ErrDuplicate, dbErr.ConstraintName)
			} else {
				column := strings.ReplaceAll(dbErr.ConstraintName, dbErr.TableName, "")
				column = strings.ReplaceAll(column, "_key", "")
				column = strings.ReplaceAll(column, "_", "")

				return errors.Wrap(ErrDuplicate, column)
			}
		case "23503":
			column := strings.ReplaceAll(dbErr.ConstraintName, dbErr.TableName, "")
			column = strings.ReplaceAll(column, "_fkey", "")
			column = strings.ReplaceAll(column, "_", "")
			if strings.Contains(dbErr.Detail, "is still referenced from table") {
				return errors.Wrap(ErrRelationInUse, column)
			}
			return errors.Wrap(ErrRelationNotFound, column)
		case "23514":
			column := strings.ReplaceAll(dbErr.ConstraintName, dbErr.TableName, "")
			column = strings.ReplaceAll(column, "_check", "")
			column = strings.ReplaceAll(column, "_", "")

			return errors.Wrap(ErrCheckFailed, column)
		case "40001":
			return ErrSerializationFailure
		case "25P02":
			return ErrTxAborted
		case "42725":
			return ErrOperatorError
		case "42P01":
			return errors.Wrap(ErrRelationNotFound, dbErr.TableName)
		case "23P01":
			return ErrExclusionViolation
		}
	}

	return err
}
