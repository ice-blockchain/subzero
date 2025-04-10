// SPDX-License-Identifier: ice License 1.0

//go:build test

package query

import (
	"context"

	"github.com/cockroachdb/errors"

	"github.com/ice-blockchain/subzero/database/query/internal/postgres/fixture"
	"github.com/ice-blockchain/subzero/model"
)

type TestDB interface {
	AcceptEvents(ctx context.Context, events ...*model.Event) error
	RollbackEvents(ctx context.Context, events ...*model.Event) error
	SelectEvents(ctx context.Context, filters ...model.Filter) EventIterator
}

func TriggerExpiredEventsCleanup(ctx context.Context) error {
	return globalDB.Client.deleteExpiredEvents(ctx)
}

func DeleteAllEvents(ctx context.Context) error {
	const stmt = `DELETE FROM events`

	_, err := globalDB.Client.ExecContext(context.WithoutCancel(ctx), stmt)

	return errors.Wrap(err, "failed to delete all events")
}

func GetDB(ctx context.Context, opts ...Option) TestDB {
	conf := mustLoadConfig(opts...)
	client := openDatabase(conf.URL, true).
		WithPrivateKey(conf.PrivateKey).
		WithRelayURL(conf.RelayURL)

	go client.StartExpiredEventsCleanup(ctx)

	go func() {
		<-ctx.Done()
		client.Close()
	}()
	globalDB.Once.Do(func() {
		globalDB.Client = client
	})
	return client
}

func NewTestDatabase(ctx context.Context) (string, func() error) {
	container := fixture.New(ctx)

	return container.ConnectionString(ctx, ""), func() error {
		return container.Close(context.Background())
	}
}
