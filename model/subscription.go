// SPDX-License-Identifier: ice License 1.0

package model

import (
	"context"
)

type (
	Subscription struct {
		ID      string
		Filters Filters
		OneShot bool

		reduce func(*Event) (skip bool)
	}
)

func NewSubscription(id string, filters Filters) *Subscription {
	return &Subscription{
		ID:      id,
		Filters: filters,
	}
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

func (s *Subscription) Match(ctx context.Context, event *Event) bool {
	if s.Filters == nil {
		return true
	}

	master, device, _, _ := GetUserDataFromContext(ctx)

	return FiltersMatch(s.Filters, event, master, device)
}
