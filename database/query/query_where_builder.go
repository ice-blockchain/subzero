package query

import (
	"log"
	"strconv"
	"strings"

	"github.com/nbd-wtf/go-nostr"
)

const (
	whereBuilderDefaultWhere = "1=1"
)

type whereBuilder struct {
	Params map[string]any
	strings.Builder
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
	if len(s) == 0 {
		return s
	}

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

func buildFromSlice[T comparable](builder *whereBuilder, filterID string, s []T, name string) {
	if len(s) == 0 {
		return
	}

	builder.maybeAND()
	builder.WriteString(name)
	s = deduplicateSlice(s)
	if len(s) == 1 {
		// X = :X_name.
		builder.WriteString(" = :")
		builder.WriteString(builder.addParam(filterID, name, s[0]))

		return
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
}

func (w *whereBuilder) isOnBegin() bool {
	if w.Len() == 1 && w.String() == "(" {
		return true
	} else if w.Len() > 1 {
		s := w.String()

		return s[len(s)-1] == '(' || s[len(s)-2:] == "( "
	}

	return false
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

func (w *whereBuilder) applyFilterTags(filterID string, ids []string, tags nostr.TagMap) {
	const valuesMax = 4

	if len(tags) == 0 {
		return
	}

	idsBuilder := &whereBuilder{Params: w.Params}
	buildFromSlice(idsBuilder, filterID, ids, "event_id")

	tagID := 0
	for tag, values := range tags {
		w.maybeAND()
		if len(values) > valuesMax {
			log.Printf("%#v: too many values for tag %q, only the first %d will be used", values, tag, valuesMax)
			values = values[:valuesMax]
		}

		tagID++
		if idsBuilder.Len() > 0 {
			// We already have some IDs, so we need to check if they have the tag.
			w.WriteString("EXISTS (select 42 from event_tags where ")
			w.WriteString(idsBuilder.String())
			w.maybeAND()
		} else {
			// No IDs, so select all events that belong to the given tag.
			w.WriteString("id IN (select event_id from event_tags where ")
		}
		w.WriteString("event_tag_key = :")
		w.WriteString(w.addParam(filterID, "tag"+strconv.Itoa(tagID), tag))

		for i, value := range values {
			w.WriteString(" AND ")
			w.WriteString("event_tag_value")
			w.WriteString(strconv.Itoa(i + 1))
			w.WriteString(" = :")
			w.WriteString(w.addParam(filterID, "tagvalue"+strconv.Itoa(tagID<<8|i+1), value))
		}
		w.WriteRune(')')
	}
}

func isFilterEmpty(filter *nostr.Filter) bool {
	return len(filter.IDs) == 0 &&
		len(filter.Kinds) == 0 &&
		len(filter.Authors) == 0 &&
		len(filter.Tags) == 0 &&
		filter.Since == nil &&
		filter.Until == nil
}

func (w *whereBuilder) applyFilter(idx int, filter *nostr.Filter) {
	if isFilterEmpty(filter) {
		return
	}

	filterID := "filter" + strconv.Itoa(idx) + "_"
	w.WriteRune('(') // Begin the filter section.
	buildFromSlice(w, filterID, filter.IDs, "id")
	buildFromSlice(w, filterID, filter.Kinds, "kind")
	buildFromSlice(w, filterID, filter.Authors, "pubkey")
	w.applyFilterTags(filterID, filter.IDs, filter.Tags)

	// If a filter includes the `since` property, events with `created_at` greater than or equal to since are considered to match the filter.
	if filter.Since != nil {
		w.maybeAND()
		w.WriteString("created_at >= :")
		w.WriteString(w.addParam(filterID, "since", *filter.Since))
	}

	// The `until` property is similar except that `created_at` must be less than or equal to `until`.
	if filter.Until != nil {
		w.maybeAND()
		w.WriteString("created_at <= :")
		w.WriteString(w.addParam(filterID, "until", *filter.Until))
	}

	w.WriteRune(')') // End the filter section.
}

func (w *whereBuilder) Build(filters ...nostr.Filter) (sql string, params map[string]any) {
	for idx := range filters {
		w.maybeOR()
		w.applyFilter(idx, &filters[idx])
	}

	// If there are no filters, return the default WHERE clause.
	if w.Len() == 0 {
		return whereBuilderDefaultWhere, w.Params
	}

	return w.String(), w.Params
}
