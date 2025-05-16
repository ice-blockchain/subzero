// SPDX-License-Identifier: ice License 1.0

//go:build test

package query

import (
	"context"
	"sync"

	"github.com/cockroachdb/errors"

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
	return newQueryBuilder().Build(filter...)
}

func DeleteAllEvents(ctx context.Context) error {
	const stmt = `DELETE FROM events`

	_, err := globalDB.Client.ExecContext(context.WithoutCancel(ctx), stmt)

	return errors.Wrap(err, "failed to delete all events")
}

func NewTestContainer(ctx context.Context) *Container {
	return fixture.New(ctx)
}

func NewTestDatabaseClient(ctx context.Context, container *Container) (TestDB, func() error) {
	tempAddress, release := container.MustTempDB(ctx)

	conf := mustLoadConfig(WithConfig(&Config{
		URL: tempAddress,
	}))
	client := openDatabase(conf.URL, true).
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

type MemDB struct {
	mu     sync.RWMutex
	events []*model.Event
}

func (m *MemDB) AcceptEvents(_ context.Context, events ...*model.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.events = append(m.events, events...)

	return nil
}

func (m *MemDB) SelectEvents(_ context.Context, filters ...model.Filter) EventIterator {
	return func(yield func(*model.Event, error) bool) {
		m.mu.RLock()
		defer m.mu.RUnlock()

		for i := range m.events {
			if !model.Filters(filters).Match(&m.events[i].Event) {
				continue
			}
			if !yield(m.events[i], nil) {
				return
			}
		}
	}
}
