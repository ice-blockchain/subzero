// SPDX-License-Identifier: ice License 1.0

package connector

import (
	"context"
	"errors"
	"iter"
	"sync"
	"sync/atomic"

	"github.com/georgysavva/scany/v2/dbscan"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
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
	ErrInvalidData          = errors.New("invalid data")
	ErrInternal             = errors.New("internal error")
	ErrReadOnly             = errors.New("read only")
	errPreferredAvailable   = errors.New("preferred available")
)

type (
	Option func(context.Context, *DB) error
	DB     struct {
		ddl     string
		logging bool
		writeLB *writeLB
		readLB  *readLB
		closed  *atomic.Bool
	}
	Iterator[T any] = iter.Seq2[T, error]
	Error           = pgconn.PgError
	NameMapperFunc  = dbscan.NameMapperFunc
)

type (
	readLB struct {
		Replicas     []*pgxpool.Pool
		CurrentIndex uint64
	}
	writeLB struct {
		Masters                     []string
		PreferredUrls               []uint64
		Active                      atomic.Pointer[pgxpool.Pool]
		CurrentIndex                uint64
		SwitchMu                    sync.Mutex
		cancelPreferredMasterSwitch context.CancelFunc
	}
)
