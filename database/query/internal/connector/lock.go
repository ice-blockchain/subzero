// SPDX-License-Identifier: ice License 1.0

package connector

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/zeebo/xxh3"
)

func NewMutex(db *DB, lockID string) Mutex {
	return &advisoryLockMutex{
		DB: db,
		ID: int64(xxh3.HashString(lockID)),
	}
}

func (l *advisoryLockMutex) Lock(ctx context.Context) error {
	var isLockAquired bool

	conn, err := l.DB.primary().Acquire(ctx)
	if err != nil {
		return errors.Wrapf(err, "failed to acquire connection to DB")
	}
	if err = conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1);", l.ID).Scan(&isLockAquired); err != nil {
		defer conn.Release()
		return errors.Wrapf(err, "failed to pg_try_advisory_lock for advisoryLockMutex %v", l.ID)
	}
	if !isLockAquired {
		defer conn.Release()
		return ErrMutexNotLocked
	}

	l.Conn = conn
	l.DB.acquiredLocks.Store(l.ID, l.Conn)

	return nil
}

func (l *advisoryLockMutex) Unlock(ctx context.Context) error {
	_, err := l.Conn.Exec(ctx, "SELECT pg_advisory_unlock($1);", l.ID)
	if err != nil {
		return errors.Wrapf(err, "failed to pg_advisory_unlock for advisoryLockMutex %v", l.ID)
	}
	l.Conn.Release()
	l.DB.acquiredLocks.Delete(l.ID)
	l.Conn = nil

	return nil
}

func (l *advisoryLockMutex) EnsureLocked(ctx context.Context) error {
	if l.Conn == nil {
		// Another runtime.
		if existsErr := l.checkIfAnotherRuntimeHandlesLock(ctx); existsErr != nil && errors.Is(existsErr, ErrNotFound) {
			return l.Lock(ctx)
		}

		return ErrMutexNotLocked
	}
	if l.DB.closed.Load() {
		return ErrTxAborted
	}
	if l.Conn.Conn().IsClosed() {
		return l.Lock(ctx)
	}
	if l.Conn.Ping(ctx) != nil {
		l.Conn.Release()

		return l.Lock(ctx)
	}

	return nil
}

func (l *advisoryLockMutex) checkIfAnotherRuntimeHandlesLock(ctx context.Context) error {
	_, err := execOne[struct {
		PID int32 `db:"pid"`
	}](ctx, l.DB.primary(), "SELECT pid FROM pg_locks WHERE objid = $1 and granted = true", int32(l.ID))

	return err
}
