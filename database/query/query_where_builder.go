// SPDX-License-Identifier: ice License 1.0

package query

import (
	"cmp"
	"context"
	"encoding/base64"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"
	"github.com/rs/zerolog/log"

	"github.com/ice-blockchain/subzero/model"
)

const (
	whereBuilderDefaultWhere    = "e.hidden=false"
	whereBuilderCommunityFilter = "(case when e.kind in (1, 30023, 30175) then e.id = e.h_tag else true end)"
	whereBuilderNoSoftDeleted   = "e.deleted=false"
	whereBuilderDefaultOrderBy  = "lookup_created_at DESC"

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

const (
	writeFieldFlagMaskKindEphemeralEmbedding uint = 1 << iota // If set, kind field will have the high bit masked to indicate ephemeral embedding.
)

var (
	ErrWhereBuilderInvalidTimeRange = errors.New("invalid time range")
	ErrEmptyFilter                  = errors.New("empty filter")

	errUnsupportedCombination = errors.New("unsupported filter combination")
)

var (
	maskKindEphemeralEmbedding = "|0x" + strconv.FormatInt(ephemeralEmbeddingBit, 16) + " as kind"
)

type (
	rank         int
	queryBuilder struct {
		Params map[string]any
		strings.Builder
	}
	queryBuildResult struct {
		Params    map[string]any
		Filters   map[string]*databaseFilterTree // Origin/cte name -> filter.
		Statement string
	}
	queryBuilderValue struct {
		Value  any
		Name   string
		CastTo string // If empty, no cast is applied.
		Func   string // If non-empty, the value is passed to the function.
	}
	databaseFilterTree struct {
		Root  *databaseFilterSearch
		Leafs map[string]*filterDependency // Origin/dependency name -> dependency.
	}
	databaseFilterSearch struct {
		Expiration        *bool
		Videos            *bool
		Images            *bool
		Media             *bool
		Quotes            *bool
		References        *bool
		CurrentUserPubkey *string
		ID                string
		SearchText        string
		model.Filter
		TagMarkers   []databaseFilterMarker
		Dependencies []*filterDependency
		Extra        []string // Extra `where` clauses, ANDed to the main filter.
		Rank         rank
	}
	databaseFilterDelete struct {
		Author        string
		IDs           []string
		Addresses     []string
		AccountDelete bool
	}
	databaseFilterMarker struct {
		Tag     string
		Marker  string
		Exclude bool
	}
	databaseCTE struct {
		Filter  *databaseFilterSearch
		Name    string
		Body    string
		OrderBy string
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
			if v := tag.Value(); v != "" {
				filter.Addresses = append(filter.Addresses, v)
			}
		}
	}

	// Account deletion request contains no IDs nor addresses, and must be signed by the master key only.
	filter.AccountDelete =
		len(filter.IDs) == 0 &&
			len(filter.Addresses) == 0 &&
			filter.Author == e.PubKey

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

func (b *queryBuilder) WriteFields(fields ...string) {
	for i, field := range fields {
		if i > 0 {
			b.WriteRune(',')
		}
		b.WriteString(field)
	}
}

func (b *queryBuilder) WriteValues(filterID string, values []queryBuilderValue) {
	if len(values) == 0 {
		return
	}

	b.WriteRune('(')
	for i, v := range values {
		if i > 0 {
			b.WriteString(", ")
		}
		if v.Func != "" {
			b.WriteString(v.Func)
			b.WriteRune('(')
		}
		if v.CastTo != "" {
			b.WriteTypedValue(filterID, v.Name, v.CastTo, v.Value)
		} else {
			b.WriteRune(':')
			b.WriteValue(filterID, v.Name, v.Value)
		}
		if v.Func != "" {
			b.WriteRune(')')
		}
	}
	b.WriteRune(')')
}

func (b *queryBuilder) WriteValue(filterID, name string, value any) {
	b.WriteString(b.PushValue(filterID, name, value))
}

func (b *queryBuilder) WriteTypedValue(filterID, name, castTo string, value any) {
	b.WriteString(`cast(:`)
	b.WriteValue(filterID, name, value)
	b.WriteString(` as `)
	b.WriteString(castTo)
	b.WriteRune(')')
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
	ParamName string
	Slice     []T
	Op        int
	Negative  bool
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
	if b.Negative {
		builder.WriteString(" != ")
	} else {
		builder.WriteString(" = ")
	}

	s := model.DeduplicateSlice(b.Slice, func(elem T) T { return elem })
	if len(s) == 1 {
		// X = :X_name0.
		builder.WriteRune(':')
		builder.WriteString(builder.PushValue(filterID, b.ParamName, s[0]))
	} else {
		// X = ANY/ALL(...).
		if b.Negative {
			builder.WriteString("ALL(:")
		} else {
			builder.WriteString("ANY(:")
		}
		builder.WriteString(builder.PushValue(filterID, b.ParamName, s))
		builder.WriteRune(')')
	}

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

		switch {
		// Special case for [!]<e, a>marker:reply, like `!amarker:reply`.
		case (marker.Tag == "a" || marker.Tag == "e") && marker.Marker == model.TagMarkerReply:
			b.WriteString(`e.is_reply = :`)
			b.WriteValue(filterID, "mtagvalue"+strconv.Itoa(id), !marker.Exclude)
		default:
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
}

func (b *queryBuilder) ApplyFilterTags(filterID string, tags model.TagMap) {
	if len(tags) == 0 {
		return
	}

	if v, ok := tags["d"]; ok && len(v) == 1 && len(tags) == 1 && len(v[0]) == 1 && v[0][0] != nil {
		// Special case for "d" tag.
		b.MaybeAND()
		b.WriteString("e.d_tag = :")
		b.WriteValue(filterID, "dtag", *v[0][0])

		return
	}

	var tagID, tagValue uint64
main:
	for tagName, tagValues := range tags {
		tagID++

		switch tagName {
		case "t", "!t":
			// Handled in ApplyFilterTtags.
			continue main
		}

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
		beforeTagLoopBufLen := b.Len()
	valuesLoop:
		for _, values := range tagValues {
			if values.Empty() {
				continue
			}

			switch tagName {
			case "l": // Either language filter OR color filter.
				if len(values) == 2 && values[0] != nil && values[1] != nil && strings.EqualFold(*values[1], model.LangISO) {
					// Language filter, handled by ApplyFilterLang.
					continue valuesLoop
				}
			}

			if len(values) > maxTagValues {
				filter := func() string {
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
				}()
				log.Trace().
					Str("filter_id", filterID).
					Str("filter", filter).
					Str("tag_name", tagName).
					Int("max_tag_values", maxTagValues).
					Msg("filter: too many values for tag, only the first will be used")
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
		if b.Len() == beforeTagLoopBufLen {
			// No valid values were found for this tag, add dummy condition.
			b.WriteString("1=1")
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
		len(filter.Addresses) == 0 &&
		filter.Since == nil &&
		filter.Until == nil &&
		filter.Expiration == nil &&
		filter.Videos == nil &&
		filter.Quotes == nil &&
		filter.References == nil &&
		filter.Images == nil &&
		filter.Media == nil &&
		filter.SearchText == ""
}

func (b *queryBuilder) ApplyTimeRange(filterID string, since, until *model.Timestamp) error {
	if since != nil && until != nil {
		if *since == *until {
			b.MaybeAND()
			b.WriteString("e.lookup_created_at = :")
			b.WriteValue(filterID, "timestamp", since.Time().UnixNano())

			return nil
		} else if since.After(*until) {
			return errors.Wrapf(ErrWhereBuilderInvalidTimeRange, "since [%s] is greater than until [%s]",
				since.Time().Format(time.RFC3339Nano),
				until.Time().Format(time.RFC3339Nano),
			)
		}
	}

	// If a filter includes the `since` property, events with `created_at` greater than or equal to since are considered to match the filter.
	if since != nil && *since > 0 {
		b.MaybeAND()
		b.WriteString("e.lookup_created_at >= :")
		b.WriteValue(filterID, "since", since.Time().UnixNano())
	}

	// The `until` property is similar except that `created_at` must be less than or equal to `until`.
	if until != nil && *until > 0 {
		b.MaybeAND()
		b.WriteString("e.lookup_created_at <= :")
		b.WriteValue(filterID, "until", until.Time().UnixNano())
	}

	return nil
}

func (b *queryBuilder) ApplyFilterLang(filter *databaseFilterSearch) {
	values, ok := filter.Tags["l"]
	if !ok {
		return
	}

	var langs []string
	for _, v := range values {
		// Expecting two values: [language, "ISO-639-1"].
		if len(v) != 2 || v[0] == nil || v[1] == nil || !strings.EqualFold(*v[1], model.LangISO) {
			continue
		}
		langs = append(langs, *v[0])
	}

	if len(langs) == 0 {
		return
	}

	buildFromSlice(b, sqlOpCodeAND, filter.ID, langs, "e.lang", "langs")
}

func (b *queryBuilder) applyFilterTtags(filter *databaseFilterSearch, exclude bool, values []string) {
	name := "ttags"

	if len(values) == 0 {
		return
	}

	b.MaybeAND()
	if exclude {
		name += "_exclude"
		b.WriteString("(NOT ")
	}

	b.WriteString(`(e.t_tags && `)
	b.WriteTypedValue(filter.ID, name, "text[]", values)
	b.WriteRune(')')

	if exclude {
		b.WriteRune(')')
	}
}

func (b *queryBuilder) ApplyFilterTtags(filter *databaseFilterSearch) {
	if values := filter.Tags.All("!t"); len(values) > 0 {
		b.applyFilterTtags(filter, true, values)
	}

	if values := filter.Tags.All("t"); len(values) > 0 {
		b.applyFilterTtags(filter, false, values)
	}
}

func (b *queryBuilder) ApplyFilterForExtensions(filter *databaseFilterSearch) {
	if filter.Media != nil {
		b.MaybeAND()
		b.WriteString("(e.has_videos=:")
		b.WriteValue(filter.ID, "media", *filter.Media)
		if *filter.Media {
			// Check for either videos or images.
			b.WriteString(" OR ")
		} else {
			// Return only events without media.
			b.WriteString(" AND ")
		}
		b.WriteString("e.has_images=:")
		b.WriteValue(filter.ID, "media", *filter.Media)
		b.WriteString(")")
	} else {
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
	}

	if filter.Quotes != nil {
		b.MaybeAND()
		b.WriteString("e.is_quote=:")
		b.WriteValue(filter.ID, "quote", *filter.Quotes)
	}

	if filter.Expiration != nil {
		b.MaybeAND()
		if *filter.Expiration {
			now := time.Now().UnixNano()
			b.WriteString(`(e.expiration > `)
			b.WriteTypedValue(filter.ID, "expiration_after", "bigint", now)
			b.WriteRune(')')
		} else {
			b.WriteString(`(e.expiration is null)`)
		}
	}

	if filter.References != nil {
		b.MaybeAND()
		b.WriteString(`(case when e.reference_id is not null then true else e.has_references=:`)
		b.WriteValue(filter.ID, "references", *filter.References)
		b.WriteString(` end)`)
	}
}

func (b *queryBuilder) ApplyFilterGiftWrap(filter *databaseFilterSearch) {
	b.MaybeAND()
	b.WriteString(`(e.gift_receiver_pubkey is null`)
	if filter.CurrentUserPubkey != nil {
		b.WriteString(` OR e.gift_receiver_pubkey = :`)
		b.WriteValue(filter.ID, "gift_receiver_pubkey", *filter.CurrentUserPubkey)
	}
	b.WriteRune(')')
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
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) || unicode.IsMark(r) {
			return r
		}

		return -1
	}, input)
}

func (b *queryBuilder) ApplyTextSearch(filter *databaseFilterSearch) {
	if filter.SearchText == "" {
		return
	}

	text := replaceSpecialChars(filter.SearchText)

	b.MaybeAND()
	b.WriteString("(e.lookup ILIKE :")
	b.WriteValue(filter.ID, "fts", "%"+text+"%")
	b.WriteString(")")
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

func (b *queryBuilder) ApplySpecialKinds(filter *databaseFilterSearch) (kinds []int) {
	var repostKinds []string

	kinds = make([]int, 0, len(filter.Kinds))
	for _, kind := range filter.Kinds {
		switch kind {
		case nostr.KindGenericRepost:
			// Request filter contains generic repost already, just use it as is.
			return filter.Kinds

		case model.CustomIONKindRepostOfEditableTextNote:
			repostKinds = append(repostKinds, strconv.Itoa(model.CustomIONKindEditableTextNote))

		case model.CustomIONKindRepostOfArticle:
			repostKinds = append(repostKinds, strconv.Itoa(nostr.KindArticle))

		case model.CustomIONKindRepostOfTokenizedCommunityDefination:
			repostKinds = append(repostKinds, strconv.Itoa(model.CustomIONKindTokenizedCommunityDefinition))

		case model.CustomIONKindRepostOfTokenizedCommunityAction:
			repostKinds = append(repostKinds, strconv.Itoa(model.CustomIONKindTokenizedCommunityAction))

		default:
			kinds = append(kinds, kind)
		}
	}

	if len(repostKinds) > 0 {
		kinds = append(kinds, nostr.KindGenericRepost)
		b.MaybeAND()
		b.WriteString(`(case when e.kind = 16
		then
			exists (select true from event_tags rk where event_id = e.id AND rk.event_tag_key = 'k' and `)
		buildFromSlice(b, sqlOpCodeNONE, filter.ID, repostKinds, "rk.event_tag_value1", "repost_kind")
		b.WriteString(`) else true end)`)
	}

	return kinds
}

func (b *queryBuilder) ApplyKinds(filter *databaseFilterSearch, kinds []int) {
	if len(kinds) == 0 {
		return
	}

	var negative, positive []int
	for _, kind := range kinds {
		if kind < 0 {
			negative = append(negative, -kind)
		} else {
			positive = append(positive, kind)
		}
	}

	buildFromSlice(b, sqlOpCodeAND, filter.ID, positive, "e.kind", "kind_positive")
	if len(positive) == 0 {
		// Include `negative` kinds only if there are no positive ones, because it does not make sense to
		// have both positive and negative kinds in the same filter.
		buildFromSliceNegative(b, sqlOpCodeAND, filter.ID, negative, "e.kind", "kind_negative")
	}
}

func (b *queryBuilder) ApplyFilter(filter *databaseFilterSearch) error {
	if isFilterEmpty(filter) {
		return nil
	}

	b.WriteRune('(') // Begin the filter section.
	buildFromSlice(b, sqlOpCodeNONE, filter.ID, filter.IDs, "e.id", "")
	buildFromSlice(b, sqlOpCodeAND, filter.ID, filter.Addresses, "e.address", "")
	b.ApplyKinds(filter, b.ApplySpecialKinds(filter))
	b.ApplyFilterForExtensions(filter)
	b.ApplyFilterTtags(filter)
	b.ApplyFilterLang(filter)
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
	if filter.Kind != model.KindAny {
		sb.WriteString(cteName)
		sb.WriteString(".kind = :")
		sb.WriteString(b.PushValue(filterID, "kind", filter.Kind))
	} else {
		sb.WriteString("1=1") // No kind filter, always true.
	}
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
union all
select
	6400,
	cast (EXTRACT(EPOCH FROM CURRENT_TIMESTAMP) as bigint) as created_at,
	to_timestamp_nano(cast (EXTRACT(EPOCH FROM CURRENT_TIMESTAMP) as bigint)) as lookup_created_at,
	'' as id,
	'' as address,
	mainev.pubkey,
	mainev.master_pubkey,
	'' as sig,
	CAST(
		COALESCE(
			jsonb_object_agg(
				ec.reference_type,
				ec.value
			) FILTER (WHERE ec.kind IS NOT NULL),
			CAST('{}' AS jsonb)
		)
	AS text) as content,
	cast(jsonb_build_array(jsonb_build_object(
		'kinds', jsonb_build_array(1754),
		subzero_nostr_get_event_address_tag(mainev.kind), jsonb_build_array(mainev.address))
	) as text) as d_tag,
	mainev.h_tag,
	jsonb_build_array(
		jsonb_build_array('output', 'JSON'),
		jsonb_build_array('param', 'group', 'content')
	) as tags,
	:`)
	b.WriteValue(filterID, "poll_origin", filterID)
	b.WriteString(` as origin
from `)
	b.WriteString(cteName)
	b.WriteString(` mainev
	left join event_counters ec
		ON ec.reference_id = mainev.address AND ec.kind = 1754
	where
		exists (select true from event_tags WHERE event_id = mainev.id AND event_tag_key = 'poll')
		and mainev.kind = :`)
	b.WriteValue(filterID, "kind", filter.Start.Kind)
	b.WriteString(`
	group by mainev.id, mainev.pubkey, mainev.master_pubkey, mainev.kind, mainev.h_tag, mainev.d_tag, mainev.address
`)
}

func (b *queryBuilder) CountStoriesOf(filterID, cteName string, filter *databaseFilterSearch, current *filterDependency) {
	b.WriteString(`
union all
select
	6400 as kind,
	cast (EXTRACT(EPOCH FROM CURRENT_TIMESTAMP) as bigint) as created_at,
	to_timestamp_nano(cast (EXTRACT(EPOCH FROM CURRENT_TIMESTAMP) as bigint)) as lookup_created_at,
	'' as id,
	'' as address,
	t.pubkey,
	t.master_pubkey,
	'' as sig,
	cast(coalesce(t.c, 0) as text) as content,
	cast(jsonb_build_array(jsonb_build_object(
		'search', 'expiration::true',
		'kinds', jsonb_build_array(cast(:`)
	b.WriteValue(filterID, "reduce_kind", current.Reduce.Kinds[1])
	b.WriteString(` as int)),
		'authors', jsonb_build_array(t.master_pubkey))
	) as text) as d_tag,
	'' as h_tag,
	jsonb_build_array(
		jsonb_build_array('output', 'JSON')
	) as tags,
	:`)
	b.WriteValue(filterID, "story_origin", filterID)
	b.WriteString(` as origin
from (
	select
		(select count(id) from events cev where
			cev.kind = :`)
	b.WriteValue(filterID, "reduce_kind", current.Reduce.Kinds[1])
	b.WriteString(`
			and cev.expiration is not null
			and cev.expiration > :`)
	b.WriteValue(filterID, "story_now", time.Now().UnixNano())
	b.WriteString(`
			and ((cev.pubkey = ev_source.pubkey and cev.hidden=false) or (cev.master_pubkey = ev_source.master_pubkey and cev.hidden=false))
			and cev.hidden=false
		) as c,
		ev_source.pubkey,
		ev_source.master_pubkey
	from `)
	b.WriteString(cteName)
	b.WriteString(` ev_source where kind = :`)
	b.WriteValue(filterID, "start_kind", current.Start.Kind)
	b.WriteString(` ) t`)
}

func (b *queryBuilder) BuildForMostRelevantFollowers(filterID, cteName string, filter *databaseFilterSearch, current *filterDependency) {
	limit := filter.Limit
	if limit <= 0 || limit > whereBuilderDefaultLimit {
		limit = whereBuilderDefaultLimit
	}
	b.WriteString(`
	union all
	select
		coalesce(e.kind, 0) as kind,
		coalesce(e.created_at, cast(EXTRACT(EPOCH FROM CURRENT_TIMESTAMP) as bigint)) as created_at,
		coalesce(e.lookup_created_at, 0) as lookup_created_at,
		coalesce(e.id, '') as id,
		coalesce(e.address, '') as address,
		coalesce(e.pubkey, t.master_pubkey) as pubkey,
		coalesce(e.master_pubkey, t.master_pubkey) as master_pubkey,
		coalesce(e.sig, 'ENRICH') as sig,
		coalesce(e.content, '') as content,
		coalesce(e.d_tag, '') as d_tag,
		coalesce(e.h_tag, '') as h_tag,
		coalesce(e.tags, jsonb_build_array()) as tags,
		:`)
	b.WriteValue(filterID, "profile_origin", filterID)
	b.WriteString(` as origin`)
	authors := strings.Split(current.Reduce.Author, ",")
	b.WriteString(` FROM
	(
		SELECT DISTINCT e_inner.master_pubkey
		FROM events e_inner
		WHERE e_inner.kind = 3
			AND e_inner.hidden = FALSE
			AND EXISTS (
				SELECT 1
				FROM event_tags et
				WHERE et.event_id = e_inner.id
					AND et.event_tag_key = 'p'
					AND `)
	buildFromSlice(b, sqlOpCodeNONE, filterID, authors, "et.event_tag_value1", "mrf")
	b.WriteString(`
			)
			AND EXISTS (
				SELECT 1
				FROM event_tags et
				INNER JOIN filter0_events_cte em ON et.event_id = em.id
				WHERE et.event_tag_key = 'p'
					AND et.event_tag_value1 = e_inner.master_pubkey
			)
			AND `)
	buildFromSliceNegative(b, sqlOpCodeNONE, filterID, authors, "e_inner.master_pubkey", "mrf")
	b.WriteString(` limit :`)
	b.WriteValue(filterID, "mrf_limit", limit)
	b.WriteString(`) AS t LEFT JOIN events e ON t.master_pubkey = e.master_pubkey AND e.kind = 0 AND e.hidden = FALSE`)
}

func (b *queryBuilder) BuildForTCDataFromPost(filterID, cteName string, filter *databaseFilterSearch, current *filterDependency) {
	b.WriteString(` union all select `)
	b.WriteFields(b.fieldsNames("tc", "", writeFieldFlagMaskKindEphemeralEmbedding)...)
	startKind := b.PushValue(filterID, "startKind", current.Start.Kind)
	b.WriteString(` from (
	with tc_definitions as (
		select e.*
		from events e
		where
			e.kind = 31175
			and e.hidden = false
			and exists (
				select 1
				from event_tags et
				inner join ` + cteName + ` r on et.event_tag_value1 = r.address 
				where
					et.event_id = e.id
					and et.event_tag_key in ('e', 'a')
					and r.kind = :` + startKind + `
			)
			and not (e.t_tags && cast(array['community_token_action'] as text[]))
	),
	tc_definitions_first_buy as (
		select e.*
		from events e
		inner join event_tags et on e.id = et.event_id and et.event_tag_key = 'p'
		inner join tc_definitions td ON td.master_pubkey = et.event_tag_value1
		where
			e.hidden = false
			and e.kind = 31175
			and e.t_tags && cast(array['community_token_action'] as text[])
	),
	tc_action_first_buy as (
		select e.*
		from events e
		inner join event_tags et on e.id = et.event_id and et.event_tag_key in ('e', 'a')
		inner join tc_definitions td ON td.address = et.event_tag_value1
		where
			e.hidden = false
			and e.kind = 1175
			and e.first_1175_address is null
	)
	select `)
	b.WriteFields(b.fieldsNames("tc_definitions", filterID+"tc_def")...)
	b.WriteString(` from tc_definitions union all select `)
	b.WriteFields(b.fieldsNames("tc_definitions_first_buy", filterID+"first_31175")...)
	b.WriteString(` from tc_definitions_first_buy union all select `)
	b.WriteFields(b.fieldsNames("tc_action_first_buy", filterID+"first_1175")...)
	b.WriteString(` from tc_action_first_buy ) tc`)
}

func (b *queryBuilder) BuildDependency(filterID, cteName string, filter *databaseFilterSearch, current *filterDependency) {
	if len(current.Reduce.Kinds) > 0 && current.Reduce.Kinds[0] == model.KindDVMCountResponse {
		if len(current.Reduce.Kinds) > 1 {
			if current.Reduce.Kinds[1] == model.CustomIONKindPollVote && current.Reduce.Group {
				b.CountVotesOf(filterID, cteName, current)
				return
			}
			if len(current.Reduce.Kinds) == 2 && current.Reduce.Expiration {
				b.CountStoriesOf(filterID, cteName, filter, current)
				return
			}
		}
		b.WriteString(`
union all
select
	6400 as kind,
	cast (EXTRACT(EPOCH FROM CURRENT_TIMESTAMP) as bigint) as created_at,
	to_timestamp_nano(cast (EXTRACT(EPOCH FROM CURRENT_TIMESTAMP) as bigint)) as lookup_created_at,
	'' as address,
	case when counters.kind = 3 then '' else counters.reference_id end as id,
	coalesce(evr.pubkey, '') as pubkey,
	coalesce(evr.master_pubkey, '') as master_pubkey,
	'' as sig,
`)
		if current.Reduce.Group && len(current.Reduce.Kinds) > 1 && current.Reduce.Kinds[1] == nostr.KindReaction {
			b.WriteString(`text(jsonb_object_agg(coalesce(nullif(counters.reference_type, ''), '+'), counters.value)) as content,`)
		} else {
			b.WriteString(`cast(counters.value as text) as content,`)
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
							counters.reference_id
						end`)
		if current.Reduce.Context == "root" || current.Reduce.Context == "reply" {
			b.WriteString(`, null, text(:` + filterID + "context" + `)`)
		}
		b.WriteString(`))))) as d_tag,
	evr.h_tag,
	case when
		counters.kind = 7 then
			jsonb_build_array(
				jsonb_build_array('output', 'JSON'),
				jsonb_build_array('param', 'group', text(:` + (filterID + "context") + `)
			))
		else
			jsonb_build_array()
		end as tags,
		'' as origin
from
	` + cteName + ` evr
left join lateral (
	select ec.kind, ec.value, ec.reference_type, ec.reference_id
	from event_counters ec
	where evr.kind = :` + (filterID + "kind") + `
		and ec.kind = 3
		and ec.reference_id in (evr.master_pubkey, evr.pubkey)
	union all
	select ec.kind, ec.value, ec.reference_type, ec.reference_id
	from event_counters ec
	where evr.kind = :` + (filterID + "kind") + `
	and ec.reference_id = evr.address
) counters on true
where
	exists (select 1 FROM ` + cteName + ` ) AND
`)
	} else if len(current.Reduce.Kinds) > 0 && current.Reduce.Kinds[0] == nostr.KindProfileMetadata && current.Reduce.Author != "" {
		b.BuildForMostRelevantFollowers(filterID, cteName, filter, current)
		return
	} else if len(current.Reduce.Kinds) > 0 && current.Reduce.Kinds[0] == model.CustomIONKindTokenizedCommunityDefinition &&
		current.Start.KindIn(nostr.KindProfileMetadata, nostr.KindTextNote, nostr.KindArticle, model.CustomIONKindEditableTextNote) {
		// kind[0/30175/1/30023]>kind31175 with additional data.
		b.BuildForTCDataFromPost(filterID, cteName, filter, current)
		return
	} else {
		b.WriteString(` union all select `)
		for i, f := range b.fieldsNames("e", filterID) {
			if i > 0 {
				b.WriteString(", ")
			}
			if f == "e.kind" && len(current.Reduce.Kinds) > 0 {
				switch current.Reduce.Kinds[0] {
				case model.CustomIONKindTokenizedCommunityAction, model.CustomIONKindTokenizedCommunityDefinition:
					f += maskKindEphemeralEmbedding
				}
			}
			b.WriteString(f)
		}
		b.WriteString(` from events e`)
		b.WriteString(` where exists (select 1 FROM ` + cteName + ` ) AND e.id not in (select `)
		b.WriteString(cteName)
		b.WriteString(`.id from `)
		b.WriteString(cteName)
		b.WriteString(`) AND `)
	}

	switch current.Reduce.Kinds[0] {
	case nostr.KindTextNote,
		nostr.KindRepost,
		nostr.KindReaction,
		nostr.KindArticle,
		nostr.KindGenericRepost,
		model.CustomIONKindEditableTextNote,
		model.CustomIONKindPollVote:
		tag := current.Reduce.Tag // Could be "q" or "e" or "p" or empty.
		b.WriteString(" e.id in (select (select mctx.event_id from event_tags mctx inner join events et ON mctx.event_id = et.id and et.deleted = false ")
		if current.Reduce.Author != "" {
			b.WriteString(" and ((et.pubkey = :")
			b.WriteValue(filterID, "rauthor", current.Reduce.Author)
			b.WriteString(" and et.hidden = false) or (et.master_pubkey = :")
			b.WriteValue(filterID, "rauthor", current.Reduce.Author)
			b.WriteString(" and et.hidden = false))")
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
				b.WriteString(` and NOT EXISTS (select true from event_tags rctx where rctx.event_id = et.id AND et.deleted = false AND rctx.event_tag_key = mctx.event_tag_key and rctx.event_tag_value3 = 'reply')`)
			}
		}
		b.WriteString(` LIMIT 1) FROM ` + cteName + ` em) AND e.hidden = FALSE AND e.deleted = FALSE`)

	case model.CustomIONKindTokenizedCommunityDefinition:
		reduceKindParam := b.PushValue(filterID, "rkind", current.Reduce.Kinds[0])
		b.WriteString(`e.kind = :`)
		b.WriteString(reduceKindParam)
		b.WriteString(` AND e.hidden = false AND e.id IN (
		select et.event_id
		from event_tags et
		where et.event_tag_key IN ('e', 'a')
			AND et.event_tag_value1 IN (`)
		b.WriteString(b.BuildQueryForDependencyStart(filterID, cteName, "address", &current.Start))
		b.WriteString(`))`)

	// kind[0/30175/1/30023]>kind1175 - find first buy action for each post.
	case model.CustomIONKindTokenizedCommunityAction:
		switch current.Start.Kind {
		case nostr.KindProfileMetadata,
			nostr.KindTextNote,
			nostr.KindArticle,
			model.CustomIONKindEditableTextNote:
			b.WriteString(` e.id IN (
				SELECT DISTINCT ON (tc_def.id) act.id
				FROM (
					SELECT def.address AS def_address, def.id
					FROM events def
					INNER JOIN event_tags et ON def.id = et.event_id
					WHERE def.kind = 31175
						AND def.hidden = false
						AND et.event_tag_key IN ('e', 'a')
						AND et.event_tag_value1 IN (`)
			b.WriteString(b.BuildQueryForDependencyStart(filterID, cteName, "address", &current.Start))
			b.WriteString(`)
				) tc_def
				INNER JOIN event_tags act_et ON act_et.event_tag_value1 = tc_def.def_address
				INNER JOIN events act ON act.id = act_et.event_id
				WHERE act.kind = :`)
			b.WriteValue(filterID, "rkind", current.Reduce.Kinds[0])
			b.WriteString(`
					AND act.hidden = false
					AND act.first_1175_address IS NULL
					AND act_et.event_tag_key IN ('e', 'a')
			) AND e.hidden = false`)
		default:
			log.Warn().Int("start_kind", current.Start.Kind).
				Msg("unsupported start kind for tokenized community action reduce")
		}
	case nostr.KindProfileBadges:
		// kindXXX>kind30008+profile_badges>kind30009>kind8.
		if current.Start.ProfileBadges &&
			slices.Equal(
				current.Reduce.Kinds,
				[]int{nostr.KindProfileBadges, nostr.KindBadgeDefinition, nostr.KindBadgeAward},
			) {

			kind := b.PushValue(filterID, "start_kind", current.Start.Kind)
			usersWithBadges := `e.kind = 30008 AND e.d_tag='profile_badges'
AND (
	(e.master_pubkey = ANY(ARRAY(
		select
			distinct (mk.master_pubkey)
		from ` + cteName + ` mk
		where
			mk.kind =:` + kind + `
		)) and e.hidden=false)
	OR
	(e.pubkey = ANY(ARRAY(
		select
			distinct (pubkey)
		from ` + cteName + ` pk
		where
			pk.kind =:` + kind + `
		)) and e.hidden=false)
)
AND e.hidden=false`
			b.WriteString(usersWithBadges)
			b.WriteString(` union select `)
			for i, f := range b.fieldsNames("en", filterID) {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString(f)
			}
			b.WriteString(`
from
	events en
where
	en.kind in (8, 30009)
	AND en.address in (
			select
				et.event_tag_value1
			from
				event_tags et
			where
				et.event_id in (select e.id from events e where ` + usersWithBadges + `)
				AND et.event_tag_key in ('e', 'a')
	) and en.hidden=false`)
		}

	case nostr.KindBadgeDefinition:
		startFilter := b.BuildQueryForDependencyStart(filterID, cteName, "id", &current.Start)
		b.WriteString("e.id in ((select event_tag_value1 from event_tags where event_id in (")
		b.WriteString(startFilter)
		b.WriteString(") and event_tag_key = 'e')")
		b.WriteString(" UNION ALL ")
		b.WriteString(`(select ee.id from (select subzero_nostr_tag_a_get_pk(event_tag_value1) as pk, subzero_nostr_tag_a_get_dtag(event_tag_value1) as name from event_tags where event_id in (`)
		b.WriteString(startFilter)
		b.WriteString(") and event_tag_key = 'a') badge, events ee where badge.pk = ANY(ARRAY[ee.pubkey, ee.master_pubkey]) and ee.d_tag = badge.name and ee.kind = 30009 and hidden = false)) AND e.hidden=false")

	case nostr.KindMuteList, nostr.KindRelayListMetadata:
		reduceKindParam := b.PushValue(filterID, "rkind", current.Reduce.Kinds[0])
		b.WriteString("e.kind = :")
		b.WriteString(reduceKindParam)
		b.WriteString(` AND e.hidden = false AND (e.master_pubkey = ANY(ARRAY(`)
		b.WriteString(b.BuildQueryForDependencyStart(filterID, cteName, "DISTINCT master_pubkey", &current.Start))
		b.WriteString(`)) OR e.pubkey = ANY(ARRAY(`)
		b.WriteString(b.BuildQueryForDependencyStart(filterID, cteName, "DISTINCT pubkey", &current.Start))
		b.WriteString(`)))`)
		b.WriteString(`
union all
select
	20002,
	cast (EXTRACT(EPOCH FROM CURRENT_TIMESTAMP) as bigint) as created_at,
	to_timestamp_nano(cast (EXTRACT(EPOCH FROM CURRENT_TIMESTAMP) as bigint)) as lookup_created_at,
	'' as id,
	'' as address,
	e.pubkey,
	e.master_pubkey,
	'' as sig,
	'' as content,
	'' as d_tag,
	'' as h_tag,
	'[]' as tags,
	'' as origin
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
	subev.pubkey = ANY(ARRAY[e.master_pubkey, e.pubkey])
	or
	subev.master_pubkey = ANY(ARRAY[e.master_pubkey, e.pubkey])
) and subev.hidden=false)
and e.hidden=false
group by e.master_pubkey, e.pubkey`)

	case nostr.KindProfileMetadata, model.CustomIONKindAttestation, nostr.KindFollowList:
		b.WriteString("e.kind = :")
		b.WriteValue(filterID, "rkind", current.Reduce.Kinds[0])
		b.ApplyTextSearch(filter)
		if current.Reduce.Author == "" {
			b.WriteString(` AND (e.master_pubkey = ANY(ARRAY(`)
			b.WriteString(b.BuildQueryForDependencyStart(filterID, cteName, "DISTINCT master_pubkey", &current.Start))
			b.WriteString(`)) OR e.pubkey = ANY(ARRAY(`)
			b.WriteString(b.BuildQueryForDependencyStart(filterID, cteName, "DISTINCT pubkey", &current.Start))
			b.WriteString(`)))`)
		}
		b.WriteString(" and e.hidden=false")

	case model.KindDVMCountResponse:
		b.WriteString("counters.kind = :")
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
			b.WriteString(" AND counters.reference_type = :")
			b.WriteString(b.PushValue(filterID, "rref", refType))
		}
		b.WriteString(" AND counters.reference_id IN (")
		if current.Reduce.Kinds[1] == nostr.KindFollowList {
			b.WriteString(b.BuildQueryForDependencyStart(filterID, cteName, "pubkey", &current.Start))
			b.WriteString(" UNION ALL ")
			b.WriteString(b.BuildQueryForDependencyStart(filterID, cteName, "master_pubkey", &current.Start))
		} else {
			b.WriteString(b.BuildQueryForDependencyStart(filterID, cteName, "address", &current.Start))
		}
		b.WriteString(")")
		if current.Reduce.Group && current.Reduce.Kinds[1] == nostr.KindReaction {
			b.WriteString(" GROUP BY reference_id, counters.kind, evr.pubkey, evr.master_pubkey, evr.h_tag, evr.id, evr.kind, evr.master_pubkey, evr.d_tag, evr.address")
		}
	default:
		log.Warn().Ints("reduce_kinds", current.Reduce.Kinds).
			Msg("unsupported reduce kinds in dependency")
	}
}

func (b *queryBuilder) ParseFilters(ctx context.Context, in ...model.Filter) (out []*databaseFilterSearch, err error) {
	var currentUserPubkey *string

	if data := model.GetUserDataFromContext(ctx); data.Authenticated && data.PublicKey != "" {
		currentUserPubkey = &data.PublicKey
	}

	if len(in) == 0 {
		return []*databaseFilterSearch{{
			ID:                "empty",
			CurrentUserPubkey: currentUserPubkey,
			Extra:             []string{whereBuilderCommunityFilter, whereBuilderNoSoftDeleted},
		}}, nil
	}

	for i := range in {
		filter, err := parseNostrFilter(in[i])
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse filter %d", i)
		}
		filter.CurrentUserPubkey = currentUserPubkey
		filter.ID = "filter" + strconv.Itoa(i) + "_"

		out = append(out, filter)
	}

	return out, nil
}

func (b *queryBuilder) BuildSingleWhere(ctx context.Context, filters ...model.Filter) (whereClause string, params map[string]any, err error) {
	databaseFilters, err := b.ParseFilters(ctx, filters...)
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

func (b *queryBuilder) Build(ctx context.Context, filters ...model.Filter) (*queryBuildResult, error) {
	var ctes []*databaseCTE

	databaseFilters, err := b.ParseFilters(ctx, filters...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse filters")
	}

	definedOrder := false
	filtersTree := make(map[string]*databaseFilterTree)
	for _, filter := range databaseFilters {
		cte, err := b.BuildCTE(filter)
		if err != nil {
			return nil, errors.Wrap(err, "failed to build filter")
		}
		ctes = append(ctes, cte)
		definedOrder = definedOrder || cte.OrderBy != ""
		filtersTree[cte.Name] = &databaseFilterTree{
			Root:  filter,
			Leafs: make(map[string]*filterDependency),
		}
	}

	b.Reset()
	b.WriteString("--- Filters section begin ---\n--- ")
	b.WriteString(base64.StdEncoding.EncodeToString([]byte(model.Filters(filters).String())))
	b.WriteString("\n--- Filters section end ---\n")
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
			b.WriteString(" UNION ALL \n")
		}
		b.WriteString(` (SELECT `)
		for x, f := range b.fieldsNames(ctes[i].Name, "") {
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
			depName := ctes[i].Name + "_dep" + strconv.Itoa(j)
			filtersTree[ctes[i].Name].Leafs[depName] = ctes[i].Filter.Dependencies[j]
			b.BuildDependency(
				depName,
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

	return &queryBuildResult{
		Statement: b.String(),
		Params:    b.Params,
		Filters:   filtersTree,
	}, nil
}

func (b *queryBuilder) fieldsNames(table, origin string, flags ...uint) []string {
	var options uint
	fields := []string{
		"kind",
		"created_at",
		"lookup_created_at",
		"id",
		"address",
		"pubkey",
		"master_pubkey",
		"sig",
		"content",
		"d_tag",
		"h_tag",
		"tags",
	}

	for _, f := range flags {
		options |= f
	}

	if origin == "" {
		fields = append(fields, "origin")
	}

	if table == "" {
		return fields
	}

	for i := range fields {
		if fields[i] == "kind" && (options&writeFieldFlagMaskKindEphemeralEmbedding) != 0 {
			fields[i] += maskKindEphemeralEmbedding
		}
		fields[i] = table + "." + fields[i]
	}

	if origin != "" {
		fields = append(fields, `'`+origin+`' as origin`)
	}

	return fields
}

func shouldApplyVerifiedFirst(filter *databaseFilterSearch) bool {
	// Has topics.
	if filter.Tags.HasValues("t") {
		return true
	}

	// Unclassified only.
	if filter.Tags.HasValues("!t") && slices.Compare(filter.Tags.All("!t"), []string{"unclassified"}) == 0 {
		return true
	}

	// For any single event request with some kind filter, apply verified first.
	return filter.Limit == 1 && len(filter.Kinds) > 0 && len(filter.Authors) == 0 && len(filter.IDs) == 0
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

	// TODDO: remove this hack later.
	// Filter out requests like this to avoid heavy queries.
	// ["REQ","XX",{"kinds":[3],"limit":20,"search":"include:dependencies:kind3>kind0","#p":["XXXX"]}]
	excludeKind3 := len(filter.Kinds) == 1 &&
		filter.Kinds[0] == nostr.KindFollowList &&
		len(filter.Dependencies) == 1 &&
		filter.Dependencies[0].Start.Kind == nostr.KindFollowList &&
		len(filter.Dependencies[0].Reduce.Kinds) == 1 &&
		filter.Dependencies[0].Reduce.Kinds[0] == nostr.KindProfileMetadata &&
		len(filter.Tags["p"]) == 1

	var orderBy string
	name := filter.ID + "events_cte"
	fields := b.fieldsNames("e", name)
	var joinString string
	switch filter.Rank {
	case rankTOP:
		// All time top.
		joinString = ` inner join ranked_events r on e.id = r.event_id`
		orderBy = `verified desc, score desc`
		fields = append(fields, "r.event_verified as verified", "r.score")

	case rankTrending:
		// 24h trending.
		dayAgo := time.Now().Add(-24 * time.Hour).UnixNano()
		joinString = ` inner join ranked_events r on e.id = r.event_id and e.lookup_created_at > :` +
			b.PushValue(filter.ID, "trending_since", dayAgo)
		orderBy = `verified desc, score desc`
		fields = append(fields, "r.event_verified as verified", "r.score")
	}

	if orderBy == "" && shouldApplyVerifiedFirst(filter) {
		fields = append(fields, "verified")
		orderBy = `verified desc, ` + whereBuilderDefaultOrderBy
	}

	if excludeKind3 {
		log.Trace().Msg("excluding kind 3 query")
		for i := range fields {
			switch fields[i] {
			case "e.tags":
				// Do not load large tags field if we are going to ignore it anyway.
				// But keep the master pubkey for the later use.
				fields[i] = `jsonb_build_array(jsonb_build_array('b', e.master_pubkey)) as tags`
			case "e.sig":
				fields[i] = `'PACK' as sig`
			}
		}
	}

	// Additional fields that are not visible in the main select but used for filtering/sorting.
	fields = append(fields, "first_1175_address")

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
		Name:    name,
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
	b.ApplyFilterGiftWrap(filter)

	return b.String(), b.Params, nil
}

func (b *queryBuilder) BuildForAccountDelete(masterKey string) (where string, params map[string]any, err error) {
	masterValueName := b.PushValue("accountdelete", "master_pubkey", masterKey)
	b.WriteString(`e.id in (
		select
			ev.id
		from
			events ev
		where
			ev.master_pubkey = :` + masterValueName + `
			and ev.hidden=false
			and ev.kind not in (1175, 31175)
		union all
		select
			badges.id
		from
			event_tags et
		inner join events badges on et.event_id = badges.id
		where
			badges.kind in (30009, 8)
			and badges.pubkey = badges.master_pubkey
			and badges.hidden=false
			and et.event_tag_key = 'p'
			and et.event_tag_value1 = :` + masterValueName + `
		)
	`)
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
	if len(filter.IDs) > 0 || len(filter.Addresses) > 0 {
		b.WriteRune('(')
		buildFromSlice(b, sqlOpCodeNONE, filterID, filter.IDs, "id", "")
		for i := range filter.Addresses {
			idxStr := strconv.Itoa(i)
			b.MaybeOR()
			b.WriteString("(address = :")
			b.WriteValue(filterID, "address"+idxStr, filter.Addresses[i])
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
	b.WriteString("subzero_nostr_onbehalf_is_allowed_on_time(jsonb(coalesce((select p.tags from events p where p.master_pubkey = master_pubkey and p.kind = 10100 and hidden=false limit 1), '[]')), :")
	b.WriteString(owner)
	b.WriteString(", kind, :")
	b.WriteValue(filterID, "current_timestamp_nano", time.Now().UnixNano())
	b.WriteString(")))))")
}

func (b *queryBuilder) BuildForDelete(filters ...databaseFilterDelete) (where string, params map[string]any, err error) {
	if len(filters) == 1 && filters[0].AccountDelete {
		return b.BuildForAccountDelete(filters[0].Author)
	}

	for idx := range filters {
		b.MaybeOR()
		b.ApplyDeleteFilter(idx, &filters[idx])
	}

	if b.Len() == 0 {
		return "", nil, ErrEmptyFilter
	}

	b.WriteString(" AND ")
	b.WriteString(whereBuilderDefaultWhere)

	// Exclude tokenized community actions.
	b.WriteString(" AND kind not in (1175)")

	// Exclude tc_definitions that have corresponding tc_actions with posts.
	// `e` is the main events table alias.
	b.WriteString(` AND (
		case
			when e.kind = 31175 then
				NOT EXISTS (
					select 1
					from events tc_def
					inner join event_tags et on tc_def.id = et.event_id
					where
						et.event_tag_key IN ('e', 'a')
						and et.event_tag_value1 = e.address
						and tc_def.kind = 1175
						and tc_def.hidden = false
				)
			when e.kind in (0, 1, 30023, 30175) then
				NOT EXISTS (
					select 1
					from events tc_def
					inner join event_tags et on tc_def.id = et.event_id
					where
						et.event_tag_key IN ('e', 'a')
						and et.event_tag_value1 = e.address
						and tc_def.kind = 31175
						and tc_def.hidden = false
				)
			else
				true
		end)`)

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
		nostr.KindReaction:                              {},
		nostr.KindFollowList:                            {},
		nostr.KindTextNote:                              {},
		nostr.KindRepost:                                {},
		nostr.KindArticle:                               {},
		nostr.KindGenericRepost:                         {},
		model.CustomIONKindEditableTextNote:             {},
		model.CustomIONKindCommunityJoin:                {},
		model.CustomIONKindTokenizedCommunityDefinition: {},
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

				case nostr.KindTextNote,
					nostr.KindRepost,
					nostr.KindArticle,
					nostr.KindGenericRepost,
					model.CustomIONKindTokenizedCommunityDefinition,
					model.CustomIONKindEditableTextNote:
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
