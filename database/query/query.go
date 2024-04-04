package query

import (
	"context"
	"encoding/json"
	"github.com/gookit/goutil/errorx"
	"github.com/ice-blockchain/subzero/model"
	_ "github.com/mattn/go-sqlite3"
)

func AcceptEvent(ctx context.Context, event *model.Event) error {
	isEphemeralEvent := (20000 <= event.Kind && event.Kind < 30000)
	if isEphemeralEvent {
		return nil
	}
	return globalDB.SaveEvent(ctx, event)
}

func (db *dbClient) SaveEvent(ctx context.Context, event *model.Event) error {
	jtags, _ := json.Marshal(event.Tags)

	sql := `insert or replace into events( kind,  created_at,  id,  pubkey,   sig,  content,  temp_tags, d_tag)
								   values(:kind, :createdat, :id, :pubkey, :sig, :content, :jtags,      (select value->>1 from json_each(jsonb(:jtags)) where value->>0 = 'd' limit 1))`
	rowsAffected, err := db.exec(ctx, sql, &struct {
		*model.Event
		JTags string `db:"jtags"`
	}{
		Event: event,
		JTags: string(jtags),
	})
	if err != nil {
		return errorx.With(err, "failed to exec insert event sql")
	}
	if rowsAffected == 0 {
		return errorx.Newf("unexpected rowsAffected:`%v`", rowsAffected)
	}

	return nil
}

func (db *dbClient) SelectEvents(ctx context.Context, subscription *model.Subscription) (dest []*model.Event, err error) {
	sql, params := generateSelectEventsSQL(subscription)
	if err = db.query(ctx, sql, params, &dest); err != nil {
		return nil, errorx.With(err, "failed to query select events sql")
	}

	return dest, nil
}

func GetStoredEvents(ctx context.Context, subscription *model.Subscription) (dest []*model.Event, err error) {
	return globalDB.SelectEvents(ctx, subscription)
}

func generateSelectEventsSQL(subscription *model.Subscription) (sql string, params map[string]any) {
	return `select e.kind,
				   e.created_at as createdat,  
				   e.id,  
				   e.pubkey,   
				   e.sig,  
				   e.content,  
				   '[]' as tags 
				   from events e`, map[string]any{} //TODO impl it properly
}
