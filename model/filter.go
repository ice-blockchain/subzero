// SPDX-License-Identifier: ice License 1.0

package model

import (
	"maps"
	"slices"

	"github.com/nbd-wtf/go-nostr"
)

type (
	TagValues []*string
	TagMap    map[string][]TagValues
	Filters   []Filter
	Filter    struct {
		IDs     []string
		Kinds   []int
		Authors []string
		Tags    TagMap
		Since   *Timestamp
		Until   *Timestamp
		Limit   int
		Search  string

		// LimitZero is or must be set when there is a "limit":0 in the filter, and not when "limit" is just omitted
		LimitZero bool `json:"-"`
	}
)

func PointerOf[T any](v T) *T {
	return &v
}

func (eff Filters) Match(event *Event) bool {
	for _, filter := range eff {
		if filter.Matches(event) {
			return true
		}
	}
	return false
}

func (eff Filters) ToNostr() nostr.Filters {
	var filters nostr.Filters
	for _, filter := range eff {
		filters = append(filters, filter.ToNostr()...)
	}
	return filters
}

func (ef Filter) ToNostr() (result []nostr.Filter) {
	base := nostr.Filter{
		IDs:     ef.IDs,
		Kinds:   ef.Kinds,
		Authors: ef.Authors,
		Since:   ef.Since,
		Until:   ef.Until,
		Limit:   ef.Limit,
		Search:  ef.Search,
		Tags:    make(nostr.TagMap),
	}

	var extTags []string
	for tag, values := range ef.Tags {
		if len(values) == 0 {
			base.Tags[tag] = nil
			continue
		} else if len(values) == 1 {
			if values[0].Empty() {
				base.Tags[tag] = nil
			} else {
				for i := range values[0] {
					if values[0][i] != nil {
						base.Tags[tag] = append(base.Tags[tag], *values[0][i])
					}
				}
			}
		} else {
			extTags = append(extTags, tag)
		}
	}

	if len(extTags) == 0 {
		return []nostr.Filter{base}
	}

	for _, tag := range extTags {
		for _, values := range ef.Tags[tag] {
			filter := base
			filter.Tags = make(nostr.TagMap)
			maps.Copy(filter.Tags, base.Tags)
			for i := range values {
				if values[i] != nil {
					filter.Tags[tag] = append(filter.Tags[tag], *values[i])
				}
			}
			result = append(result, filter)
		}
	}

	return
}

func (ef Filter) Matches(event *Event) bool {
	if !ef.MatchesIgnoringTimestampConstraints(event) {
		return false
	}

	if ef.Since != nil && event.CreatedAt < *ef.Since {
		return false
	}

	if ef.Until != nil && event.CreatedAt > *ef.Until {
		return false
	}

	return true
}

func (ef Filter) MatchesIgnoringTimestampConstraints(event *Event) bool {
	if event == nil {
		return false
	}

	if ef.IDs != nil && !slices.Contains(ef.IDs, event.ID) {
		return false
	}

	if ef.Kinds != nil && !slices.Contains(ef.Kinds, event.Kind) {
		return false
	}

	if ef.Authors != nil && !slices.Contains(ef.Authors, event.PubKey) {
		return false
	}

	for tag, values := range ef.Tags {
		eventTagValues := event.GetTag(tag)
		if eventTagValues == nil {
			return false
		}
		eventTagValues = eventTagValues[1:] // Skip the tag name.
		for i := range values {
			for j := range values[i] {
				if values[i][j] == nil {
					continue
				}
				if j >= len(eventTagValues) || *values[i][j] != eventTagValues[j] {
					return false
				}
			}
		}
	}

	return true
}

func (m TagMap) SetLiterals(tag string, values ...string) TagMap {
	tagValues := make(TagValues, len(values))
	for i := range values {
		tagValues[i] = &values[i]
	}

	return m.Set(tag, tagValues...)
}

func (m TagMap) Set(tag string, values ...*string) TagMap {
	m[tag] = []TagValues{values}

	return m
}

func (m TagMap) Append(tag string, values ...*string) TagMap {
	m[tag] = append(m[tag], values)

	return m
}

func (m TagMap) HasValues(tag string) bool {
	for _, values := range m[tag] {
		if !values.Empty() {
			return true
		}
	}
	return false
}

func (v TagValues) Empty() bool {
	for i := range v {
		if v[i] != nil {
			return false
		}
	}
	return true
}
