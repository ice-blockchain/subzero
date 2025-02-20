// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/jmoiron/sqlx"
	"github.com/jmoiron/sqlx/reflectx"
	"github.com/mattn/go-sqlite3"
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

func init() {
	sql.Register("sqlite3_subzero",
		&sqlite3.SQLiteDriver{
			ConnectHook: func(conn *sqlite3.SQLiteConn) error {
				funcTable := []struct {
					// Function name to use in SQL.
					Name string
					// Pointer to the function.
					Ptr any
					// Pure flag.
					Pure bool
				}{
					{
						Name: "subzero_nostr_tags_reorder",
						Ptr:  sqlEventTagsReorderJSON,
						Pure: true,
					},
					{
						Name: "subzero_nostr_onbehalf_is_allowed",
						Ptr:  sqlObehalfIsAllowed,
						Pure: true,
					},
					{
						Name: "subzero_nostr_attestation_update_is_allowed",
						Ptr:  sqlAttestationUpdateIsAllowed,
						Pure: true,
					},
					{
						Name: "subzero_nostr_tag_a_get_kind",
						Ptr:  sqlTagAGetAt(0),
						Pure: true,
					},
					{
						Name: "subzero_nostr_tag_a_get_pk",
						Ptr:  sqlTagAGetAt(1),
						Pure: true,
					},
					{
						Name: "subzero_nostr_tag_a_get_dtag",
						Ptr:  sqlTagAGetAt(2),
						Pure: true,
					},
					{
						Name: "subzero_nostr_get_event_address",
						Ptr:  sqlGetEventAddress,
						Pure: true,
					},
					{
						Name: "subzero_nostr_generate_content_metadata",
						Ptr:  sqlGenerateContentMetadata,
						Pure: true,
					},
					{
						Name: "subzero_nostr_replace_special_chars",
						Ptr:  subzeroNostrReplaceSpecialChars,
						Pure: true,
					},
					{
						Name: "subzero_nostr_event_detect_systemd_kind",
						Ptr:  sqlEventDetectSystemdKind,
						Pure: true,
					},
				}

				for idx := range funcTable {
					if err := conn.RegisterFunc(funcTable[idx].Name, funcTable[idx].Ptr, funcTable[idx].Pure); err != nil {
						return errors.Wrapf(err, "failed to register func %q", funcTable[idx].Name)
					}
				}

				return nil
			},
		})
}

func openDatabase(target string, runDDL bool) *dbClient {
	client := &dbClient{
		DB:          sqlx.MustConnect("sqlite3_subzero", target),
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
		case "contentmetadata":
			out = "content_metadata"
		default:
			out = n
		}

		return out
	})

	if runDDL {
		tx := client.MustBegin()
		defer tx.Rollback()
		for _, statement := range strings.Split(ddl, "--------") {
			tx.MustExec(statement)
		}
		client.alterEventsTable(tx)
		tx.Commit()
	}

	return client
}

func (db *dbClient) alterEventsTable(tx *sqlx.Tx) {
	columns := map[string]string{
		"system_kind": "ALTER TABLE events ADD COLUMN system_kind integer",
		"deleted":     "ALTER TABLE events ADD COLUMN deleted integer not null DEFAULT 0",
		"address": `ALTER TABLE events ADD COLUMN address text not null generated always as (
CASE
WHEN (10000 <= kind AND kind < 20000) OR kind = 0 OR kind = 3 THEN concat(coalesce(kind,0), ':', coalesce(master_pubkey,pubkey,''),':')
WHEN 30000 <= kind AND kind < 40000                           THEN concat(coalesce(kind,0), ':', coalesce(master_pubkey,pubkey,''),':',coalesce(d_tag,''))
ELSE id
END) VIRTUAL`,
	}

	for column, statement := range columns {
		var doAlter bool

		err := tx.QueryRow("SELECT not exists (select name from pragma_table_xinfo('events') WHERE name = $1)", column).Scan(&doAlter)
		if err != nil {
			panic("failed to check if column " + column + " exists: " + err.Error())
		}

		if doAlter {
			tx.MustExec(statement)
		}
	}
	// TODO: move it to the ddl after the migration.
	tx.MustExec(`CREATE INDEX IF NOT EXISTS idx_events_address ON events(address)`)

	var minEventDate int64
	err := tx.QueryRow("select coalesce(min(event_created_at), 0) from ranked_events").Scan(&minEventDate)
	if err != nil {
		panic("failed to get min event date: " + err.Error())
	}

	dateCutOff := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
	if minEventDate == 0 || minEventDate > dateCutOff {
		tx.MustExec(`
		update events
		set
			system_kind = iif(subzero_nostr_event_detect_systemd_kind(tags) >= 0, subzero_nostr_event_detect_systemd_kind(tags), NULL)
		where
			system_kind is null
			and hidden=0
			and deleted=0
		`)
		tx.MustExec(`delete from ranked_events`)

		const rankUpdate = `
		insert into ranked_events(event_rid, event_kind, event_created_at, points, score)
		with cte as (
			select
				e.*,
				et.event_tag_value1 as event_address
			from
				events e
			inner join event_tags et on e.id = et.event_id
			where
				et.event_tag_key in ('a', 'e', 'q', 'Q')
				and (e.system_kind is null or
					case
						when e.system_kind = 2 then et.event_tag_value3 = 'root'
						when e.system_kind = 3 then false -- ignore replies
						else true
					end)
				and hidden=0
				and deleted=0
		)
		select
			e.rid,
			e.kind,
			e.created_at,
			case
				when cte.kind = 7 then 1                                        -- like
				when cte.kind in (6, 16) then 3                                 -- repost
				when cte.system_kind is not null and cte.system_kind = 1 then 4 -- quote
				when cte.system_kind is not null and cte.system_kind = 2 then 2 -- top level comment (root)
				else 0
			end,
			(round(( case
				when cte.kind = 7 then 1
				when cte.kind in (6, 16) then 3
				when cte.system_kind is not null and cte.system_kind = 1 then 4
				when cte.system_kind is not null and cte.system_kind = 2 then 2
				else 0
			end / power((1 + (unixepoch() - min(unixepoch(), e.created_at))/3600.0), 0.9)), 4))
		from
			events e
		inner join cte on e.address = cte.event_address
		where
			e.hidden = 0
			and e.deleted = 0
			and e.id = e.h_tag
			and e.created_at > 0
			and e.kind in (1, 30023, 30175)
			and (cte.system_kind is null or cte.system_kind != 3)
		on conflict do update
		set
			points = points + excluded.points,
			score = (round((points + excluded.points / power((1 + (unixepoch() - min(unixepoch(), event_created_at))/3600.0), 0.9)), 4));
		`
		tx.MustExec(rankUpdate)
	}
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
