// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"log"
	"strings"
	"sync"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	"github.com/jmoiron/sqlx/reflectx"
)

type (
	dbClient struct {
		*sqlx.DB
		relayPrivateKey string
		relayURL        string
		stmtCacheMx     *sync.RWMutex
		stmtCache       map[string]*sqlx.NamedStmt
	}
)

var (
	//go:embed DDL.sql
	ddl string
)

func openDatabase(target string, runDDL bool) *dbClient {
	client := &dbClient{
		DB:          sqlx.MustConnect("pgx", target),
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
		case "systemkind":
			out = "system_kind"
		case "referenceid":
			out = "reference_id"
		case "sigalg":
			out = "sig_alg"
		case "keyalg":
			out = "key_alg"
		case "masterpubkey":
			out = "master_pubkey"
		case "dtag":
			out = "d_tag"
		case "htag":
			out = "h_tag"
		case "hasimages":
			out = "has_images"
		case "hasvideos":
			out = "has_videos"
		default:
			out = n
		}

		return out
	})

	if runDDL {
		tx := client.MustBegin()
		defer tx.Rollback()
		for statement := range strings.SplitSeq(ddl, "--------") {
			_, err := tx.Exec(statement)
			if err != nil {
				log.Fatalf("DDL failed:\n%s\nERROR: %s", statement, err)
			}
		}
		tx.Commit()
	}

	return client
}

func (db *dbClient) WithRelayURL(relayURL string) *dbClient {
	db.relayURL = relayURL

	return db
}

func (db *dbClient) WithPrivateKey(privateKey string) *dbClient {
	if privateKey == "" {
		panic("private key is empty")
	}
	db.relayPrivateKey = privateKey

	return db
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
