// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"iter"
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
	ErrOnBehalfAccessDenied        = errors.New("on-behalf access denied")
	ErrAttestationUpdateRejected   = errors.New("attestation update rejected")
	errEventIteratorInterrupted    = errors.New("interrupted")
)

type databaseEvent struct {
	model.Event
	SystemCreatedAt int64
	ReferenceID     sql.NullString
	Jtags           string
	SigAlg          string
	KeyAlg          string
	MasterPubKey    string
}

type EventIterator iter.Seq2[*model.Event, error]

func (db *dbClient) AcceptEvent(ctx context.Context, event *model.Event) error {
	if event.IsEphemeral() {
		return nil
	}
	switch event.Kind {
	case nostr.KindDeletion:
		refs, err := model.ParseEventReference(event.Tags)
		if err != nil {
			return errors.Wrap(err, "failed to detect events for delete")
		}
		filters := model.Filters{}
		for _, r := range refs {
			filters = append(filters, r.Filter())
		}
		if err = db.DeleteEvents(ctx, &model.Subscription{Filters: filters}, event.PubKey); err != nil {
			return errors.Wrapf(err, "failed to delete events %+v", filters)
		}

		return nil

	case nostr.KindReaction:
		if ev, err := getReactionTargetEvent(ctx, db, event); err != nil || ev == nil {
			return errors.Wrap(ErrTargetReactionEventNotFound, "can't find target event for reaction kind")
		}
	}

	return db.saveEvent(ctx, event)
}

func getReactionTargetEvent(ctx context.Context, db *dbClient, event *model.Event) (res *model.Event, err error) {
	refs, err := model.ParseEventReference(event.Tags)
	if err != nil {
		return nil, errors.Wrap(err, "failed to detect events for delete")
	}
	filters := model.Filters{}
	for _, r := range refs {
		filters = append(filters, r.Filter())
	}
	eLastTag := event.Tags.GetLast([]string{"e"})
	pLastTag := event.Tags.GetLast([]string{"p"})
	aTag := event.Tags.GetFirst([]string{"a"})
	kTag := event.Tags.GetFirst([]string{"k"})
	for ev, evErr := range db.SelectEvents(ctx, &model.Subscription{Filters: filters}) {
		if evErr != nil {
			return nil, errors.Wrapf(evErr, "can't select reaction events for:%+v", ev)
		}
		if ev == nil || (ev.IsReplaceable() && aTag.Value() != fmt.Sprintf("%v:%v:%v", ev.Kind, ev.PubKey, ev.Tags.GetD())) {
			continue
		}
		if ev.ID != eLastTag.Value() || ev.PubKey != pLastTag.Value() || (kTag != nil && kTag.Value() != fmt.Sprint(ev.Kind)) {
			continue
		}

		return ev, nil
	}

	return
}

func parseSigKeyAlg(event *model.Event) (sigAlg, keyAlg string, err error) {
	sAlg, kAlg, _, err := event.ExtractSignature()
	if err != nil {
		return "", "", errors.Wrap(err, "failed to extract signature")
	}

	return string(sAlg), string(kAlg), nil
}

func (db *dbClient) saveEvent(ctx context.Context, event *model.Event) error {
	const stmt = `insert into events
	(kind, created_at, system_created_at, id, pubkey, master_pubkey, sig, sig_alg, key_alg, content, tags, d_tag, reference_id)
values
	(:kind, :created_at, :system_created_at, :id, :pubkey, :master_pubkey, :sig, :sig_alg, :key_alg, :content, :jtags, COALESCE((select value->>1 from json_each(jsonb(:jtags)) where value->>0 = 'd' limit 1), ''), :reference_id)
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

	jtags, err := json.Marshal(event.Tags)
	if err != nil {
		return errors.Wrap(err, "failed to marshal tags")
	}

	dbEvent := databaseEvent{
		Event:           *event,
		MasterPubKey:    event.GetMasterPublicKey(),
		SystemCreatedAt: time.Now().UnixNano(),
		Jtags:           string(jtags),
	}
	dbEvent.SigAlg, dbEvent.KeyAlg, err = parseSigKeyAlg(event)
	if err != nil {
		return err
	}

	rowsAffected, err := db.exec(ctx, stmt, dbEvent)
	if err != nil {
		err = errors.Wrap(db.handleError(err), "failed to exec insert event sql")
	} else if rowsAffected == 0 {
		err = ErrUnexpectedRowsAffected
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
			err = ErrOnBehalfAccessDenied
		case "attestation list update must be linear":
			err = ErrAttestationUpdateRejected
		}
	}

	return err
}

func (db *dbClient) deleteOwnEvents(ctx context.Context, whereClause string, params map[string]any) (int64, error) {
	stmt := `delete from events where ` + whereClause + ` AND (pubkey = :owner_pub_key)`
	rowsAffected, err := db.exec(ctx, stmt, params)

	return rowsAffected, errors.Wrap(db.handleError(err), "failed to exec delete event sql")
}

func (db *dbClient) deleteOnBehalfEvents(ctx context.Context, whereClause string, params map[string]any) (int64, error) {
	stmt := `
-- Fetch attestations tags, if any.
with cte as (
	select
		parent.*
	from
		event_tags
	left join events parent on
		parent.id = event_tags.event_id and
		parent.kind = :custom_ion_kind_attestation
	where
		event_tag_key = 'p' and
		event_tag_value1 = :owner_pub_key
)
delete from events where ` + whereClause +
		` AND (
		     -- Must be the event owner.
			(master_pubkey = :owner_pub_key) OR (
				-- OR must be on behalf of the event owner.
				(master_pubkey = (select pubkey from cte)) AND
				-- AND the event must be pablished on behalf of someone else.
				(pubkey != master_pubkey) AND
				-- AND with valid on-behalf access.
				subzero_nostr_onbehalf_is_allowed(coalesce((select tags from cte), '[]'), :owner_pub_key, master_pubkey, kind, unixepoch())
			)
		)`

	params["custom_ion_kind_attestation"] = model.CustomIONKindAttestation
	rowsAffected, err := db.exec(ctx, stmt, params)

	return rowsAffected, errors.Wrap(db.handleError(err), "failed to exec delete related event sql")
}

func (db *dbClient) DeleteEvents(ctx context.Context, subscription *model.Subscription, ownerPubKey string) error {
	where, params, err := generateEventsWhereClause(subscription)
	if err != nil {
		return errors.Wrap(err, "failed to generate events where clause")
	}
	params["owner_pub_key"] = ownerPubKey

	ownDeleted, err := db.deleteOwnEvents(ctx, where, params)
	if err != nil {
		return err
	}

	relatedDeleted, err := db.deleteOnBehalfEvents(ctx, where, params)
	if err != nil {
		return err
	}

	if ownDeleted == 0 && relatedDeleted == 0 {
		err = ErrUnexpectedRowsAffected
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
