// SPDX-License-Identifier: ice License 1.0

package model

import (
	"math"
	"math/rand/v2"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

type (
	Subscription struct {
		ID      string
		Filters Filters
		OneShot bool

		live      atomic.Bool
		addresses map[string]int
		pending   Events
		reduce    func(*Event) (skip bool)
		oneShot   bool
		mu        sync.Mutex
	}
)

func NewSubscription(id string, filters Filters) *Subscription {
	s := &Subscription{
		ID:        id,
		Filters:   filters,
		addresses: make(map[string]int),
	}

	if s.ID == "" {
		s.ID = "autogen-" + time.Now().Format(time.RFC3339Nano) + "-" + strconv.FormatUint(rand.Uint64N(math.MaxUint64), 16)
	}

	return s
}

func (s *Subscription) WithReduce(reduce func(*Event) (skip bool)) *Subscription {
	s.reduce = reduce
	return s
}

func (s *Subscription) Reduce(event *Event) bool {
	if s.reduce == nil {
		return false
	}
	return s.reduce(event)
}

func (s *Subscription) Push(event *Event) {
	if s.Reduce(event) {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	addr := event.Address()
	idx, ok := s.addresses[addr]
	if ok {
		// If the event is already in the queue, just update it.
		s.pending[idx] = event
		return
	}

	s.pending = append(s.pending, event)
	s.addresses[addr] = len(s.pending) - 1
}

func (s *Subscription) IsLive() bool {
	return s.live.Load()
}

func (s *Subscription) SetLive() {
	s.live.Store(true)
}

func (s *Subscription) GetPending() Events {
	s.mu.Lock()
	events := s.pending
	s.pending = nil
	s.addresses = make(map[string]int)
	s.mu.Unlock()

	return events
}
