// SPDX-License-Identifier: ice License 1.0

package connector

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	_ QueryExecerTx = (*readOnlyDB)(nil)
)

type readOnlyDB struct{}

func (*readOnlyDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.WithDetail(ErrReadOnly, "write-urls are not configured")
}

func (*readOnlyDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.WithDetail(ErrReadOnly, "read-urls are not configured")
}

func (*readOnlyDB) BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error) {
	return nil, errors.WithDetail(ErrReadOnly, "transaction is not supported in read-only mode")
}
