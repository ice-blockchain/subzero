// SPDX-License-Identifier: ice License 1.0

package connector

import (
	"context"
	"errors"
	"io/fs"
	"iter"
	"sync"
	"sync/atomic"

	"github.com/georgysavva/scany/v2/dbscan"
	"github.com/georgysavva/scany/v2/pgxscan"
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
)

type (
	Option func(context.Context, *DB) error
	DB     struct {
		writeLB *writeLB
		readLB  *readLB
		closed  *atomic.Bool
		scanner *pgxscan.API
		ddl     fs.FS
		logging bool
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
		Active                      atomic.Pointer[pgxpool.Pool]
		CancelPreferredMasterSwitch context.CancelFunc
		Masters                     []string
		PreferredUrl                uint64
		CurrentIndex                uint64
		SwitchMu                    sync.Mutex
	}
)

const (
	pingsForPreferredMasterSwitch = 6
)

var (
	errPreferredAvailable = errors.New("preferred available")
)
