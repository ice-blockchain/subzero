// SPDX-License-Identifier: ice License 1.0

package connector

import (
	"context"
	"math/rand/v2"
	"net"
	"time"

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
	QueryExecerTx interface {
		QueryExecer
		BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error)
	}
)

func DoInTransaction(ctx context.Context, db *DB, fn func(conn QueryExecer) error) error {
	txOptions := pgx.TxOptions{
		IsoLevel:       pgx.Serializable,
		AccessMode:     pgx.ReadWrite,
		DeferrableMode: pgx.NotDeferrable,
	}

	for ctx.Err() == nil {
		err := executeTransaction(ctx, db, txOptions, fn)
		if shouldRetryTransaction(err) {
			time.Sleep(time.Duration(10+rand.IntN(200)) * time.Millisecond)
			continue
		}
		return err
	}

	return ctx.Err()
}

func executeTransaction(ctx context.Context, db *DB, txOptions pgx.TxOptions, fn func(conn QueryExecer) error) error {
	_, err := withRetry(ctx, func() (any, error) {
		txErr := parseError(pgx.BeginTxFunc(ctx, db.primary(), txOptions, func(tx pgx.Tx) error {
			return fn(tx)
		}))

		if txErr == nil {
			return nil, retryStop(nil)
		}

		if errors.IsAny(txErr, ErrReadOnly) {
			return nil, retryStop(txErr)
		}

		if isInstanceDead(txErr) {
			return nil, errors.Join(txErr, db.switchMaster(ctx, txErr, nil, nil))
		} else if IsUnexpected(txErr) {
			return nil, txErr
		}

		return nil, retryStop(txErr)
	})

	return err
}

func shouldRetryTransaction(err error) bool {
	return errors.IsAny(err, ErrSerializationFailure, ErrTxAborted)
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
		resp, err := exec(ctx, db, sql, args...)
		if isInstanceDead(err) {
			return resp, errors.Join(err, switchMaster(ctx, db, err))
		} else if IsUnexpected(err) {
			return resp, err
		}
		return resp, retryStop(err)
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
		resp, err := execOne[T](ctx, db, sql, args...)
		if isInstanceDead(err) {
			return resp, errors.Join(err, switchMaster(ctx, db, err))
		} else if IsUnexpected(err) {
			return resp, err
		}
		return resp, retryStop(err)
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
		resp, err := execMany[T](ctx, db, sql, args...)
		if isInstanceDead(err) {
			return resp, errors.Join(err, switchMaster(ctx, db, err))
		} else if IsUnexpected(err) {
			return resp, err
		}
		return resp, retryStop(err)
	})
}

func ExecManyWithCustomRetry[T any](ctx context.Context, db Querier, retryIf func(err error) bool, sql string, args ...any) ([]*T, error) {
	return withRetry(ctx, func() ([]*T, error) {
		resp, err := execMany[T](ctx, db, sql, args...)
		if err == nil {
			return resp, nil
		}
		if isInstanceDead(err) {
			return resp, errors.Join(err, switchMaster(ctx, db, err))
		} else if retryIf(err) {
			return resp, err
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

	return errors.As(err, &netOpErr) || errors.IsAny(err, ErrSerializationFailure)
}

func isInstanceDead(err error) bool {
	var (
		netOpErr  *net.OpError
		pgErr     *pgconn.PgError
		pgconnErr *pgconn.ConnectError
	)

	if errors.As(err, &netOpErr) || errors.As(err, &pgconnErr) {
		return true
	}

	if errors.As(err, &pgErr) {
		code := pgErr.SQLState()
		return pgerrcode.IsConnectionException(code) ||
			pgerrcode.IsSystemError(code) ||
			pgerrcode.IsInternalError(code) ||
			pgerrcode.IsConfigurationFileError(code) ||
			pgerrcode.IsOperatorIntervention(code)
	}

	return false
}

func switchMaster(ctx context.Context, db any, reason error) error {
	lb, ok := db.(*DB)
	if !ok {
		return nil
	}
	return lb.switchMaster(ctx, reason, nil, nil)
}
