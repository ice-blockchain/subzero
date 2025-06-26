// SPDX-License-Identifier: ice License 1.0

package fixture

import (
	"context"
	"sync"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

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

func (m *MemDB) SelectEvents(ctx context.Context, filters ...model.Filter) query.EventIterator {
	return func(yield func(*model.Event, error) bool) {
		m.mu.RLock()
		defer m.mu.RUnlock()

		master, device, _, _ := model.GetUserDataFromContext(ctx)
		for i := range m.events {
			if !model.FiltersMatch(filters, m.events[i], master, device) {
				continue
			}
			if !yield(m.events[i], nil) {
				return
			}
		}
	}
}
