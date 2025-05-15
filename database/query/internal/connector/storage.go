// SPDX-License-Identifier: ice License 1.0

package connector

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/puzpuzpuz/xsync/v4"
)

func WithMaster(connectionString string) Option {
	return func(ctx context.Context, db *DB) error {
		conn, err := poolConnect(ctx, connectionString)
		if err != nil {
			return errors.Wrap(err, "cannot connect to master")
		}

		db.master = conn

		return nil
	}
}

func WithReplicas(connectionStrings []string) Option {
	return func(ctx context.Context, db *DB) error {
		for _, connectionString := range connectionStrings {
			conn, err := poolConnect(ctx, connectionString)
			if err != nil {
				return errors.Wrap(err, "cannot connect to replica")
			}

			db.lb.Replicas = append(db.lb.Replicas, conn)
		}
		return nil
	}
}

func WithDDL(ddl string) Option {
	return func(_ context.Context, db *DB) error {
		db.ddl = ddl

		return nil
	}
}

func New(ctx context.Context, opts ...Option) (*DB, error) {
	db := &DB{
		lb:            new(lb),
		closed:        new(atomic.Bool),
		acquiredLocks: xsync.NewMap[int64, *pgxpool.Conn](),
	}

	for i := range opts {
		if err := opts[i](ctx, db); err != nil {
			return nil, err
		}
	}

	if db.ddl != "" {
		if db.master == nil {
			return nil, errors.Errorf("ddl is set but master is not set")
		}
		err := DoInTransaction(ctx, db, func(conn QueryExecer) error {
			for statement := range strings.SplitSeq(db.ddl, "--------") {
				_, err := conn.Exec(ctx, statement)
				if err != nil {
					return errors.Wrapf(err, "statement failed: %s", statement)
				}
			}
			return nil
		})
		if err != nil {
			return nil, errors.Wrap(err, "ddl failed")
		}
	}

	return db, nil
}

func poolConnect(ctx context.Context, connectionString string) (*pgxpool.Pool, error) {
	conf, err := pgxpool.ParseConfig(connectionString)
	if err != nil {
		return nil, errors.Wrap(err, "cannot parse connection string")
	}

	conf.ConnConfig.StatementCacheCapacity = 1024
	conf.ConnConfig.DescriptionCacheCapacity = 1024
	conf.ConnConfig.Config.ConnectTimeout = 30 * time.Second
	if !strings.Contains(strings.ToLower(connectionString), "pool_max_conn_idle_time") {
		conf.MaxConnIdleTime = time.Minute
	}
	if !strings.Contains(strings.ToLower(connectionString), "pool_health_check_period") {
		conf.HealthCheckPeriod = 30 * time.Second
	}

	conf.MaxConnLifetimeJitter = 10 * time.Minute
	conf.MaxConnLifetime = 24 * time.Hour
	conf.AfterConnect = poolDoAfterConnect
	conf.MinConns = 1

	return pgxpool.NewWithConfig(ctx, conf)
}

func poolDoAfterConnect(ctx context.Context, conn *pgx.Conn) error {
	const actualTimeout = "30s"

	customConnectionParameters := map[string]string{
		"statement_timeout":                   actualTimeout,
		"idle_in_transaction_session_timeout": actualTimeout,
		"lock_timeout":                        actualTimeout,
		// "tcp_user_timeout":                 actualTimeout,.
		"enable_partitionwise_join":      "on",
		"enable_partitionwise_aggregate": "on",
	}
	values := make([]string, 0, len(customConnectionParameters))
	for name, setting := range customConnectionParameters {
		values = append(values, fmt.Sprintf("'%v'", name))
		if _, qErr := conn.Exec(ctx, fmt.Sprintf(`SET %v = '%v'`, name, setting)); qErr != nil {
			return qErr
		}
	}

	sql := fmt.Sprintf(`SELECT name, setting
							FROM pg_settings
							WHERE name IN (%v)`, strings.Join(values, ","))
	rows, qErr := conn.Query(ctx, sql)
	if qErr != nil {
		return errors.Wrapf(qErr, "validation select failed")
	}
	var res []*struct{ Name, Setting string }
	if qErr = pgxscan.ScanAll(&res, rows); qErr != nil {
		return errors.New("scanning validation select rows failed")
	}
	actual := make(map[string]string, len(res))
	for _, row := range res {
		actual[row.Name] = strings.ReplaceAll(row.Setting, "0000", "0s")
	}
	if !reflect.DeepEqual(actual, customConnectionParameters) {
		return errors.Errorf("db validation failed, expected:%#v, actual:%#v", customConnectionParameters, actual)
	}

	return nil
}

func (db *DB) Close() error {
	db.closed.Store(true)

	db.acquiredLocks.Range(func(key int64, conn *pgxpool.Conn) bool {
		conn.Release()
		db.acquiredLocks.Delete(key)
		return true
	})

	if db.master != nil {
		db.master.Close()
	}
	for _, replica := range db.lb.Replicas {
		replica.Close()
	}

	return nil
}

func (db *DB) Ping(ctx context.Context) (err error) {
	var wg sync.WaitGroup

	errChan := make(chan error, len(db.lb.Replicas)+1)
	if db.master != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errChan <- errors.Wrap(db.master.Ping(ctx), "ping failed for master")
		}()
	}
	wg.Add(len(db.lb.Replicas))
	for ii := range db.lb.Replicas {
		go func(ix int) {
			defer wg.Done()
			errChan <- errors.Wrapf(db.lb.Replicas[ix].Ping(ctx), "ping failed for replica[%v]", ix)
		}(ii)
	}

	wg.Wait()
	close(errChan)
	for errValue := range errChan {
		err = errors.Join(err, errValue)
	}
	return err
}

func (db *DB) primary() *pgxpool.Pool {
	return db.master
}

func (db *DB) replica() *pgxpool.Pool {
	if len(db.lb.Replicas) == 0 {
		return db.primary()
	}
	return db.lb.Replicas[atomic.AddUint64(&db.lb.CurrentIndex, 1)%uint64(len(db.lb.Replicas))]
}

func (*DB) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	panic("should not be used because its implemented just for type matching")
}

func (*DB) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	panic("should not be used because its implemented just for type matching")
}
