// SPDX-License-Identifier: ice License 1.0

package connector

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/georgysavva/scany/v2/dbscan"
	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/tracelog"

	"github.com/ice-blockchain/subzero/model"
)

func WithWriteURLs(urls ...string) Option {
	return func(ctx context.Context, db *DB) error {
		for i, connectionString := range urls {
			if strings.Contains(connectionString, "preferred=true") {
				db.writeLB.PreferredUrls = append(db.writeLB.PreferredUrls, uint64(i))
				urls[i] = strings.ReplaceAll(connectionString, "preferred=true", "")
			}
		}
		for _, prefIdx := range db.writeLB.PreferredUrls {
			conn, err := poolConnect(ctx, urls[prefIdx], db)
			if err != nil {
				log.Printf("[DATABASE]: WARNING: cannot connect to preferred master %s: %v", urls[prefIdx], err)
				continue
			}
			db.writeLB.Active.Store(conn)
			db.writeLB.CurrentIndex = uint64(prefIdx)
			break // Use the first successfully connected master as the active one.
		}
		db.writeLB.Masters = urls
		if db.writeLB.Active.Load() == nil {
			for i, connectionString := range urls {
				conn, err := poolConnect(ctx, connectionString, db)
				if err != nil {
					log.Printf("[DATABASE]: WARNING: cannot connect to master %s: %v", connectionString, err)
					continue
				}
				db.writeLB.Active.Store(conn)
				db.writeLB.CurrentIndex = uint64(i)
				break // Use the first successfully connected master as the active one.
			}
		}
		if len(db.writeLB.Masters) == 0 {
			return errors.New("no write URLs provided")
		} else if db.writeLB.Active.Load() == nil {
			return errors.Errorf("no active master was found among %d write URLs", len(db.writeLB.Masters))
		}
		return nil
	}
}

func WithReadURLs(urls ...string) Option {
	return func(ctx context.Context, db *DB) error {
		for _, connectionString := range urls {
			conn, err := poolConnect(ctx, connectionString, db)
			if err != nil {
				return errors.Wrap(err, "cannot connect to replica")
			}

			db.readLB.Replicas = append(db.readLB.Replicas, conn)
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

func WithLogging(enabled bool) Option {
	return func(_ context.Context, db *DB) error {
		db.logging = enabled

		return nil
	}
}

func WithFieldNameMapper(mapper NameMapperFunc) Option {
	return func(context.Context, *DB) error {
		scanApi, err := pgxscan.NewDBScanAPI(dbscan.WithFieldNameMapper(mapper))
		if err != nil {
			return errors.Wrap(err, "cannot create db scan api")
		}
		api, err := pgxscan.NewAPI(scanApi)
		if err != nil {
			return errors.Wrap(err, "cannot create scan api")
		}

		// Override the default API with the new one with the custom field name mapper.
		pgxscan.DefaultAPI = api

		return nil
	}
}

func New(ctx context.Context, opts ...Option) (*DB, error) {
	db := &DB{
		readLB:  new(readLB),
		writeLB: new(writeLB),
		closed:  new(atomic.Bool),
		logging: true,
	}

	for i := range opts {
		if err := opts[i](ctx, db); err != nil {
			return nil, err
		}
	}

	if db.ddl != "" && len(db.writeLB.Masters) > 0 {
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
			if errors.Is(err, ErrReadOnly) {
				log.Printf("INFO: DDL failed because the database is in read-only mode: %v %s", err, errors.FlattenDetails(err))
			} else {
				return nil, errors.Wrap(err, "ddl failed")
			}
		}
	}

	return db, nil
}

func poolConnect(ctx context.Context, connectionString string, log tracelog.Logger) (*pgxpool.Pool, error) {
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
	conf.ConnConfig.Tracer = &tracelog.TraceLog{Logger: log, LogLevel: tracelog.LogLevelDebug}
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

	if instance := db.writeLB.Active.Swap(nil); instance != nil {
		instance.Close()
	}
	for _, replica := range db.readLB.Replicas {
		replica.Close()
	}

	return nil
}

func (db *DB) Ping(ctx context.Context) (err error) {
	var wg sync.WaitGroup

	errChan := make(chan error, len(db.readLB.Replicas)+1)
	if instance := db.writeLB.Active.Load(); instance != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errChan <- errors.Wrap(instance.Ping(ctx), "ping failed for master")
		}()
	}
	wg.Add(len(db.readLB.Replicas))
	for ii := range db.readLB.Replicas {
		go func(ix int) {
			defer wg.Done()
			errChan <- errors.Wrapf(db.readLB.Replicas[ix].Ping(ctx), "ping failed for replica[%v]", ix)
		}(ii)
	}

	wg.Wait()
	close(errChan)
	for errValue := range errChan {
		err = errors.Join(err, errValue)
	}
	return err
}

func CalculateConnectOrder(addresses []string, currentIndex int) []int {
	all := make([]int, len(addresses))
	for i := range all {
		all[i] = i
	}
	return append(all[currentIndex+1:], all[:currentIndex]...)
}

func (db *DB) switchMaster(ctx context.Context, reason error, preferredConn *pgxpool.Pool, preferredIdx *uint64) error {
	var oldMaster *pgxpool.Pool
	var oldMasterIdx uint64

	if len(db.writeLB.Masters) == 0 {
		return errors.Wrap(ErrReadOnly, "no write URLs provided")
	}

	currentMaster := db.writeLB.Active.Load()
	db.writeLB.SwitchMu.Lock()
	defer db.writeLB.SwitchMu.Unlock()

	if currentMaster != nil && currentMaster != db.writeLB.Active.Load() {
		// Already switched to a new master, no need to switch again.
		return nil
	}
	if preferredConn == nil || preferredIdx == nil {
		for _, i := range CalculateConnectOrder(db.writeLB.Masters, int(db.writeLB.CurrentIndex)) {
			conn, err := poolConnect(ctx, db.writeLB.Masters[i], db)
			if err != nil {
				log.Printf("[DATABASE]: WARNING: cannot connect to master %s: %v", db.writeLB.Masters[i], err)
				continue
			}
			log.Printf("[DATABASE]: INFO: switching master: %d -> %d due to %s", db.writeLB.CurrentIndex, i, reason)
			oldMasterIdx = db.writeLB.CurrentIndex
			oldMaster = db.writeLB.Active.Swap(conn)
			db.writeLB.CurrentIndex = uint64(i)
			break
		}
	} else {
		log.Printf("[DATABASE]: INFO: switching master: %d -> %d due to %s", db.writeLB.CurrentIndex, *preferredIdx, reason)
		oldMasterIdx = db.writeLB.CurrentIndex
		oldMaster = db.writeLB.Active.Swap(preferredConn)
		db.writeLB.CurrentIndex = uint64(*preferredIdx)
	}

	if oldMaster != nil {
		if slices.Contains(db.writeLB.PreferredUrls, oldMasterIdx) &&
			!slices.Contains(db.writeLB.PreferredUrls, db.writeLB.CurrentIndex) {
			if db.writeLB.cancelPreferredMasterSwitch != nil {
				db.writeLB.cancelPreferredMasterSwitch()
			}
			waitCtx, cancel := context.WithCancel(context.Background())
			db.writeLB.cancelPreferredMasterSwitch = cancel
			go db.connectToPreferredMasterOnceAvailable(waitCtx, oldMasterIdx)
		}
		if !errors.Is(reason, errPreferredAvailable) {
			oldMaster.Close()
		} else {
			go db.waitPoolFreeToClose(ctx, oldMaster)
		}

		return nil
	}

	return errors.Errorf("no active master was found among %d write URLs", len(db.writeLB.Masters))
}

func (db *DB) waitPoolFreeToClose(ctx context.Context, oldMaster *pgxpool.Pool) {
	defer func() {
		if db.writeLB.cancelPreferredMasterSwitch != nil {
			db.writeLB.cancelPreferredMasterSwitch()
		}
	}()
	for ctx.Err() == nil {
		stat := oldMaster.Stat()
		if stat.TotalConns() == 0 || stat.TotalConns() == stat.IdleConns() {
			oldMaster.Close()
			break
		}
		select {
		case <-time.After(10 * time.Second):
			continue
		case <-ctx.Done():
			oldMaster.Close()
			break
		}
	}
}

func (db *DB) connectToPreferredMasterOnceAvailable(ctx context.Context, preferredIdx uint64) {
	for ctx.Err() == nil {
		conn, err := poolConnect(ctx, db.writeLB.Masters[preferredIdx], db)
		if err != nil {
			log.Printf("[DATABASE]: WARNING: cannot connect to preferred master %s, still down: %v", db.writeLB.Masters[preferredIdx], err)
			select {
			case <-time.After(10 * time.Second):
				continue
			case <-ctx.Done():
				break
			}
		}
		pooledConn, err := conn.Acquire(ctx)
		if pooledConn != nil {
			defer pooledConn.Release()
			log.Printf("[DATABASE]: INFO: connecting to preferred master: %d -> %d", db.writeLB.CurrentIndex, preferredIdx)
			if err = db.switchMaster(ctx, errors.Wrapf(errPreferredAvailable, "preferred master %d is available", preferredIdx), conn, &preferredIdx); err != nil {
				log.Printf("[DATABASE]: WARNING: cannot connect to preferred master %s: %v", db.writeLB.Masters[preferredIdx], err)
			}
			break
		}
		log.Printf("[DATABASE]: WARNING: cannot connect to preferred master %s, still down: %v", db.writeLB.Masters[preferredIdx], err)
		select {
		case <-time.After(10 * time.Second):
			continue
		case <-ctx.Done():
			break
		}
	}
}

func (db *DB) primary() QueryExecerTx {
	if len(db.writeLB.Masters) == 0 {
		return new(readOnlyDB)
	}
	return db.writeLB.Active.Load()
}

func (db *DB) replica() Querier {
	next := db.readLB.Next()
	if next == nil {
		next = db.primary()
	}
	return next
}

func (*DB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	panic("should not be used because its implemented just for type matching")
}

func (*DB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	panic("should not be used because its implemented just for type matching")
}

func (r *readLB) Next() Querier {
	if len(r.Replicas) == 0 {
		return nil
	}

	index := atomic.AddUint64(&r.CurrentIndex, 1) % uint64(len(r.Replicas))
	return r.Replicas[index]
}

func (db *DB) Log(ctx context.Context, level tracelog.LogLevel, msg string, data map[string]any) {
	if !db.logging {
		return
	}

	prefix := "[PGX] " + time.Now().Format(time.RFC3339Nano) + " "
	if v := model.GetUserDataFromContext(ctx); v.Authenticated {
		prefix += " master: [" + v.MasterPublicKey + "]"
		if v.UserAgent != "" {
			prefix += " agent: [" + v.UserAgent + "]"
		}
	}
	log.Printf(prefix+": %s: %s %v", level, msg, data)
}
