// SPDX-License-Identifier: ice License 1.0

package model

import (
	"github.com/nbd-wtf/go-nostr"
)

func (eff Filters) Match(event *Event) bool {
	for _, filter := range eff {
		if filter.Matches(event) {
			return true
		}
	}

	return false
}

func (ef Filter) Matches(event *Event) bool {
	// TODO: add custom logic here for new filter fields.
	return ef.Filter.Matches(&event.Event)
}

func FromNostrFilters(filters nostr.Filters) Filters {
	if len(filters) == 0 {
		return nil
	}

	result := make(Filters, len(filters))
	for idx := range filters {
		result[idx] = Filter{Filter: filters[idx]}
	}

	return result
}
