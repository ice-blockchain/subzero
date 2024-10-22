// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/jmoiron/sqlx"
	"github.com/mattn/go-sqlite3"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/model"
)

const (
	selectDefaultBatchLimit = 100
)

var (
	ErrUnexpectedRowsAffected      = errors.New("unexpected rows affected")
	ErrTargetReactionEventNotFound = errors.New("target reaction event not found")
	ErrAttestationUpdateRejected   = errors.New("attestation update rejected")

	errEventIteratorInterrupted = errors.New("interrupted")
)

type databaseEvent struct {
	model.Event
	SystemCreatedAt int64
	ReferenceID     sql.NullString
	Jtags           string
	SigAlg          string
	KeyAlg          string
	MasterPubKey    string
	Dtag            string
}

type databaseBatchRequest struct {
	// Events to store or replace.
	InsertOrReplace []databaseEvent

	// Events to delete.
	Delete []databaseFilterDelete
}

func (req *databaseBatchRequest) Save(e *model.Event) error {
	jtags, err := json.Marshal(e.Tags)
	if err != nil {
		return errors.Wrap(err, "failed to marshal tags")
	}

	sigAlg, keyAlg, err := parseSigKeyAlg(e)
	if err != nil {
		return err
	}

	req.InsertOrReplace = append(req.InsertOrReplace, databaseEvent{
		Event:           *e,
		MasterPubKey:    e.GetMasterPublicKey(),
		SystemCreatedAt: time.Now().UnixNano(),
		Jtags:           string(jtags),
		SigAlg:          sigAlg,
		KeyAlg:          keyAlg,
		Dtag:            e.Tags.GetD(),
	})

	return nil
}

func (req *databaseBatchRequest) Remove(e *model.Event) error {
	f, err := parseEventAsFilterForDelete(e)
	if err != nil {
		return errors.Wrap(err, "failed to detect events for delete")
	}

	req.Delete = append(req.Delete, *f)

	return nil
}

func (req *databaseBatchRequest) Empty() bool {
	return len(req.InsertOrReplace) == 0 && len(req.Delete) == 0
}

func (db *dbClient) AcceptEvents(ctx context.Context, events ...*model.Event) error {
	var req databaseBatchRequest

	for i := range events {
		if events[i].IsEphemeral() {
			continue
		}

		if events[i].Kind == nostr.KindDeletion {
			if err := req.Remove(events[i]); err != nil {
				return err
			}
		} else {
			req.Save(events[i])
		}
	}

	return db.executeBatch(ctx, &req)
}

func parseSigKeyAlg(event *model.Event) (sigAlg, keyAlg string, err error) {
	sAlg, kAlg, _, err := event.ExtractSignature()
	if err != nil {
		return "", "", errors.Wrap(err, "failed to extract signature")
	}

	return string(sAlg), string(kAlg), nil
}

func (db *dbClient) deleteEvents(ctx context.Context, filters []databaseFilterDelete) error {
	where, params, err := newWhereBuilder().BuildForDelete(filters...)
	if err != nil {
		return errors.Wrap(err, "failed to generate events where clause")
	}

	stmt := `delete from events where ` + where

	rows, err := db.exec(ctx, stmt, params)
	if err != nil {
		err = errors.Wrap(db.handleError(err), "failed to exec delete event sql")
	} else if rows == 0 {
		err = ErrUnexpectedRowsAffected
	}

	return err
}

func (db *dbClient) saveEvents(ctx context.Context, events []databaseEvent) error {
	const stmt = `insert into events
	(kind, created_at, system_created_at, id, pubkey, master_pubkey, sig, sig_alg, key_alg, content, tags, d_tag, reference_id)
values
	(:kind, :created_at, :system_created_at, :id, :pubkey, :master_pubkey, :sig, :sig_alg, :key_alg, :content, :jtags, :d_tag, :reference_id)
on conflict do update set
	id                = excluded.id,
	kind              = excluded.kind,
	created_at        = excluded.created_at,
	system_created_at = excluded.system_created_at,
	pubkey            = excluded.pubkey,
	master_pubkey     = excluded.master_pubkey,
	sig               = excluded.sig,
	sig_alg           = excluded.sig_alg,
	key_alg           = excluded.key_alg,
	content           = excluded.content,
	tags              = excluded.tags,
	d_tag             = excluded.d_tag,
	reference_id      = excluded.reference_id,
	hidden            = 0
`

	result, err := db.NamedExecContext(ctx, stmt, events)
	if err != nil {
		err = errors.Wrap(db.handleError(err), "failed to exec insert event sql")
	} else if rows, err := result.RowsAffected(); err != nil {
		err = errors.Wrap(err, "failed to get rows affected")
	} else if expected := int64(len(events)); rows < expected {
		err = errors.Wrapf(ErrUnexpectedRowsAffected, "expected %d rows affected, got %d", expected, rows)
	}

	return err
}

func (db *dbClient) executeBatch(ctx context.Context, req *databaseBatchRequest) (err error) {
	if req.Empty() {
		return nil
	}

	if len(req.InsertOrReplace) > 0 {
		err = errors.Join(err, errors.Wrap(db.saveEvents(ctx, req.InsertOrReplace), "failed to save events"))
	}

	if len(req.Delete) > 0 {
		deleteErr := db.deleteEvents(ctx, req.Delete)
		if errors.Is(deleteErr, ErrUnexpectedRowsAffected) && len(req.InsertOrReplace) > 0 {
			deleteErr = nil
		}
		err = errors.Join(err, errors.Wrap(deleteErr, "failed to delete events"))
	}

	return err
}

func (db *dbClient) SelectEvents(ctx context.Context, subscription *model.Subscription) EventIterator {
	limit := int64(selectDefaultBatchLimit)
	hasLimitFilter := subscription != nil && len(subscription.Filters) > 0 && subscription.Filters[0].Limit > 0
	if hasLimitFilter {
		limit = int64(subscription.Filters[0].Limit)
	}

	it := &eventIterator{
		oneShot: hasLimitFilter && limit <= selectDefaultBatchLimit,
		fetch: func(pivot int64) (*sqlx.Rows, error) {
			if limit <= 0 {
				return nil, nil
			}

			sql, params, err := generateSelectEventsSQL(subscription, pivot, min(selectDefaultBatchLimit, limit))
			if err != nil {
				return nil, err
			}

			stmt, err := db.prepare(ctx, sql, hashSQL(sql))
			if err != nil {
				return nil, errors.Wrapf(err, "failed to prepare query sql: %q", sql)
			}

			rows, err := stmt.QueryxContext(ctx, params)
			if err != nil {
				err = errors.Wrapf(err, "failed to query query events sql: %q", sql)
			}

			if hasLimitFilter && err == nil {
				limit -= selectDefaultBatchLimit
			}

			return rows, err
		}}

	return func(yield func(*model.Event, error) bool) {
		err := it.Each(ctx, func(event *model.Event) error {
			if !yield(event, nil) {
				return errEventIteratorInterrupted
			}

			return nil
		})

		if err != nil && !errors.Is(err, errEventIteratorInterrupted) {
			yield(nil, errors.Wrap(err, "failed to iterate events"))
		}
	}
}

func (db *dbClient) handleError(err error) error {
	var sqlError sqlite3.Error

	if err == nil {
		return err
	}

	if errors.As(err, &sqlError) && sqlError.Code == sqlite3.ErrConstraint {
		switch sqlError.Error() {
		case "onbehalf permission denied":
			err = model.ErrOnBehalfAccessDenied
		case "attestation list update must be linear":
			err = ErrAttestationUpdateRejected
		}
	}

	return err
}

func (db *dbClient) CountEvents(ctx context.Context, subscription *model.Subscription) (count int64, err error) {
	where, params, err := generateEventsWhereClause(subscription)
	if err != nil {
		return -1, errors.Wrap(err, "failed to generate events where clause")
	}

	sql := `select count(id) from events e where ` + where

	stmt, err := db.prepare(ctx, sql, hashSQL(sql))
	if err != nil {
		return -1, errors.Wrapf(err, "failed to prepare query sql: %q", sql)
	}

	err = errors.Wrapf(stmt.GetContext(ctx, &count, params), "failed to query events count sql: %q", sql)

	return count, err
}

func generateSelectEventsSQL(subscription *model.Subscription, systemCreatedAtPivot, limit int64) (sql string, params map[string]any, err error) {
	where, params, err := generateEventsWhereClause(subscription)
	if err != nil {
		return "", nil, errors.Wrap(err, "failed to generate events where clause")
	}

	var systemCreatedAtFilter string
	if systemCreatedAtPivot != 0 {
		systemCreatedAtFilter = " (system_created_at < :system_created_at_pivot) AND "
		params["system_created_at_pivot"] = systemCreatedAtPivot
	}

	var limitQuery string
	if limit > 0 {
		params["mainlimit"] = limit
		limitQuery = " limit :mainlimit"
	}

	return `
select
	e.kind,
	e.created_at,
	e.system_created_at,
	e.id,
	e.pubkey,
	e.sig,
	e.content,
	tags as jtags
from
	events e
where ` + systemCreatedAtFilter + `(` + where + `)
order by
	system_created_at desc
` + limitQuery, params, nil
}

func generateEventsWhereClause(subscription *model.Subscription) (clause string, params map[string]any, err error) {
	var filters []model.Filter

	if subscription != nil {
		filters = subscription.Filters
	}

	return newWhereBuilder().Build(filters...)
}

func (db *dbClient) deleteExpiredEvents(ctx context.Context) error {
	params := map[string]any{}
	_, err := db.exec(ctx, `delete from events
								where id in (
									select event_id from event_tags
										where (((event_tag_key = 'expiration')
											AND cast(event_tag_value1 as integer) <= unixepoch())))`, params)

	return errors.Wrap(err, "failed to exec delete expired events")
}
