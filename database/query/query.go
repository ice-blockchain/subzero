// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"iter"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/nbd-wtf/go-nostr"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/subzero/model"
)

var (
	ErrUnexpectedRowsAffected      = errors.New("unexpected rows affected")
	ErrTargetReactionEventNotFound = errors.New("target reaction event not found")
	errEventIteratorInterrupted    = errors.New("interrupted")
)

type databaseEvent struct {
	model.Event
	SystemCreatedAt int64
	ReferenceID     sql.NullString
	Jtags           string
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

	case nostr.KindRepost:
		return db.saveRepost(ctx, event)

	case nostr.KindReaction:
		if ev, err := getReactionTargetEvent(ctx, db, event); err != nil || ev == nil {
			return errors.Wrap(ErrTargetReactionEventNotFound, "can't find target event for reaction kind")
		}
	}

	return db.SaveEvent(ctx, event)
}

func (db *dbClient) saveRepost(ctx context.Context, event *model.Event) error {
	var childEvent model.Event

	err := json.Unmarshal([]byte(event.Content), &childEvent)
	if err != nil {
		return errors.Wrap(err, "failed to unmarshal original event")
	}

	err = db.SaveEvent(ctx, &childEvent)
	if err != nil {
		return errors.Wrap(err, "failed to save original event")
	}

	// Link the repost event to the original event.
	dbEvent := eventToDatabaseEvent(event)
	dbEvent.ReferenceID = sql.NullString{String: childEvent.ID, Valid: true}

	return db.SaveDatabaseEvent(ctx, dbEvent)
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

func (db *dbClient) SaveDatabaseEvent(ctx context.Context, event *databaseEvent) error {
	const stmt = `
insert or replace into events
	(kind, created_at, system_created_at, id, pubkey, sig, content, temp_tags, d_tag, reference_id)
values
	(:kind, :created_at, :system_created_at, :id, :pubkey, :sig, :content, :jtags, (select value->>1 from json_each(jsonb(:jtags)) where value->>0 = 'd' limit 1), :reference_id)`

	event.SystemCreatedAt = time.Now().UnixNano()
	rowsAffected, err := db.exec(ctx, stmt, event)
	if err != nil {
		return errors.Wrap(err, "failed to exec insert event sql")
	}
	if rowsAffected == 0 {
		return ErrUnexpectedRowsAffected
	}

	return nil
}

func eventToDatabaseEvent(event *model.Event) *databaseEvent {
	jtags, _ := marshalTags(event.Tags)

	return &databaseEvent{
		Event: *event,
		Jtags: string(jtags),
	}
}

func (db *dbClient) SaveEvent(ctx context.Context, event *model.Event) error {
	return db.SaveDatabaseEvent(ctx, eventToDatabaseEvent(event))
}

func (db *dbClient) SelectEvents(ctx context.Context, subscription *model.Subscription) EventIterator {
	const batchSize = 1000

	limit := int64(batchSize)
	hasLimitFilter := subscription != nil && len(subscription.Filters) > 0 && subscription.Filters[0].Limit > 0
	if hasLimitFilter {
		limit = int64(subscription.Filters[0].Limit)
	}

	it := &eventIterator{
		oneShot: hasLimitFilter && limit <= batchSize,
		fetch: func(pivot int64) (*sqlx.Rows, error) {
			if limit <= 0 {
				return nil, nil
			}

			sql, params, err := generateSelectEventsSQL(subscription, pivot, min(batchSize, limit))
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
				limit -= batchSize
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

func (db *dbClient) DeleteEvents(ctx context.Context, subscription *model.Subscription, ownerPubKey string) error {
	where, params, err := generateEventsWhereClause(subscription)
	if err != nil {
		return errors.Wrap(err, "failed to generate events where clause")
	}

	params["owner_pub_key"] = ownerPubKey
	rowsAffected, err := db.exec(ctx, fmt.Sprintf(`delete from events where %v AND pubkey = :owner_pub_key`, where), params)
	if err != nil {
		return errors.Wrap(err, "failed to exec delete events sql")
	}
	if rowsAffected == 0 {
		return ErrUnexpectedRowsAffected
	}

	return nil
}

func (db *dbClient) CountEvents(ctx context.Context, subscription *model.Subscription) (count int64, err error) {
	where, params, err := generateEventsWhereClause(subscription)
	if err != nil {
		return -1, errors.Wrap(err, "failed to generate events where clause")
	}

	sql := `select count(id) from events where ` + where

	stmt, err := db.prepare(ctx, sql, hashSQL(sql))
	if err != nil {
		return -1, errors.Wrapf(err, "failed to prepare query sql: %q", sql)
	}

	err = stmt.GetContext(ctx, &count, params)
	if err != nil {
		err = errors.Wrapf(err, "failed to query events count sql: %q", sql)
	}

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
	'[]' as tags,
	(select json_group_array(
		json_array(
			event_tag_key,
			event_tag_value1,event_tag_value2,event_tag_value3,event_tag_value4,
			event_tag_value5,event_tag_value6,event_tag_value7,event_tag_value8,
			event_tag_value9,event_tag_value10,event_tag_value11,event_tag_value12,
			event_tag_value13,event_tag_value14,event_tag_value15,event_tag_value16,
			event_tag_value17,event_tag_value18,event_tag_value19,event_tag_value20,
			event_tag_value21
		)
	) from event_tags where event_id = e.id) as jtags
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
