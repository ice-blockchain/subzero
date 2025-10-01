// SPDX-License-Identifier: ice License 1.0

package connector

import (
	"context"
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
	const threshold = 150 * time.Millisecond

	query, argList, err := bindNamed(stmt, params)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	res, err := ExecManyWithCustomRetry[T](ctx, db, retryIf, query, argList...)
	duration := time.Since(start)
	if duration < threshold {
		return res, err
	}

	conn := db.(*DB).writeLB.Active.Load()
	logger := log.Trace().
		Str("host", conn.Config().ConnConfig.Host).
		Dur("duration", duration).
		Str("context", "DATABASE").
		Str("query", query).
		Fields(map[string]any{"params": params}).
		Fields(formatStat(conn.Stat()))

	if v := model.GetUserDataFromContext(ctx); v.Authenticated {
		logger = logger.Str("master_key", v.MasterPublicKey)
		if v.UserAgent != "" {
			logger = logger.Str("user_agent", v.UserAgent)
		}
	}
	logger.Msg("slow query")

	return res, err
}

func formatStat(stat *pgxpool.Stat) map[string]any {
	return map[string]any{
		"acquire_count":              stat.AcquireCount(),
		"acquire_duration":           stat.AcquireDuration().String(),
		"acquired_conns":             stat.AcquiredConns(),
		"constructing_conns":         stat.ConstructingConns(),
		"canceled_acquire_count":     stat.CanceledAcquireCount(),
		"empty_acquire_count":        stat.EmptyAcquireCount(),
		"empty_acquire_wait_time":    stat.EmptyAcquireWaitTime().String(),
		"idle_conns":                 stat.IdleConns(),
		"max_idle_destroy_count":     stat.MaxIdleDestroyCount(),
		"new_conns_count":            stat.NewConnsCount(),
		"total_conns":                stat.TotalConns(),
		"max_lifetime_destroy_count": stat.MaxLifetimeDestroyCount(),
	}
}
