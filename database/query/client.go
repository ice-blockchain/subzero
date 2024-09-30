// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"strings"
	"sync"

	"github.com/jmoiron/sqlx"
	"github.com/jmoiron/sqlx/reflectx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/pkg/errors"
)

type (
	dbClient struct {
		*sqlx.DB

		stmtCacheMx *sync.RWMutex
		stmtCache   map[string]*sqlx.NamedStmt
	}
)

var (
	//go:embed DDL.sql
	ddl string
)

func openDatabase(target string, runDDL bool) *dbClient {
	client := &dbClient{
		DB:          sqlx.MustConnect("sqlite3", target), //TODO impl this properly
		stmtCacheMx: new(sync.RWMutex),
		stmtCache:   make(map[string]*sqlx.NamedStmt),
	}
	client.Mapper = reflectx.NewMapperFunc("subzero", func(in string) (out string) {
		n := strings.ToLower(in)
		switch n {
		case "createdat":
			out = "created_at"
		case "systemcreatedat":
			out = "system_created_at"
		case "referenceid":
			out = "reference_id"
		default:
			out = n
		}

		return out
	})

	if runDDL {
		for _, statement := range strings.Split(ddl, "--------") {
			client.MustExec(statement)
		}
	}

	return client
}

func (db *dbClient) exec(ctx context.Context, sql string, arg any) (rowsAffected int64, err error) {
	var (
		hash = hashSQL(sql)
	)

	stmt, err := db.prepare(ctx, sql, hash)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to prepare exec sql: `%v`", sql)
	}

	result, err := stmt.ExecContext(ctx, arg)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to exec prepared sql: `%v`", sql)
	}
	if rowsAffected, err = result.RowsAffected(); err != nil {
		return 0, errors.Wrapf(err, "failed to process rows affected for exec prepared sql: `%v`", sql)
	}

	return rowsAffected, nil
}

func (db *dbClient) prepare(ctx context.Context, sql, hash string) (stmt *sqlx.NamedStmt, err error) {
	db.stmtCacheMx.RLock()
	stmt, found := db.stmtCache[hash]
	db.stmtCacheMx.RUnlock()
	if found {
		return stmt, nil
	}

	db.stmtCacheMx.Lock()
	stmt, found = db.stmtCache[hash]
	if found {
		db.stmtCacheMx.Unlock()

		return stmt, nil
	}

	stmt, err = db.PrepareNamedContext(ctx, sql)
	if err == nil {
		db.stmtCache[hash] = stmt
	}
	db.stmtCacheMx.Unlock()

	return stmt, err
}

func hashSQL(sql string) (hash string) {
	sum := sha256.Sum256([]byte(sql))

	return string(sum[:])
}
