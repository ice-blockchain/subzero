package query

import (
	"context"
	"crypto/sha256"
	"github.com/gookit/goutil/errorx"
	"github.com/jmoiron/sqlx"
	"sync"
)

var globalDB *dbClient

type (
	dbClient struct {
		*sqlx.DB

		stmtCacheMx *sync.Mutex
		stmtCache   map[string]*sqlx.NamedStmt
	}
)

func MustInit() {
	globalDB = &dbClient{
		DB:          sqlx.MustConnect("sqlite3", ":memory:"), //TODO impl this properly
		stmtCacheMx: new(sync.Mutex),
		stmtCache:   make(map[string]*sqlx.NamedStmt),
	}
}

func (db *dbClient) exec(ctx context.Context, sql string, arg any) (rowsAffected int64, err error) {
	var (
		hash = hashSQL(sql)
	)
	if err = db.prepare(ctx, sql, hash); err != nil {
		return 0, errorx.Withf(err, "failed to prepare exec sql: `%v`", sql)
	}

	result, err := db.stmtCache[hash].ExecContext(ctx, arg)
	if err != nil {
		return 0, errorx.Withf(err, "failed to exec prepared sql: `%v`", sql)
	}
	if rowsAffected, err = result.RowsAffected(); err != nil {
		return 0, errorx.Withf(err, "failed to process rows affected for exec prepared sql: `%v`", sql)
	}

	return rowsAffected, nil
}

func (db *dbClient) query(ctx context.Context, sql string, arg, dest any) error {
	var (
		hash = hashSQL(sql)
	)
	if err := db.prepare(ctx, sql, hash); err != nil {
		return errorx.Withf(err, "failed to prepare query sql: `%v`", sql)
	}

	if err := db.stmtCache[hash].SelectContext(ctx, dest, arg); err != nil {
		return errorx.Withf(err, "failed to select prepared sql: `%v`, arg: %#v", sql, arg)
	}

	return nil
}

func (db *dbClient) prepare(ctx context.Context, sql, hash string) (err error) {
	if stmt, found := db.stmtCache[hash]; !found {
		db.stmtCacheMx.Lock()
		if stmt, err = db.PrepareNamedContext(ctx, sql); err != nil {
			db.stmtCacheMx.Unlock()

			return err
		}
		db.stmtCache[hash] = stmt
		db.stmtCacheMx.Unlock()
	}

	return nil
}

func hashSQL(sql string) (hash string) {
	h := sha256.New()
	if _, err := h.Write([]byte(sql)); err != nil {
		panic(err)
	}
	hash = string(h.Sum(nil))

	return hash
}
