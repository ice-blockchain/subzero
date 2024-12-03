// SPDX-License-Identifier: ice License 1.0

package query

import (
	"cmp"
	"log"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/model"
)

const (
	whereBuilderDefaultWhere = "hidden=0"
)

const (
	extensionExpiration = 1 << iota
	extensionVideos
	extensionImages
	extensionQuotes
	extensionReferences
)

const (
	sqlOpCodeNONE = iota
	sqlOpCodeAND
	sqlOpCodeOR
)

var (
	ErrWhereBuilderInvalidTimeRange = errors.New("invalid time range")
	ErrEmptyFilter                  = errors.New("empty filter")

	errUnsupportedCombination = errors.New("unsupported filter combination")
)

type (
	whereBuilder struct {
		Params       map[string]any
		Dependencies []*filterDependencies
		strings.Builder
	}
	databaseFilterSearch struct {
		model.Filter
		Expiration   *bool
		Videos       *bool
		Images       *bool
		Quotes       *bool
		References   *bool
		TagMarkers   []databaseFilterMarker
		Dependencies []*filterDependencies
	}
	databaseFilterDelete struct {
		Author string
		IDs    []string
		Events []struct {
			Kind   int
			Author string
			TagD   string
		}
	}
	databaseFilterMarker struct {
		Tag    string
		Marker string
	}
)

func parseEventAsFilterForDelete(e *model.Event) (*databaseFilterDelete, error) {
	filter := databaseFilterDelete{
		Author: e.PubKey,
	}

	for _, tag := range e.Tags {
		switch tag.Key() {
		case "e":
			if v := tag.Value(); v != "" {
				filter.IDs = append(filter.IDs, v)
			}

		case "a":
			vals := strings.Split(tag.Value(), ":")
			if len(vals) != 3 {
				return nil, errors.Errorf("failed to parse replaceable event reference, len != 3: %v", tag.Value())
			}

			kind, err := strconv.ParseInt(vals[0], 10, 64)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to parse replaceable event kind %v", tag.Value())
			}

			filter.Events = append(filter.Events, struct {
				Kind   int
				Author string
				TagD   string
			}{
				Kind:   int(kind),
				Author: vals[1],
				TagD:   vals[2],
			})
		}
	}

	if len(filter.IDs) == 0 && len(filter.Events) == 0 {
		return nil, errors.Errorf("failed to parse event reference, no filters found: %v", e)
	}

	return &filter, nil
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

//go:inline
func maybeOpCode(builder *whereBuilder, op int) {
	switch op {
	case sqlOpCodeAND:
		builder.maybeAND()
	case sqlOpCodeOR:
		builder.maybeOR()
	}
}

func buildFromSlice[T comparable](builder *whereBuilder, op int, filterID string, s []T, name, paramName string) *whereBuilder {
	if len(s) == 0 {
		return builder
	}

	if paramName == "" {
		paramName = name
	}

	maybeOpCode(builder, op)
	builder.WriteString(name)
	s = model.DeduplicateSlice(s, func(elem T) T { return elem })
	if len(s) == 1 {
		// X = :X_name.
		builder.WriteString(" = :")
		builder.WriteString(builder.addParam(filterID, paramName, s[0]))

		return builder
	}

	// X in (:X_name0, :X_name1, ...).
	builder.WriteString(" IN (")
	for i := range len(s) - 1 {
		builder.WriteRune(':')
		builder.WriteString(builder.addParam(filterID, paramName+strconv.Itoa(i), s[i]))
		builder.WriteRune(',')
	}
	builder.WriteRune(':')
	builder.WriteString(builder.addParam(filterID, paramName+strconv.Itoa(len(s)-1), s[len(s)-1]))
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

func (w *whereBuilder) applyFilterTagMarkers(name string, markers []databaseFilterMarker) {
	if len(markers) == 0 {
		return
	}

	for id, marker := range markers {
		w.maybeAND()
		w.WriteString("EXISTS (select true from event_tags where event_id = e.id AND event_tag_key = :")
		w.WriteString(w.addParam(name, "mtag"+strconv.Itoa(id), marker.Tag))
		w.WriteString(" AND event_tag_value3 = :")
		w.WriteString(w.addParam(name, "mtagvalue"+strconv.Itoa(id), marker.Marker))
		w.WriteRune(')')
	}
}

func (w *whereBuilder) applyFilterTags(name string, tags model.TagMap) {
	const valuesMax = 21

	if len(tags) == 0 {
		return
	}

	var tagID int
	for tagName, tagValues := range tags {
		tagID++

		w.maybeAND()
		tagParam := w.addParam(name, "tag"+strconv.Itoa(tagID), tagName)

		// Only the tag name is specified, no values.
		if !tags.HasValues(tagName) {
			w.WriteString("EXISTS (select event_id from event_tags where event_id = e.id AND event_tag_key = :")
			w.WriteString(tagParam)
			w.WriteRune(')')

			continue
		}

		w.WriteRune('(')
		for i, values := range tagValues {
			if values.Empty() {
				continue
			}

			if len(values) > valuesMax {
				log.Printf("%#v: too many values for tag %q, only the first %d will be used", values, tagName, valuesMax)
				values = values[:valuesMax]
			}

			w.maybeOR()
			w.WriteString("EXISTS (select event_id from event_tags where event_id = e.id AND event_tag_key = :")
			w.WriteString(tagParam)
			for j := range values {
				if values[j] == nil {
					// Skip empty values.
					continue
				}
				w.WriteString(" AND ")
				w.WriteString("event_tag_value")
				w.WriteString(strconv.Itoa(j + 1))
				w.WriteString(" = :")
				w.WriteString(w.addParam(name, "tagvalue"+strconv.Itoa(tagID<<8|(j+1)*(i+1)), *values[j]))
			}
			w.WriteRune(')')
		}
		w.WriteRune(')')
	}
}

func isFilterEmpty(filter *databaseFilterSearch) bool {
	return len(filter.IDs) == 0 &&
		len(filter.Kinds) == 0 &&
		len(filter.Authors) == 0 &&
		len(filter.Tags) == 0 &&
		len(filter.TagMarkers) == 0 &&
		filter.Since == nil &&
		filter.Until == nil &&
		filter.Expiration == nil &&
		filter.Videos == nil &&
		filter.Quotes == nil &&
		filter.References == nil &&
		filter.Images == nil
}

func (w *whereBuilder) applyTimeRange(name string, since, until *model.Timestamp) error {
	if since != nil && until != nil {
		if *since == *until {
			w.maybeAND()
			w.WriteString("created_at = :")
			w.WriteString(w.addParam(name, "timestamp", *since))

			return nil
		} else if *since > *until {
			return errors.Wrapf(ErrWhereBuilderInvalidTimeRange, "since [%d] is greater than until [%d]", *since, *until)
		}
	}

	// If a filter includes the `since` property, events with `created_at` greater than or equal to since are considered to match the filter.
	if since != nil && *since > 0 {
		w.maybeAND()
		w.WriteString("created_at >= :")
		w.WriteString(w.addParam(name, "since", *since))
	}

	// The `until` property is similar except that `created_at` must be less than or equal to `until`.
	if until != nil && *until > 0 {
		w.maybeAND()
		w.WriteString("created_at <= :")
		w.WriteString(w.addParam(name, "until", *until))
	}

	return nil
}

func filterHasExtensions(filter *databaseFilterSearch) (positive, negative int) {
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

func (w *whereBuilder) applyFilterForExtensions(filter *databaseFilterSearch, include bool) {
	separator := w.maybeOR
	if !include {
		w.WriteString("NOT ")
	}
	w.WriteString("exists (select true from event_tags where event_id in (e.id, e.reference_id) AND (")

	if filter.Quotes != nil && *filter.Quotes == include {
		separator()
		w.WriteString("(event_tag_key = 'q')")
	}
	if filter.References != nil && *filter.References == include {
		separator()
		result := "true"
		if !include {
			result = "false"
		}
		w.WriteString("(case when e.reference_id is not null then " + result + " else event_tag_key = 'e' end)")
	}
	if filter.Images != nil && *filter.Images == include {
		separator()
		w.WriteString("(event_tag_key = 'imeta' AND ")
		w.WriteString(tagValueMimeType)
		w.WriteString(" IN ('m image/png', 'm image/jpeg', 'm image/gif', 'm image/webp', 'm image/avif'))")
	}
	if filter.Videos != nil && *filter.Videos == include {
		separator()
		w.WriteString("(event_tag_key = 'imeta' AND ")
		w.WriteString(tagValueMimeType)
		w.WriteString(" IN ('m video/mp4', 'm video/mpeg', 'm video/mpeg4'))")
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
	w.WriteString("))")
}

func filterMainIndexField(filter *databaseFilterSearch) string {
	if len(filter.Authors) > 0 {
		return "master_pubkey"
	}

	if len(filter.Kinds) > 0 {
		return "kind"
	}

	return ""
}

func filterMaybeForceIndex(filter *databaseFilterSearch, field string) string {
	main := filterMainIndexField(filter)
	if main == field {
		field = "+" + field
	}
	return field
}

func (w *whereBuilder) applyFilter(idx int, filter *databaseFilterSearch) error {
	if isFilterEmpty(filter) {
		return nil
	}

	name := "filter" + strconv.Itoa(idx) + "_"
	positiveExtensions, negativeExtensions := filterHasExtensions(filter)
	w.WriteRune('(') // Begin the filter section.
	buildFromSlice(w, sqlOpCodeNONE, name, filter.IDs, "id", "")
	buildFromSlice(w, sqlOpCodeAND, name, filter.Kinds, filterMaybeForceIndex(filter, "kind"), "kind")
	if positiveExtensions > 0 {
		w.maybeAND()
		w.applyFilterForExtensions(filter, true)
	}
	if negativeExtensions > 0 {
		w.maybeAND()
		w.applyFilterForExtensions(filter, false)
	}
	if len(filter.Authors) > 0 {
		w.maybeAND()
		w.WriteRune('(')
		buildFromSlice(w, sqlOpCodeNONE, name, filter.Authors, "pubkey", "")
		w.WriteString(" and hidden=0 OR ")
		buildFromSlice(w, sqlOpCodeNONE, name, filter.Authors, "master_pubkey", "pubkey")
		w.WriteString(" and hidden=0)")
	}
	if err := w.applyTimeRange(name, filter.Since, filter.Until); err != nil {
		return err
	}
	w.applyFilterTags(name, filter.Tags)
	w.applyFilterTagMarkers(name, filter.TagMarkers)

	w.WriteRune(')') // End the filter section.

	return nil
}

func (w *whereBuilder) createWhereForDepFilter(filterID, cteName, field string, filter *filterDependenciesStart) string {
	var sb strings.Builder

	sb.WriteString("select ")
	sb.WriteString(field)
	sb.WriteString(" from ")
	sb.WriteString(cteName)
	sb.WriteString(" where ")
	sb.WriteString(cteName)
	sb.WriteString(".kind = :")
	sb.WriteString(w.addParam(filterID, "kind", filter.Kind))
	if filter.ProfileBadges {
		sb.WriteString(" AND d_tag='profile_badges'")
	}
	if filter.Tag != "" {
		sb.WriteString(" AND EXISTS (select 42 from event_tags where event_id = ")
		sb.WriteString(cteName)
		sb.WriteString(".id AND event_tag_key = :")
		sb.WriteString(w.addParam(filterID, "tag", filter.Tag))
		sb.WriteString(")")
	}

	return sb.String()
}

func (w *whereBuilder) applyDepFilter(filterID, cteName string, filter *filterDependencies) {
	if len(filter.Reduce.Kinds) > 0 && filter.Reduce.Kinds[0] == model.KindDVMCountResponse {
		w.WriteString(`
union all
select
	6400,
	unixepoch(),
	0,
	case when f.kind = 3 then '' else f.reference_id end as id,
	coalesce(evr.pubkey, ''),
	coalesce(evr.master_pubkey, ''),
	'',
	case when f.kind = 7 then json_object('+', f.value) else cast(f.value as text) end as content,
	json_array(json_object('kinds', json_array(:` + (filterID + "fkind") + `),:` + (filterID + "ftagname") + `,json_array(f.reference_id))) as d_tag,
	case when
		f.kind = 7 then
			json_array(
				json_array('output', 'JSON'),
				json_array('param', 'group', :` + (filterID + "context") + `
			))
		else
			json_array(json_array('param', 'group', :` + (filterID + "context") + `))
		end as jtags
from
	event_counters f
inner join ` + cteName + ` evr on evr.kind = :` + (filterID + "kind") + ` and f.reference_id in (evr.id, evr.pubkey, evr.master_pubkey)
where
`)
	} else {
		w.WriteString(`
union all
select
	e.kind,
	e.created_at,
	e.system_created_at,
	e.id,
	e.pubkey,
	e.master_pubkey,
	e.sig,
	e.content,
	e.d_tag,
	tags as jtags
from
	events e
where
`)
		w.WriteString(`e.id not in (select `)
		w.WriteString(cteName)
		w.WriteString(`.id from `)
		w.WriteString(cteName)
		w.WriteString(`) AND `)
	}

	if filter.Expiration != nil && *filter.Expiration {
		w.WriteString(`e.master_pubkey IN (select master_pubkey from ` + cteName + `)
and e.hidden=0
and e.kind in (1, 30023)
and exists (select true from event_tags where event_id = e.id and event_tag_key = 'expiration' and cast(` + tagValueExpiration + ` as integer) > unixepoch())`)

		return
	}

	switch filter.Reduce.Kinds[0] {
	case nostr.KindTextNote, nostr.KindRepost, nostr.KindReaction, nostr.KindArticle, nostr.KindGenericRepost:
		w.WriteString("e.kind = :")
		w.WriteString(w.addParam(filterID, "rkind", filter.Reduce.Kinds[0]))
		if filter.Reduce.Author != "" {
			w.WriteString(" AND :")
			w.WriteString(w.addParam(filterID, "author", filter.Reduce.Author))
			w.WriteString(" IN (e.pubkey, e.master_pubkey) AND ")
		}
		tag := filter.Reduce.Tag
		if tag == "" {
			// Repost, reaction.
			tag = "e"
		}
		w.WriteString("e.id in (select event_id from event_tags where event_tag_key = :")
		w.WriteString(w.addParam(filterID, "rtag", tag))
		w.WriteString(" and event_tag_value1 in (")
		w.WriteString(w.createWhereForDepFilter(filterID, cteName, "id", &filter.Start))
		w.WriteRune(')')
		if filter.Reduce.Context != "" {
			w.WriteString(" and event_tag_value3 = :")
			w.WriteString(w.addParam(filterID, "rcontext", filter.Reduce.Context))
		}
		w.WriteString(" group by event_tag_value1) AND e.hidden=0")

	case nostr.KindBadgeDefinition:
		startFilter := w.createWhereForDepFilter(filterID, cteName, "id", &filter.Start)
		w.WriteString("e.id in ((select event_tag_value1 from event_tags where event_id in (")
		w.WriteString(startFilter)
		w.WriteString(") and event_tag_key = 'e'),")
		w.WriteString(`(select ee.id from (select subzero_nostr_tag_a_get_pk(event_tag_value1) as pk, subzero_nostr_tag_a_get_dtag(event_tag_value1) as name from event_tags where event_id in (`)
		w.WriteString(startFilter)
		w.WriteString(") and event_tag_key = 'a') badge, events ee where badge.pk in (ee.pubkey, ee.master_pubkey) and ee.d_tag = badge.name and ee.kind = 30009 and hidden = 0)) AND e.hidden=0")

	case nostr.KindRelayListMetadata:
		w.WriteString("e.kind = :")
		w.WriteString(w.addParam(filterID, "rkind", filter.Reduce.Kinds[0]))
		w.WriteString(" AND ( master_pubkey IN (")
		w.WriteString(w.createWhereForDepFilter(filterID, cteName, "master_pubkey", &filter.Start))
		w.WriteString(") OR pubkey IN (")
		w.WriteString(w.createWhereForDepFilter(filterID, cteName, "pubkey", &filter.Start))
		w.WriteString(")) AND e.hidden=0")
		w.WriteString(`
union all
select
	20002,
	0 as created_at,
	0 as system_created_at,
	'' as id,
	e.pubkey,
	e.master_pubkey,
	'' as sig,
	'' as content,
	'' as d_tag,
	'[]' as jtags
from
	events e
inner join `)
		w.WriteString(cteName)
		w.WriteString(` on e.id = `)
		w.WriteString(cteName)
		w.WriteString(`.id where e.kind =:`)
		w.WriteString(w.addParam(filterID, "kind", filter.Start.Kind))
		if filter.Start.Tag != "" {
			w.WriteString(" AND EXISTS (select true from event_tags where event_id = ")
			w.WriteString(cteName)
			w.WriteString(".id AND event_tag_key = :")
			w.WriteString(w.addParam(filterID, "tag", filter.Start.Tag))
			w.WriteString(")")
		}
		w.WriteString(` AND
not exists (select true from events subev where subev.kind = 10002 and
(
	(subev.pubkey = e.pubkey               and subev.hidden = 0) or
	(subev.master_pubkey = e.master_pubkey and subev.hidden = 0) or
	(subev.master_pubkey = e.pubkey        and subev.hidden = 0) or
	(subev.pubkey = e.master_pubkey        and subev.hidden = 0)
)) and e.hidden=0
group by e.pubkey, e.master_pubkey`)

	case nostr.KindProfileMetadata:
		w.WriteString("e.kind = :")
		w.WriteString(w.addParam(filterID, "rkind", filter.Reduce.Kinds[0]))
		w.WriteString(" AND ( master_pubkey IN (")
		w.WriteString(w.createWhereForDepFilter(filterID, cteName, "master_pubkey", &filter.Start))
		w.WriteString(") OR pubkey IN (")
		w.WriteString(w.createWhereForDepFilter(filterID, cteName, "pubkey", &filter.Start))
		w.WriteString(")) AND e.hidden=0")

	case model.KindDVMCountResponse:
		w.WriteString("f.kind = :")
		w.WriteString(w.addParam(filterID, "rkind", filter.Reduce.Kinds[1]))
		w.addParam(filterID, "ftagname", "#e")
		w.addParam(filterID, "fkind", filter.Reduce.Kinds[1])
		var refType string
		switch {
		case filter.Reduce.Tag == "q":
			w.addParam(filterID, "ftagname", "#q")
			w.addParam(filterID, "context", filter.Reduce.Tag)
			refType = "quote"

		case filter.Reduce.Context == "content" || filter.Reduce.Tag == "e":
			w.addParam(filterID, "context", cmp.Or(filter.Reduce.Context, filter.Reduce.Tag))

		case filter.Reduce.Context == "root" || filter.Reduce.Context == "reply":
			w.addParam(filterID, "context", filter.Reduce.Context)
			refType = "reply"

		case filter.Reduce.Tag == "p":
			w.addParam(filterID, "ftagname", "#p")
			w.addParam(filterID, "context", filter.Reduce.Tag)
			refType = "follower"
		}
		w.WriteString(" AND f.reference_type = :")
		w.WriteString(w.addParam(filterID, "rref", refType))
		w.WriteString(" AND f.reference_id IN (")
		if filter.Reduce.Kinds[1] == nostr.KindFollowList {
			w.WriteString(w.createWhereForDepFilter(filterID, cteName, "pubkey", &filter.Start))
			w.WriteString(" UNION ALL ")
			w.WriteString(w.createWhereForDepFilter(filterID, cteName, "master_pubkey", &filter.Start))
		} else {
			w.WriteString(w.createWhereForDepFilter(filterID, cteName, "id", &filter.Start))
		}
		w.WriteString(")")
	}
}

func (w *whereBuilder) BuildDependencies(cteName string) (sql string, params map[string]any, err error) {
	if len(w.Dependencies) == 0 {
		return "", w.Params, nil
	}

	w.Reset()
	for idx, filter := range w.Dependencies {
		filterID := "dep" + cteName + strconv.Itoa(idx) + "_"
		w.applyDepFilter(filterID, cteName, filter)
	}

	return w.String(), w.Params, nil
}

func (w *whereBuilder) Build(filters ...model.Filter) (sql string, params map[string]any, err error) {
	for idx := range filters {
		w.maybeOR()
		dbFilter, err := parseNostrFilter(filters[idx])
		if err != nil {
			return "", nil, errors.Wrapf(err, "failed to parse filter %d", idx)
		}
		if err := w.applyFilter(idx, dbFilter); err != nil {
			return "", nil, errors.Wrapf(err, "failed to apply filter %d", idx)
		}
		if dbFilter.Dependencies != nil {
			w.Dependencies = append(w.Dependencies, dbFilter.Dependencies...)
		}
	}

	if w.Len() > 0 {
		w.WriteString(" AND ")
	}
	w.WriteString(whereBuilderDefaultWhere)

	return w.String(), w.Params, nil
}

func (w *whereBuilder) applyLiteFilter(idx int, filter *databaseFilterDelete) {
	filterID := "litefilter" + strconv.Itoa(idx) + "_"

	// Filter expression consists of two parts: (event filter) AND (access filter):
	// - Event filter (ORed):
	//   - By ID.
	//   - By kind and author and D tag.
	// - Account filter (ORed):
	//   - By author.
	//   - By master pubkey.
	//   - By onbehalf attestations.
	w.WriteRune('(')
	if len(filter.IDs) > 0 || len(filter.Events) > 0 {
		w.WriteRune('(')
		buildFromSlice(w, sqlOpCodeNONE, filterID, filter.IDs, "id", "")
		for i := range filter.Events {
			idxStr := strconv.Itoa(i)
			w.maybeOR()
			w.WriteString("(kind = :")
			w.WriteString(w.addParam(filterID, "kind"+idxStr, filter.Events[i].Kind))
			w.WriteString(" AND pubkey = :")
			w.WriteString(w.addParam(filterID, "author"+idxStr, filter.Events[i].Author))
			w.WriteString(" AND d_tag = :")
			w.WriteString(w.addParam(filterID, "dtag"+idxStr, filter.Events[i].TagD))
			w.WriteRune(')')
		}
		w.WriteString(") AND ")
	}

	owner := w.addParam(filterID, "pubkey", filter.Author)
	w.WriteString("((pubkey = :")
	w.WriteString(owner)
	w.WriteString(" OR master_pubkey = :")
	w.WriteString(owner)
	w.WriteString(") OR (pubkey != master_pubkey AND ")
	w.WriteString("subzero_nostr_onbehalf_is_allowed(coalesce((select p.tags from events p where p.master_pubkey = master_pubkey and p.kind = 10100 and hidden=0), '[]'), :")
	w.WriteString(owner)
	w.WriteString(", master_pubkey, kind, unixepoch()))))")
}

func (w *whereBuilder) BuildForDelete(filters ...databaseFilterDelete) (sql string, params map[string]any, err error) {
	for idx := range filters {
		w.maybeOR()
		w.applyLiteFilter(idx, &filters[idx])
	}

	if w.Len() == 0 {
		return "", nil, ErrEmptyFilter
	}

	w.WriteString(" AND ")
	w.WriteString(whereBuilderDefaultWhere)

	return w.String(), w.Params, nil
}

func isValidCounterFilter(filter *model.Filter) (valid bool) {
	switch {
	// Filter is required.
	case filter == nil:

	// Search by text is not supported.
	case filter.Search != "":

	// Time range is not supported.
	case filter.Since != nil || filter.Until != nil:

	// Only IDs or authors are allowed, but not both.
	case (len(filter.IDs) > 0 && len(filter.Authors) > 0):
		valid = len(filter.Kinds) > 0

	default:
		valid = true
	}

	return valid
}

func (w *whereBuilder) BuildForPrecalculatedCounters(filters ...model.Filter) (sql string, params map[string]any, err error) {
	if len(filters) == 0 {
		return "", nil, ErrEmptyFilter
	}

	for idx := range filters {
		if !isValidCounterFilter(&filters[idx]) {
			return "", nil, errors.Wrapf(errUnsupportedCombination, "filter %d", idx)
		}

		filterID := "eventcounter" + strconv.Itoa(idx) + "_"
		filter := &filters[idx]

		w.maybeOR()

		kinds := model.DeduplicateSlice(filter.Kinds, func(k int) int { return k })
		startLen := w.Len()
		w.WriteRune('(')
		if len(filter.Kinds) > 0 {
			w.WriteRune('(')
			for idx := range kinds {
				w.maybeOR()
				w.WriteString("kind = :")
				w.WriteString(w.addParam(filterID, "kind"+strconv.Itoa(idx), kinds[idx]))
				w.WriteString(" AND reference_type = :")
				var referenceType string
				switch kinds[idx] {
				case nostr.KindFollowList:
					referenceType = "follower"
				case nostr.KindTextNote, nostr.KindRepost, nostr.KindArticle, nostr.KindGenericRepost:
					if _, ok := filter.Tags["q"]; ok {
						referenceType = "quote"
					} else {
						referenceType = "reply"
					}
				}
				w.WriteString(w.addParam(filterID, "reference_type"+strconv.Itoa(idx), referenceType))

			}
			w.WriteRune(')')
		}
		buildFromSlice(w, sqlOpCodeAND, filterID, filter.Authors, "reference_id", "")
		buildFromSlice(w, sqlOpCodeAND, filterID, filter.IDs, "reference_id", "")
		if w.Len() == startLen+1 {
			return "", nil, errUnsupportedCombination
		}
		w.WriteRune(')')
	}

	return w.String(), w.Params, nil
}
