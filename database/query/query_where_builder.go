// SPDX-License-Identifier: ice License 1.0

package query

import (
	"cmp"
	"log"
	"strconv"
	"strings"
	"unicode"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/model"
)

const (
	whereBuilderDefaultWhere    = "e.hidden=false"
	whereBuilderCommunityFilter = "(case when e.kind in (1, 30023, 30175) then NOT EXISTS (select true from event_tags where event_id = e.id AND event_tag_key = 'h') else true end)"
	whereBuilderNoSoftDeleted   = "e.deleted=false"
	whereBuilderDefaultOrderBy  = "created_at DESC"

	whereBuilderDefaultLimit = 300
)

const (
	rankUndef rank = iota
	rankTOP
	rankTrending
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
	rank         int
	queryBuilder struct {
		Params map[string]any
		strings.Builder
	}
	databaseFilterSearch struct {
		model.Filter
		ID           string
		SearchText   string
		Expiration   *bool
		Videos       *bool
		Images       *bool
		Quotes       *bool
		References   *bool
		TagMarkers   []databaseFilterMarker
		Dependencies []*filterDependency
		Rank         rank
		Extra        []string // Extra `where` clauses, ANDed to the main filter.
	}
	databaseFilterDelete struct {
		Author string
		IDs    []string
		Events []databaseEventAddress
	}
	databaseFilterMarker struct {
		Tag     string
		Marker  string
		Exclude bool
	}
	databaseCTE struct {
		Name    string
		Body    string
		OrderBy string
		Filter  *databaseFilterSearch
	}
)

func parseEventAsFilterForDelete(e *model.Event) (*databaseFilterDelete, error) {
	filter := databaseFilterDelete{
		Author: e.GetMasterPublicKey(),
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

			filter.Events = append(filter.Events, databaseEventAddress{
				Kind:   int(kind),
				Pubkey: vals[1],
				Dtag:   vals[2],
			})
		}
	}

	return &filter, nil
}

func newQueryBuilder() *queryBuilder {
	return &queryBuilder{
		Params: make(map[string]any),
	}
}

func (b *queryBuilder) PushValue(filterID, name string, value any) (key string) {
	key = filterID + name
	b.Params[key] = value

	return key
}

func (b *queryBuilder) WriteValue(filterID, name string, value any) {
	b.WriteString(b.PushValue(filterID, name, value))
}

func (b *queryBuilder) MaybeOP(op int) {
	switch op {
	case sqlOpCodeAND:
		b.MaybeAND()

	case sqlOpCodeOR:
		b.MaybeOR()
	}
}

type sliceBuilder[T comparable] struct {
	Slice     []T
	Negative  bool
	ParamName string
	Op        int
}

func (b *sliceBuilder[T]) Build(builder *queryBuilder, filterID string, name string) *queryBuilder {
	if len(b.Slice) == 0 {
		return builder
	}

	if b.ParamName == "" {
		b.ParamName = name
	}

	builder.MaybeOP(b.Op)
	builder.WriteString(name)
	s := model.DeduplicateSlice(b.Slice, func(elem T) T { return elem })
	if len(s) == 1 {
		// X = :X_name.
		if b.Negative {
			builder.WriteString(" != :")
		} else {
			builder.WriteString(" = :")
		}
		builder.WriteString(builder.PushValue(filterID, b.ParamName, s[0]))

		return builder
	}

	// X in (:X_name0, :X_name1, ...).
	if b.Negative {
		builder.WriteString(" NOT IN (")
	} else {
		builder.WriteString(" IN (")
	}
	for i := range len(s) - 1 {
		builder.WriteRune(':')
		builder.WriteString(builder.PushValue(filterID, b.ParamName+strconv.Itoa(i), s[i]))
		builder.WriteRune(',')
	}
	builder.WriteRune(':')
	builder.WriteString(builder.PushValue(filterID, b.ParamName+strconv.Itoa(len(s)-1), s[len(s)-1]))
	builder.WriteRune(')')

	return builder
}

func buildFromSlice[T comparable](builder *queryBuilder, op int, filterID string, s []T, name, paramName string) *queryBuilder {
	return (&sliceBuilder[T]{
		Op:        op,
		Slice:     s,
		ParamName: paramName,
	}).Build(builder, filterID, name)
}

func buildFromSliceNegative[T comparable](builder *queryBuilder, op int, filterID string, s []T, name, paramName string) *queryBuilder {
	return (&sliceBuilder[T]{
		Op:        op,
		Negative:  true,
		Slice:     s,
		ParamName: paramName,
	}).Build(builder, filterID, name)
}

func (b *queryBuilder) IsOnBegin() bool {
	if b.Len() == 1 && b.String() == "(" {
		return true
	}

	s := b.String()

	return s[len(s)-1] == '(' || s[len(s)-2:] == "( "
}

func (b *queryBuilder) MaybeAND() {
	if b.Len() == 0 || b.IsOnBegin() {
		return
	}

	b.WriteString(" AND ")
}

func (b *queryBuilder) MaybeOR() {
	if b.Len() == 0 || b.IsOnBegin() {
		return
	}

	b.WriteString(" OR ")
}

func (b *queryBuilder) ApplyFilterTagMarkers(filterID string, markers ...databaseFilterMarker) {
	if len(markers) == 0 {
		return
	}

	for id, marker := range markers {
		b.MaybeAND()
		if marker.Exclude {
			b.WriteString("NOT ")
		}
		b.WriteString("EXISTS (select true from event_tags where event_id in (e.id, e.reference_id) AND event_tag_key = :")
		b.WriteValue(filterID, "mtagname"+strconv.Itoa(id), marker.Tag)
		b.WriteString(" AND event_tag_value3 = :")
		b.WriteValue(filterID, "mtagvalue"+strconv.Itoa(id), marker.Marker)
		b.WriteRune(')')
	}
}

func (b *queryBuilder) ApplyFilterTags(filterID string, tags model.TagMap) {
	if len(tags) == 0 {
		return
	}

	var tagID, tagValue uint64
	for tagName, tagValues := range tags {
		tagID++

		queryTagName := tagName
		exclude := false
		if tagName != "" && tagName[0] == '!' {
			exclude = true
			queryTagName = tagName[1:]
		}

		b.MaybeAND()
		tagParam := b.PushValue(filterID, "tag"+strconv.FormatUint(tagID, 10), queryTagName)

		// Only the tag name is specified, no values.
		if !tags.HasValues(tagName) {
			if exclude {
				b.WriteString("NOT ")
			}
			b.WriteString("EXISTS (select event_id from event_tags where event_id = e.id AND event_tag_key = :")
			b.WriteString(tagParam)
			b.WriteRune(')')

			continue
		}

		b.WriteRune('(')
		for _, values := range tagValues {
			if values.Empty() {
				continue
			}

			if len(values) > maxTagValues {
				log.Printf("filter %v: %q: too many values for tag %q, only the first %d will be used",
					filterID,
					func() string {
						var b strings.Builder
						for i, v := range values {
							if v == nil {
								b.WriteString("nil")
							} else {
								b.WriteString(*v)
							}
							if i < len(values)-1 {
								b.WriteString(", ")
							}
						}
						return b.String()
					}(),
					tagName,
					maxTagValues,
				)
				values = values[:maxTagValues]
			}

			b.MaybeOR()
			if exclude {
				b.WriteString("NOT ")
			}
			b.WriteString("EXISTS (select event_id from event_tags where event_id = e.id AND event_tag_key = :")
			b.WriteString(tagParam)
			for j := range values {
				if values[j] == nil {
					// Skip empty values.
					continue
				}
				b.WriteString(" AND ")
				b.WriteString("event_tag_value")
				b.WriteString(strconv.Itoa(j + 1))
				b.WriteString(" = :")
				b.WriteValue(filterID, "tagvalue"+strconv.FormatUint(tagValue, 10), *values[j])
				tagValue++
			}
			b.WriteRune(')')
		}
		b.WriteRune(')')
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
		filter.Images == nil &&
		filter.SearchText == ""
}

func (b *queryBuilder) ApplyTimeRange(filterID string, since, until *model.Timestamp) error {
	if since != nil && until != nil {
		if *since == *until {
			b.MaybeAND()
			b.WriteString("e.created_at = to_timestamp(:")
			b.WriteValue(filterID, "timestamp", *since)
			b.WriteRune(')')

			return nil
		} else if *since > *until {
			return errors.Wrapf(ErrWhereBuilderInvalidTimeRange, "since [%d] is greater than until [%d]", *since, *until)
		}
	}

	// If a filter includes the `since` property, events with `created_at` greater than or equal to since are considered to match the filter.
	if since != nil && *since > 0 {
		b.MaybeAND()
		b.WriteString("e.created_at >= to_timestamp(:")
		b.WriteValue(filterID, "since", *since)
		b.WriteRune(')')
	}

	// The `until` property is similar except that `created_at` must be less than or equal to `until`.
	if until != nil && *until > 0 {
		b.MaybeAND()
		b.WriteString("e.created_at <= to_timestamp(:")
		b.WriteValue(filterID, "until", *until)
		b.WriteRune(')')
	}

	return nil
}

func (b *queryBuilder) ApplyFilterForExtensions(filter *databaseFilterSearch) {
	if filter.Videos != nil {
		b.MaybeAND()
		b.WriteString("e.has_videos=:")
		b.WriteValue(filter.ID, "videos", *filter.Videos)
	}
	if filter.Images != nil {
		b.MaybeAND()
		b.WriteString("e.has_images=:")
		b.WriteValue(filter.ID, "images", *filter.Images)
	}

	if filter.Quotes != nil {
		b.MaybeAND()
		if !*filter.Quotes {
			b.WriteString("NOT ")
		}
		b.WriteString("exists (select true from event_tags where event_id in (e.id, e.reference_id) AND event_tag_key in ('q', 'Q'))")
	}

	if filter.Expiration != nil {
		b.MaybeAND()
		if *filter.Expiration {
			b.WriteString(`exists
(select true from event_tags where
	event_id in (e.id, e.reference_id) 
	AND event_tag_key = 'expiration'
	AND to_timestamp(cast(event_tag_value1 as bigint)) > CURRENT_TIMESTAMP)`)
		} else {
			b.WriteString("NOT exists (select true from event_tags where event_id in (e.id, e.reference_id) AND event_tag_key = 'expiration')")
		}
	}

	if filter.References != nil {
		b.MaybeAND()
		b.WriteString(`(case when e.reference_id is not null then true else `)
		if !*filter.References {
			b.WriteString("NOT ")
		}
		b.WriteString("exists (select true from event_tags where event_id = e.id AND event_tag_key in ('a', 'e')) end)")
	}
}

func (b *queryBuilder) ApplyFilterSoftDeleted(filter *databaseFilterSearch) {
	if len(filter.Kinds) > 0 && len(filter.Authors) > 0 && filter.Tags.HasValues("d") {
		// Addressable event.
		return
	}

	if len(filter.IDs) == 0 {
		b.MaybeAND()
		b.WriteString(whereBuilderNoSoftDeleted)
	}
}

func replaceSpecialChars(input string) string {
	if input == "" {
		return ""
	}

	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSymbol('_') {
			return r
		} else if unicode.IsSpace(r) {
			return -1
		}

		return '_'
	}, input)
}

func (b *queryBuilder) ApplyTextSearch(filter *databaseFilterSearch) {
	if filter.SearchText == "" {
		return
	}

	text := replaceSpecialChars(filter.SearchText)
	text = text + ":*"

	b.MaybeAND()
	b.WriteString("(e.lookup @@ to_tsquery(:")
	b.WriteValue(filter.ID, "fts", text)
	b.WriteString(`))`)
}

func (b *queryBuilder) MaybeApplyTextSearch(filter *databaseFilterSearch) {
	// Skip text search if we have kinds=3 AND kind3>kind0 as a dependency.
	if len(filter.Kinds) == 1 && filter.Kinds[0] == nostr.KindFollowList {
		for i := range filter.Dependencies {
			if filter.Dependencies[i].Start.Kind == nostr.KindFollowList &&
				len(filter.Dependencies[i].Reduce.Kinds) > 0 &&
				filter.Dependencies[i].Reduce.Kinds[0] == nostr.KindProfileMetadata {
				return
			}
		}
	}

	b.ApplyTextSearch(filter)
}

func (b *queryBuilder) ApplyFilter(filter *databaseFilterSearch) error {
	if isFilterEmpty(filter) {
		return nil
	}

	b.WriteRune('(') // Begin the filter section.
	buildFromSlice(b, sqlOpCodeNONE, filter.ID, filter.IDs, "e.id", "")
	buildFromSlice(b, sqlOpCodeAND, filter.ID, filter.Addresses, "e.address", "")
	buildFromSlice(b, sqlOpCodeAND, filter.ID, filter.Kinds, "e.kind", "")
	b.ApplyFilterForExtensions(filter)
	if len(filter.Authors) > 0 {
		b.MaybeAND()
		b.WriteRune('(')
		buildFromSlice(b, sqlOpCodeNONE, filter.ID, filter.Authors, "e.pubkey", "")
		b.WriteString(" and e.hidden=false OR ")
		buildFromSlice(b, sqlOpCodeNONE, filter.ID, filter.Authors, "e.master_pubkey", "e.pubkey")
		b.WriteString(" and e.hidden=false)")
	}
	if err := b.ApplyTimeRange(filter.ID, filter.Since, filter.Until); err != nil {
		return err
	}
	b.ApplyFilterTags(filter.ID, filter.Tags)
	b.ApplyFilterTagMarkers(filter.ID, filter.TagMarkers...)
	b.ApplyFilterSoftDeleted(filter)
	b.MaybeApplyTextSearch(filter)

	if _, ok := filter.Tags[model.CustomIONTagCommunity]; !ok {
		b.MaybeAND()
		b.WriteString(whereBuilderCommunityFilter)
	}
	b.WriteRune(')') // End the filter section.

	return nil
}

func (b *queryBuilder) BuildQueryForDependencyStart(filterID, cteName, field string, filter *filterDependencyStart) string {
	var sb strings.Builder

	sb.WriteString("select ")
	sb.WriteString(field)
	sb.WriteString(" from ")
	sb.WriteString(cteName)
	sb.WriteString(" where ")
	sb.WriteString(cteName)
	sb.WriteString(".kind = :")
	sb.WriteString(b.PushValue(filterID, "kind", filter.Kind))
	if filter.ProfileBadges {
		sb.WriteString(" AND d_tag='profile_badges'")
	}
	if filter.Tag != "" {
		sb.WriteString(" AND EXISTS (select 42 from event_tags where event_id = ")
		sb.WriteString(cteName)
		sb.WriteString(".id AND event_tag_key = :")
		sb.WriteString(b.PushValue(filterID, "tag", filter.Tag))
		sb.WriteString(")")
	}

	return sb.String()
}

func (b *queryBuilder) CountVotesOf(filterID, cteName string, filter *filterDependency) {
	b.WriteString(`
union
select
	6400,
	CURRENT_TIMESTAMP,
	'' as id,
	'' as address,
	t.pubkey,
	t.master_pubkey,
	'' as sig,
	cast(jsonb_object_agg(t.option, t.votes) as text) AS content,
	cast(jsonb_build_array(jsonb_build_object(
		'kinds', jsonb_build_array(1754),
		subzero_nostr_get_event_address_tag(t.kind), jsonb_build_array(t.address))
	) as text) as d_tag,
	t.h_tag,
	jsonb_build_array(
		jsonb_build_array('output', 'JSON'),
		jsonb_build_array('param', 'group', 'content')
	) as tags
from (
	select
		mainev.id AS poll_id,
		mainev.pubkey,
		mainev.master_pubkey,
		mainev.kind,
		mainev.address,
		mainev.h_tag,
		mainev.d_tag,
		cast(j.value as text) AS option,
		COUNT(j.value) AS votes
	from `)
	b.WriteString(cteName)
	b.WriteString(` mainev
	left join event_tags et ON et.event_tag_value1 = mainev.address AND et.event_tag_key in ('a', 'e')
	left join events ve ON ve.id = et.event_id AND ve.kind = 1754
	left join jsonb_array_elements(cast(ve.content as jsonb)) j on true
	where exists (select true from event_tags WHERE event_id = mainev.id AND event_tag_key = 'poll') and mainev.kind = :`)
	b.WriteValue(filterID, "kind", filter.Start.Kind)
	b.WriteString(`
	group by poll_id, option, mainev.pubkey, mainev.master_pubkey, mainev.kind, mainev.h_tag, mainev.d_tag, mainev.address
) t
left join jsonb_each_text(jsonb_build_object(cast(t.option AS text), t.votes)) AS json_each ON true
group by t.poll_id, t.pubkey, t.master_pubkey, t.kind, t.h_tag, t.d_tag, t.address
`)
}

func (b *queryBuilder) BuildDependency(filterID, cteName string, filter *databaseFilterSearch, current *filterDependency) {
	if len(current.Reduce.Kinds) > 0 && current.Reduce.Kinds[0] == model.KindDVMCountResponse {
		if len(current.Reduce.Kinds) > 1 && current.Reduce.Kinds[1] == model.CustomIONKindPollVote && current.Reduce.Group {
			b.CountVotesOf(filterID, cteName, current)

			return
		}
		b.WriteString(`
union all
select
	6400,
	CURRENT_TIMESTAMP,
	'' as address,
	case when f.kind = 3 then '' else f.reference_id end as id,
	coalesce(evr.pubkey, ''),
	coalesce(evr.master_pubkey, ''),
	'',
`)
		if current.Reduce.Group && len(current.Reduce.Kinds) > 1 && current.Reduce.Kinds[1] == nostr.KindReaction {
			b.WriteString(`text(jsonb_object_agg(coalesce(nullif(f.reference_type, ''), '+'), f.value)) as content,`)
		} else {
			b.WriteString(`cast(f.value as text) as content,`)
		}
		b.WriteString(`
	text(jsonb_build_array(
		jsonb_build_object(
				'kinds', jsonb_build_array(cast(:` + (filterID + "fkind") + ` as int)),
				case when :` + (filterID + "ftagname") + ` = 'lookup' then
					subzero_nostr_get_event_address_tag(evr.kind)
				else
					:` + (filterID + "ftagname") + ` end,
				jsonb_build_array(jsonb_build_array(
						case when :` + (filterID + "ftagname") + ` = 'lookup' then
							evr.address
						else
							f.reference_id
						end`)
		if current.Reduce.Context == "root" || current.Reduce.Context == "reply" {
			b.WriteString(`, null, text(:` + filterID + "context" + `)`)
		}
		b.WriteString(`))))) as d_tag,
	h_tag,
	case when
		f.kind = 7 then
			jsonb_build_array(
				jsonb_build_array('output', 'JSON'),
				jsonb_build_array('param', 'group', text(:` + (filterID + "context") + `)
			))
		else
			jsonb_build_array()
		end as tags
from
	event_counters f
inner join ` + cteName + ` evr on evr.kind = :` + (filterID + "kind") + `
	and (
		(f.kind = 3 and f.reference_id in (evr.master_pubkey, evr.pubkey))
		or
		f.reference_id = evr.address
	)
where
	exists (select 1 FROM ` + cteName + ` ) AND
`)
	} else {
		b.WriteString(` union select `)
		for i, f := range b.fieldsNames("e") {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(f)
		}
		b.WriteString(` from events e`)
		if len(current.Reduce.Kinds) > 0 && current.Reduce.Kinds[0] == nostr.KindProfileMetadata && current.Reduce.Author != "" {
			authors := strings.Split(current.Reduce.Author, ",")
			// Most relevant follwers.
			b.WriteString(`
INNER JOIN (
SELECT e.master_pubkey
FROM events e
WHERE e.kind = 3
AND EXISTS (
SELECT 1
FROM event_tags et
WHERE et.event_id = e.id
	AND et.event_tag_key = 'p'
	AND `)
			buildFromSlice(b, sqlOpCodeNONE, filterID, authors, "et.event_tag_value1", "mrf")
			b.WriteString(`)
AND EXISTS (
SELECT 1
FROM event_tags et
JOIN ` + cteName + ` em ON et.event_id = em.id
WHERE et.event_tag_key = 'p'
	AND et.event_tag_value1 = e.master_pubkey
)
AND `)
			buildFromSliceNegative(b, sqlOpCodeNONE, filterID, authors, "e.master_pubkey", "mrf")
			b.WriteString(` AND e.hidden = false) t ON e.master_pubkey = t.master_pubkey`)
		}
		b.WriteString(` where exists (select 1 FROM ` + cteName + ` ) AND e.id not in (select `)
		b.WriteString(cteName)
		b.WriteString(`.id from `)
		b.WriteString(cteName)
		b.WriteString(`) AND `)
	}

	switch current.Reduce.Kinds[0] {
	case nostr.KindTextNote, nostr.KindRepost, nostr.KindReaction, nostr.KindArticle, nostr.KindGenericRepost, model.CustomIONKindEditableTextNote:
		tag := current.Reduce.Tag // Could be "q" or "e" or "p" or empty.
		b.WriteString(" e.id in (select (select mctx.event_id from event_tags mctx inner join events et ON mctx.event_id = et.id ")
		if current.Reduce.Author != "" {
			b.WriteString(" and :")
			b.WriteValue(filterID, "rauthor", current.Reduce.Author)
			b.WriteString(" in (et.pubkey, et.master_pubkey)")
		}
		b.WriteString(" where mctx.event_tag_key ")
		switch tag {
		case "q":
			b.WriteString(" in ('q', 'Q')")
		case "", "e":
			b.WriteString(" in ('e', 'a')")
		default:
			b.WriteString(" = :")
			b.WriteValue(filterID, "rtag", tag)
		}
		b.WriteString(" and et.kind = :")
		b.WriteValue(filterID, "rkind", current.Reduce.Kinds[0])
		b.WriteString(" and mctx.event_tag_value1 = em.address")
		if current.Reduce.Context != "" {
			b.WriteString(" and mctx.event_tag_value3 = :")
			b.WriteValue(filterID, "rcontext", current.Reduce.Context)
			if current.Reduce.Context == "root" {
				b.WriteString(` and NOT EXISTS (select true from event_tags rctx where rctx.event_id = et.id AND rctx.event_tag_key = mctx.event_tag_key and rctx.event_tag_value3 = 'reply')`)
			}
		}
		b.WriteString(` LIMIT 1) FROM ` + cteName + ` em) AND e.hidden = FALSE`)

	case nostr.KindBadgeDefinition:
		startFilter := b.BuildQueryForDependencyStart(filterID, cteName, "id", &current.Start)
		b.WriteString("e.id in ((select event_tag_value1 from event_tags where event_id in (")
		b.WriteString(startFilter)
		b.WriteString(") and event_tag_key = 'e'),")
		b.WriteString(`(select ee.id from (select subzero_nostr_tag_a_get_pk(event_tag_value1) as pk, subzero_nostr_tag_a_get_dtag(event_tag_value1) as name from event_tags where event_id in (`)
		b.WriteString(startFilter)
		b.WriteString(") and event_tag_key = 'a') badge, events ee where badge.pk in (ee.pubkey, ee.master_pubkey) and ee.d_tag = badge.name and ee.kind = 30009 and hidden = false)) AND e.hidden=false")

	case nostr.KindMuteList, nostr.KindRelayListMetadata:
		reduceKindParam := b.PushValue(filterID, "rkind", current.Reduce.Kinds[0])
		b.WriteString("e.kind = :")
		b.WriteString(reduceKindParam)
		b.WriteString(" AND ( master_pubkey IN (")
		b.WriteString(b.BuildQueryForDependencyStart(filterID, cteName, "master_pubkey", &current.Start))
		b.WriteString(") OR pubkey IN (")
		b.WriteString(b.BuildQueryForDependencyStart(filterID, cteName, "pubkey", &current.Start))
		b.WriteString(")) AND e.hidden=false")
		b.WriteString(`
union all
select
	20002,
	CURRENT_TIMESTAMP,
	'' as id,
	'' as address,
	e.pubkey,
	e.master_pubkey,
	'' as sig,
	'' as content,
	'' as d_tag,
	'' as h_tag,
	'[]' as tags
from
	events e
inner join `)
		b.WriteString(cteName)
		b.WriteString(` on e.id = `)
		b.WriteString(cteName)
		b.WriteString(`.id where exists (select 1 FROM ` + cteName + ` ) AND e.kind =:`)
		b.WriteValue(filterID, "kind", current.Start.Kind)
		if current.Start.Tag != "" {
			b.WriteString(" AND EXISTS (select true from event_tags where event_id = ")
			b.WriteString(cteName)
			b.WriteString(".id AND event_tag_key = :")
			b.WriteValue(filterID, "tag", current.Start.Tag)
			b.WriteString(")")
		}
		b.WriteString(` AND
not exists (select true from events subev where subev.kind = :` + reduceKindParam + ` and
(
	(subev.pubkey = e.pubkey               and subev.hidden = false) or
	(subev.master_pubkey = e.master_pubkey and subev.hidden = false) or
	(subev.master_pubkey = e.pubkey        and subev.hidden = false) or
	(subev.pubkey = e.master_pubkey        and subev.hidden = false)
)) and e.hidden=false
group by e.master_pubkey, e.pubkey`)

	case nostr.KindProfileMetadata:
		b.WriteString("e.kind = :")
		b.WriteValue(filterID, "rkind", current.Reduce.Kinds[0])
		b.ApplyTextSearch(filter)
		if current.Reduce.Author == "" {
			b.WriteString(" AND ( master_pubkey IN (")
			b.WriteString(b.BuildQueryForDependencyStart(filterID, cteName, "master_pubkey", &current.Start))
			b.WriteString(") OR pubkey IN (")
			b.WriteString(b.BuildQueryForDependencyStart(filterID, cteName, "pubkey", &current.Start))
			b.WriteString("))")
		}
		b.WriteString(" and e.hidden=false")

	case model.KindDVMCountResponse:
		b.WriteString("f.kind = :")
		b.WriteValue(filterID, "rkind", current.Reduce.Kinds[1])
		b.PushValue(filterID, "ftagname", "lookup")
		b.PushValue(filterID, "fkind", current.Reduce.Kinds[1])
		var refType string
		switch {
		case strings.EqualFold(current.Reduce.Tag, "q"):
			b.PushValue(filterID, "ftagname", "#q")
			b.PushValue(filterID, "context", current.Reduce.Tag)
			refType = "quote"

		case current.Reduce.Context == "content" || current.Reduce.Tag == "e":
			b.PushValue(filterID, "context", cmp.Or(current.Reduce.Context, current.Reduce.Tag))

		case current.Reduce.Context == "root" || current.Reduce.Context == "reply":
			b.PushValue(filterID, "context", "reply")
			refType = current.Reduce.Context

		case current.Reduce.Tag == "p":
			b.PushValue(filterID, "ftagname", "#p")
			b.PushValue(filterID, "context", current.Reduce.Tag)
			refType = "follower"
		}
		if refType != "" {
			b.WriteString(" AND f.reference_type = :")
			b.WriteString(b.PushValue(filterID, "rref", refType))
		}
		b.WriteString(" AND f.reference_id IN (")
		if current.Reduce.Kinds[1] == nostr.KindFollowList {
			b.WriteString(b.BuildQueryForDependencyStart(filterID, cteName, "pubkey", &current.Start))
			b.WriteString(" UNION ")
			b.WriteString(b.BuildQueryForDependencyStart(filterID, cteName, "master_pubkey", &current.Start))
		} else {
			b.WriteString(b.BuildQueryForDependencyStart(filterID, cteName, "address", &current.Start))
		}
		b.WriteString(")")
		if current.Reduce.Group && current.Reduce.Kinds[1] == nostr.KindReaction {
			b.WriteString(" GROUP BY reference_id, f.kind, evr.pubkey, evr.master_pubkey, evr.h_tag, evr.id, evr.kind, evr.master_pubkey, evr.d_tag, evr.address")
		}
	}
}

func (b *queryBuilder) ParseFilters(in ...model.Filter) (out []*databaseFilterSearch, err error) {
	if len(in) == 0 {
		return []*databaseFilterSearch{{
			ID:    "empty",
			Extra: []string{whereBuilderCommunityFilter, whereBuilderNoSoftDeleted},
		}}, nil
	}

	for i := range in {
		filter, err := parseNostrFilter(in[i])
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse filter %d", i)
		}
		filter.ID = "filter" + strconv.Itoa(i) + "_"

		out = append(out, filter)
	}

	return out, nil
}

func (b *queryBuilder) BuildSingleWhere(filters ...model.Filter) (whereClause string, params map[string]any, err error) {
	databaseFilters, err := b.ParseFilters(filters...)
	if err != nil {
		return "", nil, errors.Wrap(err, "failed to parse filters")
	}

	for i := range databaseFilters {
		if i > 0 {
			b.WriteString(" OR ")
		}
		b.WriteRune('(')
		if _, _, err = b.BuildWhere(databaseFilters[i]); err != nil {
			return "", nil, err
		}
		b.WriteRune(')')
	}

	return b.String(), b.Params, nil
}

func (b *queryBuilder) Build(filters ...model.Filter) (sql string, params map[string]any, err error) {
	var ctes []*databaseCTE

	databaseFilters, err := b.ParseFilters(filters...)
	if err != nil {
		return "", nil, errors.Wrap(err, "failed to parse filters")
	}

	definedOrder := false
	for _, filter := range databaseFilters {
		cte, err := b.BuildCTE(filter)
		if err != nil {
			return "", nil, errors.Wrap(err, "failed to build filter")
		}
		ctes = append(ctes, cte)
		definedOrder = definedOrder || cte.OrderBy != ""
	}

	b.Reset()
	b.WriteString("WITH ")
	for i := range ctes {
		if i > 0 {
			b.WriteString(",\n")
		}
		b.WriteString(ctes[i].Name)
		b.WriteString(" AS ")
		b.WriteString(ctes[i].Body)
	}

	b.WriteString(" (")
	for i := range ctes {
		if i > 0 {
			b.WriteString(" UNION \n")
		}
		b.WriteString(` (SELECT `)
		for x, f := range b.fieldsNames(ctes[i].Name) {
			if x > 0 {
				b.WriteString(", ")
			}
			b.WriteString(f)
		}
		b.WriteString(` FROM `)
		b.WriteString(ctes[i].Name)
		if ctes[i].OrderBy != "" {
			b.WriteString(" ORDER BY ")
			b.WriteString(ctes[i].OrderBy)
		}
		b.WriteString(` ) `)
		for j := range ctes[i].Filter.Dependencies {
			b.BuildDependency(
				ctes[i].Name+"_dep"+strconv.Itoa(j),
				ctes[i].Name,
				ctes[i].Filter,
				ctes[i].Filter.Dependencies[j],
			)
		}
	}
	b.WriteString(" )")
	if !definedOrder {
		b.WriteString(" ORDER BY ")
		b.WriteString(whereBuilderDefaultOrderBy)
	}

	return b.String(), b.Params, nil
}

func (b *queryBuilder) fieldsNames(table string) []string {
	fields := []string{"kind", "created_at", "id", "address", "pubkey", "master_pubkey", "sig", "content", "d_tag", "h_tag", "tags"}
	if table == "" {
		return fields
	}

	for i := range fields {
		fields[i] = table + "." + fields[i]
	}

	return fields
}

func (b *queryBuilder) BuildCTE(filter *databaseFilterSearch) (cte *databaseCTE, err error) {
	whereBuffer := queryBuilder{Params: b.Params}
	where, _, err := whereBuffer.BuildWhere(filter)
	if err != nil {
		return nil, err
	}

	if filter.Limit == 0 {
		filter.Limit = whereBuilderDefaultLimit
	}

	var orderBy string
	const discoverContentCreatorsToFollow = "discover content creators to follow"
	if strings.Contains(filter.Filter.Search, discoverContentCreatorsToFollow) {
		orderBy = "random()"
	}

	fields := b.fieldsNames("e")
	var joinString string
	switch filter.Rank {
	case rankTOP:
		// All time top.
		joinString = ` inner join ranked_events r on e.id = r.event_id`
		orderBy = `score desc`
		fields = append(fields, "r.score")

	case rankTrending:
		// 24h trending.
		joinString = ` inner join ranked_events r on e.id = r.event_id and ((CURRENT_TIMESTAMP - least(CURRENT_TIMESTAMP, e.created_at)) < interval '24 hours')`
		orderBy = `score desc`
		fields = append(fields, "r.score")
	}

	var sb strings.Builder
	sb.WriteString(`( select `)
	for i, f := range fields {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(f)
	}

	sb.WriteString(` from events e `)
	if joinString != "" {
		sb.WriteString(joinString)
	}

	sb.WriteString(` where `)
	sb.WriteString(where)

	sb.WriteString(" order by ")
	sb.WriteString(cmp.Or(orderBy, whereBuilderDefaultOrderBy))

	if filter.Limit > 0 {
		sb.WriteString(` limit :`)
		sb.WriteString(b.PushValue(filter.ID, "limit", filter.Limit))
	}
	sb.WriteString(`)`)

	return &databaseCTE{
		Name:    filter.ID + "events_cte",
		Body:    sb.String(),
		OrderBy: orderBy,
		Filter:  filter,
	}, nil
}

func (b *queryBuilder) BuildWhere(filter *databaseFilterSearch) (sql string, params map[string]any, err error) {
	if err := b.ApplyFilter(filter); err != nil {
		return "", nil, errors.Wrapf(err, "failed to apply filter %s", filter.ID)
	}
	for i := range filter.Extra {
		b.MaybeAND()
		b.WriteString(filter.Extra[i])
	}
	b.MaybeAND()
	b.WriteString(whereBuilderDefaultWhere)

	return b.String(), b.Params, nil
}

func (b *queryBuilder) ApplyDeleteFilter(idx int, filter *databaseFilterDelete) {
	filterID := "deletefilter" + strconv.Itoa(idx) + "_"

	// Filter expression consists of two parts: (event filter) AND (access filter):
	// - Event filter (ORed):
	//   - By ID.
	//   - By kind and author and D tag.
	// - Account filter (ORed):
	//   - By author.
	//   - By master pubkey.
	//   - By onbehalf attestations.
	b.WriteRune('(')
	if len(filter.IDs) > 0 || len(filter.Events) > 0 {
		b.WriteRune('(')
		buildFromSlice(b, sqlOpCodeNONE, filterID, filter.IDs, "id", "")
		for i := range filter.Events {
			idxStr := strconv.Itoa(i)
			b.MaybeOR()
			b.WriteString("(kind = :")
			b.WriteValue(filterID, "kind"+idxStr, filter.Events[i].Kind)
			b.WriteString(" AND pubkey = :")
			b.WriteValue(filterID, "author"+idxStr, filter.Events[i].Pubkey)
			b.WriteString(" AND d_tag = :")
			b.WriteValue(filterID, "dtag"+idxStr, filter.Events[i].Dtag)
			b.WriteRune(')')
		}
		b.WriteString(") AND ")
	}

	owner := b.PushValue(filterID, "pubkey", filter.Author)
	b.WriteString("((pubkey = :")
	b.WriteString(owner)
	b.WriteString(" AND hidden=false) OR (master_pubkey = :")
	b.WriteString(owner)
	b.WriteString(" AND hidden=false) OR ((master_pubkey = :")
	b.WriteString(owner)
	b.WriteString(" AND pubkey != master_pubkey AND ")
	b.WriteString("subzero_nostr_onbehalf_is_allowed(jsonb(coalesce((select p.tags from events p where p.master_pubkey = master_pubkey and p.kind = 10100 and hidden=false limit 1), '[]')), :")
	b.WriteString(owner)
	b.WriteString(", kind)))))")
}

func (b *queryBuilder) BuildForDelete(filters ...databaseFilterDelete) (sql string, params map[string]any, err error) {
	for idx := range filters {
		b.MaybeOR()
		b.ApplyDeleteFilter(idx, &filters[idx])
	}

	if b.Len() == 0 {
		return "", nil, ErrEmptyFilter
	}

	b.WriteString(" AND ")
	b.WriteString(whereBuilderDefaultWhere)

	return b.String(), b.Params, nil
}

func collectValuesFromTagMap(values []model.TagValues) (data []string) {
	for _, val := range values {
		for _, v := range val {
			if v != nil {
				data = append(data, *v)
			}
		}
	}
	return data
}

func isValidPrecalculatedCounterFilter(filter *model.Filter) (references []string, valid bool) {
	if len(filter.IDs) > 0 || len(filter.Authors) > 0 || filter.Since != nil || filter.Until != nil || filter.Search != "" {
		return nil, false
	}

	// Single tag only.
	if len(filter.Tags) != 1 {
		return nil, false
	}

	var supportedTags = []string{"a", "q", "Q", "e", "p", "h"}
	for _, tag := range supportedTags {
		values, ok := filter.Tags[tag]
		if !ok {
			continue
		}

		references = collectValuesFromTagMap(values)

		break

	}
	if len(references) == 0 {
		return nil, false
	}

	var supportedKinds = map[int]struct{}{
		nostr.KindReaction:                  {},
		nostr.KindFollowList:                {},
		nostr.KindTextNote:                  {},
		nostr.KindRepost:                    {},
		nostr.KindArticle:                   {},
		nostr.KindGenericRepost:             {},
		model.CustomIONKindEditableTextNote: {},
		model.CustomIONKindCommunityJoin:    {},
	}
	for _, kind := range filter.Kinds {
		if _, ok := supportedKinds[kind]; !ok {
			return nil, false
		}
	}

	return references, true
}

func getReplyTypeFromValues(values []model.TagValues) string {
	for j := range values {
		if len(values[j]) > 2 && values[j][2] != nil {
			return *values[j][2]
		}
	}
	return ""
}

func tagsHasQuote(m model.TagMap) bool {
	for _, tag := range []string{"q", "Q"} {
		if _, ok := m[tag]; ok {
			return true
		}
	}
	return false
}

func (b *queryBuilder) BuildForPrecalculatedCounters(filters ...model.Filter) (sql string, params map[string]any, err error) {
	if len(filters) == 0 {
		return "", nil, ErrEmptyFilter
	}

	for idx := range filters {
		refs, valid := isValidPrecalculatedCounterFilter(&filters[idx])
		if !valid {
			return "", nil, errors.Wrapf(errUnsupportedCombination, "filter %d", idx)
		}

		filterID := "eventcounter" + strconv.Itoa(idx) + "_"
		filter := &filters[idx]

		b.MaybeOR()

		kinds := model.DeduplicateSlice(filter.Kinds, func(k int) int { return k })
		startLen := b.Len()
		b.WriteRune('(')
		if len(filter.Kinds) > 0 {
			b.WriteRune('(')
			for idx := range kinds {
				b.MaybeOR()
				b.WriteString("kind = :")
				b.WriteValue(filterID, "kind"+strconv.Itoa(idx), kinds[idx])
				var referenceType string
				switch kinds[idx] {
				case nostr.KindReaction:
					// Nothing to add.

				case nostr.KindFollowList:
					referenceType = "follower"

				case nostr.KindTextNote, nostr.KindRepost, nostr.KindArticle, nostr.KindGenericRepost, model.CustomIONKindEditableTextNote:
					if tagsHasQuote(filter.Tags) {
						referenceType = "quote"
					} else if _, ref := filter.Tags["e"]; ref {
						referenceType = getReplyTypeFromValues(filter.Tags["e"])
					} else if _, ref := filter.Tags["a"]; ref {
						referenceType = getReplyTypeFromValues(filter.Tags["a"])
					}
				case model.CustomIONKindCommunityJoin:
					referenceType = "members"
				}
				if referenceType != "" {
					b.WriteString(" AND reference_type = :")
					b.WriteValue(filterID, "reference_type"+strconv.Itoa(idx), referenceType)
				}
			}
			b.WriteRune(')')
		}
		buildFromSlice(b, sqlOpCodeAND, filterID, refs, "reference_id", "")
		if b.Len() == startLen+1 {
			return "", nil, errUnsupportedCombination
		}
		b.WriteRune(')')
	}

	return b.String(), b.Params, nil
}
