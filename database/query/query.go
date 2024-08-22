package query

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/nbd-wtf/go-nostr"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/subzero/model"
)

var (
	ErrUnexpectedRowsAffected   = errors.New("unexpected rows affected")
	errEventIteratorInterrupted = errors.New("interrupted")
)

type databaseEvent struct {
	model.Event
	SystemCreatedAt int64
	Jtags           string
}

type EventIterator iter.Seq2[*model.Event, error]

func (db *dbClient) AcceptEvent(ctx context.Context, event *model.Event) error {
	isEphemeralEvent := (20000 <= event.Kind && event.Kind < 30000)
	if isEphemeralEvent {
		return nil
	}
	if event.Kind == nostr.KindDeletion {
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
	}

	return db.SaveEvent(ctx, event)
}

func (db *dbClient) SaveEvent(ctx context.Context, event *model.Event) error {
	var jtags = []byte("[]")

	const stmt = `
insert or replace into events
	(kind, created_at, system_created_at, id, pubkey, sig, content, temp_tags, d_tag)
values
	(:kind, :created_at, :system_created_at, :id, :pubkey, :sig, :content, :jtags, (select value->>1 from json_each(jsonb(:jtags)) where value->>0 = 'd' limit 1))`

	if len(event.Tags) > 0 {
		jtags, _ = json.Marshal(event.Tags)
	}

	dbEvent := &databaseEvent{
		Event:           *event,
		SystemCreatedAt: time.Now().UnixNano(),
		Jtags:           string(jtags),
	}

	rowsAffected, err := db.exec(ctx, stmt, dbEvent)
	if err != nil {
		return errors.Wrap(err, "failed to exec insert event sql")
	}
	if rowsAffected == 0 {
		return ErrUnexpectedRowsAffected
	}

	return nil
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
	(select json_group_array(json_array(event_tag_key, event_tag_value1,event_tag_value2,event_tag_value3,event_tag_value4)) from event_tags where event_id = e.id) as jtags
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
