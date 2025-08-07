// SPDX-License-Identifier: ice License 1.0

//go:build test

package query

import (
	"context"

	"github.com/cockroachdb/errors"

	"github.com/ice-blockchain/subzero/database/query/internal/connector"
	"github.com/ice-blockchain/subzero/database/query/internal/postgres/fixture"
	"github.com/ice-blockchain/subzero/model"
)

type (
	TestDB interface {
		AcceptEvents(ctx context.Context, events ...*model.Event) error
		RollbackEvents(ctx context.Context, events ...*model.Event) error
		SelectEvents(ctx context.Context, filters ...model.Filter) EventIterator
	}
	Container = fixture.Container
)

func TriggerExpiredEventsCleanup(ctx context.Context) error {
	return globalDB.Client.deleteExpiredEvents(ctx)
}

func GenerateSelectEventsSQL(ctx context.Context, filter ...model.Filter) (sql string, params map[string]any, err error) {
	r, err := newQueryBuilder().Build(ctx, filter...)
	if err != nil {
		return "", nil, err
	}
	return r.Statement, r.Params, nil
}

func DeleteAllEvents(ctx context.Context) error {
	const stmt = `DELETE FROM events`

	_, err := connector.Exec(context.WithoutCancel(ctx), globalDB.Client.db, stmt)

	return errors.Wrap(err, "failed to delete all events")
}

func NewTestContainer(ctx context.Context) *Container {
	return fixture.New(ctx)
}

func NewTestDatabaseClient(ctx context.Context, container *Container) (TestDB, func() error) {
	tempAddress, release := container.MustTempDB(ctx)

	conf := mustLoadConfig(WithConfig(&Config{
		ReadURLs:  []string{tempAddress},
		WriteURLs: []string{tempAddress},
	}))
	client := openDatabase(ctx, conf.WriteURLs, conf.ReadURLs, true).
		WithPrivateKey(conf.PrivateKey).
		WithRelayURL(conf.RelayURL)

	return client, func() error {
		if err := client.Close(); err != nil {
			return errors.Wrap(err, "failed to close test database")
		}

		release()

		return nil
	}
}

func NewTestDatabase(ctx context.Context) (string, func() error) {
	container := fixture.New(ctx)

	return container.ConnectionString(ctx, ""), func() error {
		return container.Close(context.Background())
	}
}
