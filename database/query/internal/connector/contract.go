// SPDX-License-Identifier: ice License 1.0

package connector

import (
	"context"
	"errors"
	"iter"
	"sync/atomic"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/puzpuzpuz/xsync/v4"
)

var (
	ErrNotFound             = errors.New("not found")
	ErrRelationNotFound     = errors.New("relation not found")
	ErrRelationInUse        = errors.New("relation in use")
	ErrDuplicate            = errors.New("duplicate")
	ErrCheckFailed          = errors.New("check failed")
	ErrSerializationFailure = errors.New("serialization failure")
	ErrTxAborted            = errors.New("transaction aborted")
	ErrExclusionViolation   = errors.New("exclusion violation")
	ErrMutexNotLocked       = errors.New("not locked")
	ErrOperatorError        = errors.New("operator error")
	ErrException            = errors.New("exception")
)

type (
	Option func(context.Context, *DB) error
	DB     struct {
		ddl           string
		master        *pgxpool.Pool
		lb            *lb
		acquiredLocks *xsync.Map[int64, *pgxpool.Conn]
		closed        *atomic.Bool
	}
	Mutex interface {
		Lock(ctx context.Context) error
		Unlock(ctx context.Context) error
		EnsureLocked(ctx context.Context) error
	}
	Iterator[T any] = iter.Seq2[T, error]
	Error           = pgconn.PgError
)

type (
	lb struct {
		Replicas     []*pgxpool.Pool
		CurrentIndex uint64
	}
	advisoryLockMutex struct {
		conn *pgxpool.Conn
		db   *DB
		id   int64
	}
)
