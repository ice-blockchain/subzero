package query

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/gookit/goutil/errorx"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/model"
)

type StreamedEvent struct {
	Event *model.Event
	Err   error
}

type databaseEvent struct {
	model.Event
	SystemCreatedAt int64
	Jtags           string
}

type EventIterator interface {
	Stream(ctx context.Context) <-chan StreamedEvent
}

func AcceptEvent(ctx context.Context, event *model.Event) error {
	isEphemeralEvent := (20000 <= event.Kind && event.Kind < 30000)
	if isEphemeralEvent {
		return nil
	}
	if event.Kind == nostr.KindDeletion {
		refs, err := model.ParseEventReference(event.Tags)
		if err != nil {
			return errorx.Withf(err, "failed to detect events for delete")
		}
		filters := nostr.Filters{}
		for _, r := range refs {
			filters = append(filters, r.Filter())
		}
		if err = globalDB.DeleteEvents(ctx, &model.Subscription{Filters: filters}, event.PubKey); err != nil {
			return errorx.Withf(err, "failed to delete events %+v", filters)
		}
		return nil
	}

	return globalDB.SaveEvent(ctx, event)
}

func (db *dbClient) SaveEvent(ctx context.Context, event *model.Event) error {
	const stmt = `
insert or replace into events
	(kind, created_at, system_created_at, id, pubkey, sig, content, temp_tags, d_tag)
values
	(:kind, :created_at, :system_created_at, :id, :pubkey, :sig, :content, :jtags, (select value->>1 from json_each(jsonb(:jtags)) where value->>0 = 'd' limit 1))`

	jtags, _ := json.Marshal(event.Tags)
	dbEvent := &databaseEvent{
		Event:           *event,
		SystemCreatedAt: time.Now().UnixNano(),
		Jtags:           string(jtags),
	}

	rowsAffected, err := db.exec(ctx, stmt, dbEvent)
	if err != nil {
		return errorx.With(err, "failed to exec insert event sql")
	}
	if rowsAffected == 0 {
		return errorx.Newf("unexpected rowsAffected:`%v`", rowsAffected)
	}

	return nil
}

type eventIterator struct {
	fetch   func(pivot int64) (*sqlx.Rows, error)
	oneShot bool
}

func (*eventIterator) decodeTags(jtags string) (tags nostr.Tags, err error) {
	if len(jtags) == 0 {
		return
	}
	if err = tags.Scan(jtags); err != nil {
		return nil, errorx.With(err, "failed to unmarshal tags")
	}

	// Remove empty values from tags.
	// tag e: ["e", ""] -> ["e"].
	// tag p: [""] -> [].
	for i := range tags {
		for j := range tags[i] {
			if tags[i][j] == "" {
				if j == 0 {
					tags[i] = nil
				} else {
					tags[i] = tags[i][:j]
				}
				break
			}
		}
	}

	return tags, nil
}

func (it *eventIterator) scanEvent(rows *sqlx.Rows) (_ *databaseEvent, err error) {
	var ev databaseEvent

	if err := rows.StructScan(&ev); err != nil {
		return nil, errorx.With(err, "failed to struct scan")
	}

	if ev.Tags, err = it.decodeTags(ev.Jtags); err != nil {
		return nil, errorx.With(err, "failed to decode tags")
	}

	return &ev, nil
}

func (it *eventIterator) scanBatch(ctx context.Context, ch chan StreamedEvent, pivot int64) (int64, error) {
	rows, err := it.fetch(pivot)
	if err != nil {
		return -1, errorx.With(err, "failed to get events")
	}
	defer rows.Close()

	for rows.Next() && ctx.Err() == nil {
		event, err := it.scanEvent(rows)
		if err != nil {
			return -1, errorx.With(err, "failed to scan event")
		}

		if pivot == 0 || event.SystemCreatedAt < pivot {
			pivot = event.SystemCreatedAt
		}

		select {
		case ch <- StreamedEvent{Event: &event.Event}:
		case <-ctx.Done():
			return pivot, ctx.Err()
		}
	}

	return pivot, nil
}

func (it *eventIterator) scanner(ctx context.Context, ch chan StreamedEvent) {
	var pivot int64

	for ctx.Err() == nil {
		newPivot, err := it.scanBatch(ctx, ch, pivot)
		if err != nil {
			select {
			case ch <- StreamedEvent{Err: errorx.With(err, "failed to get events")}:
			case <-ctx.Done():
			}

			return
		}

		if pivot == newPivot || it.oneShot {
			return
		}

		pivot = newPivot
	}
}

func (it *eventIterator) Stream(ctx context.Context) <-chan StreamedEvent {
	ch := make(chan StreamedEvent, 10)

	go func() {
		it.scanner(ctx, ch)
		close(ch)
	}()

	return ch
}

func (db *dbClient) SelectEvents(ctx context.Context, subscription *model.Subscription) (it EventIterator) {
	const defBatchSize = 1000
	var limit int64 = defBatchSize

	hasLimitFilter := subscription != nil && len(subscription.Filters) > 0 && subscription.Filters[0].Limit > 0
	if hasLimitFilter {
		limit = int64(subscription.Filters[0].Limit)
	}

	return &eventIterator{
		oneShot: hasLimitFilter,
		fetch: func(pivot int64) (*sqlx.Rows, error) {
			sql, params := generateSelectEventsSQL(subscription, pivot, limit)

			stmt, err := db.prepare(ctx, sql, hashSQL(sql))
			if err != nil {
				return nil, errorx.Withf(err, "failed to prepare query sql: %q", sql)
			}

			rows, err := stmt.QueryxContext(ctx, params)
			if err != nil {
				err = errorx.Withf(err, "failed to query query events sql: %q", sql)
			}

			return rows, err
		}}
}

func (db *dbClient) DeleteEvents(ctx context.Context, subscription *model.Subscription, ownerPubKey string) error {
	where, params := generateEventsWhereClause(subscription)
	params["owner_pub_key"] = ownerPubKey
	rowsAffected, err := db.exec(ctx, fmt.Sprintf(`delete from events where %v AND pubkey = :owner_pub_key`, where), params)
	if err != nil {
		return errorx.With(err, "failed to exec delete events sql")
	}
	if rowsAffected == 0 {
		return errorx.Newf("unexpected rowsAffected:`%v`", rowsAffected)
	}

	return nil
}

func GetStoredEvents(ctx context.Context, subscription *model.Subscription) EventIterator {
	return globalDB.SelectEvents(ctx, subscription)
}

func generateSelectEventsSQL(subscription *model.Subscription, systemCreatedAtPivot, limit int64) (sql string, params map[string]any) {
	where, params := generateEventsWhereClause(subscription)

	var systemCreatedAtFilter string
	if systemCreatedAtPivot != 0 {
		systemCreatedAtFilter = " (system_created_at < :system_created_at_pivot) AND "
		params["system_created_at_pivot"] = systemCreatedAtPivot
	}

	var limitQuery string
	if limit > 0 {
		limitQuery = " limit " + strconv.FormatInt(limit, 10)
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
	json_group_array(
		json_array(event_tag_key, event_tag_value1,event_tag_value2,event_tag_value3,event_tag_value4)
	) filter (where event_tag_key != '') as jtags
from
	events e
left join event_tags on
	event_id = id
where ` + systemCreatedAtFilter + `(` + where + `)
group by
	id
order by
	system_created_at desc
` + limitQuery, params
}

func generateEventsWhereClause(subscription *model.Subscription) (clause string, params map[string]any) {
	builder := newWhereBuilder()

	if subscription != nil {
		return builder.Build(subscription.Filters...)
	}

	return builder.Build()
}
