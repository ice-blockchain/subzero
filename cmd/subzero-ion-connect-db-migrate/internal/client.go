// SPDX-License-Identifier: ice License 1.0

package internal

import (
	"context"
	"database/sql"
	"io"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/jmoiron/sqlx"
	"github.com/jmoiron/sqlx/reflectx"
	"github.com/mattn/go-sqlite3"

	"github.com/ice-blockchain/subzero/model"
)

type (
	DB interface {
		SelectManagementEvents(ctx context.Context) EventIterator
		SelectDataEvents(ctx context.Context) EventIterator
		Count(ctx context.Context) (int64, error)
		io.Closer
	}

	dbClient struct {
		*sqlx.DB
	}
	databaseEvent struct {
		model.Event
		SystemCreatedAt int64
		SystemKind      sql.NullInt64
		ReferenceID     sql.NullString
		Jtags           string
		SigAlg          string
		KeyAlg          string
		MasterPubKey    string
		Dtag            string
		Htag            string
		ContentMetadata string
		Rid             int64
		Deleted         bool
	}
)

var (
	errEventIteratorInterrupted = errors.New("interrupted")
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
						Name: "subzero_nostr_get_event_address_json",
						Ptr:  sqlGetEventAddressJSON,
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

func MustOpen(target string) DB {
	client := &dbClient{
		DB: sqlx.MustConnect("sqlite3_subzero", target),
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

	return client
}

func (db *dbClient) Close() error {
	if db == nil {
		return nil
	}

	if db.DB == nil {
		return nil
	}

	if err := db.DB.Close(); err != nil {
		return errors.Wrap(err, "failed to close database")
	}

	return nil
}

func (db *dbClient) Count(ctx context.Context) (count int64, err error) {
	const stmt = `select count(*) from events where hidden=false`

	err = db.QueryRowxContext(ctx, stmt).Scan(&count)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, errors.Wrap(err, "failed to count events")
	}

	return count, nil
}

func (db *dbClient) SelectManagementEvents(ctx context.Context) EventIterator {
	const stmt = `
select
	e.kind,
	e.created_at,
	e.system_created_at,
	e.id,
	e.pubkey,
	e.master_pubkey,
	e.sig,
	e.content,
	e.rid,
	tags as jtags
from
	events e
where
	e.hidden=false and kind=10100
order by
	e.rid asc
`
	return db.newReadEventIterator(ctx, stmt, nil)
}

func (db *dbClient) SelectDataEvents(ctx context.Context) EventIterator {
	const stmt = `
select
	e.kind,
	e.created_at,
	e.system_created_at,
	e.id,
	e.pubkey,
	e.master_pubkey,
	e.sig,
	e.content,
	e.rid,
	tags as jtags
from
	events e
where
	e.hidden=false and kind!=10100
order by
	e.rid asc
`
	return db.newReadEventIterator(ctx, stmt, nil)
}
