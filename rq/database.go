// SPDX-License-Identifier: ice License 1.0

package rq

import (
	"context"
	"fmt"
	"net"
	"net/url"
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
	databaseClient struct {
		Closed       *atomic.Bool
		Active       atomic.Pointer[pgxpool.Pool]
		Masters      []string
		CurrentIndex uint64
		SwitchMu     sync.Mutex
	}
)

func newDatabaseClient(ctx context.Context, username, password string, urls ...string) (*databaseClient, error) {
	client := &databaseClient{
		Closed:  new(atomic.Bool),
		Masters: urls,
	}

	if len(client.Masters) == 0 {
		return nil, errors.New("no write URLs provided")
	}

	for i := range client.Masters {
		var err error
		client.Masters[i], err = createPgURL(username, password, urls[i])
		if err != nil {
			return nil, errors.Wrapf(err, "cannot create pg URL for write URL at index %d", i)
		}
	}

	for i, connectionString := range client.Masters {
		conn, err := poolConnect(ctx, connectionString)
		if err != nil {
			log.Warn().
				Str("context", "rq-database").
				Err(err).
				Int("master_index", i).
				Msg("cannot connect to master at index")
			continue
		}
		client.Active.Store(conn)
		client.CurrentIndex = uint64(i)
		break // Use the first successfully connected master as the active one.
	}

	if client.Active.Load() == nil {
		return nil, errors.Errorf("no active master was found among %d write URLs", len(client.Masters))
	}

	return client, nil
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

func (db *databaseClient) Close() error {
	db.Closed.Store(true)
	if instance := db.Active.Swap(nil); instance != nil {
		instance.Close()
	}
	return nil
}

func (db *databaseClient) Get() *pgxpool.Pool {
	if len(db.Masters) == 0 {
		return nil
	}
	return db.Active.Load()
}

func shouldSwitchMaster(err error) bool {
	var (
		netOpErr  *net.OpError
		pgErr     *pgconn.PgError
		pgconnErr *pgconn.ConnectError
	)

	if err == nil {
		return false
	}

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

func (db *databaseClient) switchMaster(ctx context.Context, reason error) error {
	var oldMaster *pgxpool.Pool

	currentMaster := db.Active.Load()
	db.SwitchMu.Lock()
	defer db.SwitchMu.Unlock()

	if currentMaster != nil && currentMaster != db.Active.Load() {
		// Already switched to a new master, no need to switch again.
		return nil
	}

	for _, i := range calculateConnectOrder(db.Masters, int(db.CurrentIndex)) {
		conn, err := poolConnect(ctx, db.Masters[i])
		if err != nil {
			log.Warn().
				Str("context", "rq-database").
				Err(err).
				Int("master_index", i).
				Msg("cannot connect to master at index")
			continue
		}
		log.Info().
			Str("context", "rq-database").
			Int("from_index", int(db.CurrentIndex)).
			Int("to_index", i).
			Err(reason).
			Msg("switching master")
		oldMaster = db.Active.Swap(conn)
		db.CurrentIndex = uint64(i)
		break
	}

	if oldMaster != nil {
		oldMaster.Close()
		return nil
	}

	return errors.Errorf("no active master was found among %d write URLs", len(db.Masters))
}

func calculateConnectOrder(addresses []string, currentIndex int) []int {
	all := make([]int, len(addresses))
	for i := range all {
		all[i] = i
	}
	return append(all[currentIndex+1:], all[:currentIndex]...)
}

func createPgURL(username, password, target string) (string, error) {
	if !strings.HasPrefix(target, "postgres://") && !strings.HasPrefix(target, "postgresql://") {
		target = "postgres://" + target
	}

	parsed, err := url.Parse(target)
	if err != nil {
		return "", err
	}

	if parsed.User.Username() == "" {
		parsed.User = url.UserPassword(username, password)
	}

	return parsed.String(), nil
}

func (db *databaseClient) Ping(ctx context.Context) error {
	i := db.Get()
	if i == nil {
		return errors.Errorf("no active masters to ping")
	}
	return errors.Wrap(i.Ping(ctx), "ping failed for master")
}
