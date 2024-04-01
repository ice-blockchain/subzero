package query

import (
	"context"
	"github.com/gookit/goutil/errorx"
	"github.com/ice-blockchain/subzero/database/command"
	"github.com/ice-blockchain/subzero/model"
	_ "github.com/mattn/go-sqlite3"
)

func init() {
	command.RegisterQuerySyncer(acceptEvent)
}

func acceptEvent(ctx context.Context, event *model.Event) error {
	return globalDB.SaveEvent(ctx, event)
}

func (db *dbClient) SaveEvent(ctx context.Context, event *model.Event) error {
	sql := generateSaveEventSQL(event)
	rowsAffected, err := db.exec(ctx, sql, event)
	if err != nil {
		return errorx.With(err, "failed to exec insert event sql")
	}
	if rowsAffected == 0 {
		return errorx.Newf("unexpected rowsAffected:`%v`", rowsAffected)
	}

	return nil
}

func generateSaveEventSQL(event *model.Event) string {
	return ""
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
	return "", nil
}
