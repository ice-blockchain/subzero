// SPDX-License-Identifier: ice License 1.0

package connector

import (
	"context"
	"fmt"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"

	"github.com/ice-blockchain/subzero/model"
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

func ExecNamedManyWithCustomRetry[T any](ctx context.Context, db Querier, retryIf func(err error) bool, stmt string, params map[string]any) ([]*T, error) {
	query, argList, err := bindNamed(stmt, params)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	res, err := ExecManyWithCustomRetry[T](ctx, db, retryIf, query, argList...)
	duration := time.Since(now)
	if duration > 150*time.Millisecond {
		prefix := "[query]: stats: duration: [" + duration.String() + "]"
		if v := model.GetUserDataFromContext(ctx); v.Authenticated {
			prefix += " master: [" + v.MasterPublicKey + "]"
			if v.UserAgent != "" {
				prefix += " agent: [" + v.UserAgent + "]"
			}
		}
		conn := db.(*DB).writeLB.Active.Load()
		prefix += ": query saving to host %v, pool stats %v"
		log.Trace().Str("prefix", prefix).Str("host", conn.Config().ConnConfig.Host).Str("stat", formatStat(conn.Stat())).Msg("database connector stat")
	}
	return res, err
}

func formatStat(stat *pgxpool.Stat) string {
	return fmt.Sprintf(`pool{
	aquireCount: %v,
	aquireDuration: %v,
    aquiredConns: %v,
	constructingConns: %v,
	CanceledAcquireCount: %v,
	EmptyAcquireCount: %v,
	EmptyAcquireWaitTime: %v,
	Idle: %v,
	MaxIdleDestroyCount: %v,
	NewConnsCount: %v,
	Total: %v,
	MaxLifetimeDestroyCount: %v
}`,
		stat.AcquireCount(),
		stat.AcquireDuration(),
		stat.AcquiredConns(),
		stat.ConstructingConns(),
		stat.CanceledAcquireCount(),
		stat.EmptyAcquireCount(),
		stat.EmptyAcquireWaitTime(),
		stat.IdleConns(),
		stat.MaxIdleDestroyCount(),
		stat.NewConnsCount(),
		stat.TotalConns(),
		stat.MaxLifetimeDestroyCount(),
	)
}
