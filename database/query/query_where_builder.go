// SPDX-License-Identifier: ice License 1.0

package query

import (
	"log"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/nbd-wtf/go-nostr"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/subzero/model"
)

const (
	whereBuilderDefaultWhere = "1=1"
)

const (
	extensionExpiration = 1 << iota
	extensionVideos
	extensionImages
	extensionQuotes
	extensionReferences
)

var ErrWhereBuilderInvalidTimeRange = errors.New("invalid time range")

type whereBuilder struct {
	Params map[string]any
	strings.Builder
}

type filterBuilder struct {
	Name           string
	EventIds       []string
	EventIdsString string
	sync.Once
}

func (f *filterBuilder) HasEvents() bool {
	return len(f.EventIds) > 0
}

func (f *filterBuilder) BuildEvents(w *whereBuilder) string {
	f.Do(func() {
		f.EventIdsString = buildFromSlice(
			&whereBuilder{
				Params: w.Params,
			},
			f.Name,
			f.EventIds,
			"event_id",
		).String()
	})

	return f.EventIdsString
}

func newWhereBuilder() *whereBuilder {
	return &whereBuilder{
		Params: make(map[string]any),
	}
}

func (w *whereBuilder) addParam(filterID, name string, value any) (key string) {
	key = filterID + name
	w.Params[key] = value

	return key
}

func deduplicateSlice[T comparable](s []T) []T {
	seen := make(map[T]struct{}, len(s))
	j := 0
	for _, v := range s {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		s[j] = v
		j++
	}

	return s[:j]
}

func buildFromSlice[T comparable](builder *whereBuilder, filterID string, s []T, name string) *whereBuilder {
	if len(s) == 0 {
		return builder
	}

	builder.maybeAND()
	builder.WriteString(name)
	s = deduplicateSlice(s)
	if len(s) == 1 {
		// X = :X_name.
		builder.WriteString(" = :")
		builder.WriteString(builder.addParam(filterID, name, s[0]))

		return builder
	}

	// X in (:X_name0, :X_name1, ...).
	builder.WriteString(" IN (")
	for i := range len(s) - 1 {
		builder.WriteRune(':')
		builder.WriteString(builder.addParam(filterID, name+strconv.Itoa(i), s[i]))
		builder.WriteRune(',')
	}
	builder.WriteRune(':')
	builder.WriteString(builder.addParam(filterID, name+strconv.Itoa(len(s)-1), s[len(s)-1]))
	builder.WriteRune(')')

	return builder
}

func (w *whereBuilder) isOnBegin() bool {
	if w.Len() == 1 && w.String() == "(" {
		return true
	}

	s := w.String()

	return s[len(s)-1] == '(' || s[len(s)-2:] == "( "
}

func (w *whereBuilder) maybeAND() {
	if w.Len() == 0 || w.isOnBegin() {
		return
	}

	w.WriteString(" AND ")
}

func (w *whereBuilder) maybeOR() {
	if w.Len() == 0 || w.isOnBegin() {
		return
	}

	w.WriteString(" OR ")
}

func (w *whereBuilder) applyFilterTags(filter *filterBuilder, tags model.TagMap) {
	const valuesMax = 21

	if len(tags) == 0 {
		return
	}

	tagID := 0
	for tag, values := range tags {
		w.maybeAND()
		if len(values) > valuesMax {
			log.Printf("%#v: too many values for tag %q, only the first %d will be used", values, tag, valuesMax)
			values = values[:valuesMax]
		}

		tagID++
		if filter.HasEvents() {
			// We already have some IDs, so we need to check if they have the tag.
			w.WriteString("EXISTS (select 42 from event_tags where ")
			w.WriteString(filter.BuildEvents(w))
			w.maybeAND()
		} else {
			// No IDs, so select all events that belong to the given tag.
			w.WriteString("id IN (select event_id from event_tags where ")
		}
		w.WriteString("event_tag_key = :")
		w.WriteString(w.addParam(filter.Name, "tag"+strconv.Itoa(tagID), tag))

		for i, value := range values {
			w.WriteString(" AND ")
			w.WriteString("event_tag_value")
			w.WriteString(strconv.Itoa(i + 1))
			w.WriteString(" = :")
			w.WriteString(w.addParam(filter.Name, "tagvalue"+strconv.Itoa(tagID<<8|i+1), value))
		}
		w.WriteRune(')')
	}
}

func isFilterEmpty(filter *model.Filter) bool {
	return len(filter.IDs) == 0 &&
		len(filter.Kinds) == 0 &&
		len(filter.Authors) == 0 &&
		len(filter.Tags) == 0 &&
		filter.Since == nil &&
		filter.Until == nil &&
		filter.Expiration == nil &&
		filter.Videos == nil &&
		filter.Quotes == nil &&
		filter.References == nil &&
		filter.Images == nil
}

func (w *whereBuilder) applyTimeRange(filter *filterBuilder, since, until *model.Timestamp) error {
	if since != nil && until != nil {
		if *since == *until {
			w.maybeAND()
			w.WriteString("created_at = :")
			w.WriteString(w.addParam(filter.Name, "timestamp", *since))

			return nil
		} else if *since > *until {
			return errors.Wrapf(ErrWhereBuilderInvalidTimeRange, "since [%d] is greater than until [%d]", *since, *until)
		}
	}

	// If a filter includes the `since` property, events with `created_at` greater than or equal to since are considered to match the filter.
	if since != nil && *since > 0 {
		w.maybeAND()
		w.WriteString("created_at >= :")
		w.WriteString(w.addParam(filter.Name, "since", *since))
	}

	// The `until` property is similar except that `created_at` must be less than or equal to `until`.
	if until != nil && *until > 0 {
		w.maybeAND()
		w.WriteString("created_at <= :")
		w.WriteString(w.addParam(filter.Name, "until", *until))
	}

	return nil
}

func filterHasExtensions(filter *model.Filter) (positive, negative int) {
	var values = []struct {
		val *bool
		bit int
	}{
		{filter.Expiration, extensionExpiration},
		{filter.Videos, extensionVideos},
		{filter.Images, extensionImages},
		{filter.Quotes, extensionQuotes},
		{filter.References, extensionReferences},
	}

	for _, v := range values {
		if v.val == nil {
			continue
		}

		if *v.val {
			positive |= v.bit
		} else {
			negative |= v.bit
		}
	}

	return
}

func (w *whereBuilder) applyFilterForExtensions(filter *model.Filter, builder *filterBuilder, include bool) {
	separator := w.maybeOR
	w.WriteString("select event_id from event_tags where ")
	if include && builder.HasEvents() {
		w.WriteString(builder.BuildEvents(w))
		w.maybeAND()
	}

	w.WriteRune('(')
	if filter.Quotes != nil && *filter.Quotes == include {
		separator()
		w.WriteString("(event_tag_key = 'q')")
	}
	if filter.References != nil && *filter.References == include {
		separator()
		w.WriteString("(event_tag_key = 'e')")
	}
	if filter.Images != nil && *filter.Images == include {
		separator()
		w.WriteString("(event_tag_key = 'imeta' AND substr(")
		w.WriteString(tagValueMimeType)
		w.WriteString(", 3, 5) = 'image')")
	}
	if filter.Videos != nil && *filter.Videos == include {
		separator()
		w.WriteString("(event_tag_key = 'imeta' AND substr(")
		w.WriteString(tagValueMimeType)
		w.WriteString(", 3, 5) = 'video')")
	}
	if filter.Expiration != nil {
		separator()
		if *filter.Expiration {
			w.WriteRune('(')
		}
		w.WriteString("(event_tag_key = 'expiration')")
		if *filter.Expiration {
			w.WriteString(" AND cast(")
			w.WriteString(tagValueExpiration)
			w.WriteString(" as integer) > unixepoch())")
		}
	}
	w.WriteRune(')')
}

func (w *whereBuilder) applyRepostFilter(filter *model.Filter, builder *filterBuilder, positiveExtensions, negativeExtensions *int) (applied bool) {
	if (*positiveExtensions + *negativeExtensions) == 0 {
		// No extensions in the filter.
		return
	}

	repostIdx := slices.Index(filter.Kinds, nostr.KindRepost)
	if repostIdx == -1 {
		// No reposts in the filter.
		return
	}

	// Not allowed.
	filter.References = nil
	*positiveExtensions &= ^extensionReferences
	*negativeExtensions &= ^extensionReferences

	if *positiveExtensions > 0 {
		w.maybeAND()
		w.WriteString("(id IN (select e.id from events subev where subev.id = e.reference_id and subev.kind = 1 and exists (")
		w.applyFilterForExtensions(filter, builder, true)
		w.WriteString(")))")
	}

	if *negativeExtensions > 0 {
		w.maybeAND()
		w.WriteString("(id NOT IN (select e.id from events subev where subev.id = e.reference_id and and subev.kind = 1 and exists (")
		w.applyFilterForExtensions(filter, builder, false)
		w.WriteString(")))")
	}

	return (*positiveExtensions + *negativeExtensions) > 0
}

func (w *whereBuilder) applyFilter(idx int, filter *model.Filter) error {
	if isFilterEmpty(filter) {
		return nil
	}

	builder := &filterBuilder{
		Name:     "filter" + strconv.Itoa(idx) + "_",
		EventIds: filter.IDs,
	}
	positiveExtensions, negativeExtensions := filterHasExtensions(filter)
	w.WriteRune('(') // Begin the filter section.
	if w.applyRepostFilter(filter, builder, &positiveExtensions, &negativeExtensions) {
		buildFromSlice(w, builder.Name, filter.IDs, "id")
	} else {
		if positiveExtensions > 0 {
			w.WriteString("id IN (")
			w.applyFilterForExtensions(filter, builder, true)
			w.WriteRune(')')
		} else {
			buildFromSlice(w, builder.Name, filter.IDs, "id")
		}
		if negativeExtensions > 0 {
			w.maybeAND()
			w.WriteString("(id NOT IN (")
			w.applyFilterForExtensions(filter, builder, false)
			w.WriteString("))")
		}
	}
	buildFromSlice(w, builder.Name, filter.Kinds, "kind")
	buildFromSlice(w, builder.Name, filter.Authors, "pubkey")
	if err := w.applyTimeRange(builder, filter.Since, filter.Until); err != nil {
		return err
	}
	w.applyFilterTags(builder, filter.Tags)

	w.WriteRune(')') // End the filter section.

	return nil
}

func (w *whereBuilder) Build(filters ...model.Filter) (sql string, params map[string]any, err error) {
	for idx := range filters {
		w.maybeOR()
		if err := w.applyFilter(idx, &filters[idx]); err != nil {
			return "", nil, errors.Wrapf(err, "failed to apply filter %d", idx)
		}
	}

	// If there are no filters, return the default WHERE clause.
	if w.Len() == 0 {
		return whereBuilderDefaultWhere, w.Params, nil
	}

	return w.String(), w.Params, nil
}
