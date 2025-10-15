package internal

import (
	"context"
	"fmt"
	"net"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

type (
	Option        func(context.Context, *DB) error
	MigrationFunc func(ctx context.Context, pool *pgxpool.Pool) error
	DB            struct {
		closed     *atomic.Bool
		lb         *lb
		migrations map[string]MigrationFunc
	}
	lb struct {
		Masters      []string
		Active       atomic.Pointer[pgxpool.Pool]
		CurrentIndex uint64
		SwitchMu     sync.Mutex
	}
)

var (
	ErrNotFound             = errors.New("not found")
	ErrSerializationFailure = errors.New("serialization failure")
	ErrTxAborted            = errors.New("transaction aborted")
	ErrReadOnly             = errors.New("read only")
)

func WithWriteURLs(urls ...string) Option {
	return func(ctx context.Context, db *DB) error {
		db.lb.Masters = urls
		for i, connectionString := range urls {
			conn, err := poolConnect(ctx, connectionString)
			if err != nil {
				log.Printf("[DATABASE]: WARNING: cannot connect to master %s: %v", connectionString, err)
				continue
			}
			db.lb.Active.Store(conn)
			db.lb.CurrentIndex = uint64(i)
			break // Use the first successfully connected master as the active one.
		}
		if len(db.lb.Masters) == 0 {
			return errors.New("no write URLs provided")
		} else if db.lb.Active.Load() == nil {
			return errors.Errorf("no active master was found among %d write URLs", len(db.lb.Masters))
		}
		return nil
	}
}
func WithMigration(key string, f MigrationFunc) Option {
	return func(ctx context.Context, db *DB) error {
		db.migrations[key] = f
		return nil
	}
}

func NewDBConn(ctx context.Context, opts ...Option) (*DB, error) {
	db := &DB{
		lb:         new(lb),
		closed:     new(atomic.Bool),
		migrations: make(map[string]MigrationFunc, 1),
	}

	for i := range opts {
		if err := opts[i](ctx, db); err != nil {
			return nil, err
		}
	}

	if len(db.lb.Masters) > 0 {
		log.Info().Str("context", "DATABASE").Msg("running migrations")
		pool := db.lb.Active.Load()
		if pool == nil {
			log.Panic().Str("context", "DATABASE").Msg("no active master to run migrations")
		}
		for migrationKey, migrationFunc := range db.migrations {
			if err := parseError(migrationFunc(ctx, pool)); err != nil {
				if errors.Is(err, ErrReadOnly) {
					log.Info().Err(err).Str("details", errors.FlattenDetails(err)).Str("migrationKey", migrationKey).Msg("DDL for key failed because the database is in read-only mode")
				} else {
					return nil, errors.Wrap(err, "ddl failed")
				}
			}
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
	if !strings.Contains(strings.ToLower(connectionString), "pool_min_conns") {
		conf.MinConns = 1
	}
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

	if instance := db.lb.Active.Swap(nil); instance != nil {
		instance.Close()
	}

	return nil
}

func (db *DB) primary() *pgxpool.Pool {
	if len(db.lb.Masters) == 0 {
		return nil
	}
	return db.lb.Active.Load()
}

func isDBDead(err error) bool {
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

func parseError(err error) error {
	var dbErr *pgconn.PgError

	if err == nil {
		return nil
	} else if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}

	if errors.As(err, &dbErr) {
		switch dbErr.SQLState() {
		case pgerrcode.SerializationFailure, pgerrcode.DeadlockDetected:
			return errors.WithDetail(ErrSerializationFailure, dbErr.Error())
		case pgerrcode.InFailedSQLTransaction:
			return ErrTxAborted
		case pgerrcode.ReadOnlySQLTransaction, pgerrcode.FeatureNotSupported:
			return errors.WithDetail(ErrReadOnly, dbErr.Error())
		}
	}

	return err
}

func (db *DB) switchMaster(ctx context.Context, reason error) error {
	var oldMaster *pgxpool.Pool

	if len(db.lb.Masters) == 0 {
		return errors.Wrap(ErrReadOnly, "no write URLs provided")
	}

	currentMaster := db.lb.Active.Load()
	db.lb.SwitchMu.Lock()
	defer db.lb.SwitchMu.Unlock()

	if currentMaster != nil && currentMaster != db.lb.Active.Load() {
		// Already switched to a new master, no need to switch again.
		return nil
	}

	for _, i := range calculateConnectOrder(db.lb.Masters, int(db.lb.CurrentIndex)) {
		conn, err := poolConnect(ctx, db.lb.Masters[i])
		if err != nil {
			log.Printf("[DATABASE]: WARNING: cannot connect to master %s: %v", db.lb.Masters[i], err)
			continue
		}
		log.Printf("[DATABASE]: INFO: switching master: %d -> %d due to %s", db.lb.CurrentIndex, i, reason)
		oldMaster = db.lb.Active.Swap(conn)
		db.lb.CurrentIndex = uint64(i)
		break
	}

	if oldMaster != nil {
		oldMaster.Close()
		return nil
	}

	return errors.Errorf("no active master was found among %d write URLs", len(db.lb.Masters))
}

func calculateConnectOrder(addresses []string, currentIndex int) []int {
	all := make([]int, len(addresses))
	for i := range all {
		all[i] = i
	}
	return append(all[currentIndex+1:], all[:currentIndex]...)
}
