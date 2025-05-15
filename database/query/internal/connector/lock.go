// SPDX-License-Identifier: ice License 1.0

package connector

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/zeebo/xxh3"
)

func NewMutex(db *DB, lockID string) Mutex {
	return &advisoryLockMutex{
		db: db,
		id: int64(xxh3.HashString(lockID)),
	}
}

func (l *advisoryLockMutex) Lock(ctx context.Context) error {
	var isLockAquired bool

	conn, err := l.db.primary().Acquire(ctx)
	if err != nil {
		return errors.Wrapf(err, "failed to acquire connection to DB")
	}
	if err = conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1);", l.id).Scan(&isLockAquired); err != nil {
		defer conn.Release()
		return errors.Wrapf(err, "failed to pg_try_advisory_lock for advisoryLockMutex %v", l.id)
	}
	if !isLockAquired {
		defer conn.Release()
		return ErrMutexNotLocked
	}

	l.conn = conn
	l.db.acquiredLocks.Store(l.id, l.conn)

	return nil
}

func (l *advisoryLockMutex) Unlock(ctx context.Context) error {
	_, err := l.conn.Exec(ctx, "SELECT pg_advisory_unlock($1);", l.id)
	if err != nil {
		return errors.Wrapf(err, "failed to pg_advisory_unlock for advisoryLockMutex %v", l.id)
	}
	l.conn.Release()
	l.db.acquiredLocks.Delete(l.id)
	l.conn = nil

	return nil
}

func (l *advisoryLockMutex) EnsureLocked(ctx context.Context) error {
	if l.conn == nil {
		// Another runtime.
		if existsErr := l.checkIfAnotherRuntimeHandlesLock(ctx); existsErr != nil && errors.Is(existsErr, ErrNotFound) {
			return l.Lock(ctx)
		}

		return ErrMutexNotLocked
	}
	if l.db.closed.Load() {
		return ErrTxAborted
	}
	if l.conn.Conn().IsClosed() {
		return l.Lock(ctx)
	}
	if l.conn.Ping(ctx) != nil {
		l.conn.Release()

		return l.Lock(ctx)
	}

	return nil
}

func (l *advisoryLockMutex) checkIfAnotherRuntimeHandlesLock(ctx context.Context) error {
	_, err := execOne[struct {
		PID int32 `db:"pid"`
	}](ctx, l.db.primary(), "SELECT pid FROM pg_locks WHERE objid = $1 and granted = true", int32(l.id))

	return err
}
