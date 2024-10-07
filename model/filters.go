// SPDX-License-Identifier: ice License 1.0

package model

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
