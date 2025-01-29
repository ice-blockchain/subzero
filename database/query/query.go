// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/jmoiron/sqlx"
	"github.com/mattn/go-sqlite3"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/model"
)

const (
	selectDefaultBatchLimit = 100
)

var (
	ErrUnexpectedRowsAffected    = errors.New("unexpected rows affected")
	ErrAttestationUpdateRejected = errors.New("attestation update rejected")
	errEventIteratorInterrupted  = errors.New("interrupted")

	notifyExpiredEvents func(ctx context.Context, events ...*model.Event) error
)

type (
	databaseEvent struct {
		model.Event
		SystemCreatedAt int64
		ReferenceID     sql.NullString
		Jtags           string
		SigAlg          string
		KeyAlg          string
		MasterPubKey    string
		Dtag            string
		Htag            string
		Metadata        string
	}
	databaseEventAddress struct {
		Kind   int
		Pubkey string
		Dtag   string
	}
)

type databaseBatchRequest struct {
	// Events to store or replace.
	InsertOrReplace []databaseEvent

	// Events to delete.
	Delete []databaseFilterDelete
}

func toDatabaseEvent(e *model.Event) (*databaseEvent, error) {
	jtags, err := json.Marshal(e.Tags)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal tags")
	}

	sigAlg, keyAlg, err := parseSigKeyAlg(e)
	if err != nil {
		return nil, err
	}

	return &databaseEvent{
		Event:           *e,
		MasterPubKey:    e.GetMasterPublicKey(),
		SystemCreatedAt: time.Now().UnixNano(),
		Jtags:           string(jtags),
		SigAlg:          sigAlg,
		KeyAlg:          keyAlg,
		Dtag:            e.Tags.GetD(),
		Htag:            e.GetHTag(),
		Metadata:        parseMetadataContent(e),
	}, nil
}

func (req *databaseBatchRequest) Save(e *model.Event) error {
	dbEvent, err := toDatabaseEvent(e)
	if err != nil {
		return err
	}
	req.InsertOrReplace = append(req.InsertOrReplace, *dbEvent)
	return nil
}

func (req *databaseBatchRequest) Remove(e *model.Event) error {
	f, err := parseEventAsFilterForDelete(e)
	if err != nil {
		return errors.Wrap(err, "failed to detect events for delete")
	}

	req.Delete = append(req.Delete, *f)

	return nil
}

func (req *databaseBatchRequest) Empty() bool {
	return len(req.InsertOrReplace) == 0 && len(req.Delete) == 0
}

func (db *dbClient) AcceptEvents(ctx context.Context, events ...*model.Event) error {
	var req databaseBatchRequest

	for i := range events {
		if events[i].IsEphemeral() {
			continue
		}
		if events[i].IsJobRequest() || events[i].IsJobResponse() || events[i].Kind == nostr.KindJobFeedback {
			continue
		}

		if events[i].Kind == nostr.KindDeletion {
			communityFilters, err := db.prepareCommunityDeleteFilters(ctx, events[i])
			if err != nil {
				return err
			}
			if len(communityFilters) > 0 {
				req.Delete = append(req.Delete, communityFilters...)

				continue
			}
			if err := req.Remove(events[i]); err != nil {
				return err
			}
		} else {
			if err := req.Save(events[i]); err != nil {
				return err
			}
		}
	}

	return db.executeBatch(ctx, &req)
}

func parseSigKeyAlg(event *model.Event) (sigAlg, keyAlg string, err error) {
	sAlg, kAlg, _, err := event.ExtractSignature()
	if err != nil {
		return "", "", errors.Wrap(err, "failed to extract signature")
	}

	return string(sAlg), string(kAlg), nil
}

func (db *dbClient) deleteEventsWithDependencies(ctx context.Context, doAccessCheck bool, filters []databaseFilterDelete) (deletedCount int, dependencies []databaseFilterDelete, err error) {
	var (
		where  string
		params map[string]any
	)

	builder := newWhereBuilder()
	if doAccessCheck {
		where, params, err = builder.BuildForDelete(filters...)
	} else {
		var genericFilters model.Filters
		for _, f := range filters {
			if len(f.IDs) > 0 {
				genericFilter := model.Filter{
					Tags: model.TagMap{},
				}
				for _, id := range f.IDs {
					genericFilter.Tags.Append("e", &id)
				}
				genericFilters = append(genericFilters, genericFilter)
			} else if len(f.Events) > 0 {
				filterA, filterQ := model.Filter{
					Tags: model.TagMap{},
				}, model.Filter{
					Tags: model.TagMap{},
				}
				for _, e := range f.Events {
					tag := fmt.Sprintf("%d:%s:%s", e.Kind, e.Pubkey, e.Dtag)
					filterA.Tags.Append("a", &tag)
					filterQ.Tags.Append("Q", &tag)
					genericFilters = append(genericFilters, filterA, filterQ)
				}
			}
		}
		if len(genericFilters) == 0 {
			panic("attempt to delete events without filters")
		}
		where, params, err = builder.Build(db.extendWhereFilters(ctx, genericFilters...)...)
	}
	if err != nil {
		return 0, nil, errors.Wrap(err, "failed to generate events where clause")
	}

	stmt := `delete from events as e where ` + where + ` returning
	kind,
	created_at,
	system_created_at,
	id,
	pubkey,
	master_pubkey,
	sig,
	content,
	d_tag,
	h_tag,
	tags as jtags
`

	var deletedEvents []*model.Event
	for ev, err := range db.newReadEventIterator(ctx, stmt, params) {
		if err != nil {
			return 0, nil, errors.Wrap(db.handleError(err), "failed to exec delete event sql")
		}
		deletedEvents = append(deletedEvents, ev)
	}
	if len(deletedEvents) == 0 {
		return 0, nil, nil
	}

	for _, ev := range deletedEvents {
		var f databaseFilterDelete

		f.Author = ev.PubKey
		switch {
		case ev.IsReplaceable():
			f.Events = append(f.Events, databaseEventAddress{Kind: ev.Kind, Pubkey: ev.PubKey})

		case ev.IsAddressable():
			f.Events = append(f.Events, databaseEventAddress{Kind: ev.Kind, Pubkey: ev.PubKey, Dtag: ev.Tags.GetD()})

		case ev.IsRegular():
			f.IDs = append(f.IDs, ev.ID)
		}
		dependencies = append(dependencies, f)
	}

	return len(deletedEvents), dependencies, nil
}

func (db *dbClient) deleteEvents(ctx context.Context, filters []databaseFilterDelete) error {
	var selectFilters []model.Filter
	for _, filter := range filters {
		fltr := model.Filter{
			IDs:     filter.IDs,
			Authors: []string{filter.Author},
		}
		for _, e := range filter.Events {
			fltr.Authors = append(fltr.Authors, filter.Author)
			fltr.Kinds = append(fltr.Kinds, e.Kind)
			if e.Dtag != "" {
				fltr.Tags = model.TagMap{}.SetLiterals("d", e.Dtag)
			}
			selectFilters = append(selectFilters, fltr)
		}
	}

	var filtersToDelete []databaseFilterDelete
	for ev, err := range db.SelectEvents(ctx, selectFilters...) {
		if err != nil {
			return errors.Wrap(db.handleError(err), "failed to exec select events")
		}
		for _, filter := range filters {
			if len(filter.IDs) == 0 && len(filter.Events) == 0 && filter.Author == ev.PubKey {
				filtersToDelete = append(filtersToDelete, filter)

				break
			}
			for _, id := range filter.IDs {
				if id == ev.ID {
					filtersToDelete = append(filtersToDelete, filter)

					break
				}
			}
			for _, e := range filter.Events {
				if e.Pubkey == ev.PubKey && e.Kind == ev.Kind && e.Dtag == ev.Tags.GetD() {
					filtersToDelete = append(filtersToDelete, filter)

					break
				}
			}
		}
	}
	if len(filtersToDelete) == 0 {
		return nil
	}

	deleted, filtersToDelete, err := db.deleteEventsWithDependencies(ctx, true, filtersToDelete)
	if deleted == 0 && err == nil {
		err = ErrUnexpectedRowsAffected
	}

	for len(filtersToDelete) > 0 && err == nil {
		var dependencies []databaseFilterDelete
		for _, batch := range model.SplitBatch(filtersToDelete, selectDefaultBatchLimit) {
			_, batchDeps, err := db.deleteEventsWithDependencies(ctx, false, batch)
			if err != nil {
				break
			}
			dependencies = append(dependencies, batchDeps...)
		}
		filtersToDelete = dependencies
	}

	return err
}

func (db *dbClient) saveEvents(ctx context.Context, events []databaseEvent) error {
	const stmt = `insert into events
	(kind, created_at, system_created_at, id, pubkey, master_pubkey, sig, sig_alg, key_alg, content, tags, d_tag, h_tag, reference_id, metadata)
values
	(:kind, :created_at, :system_created_at, :id, :pubkey, :master_pubkey, :sig, :sig_alg, :key_alg, :content, :jtags, :d_tag, :h_tag, :reference_id, :metadata)
on conflict do update set
	id                = excluded.id,
	kind              = excluded.kind,
	created_at        = excluded.created_at,
	system_created_at = excluded.system_created_at,
	pubkey            = excluded.pubkey,
	master_pubkey     = excluded.master_pubkey,
	sig               = excluded.sig,
	sig_alg           = excluded.sig_alg,
	key_alg           = excluded.key_alg,
	content           = excluded.content,
	metadata  		  = excluded.metadata,
	tags              = excluded.tags,
	d_tag             = excluded.d_tag,
	h_tag             = excluded.h_tag,
	reference_id      = excluded.reference_id,
	hidden            = 0
`

	result, err := db.NamedExecContext(ctx, stmt, events)
	if err != nil {
		err = errors.Wrap(db.handleError(err), "failed to exec insert event sql")
	} else if rows, err := result.RowsAffected(); err != nil {
		err = errors.Wrap(err, "failed to get rows affected")
	} else if expected := int64(len(events)); rows < expected {
		err = errors.Wrapf(ErrUnexpectedRowsAffected, "expected %d rows affected, got %d", expected, rows)
	}

	return err
}

func (db *dbClient) executeBatch(ctx context.Context, req *databaseBatchRequest) (err error) {
	if req.Empty() {
		return nil
	}

	if len(req.InsertOrReplace) > 0 {
		err = errors.Join(err, errors.Wrap(db.saveEvents(ctx, req.InsertOrReplace), "failed to save events"))
	}

	if len(req.Delete) > 0 {
		deleteErr := db.deleteEvents(ctx, req.Delete)
		if errors.Is(deleteErr, ErrUnexpectedRowsAffected) && len(req.InsertOrReplace) > 0 {
			deleteErr = nil
		}
		err = errors.Join(err, errors.Wrap(deleteErr, "failed to delete events"))
	}

	return err
}

func (db *dbClient) MustSignEvent(event *databaseEvent) {
	if event.PubKey != "" {
		event.Tags = append(event.Tags, model.Tag{"p", event.PubKey})
	}
	if event.MasterPubKey != "" && event.MasterPubKey != event.PubKey {
		event.Tags = append(event.Tags, model.Tag{model.CustomIONTagOnBehalfOf, event.MasterPubKey})
	} else if event.GetTag(model.CustomIONTagOnBehalfOf) == nil {
		pubkey, _ := model.GetPublicKey(db.relayPrivateKey)
		event.Tags = append(event.Tags, model.Tag{model.CustomIONTagOnBehalfOf, pubkey})
	}

	err := event.SignWithAlg(db.relayPrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519)
	if err != nil {
		panic(errors.Wrap(err, "failed to sign event"))
	}
}

func (db *dbClient) eventTransform(event *databaseEvent) *databaseEvent {
	if event.Sig != "" {
		return event
	}

	switch event.Kind {
	case model.CustomIONKindRelayListMetadata:
		db.MustSignEvent(event)

	case model.KindDVMCountResponse:
		var ev databaseEvent
		pubkey, _ := model.GetPublicKey(db.relayPrivateKey)
		ev.Kind = model.KindJobNostrEventCount
		ev.CreatedAt = event.CreatedAt
		ev.Content = event.Dtag
		ev.Tags = append(event.Tags,
			model.Tag{"param", "relay", db.relayURL},
			model.Tag{model.CustomIONTagOnBehalfOf, pubkey},
		)
		db.MustSignEvent(&ev)

		event.Tags = model.Tags{
			{"request", ev.String()},
			{"e", ev.ID, db.relayURL},
			{"expiration", strconv.FormatInt(time.Now().Add(model.DVMJobResultExpiration).Unix(), 10)},
		}
		db.MustSignEvent(event)
	}

	return event
}

func (db *dbClient) SelectEvents(ctx context.Context, filters ...model.Filter) EventIterator {
	limit := int64(selectDefaultBatchLimit)
	hasLimitFilter := len(filters) > 0 && filters[0].Limit > 0
	if hasLimitFilter {
		limit = int64(filters[0].Limit)
	}
	it := &eventIterator{
		OneShot: hasLimitFilter && limit <= selectDefaultBatchLimit,
		Map:     db.eventTransform,
		Fetch: func(pivot int64) (*sqlx.Rows, error) {
			if limit <= 0 {
				return nil, nil
			}

			sqlQuery, params, err := db.generateSelectEventsSQL(ctx, filters, pivot, min(selectDefaultBatchLimit, limit))
			if err != nil {
				return nil, err
			}

			stmt, err := db.prepare(ctx, sqlQuery, hashSQL(sqlQuery))
			if err != nil {
				return nil, errors.Wrapf(err, "failed to prepare query sql: %q with params %v", sqlQuery, params)
			}

			rows, err := stmt.QueryxContext(ctx, params)
			if err != nil {
				err = errors.Wrapf(err, "failed to query query events sql: %q", sqlQuery)
			}

			if hasLimitFilter && err == nil {
				limit -= selectDefaultBatchLimit
			}

			return rows, err
		},
	}

	return func(yield func(*model.Event, error) bool) {
		err := it.Each(ctx, func(event *model.Event) error {
			if !yield(event, nil) {
				return errEventIteratorInterrupted
			}

			return nil
		})

		if err != nil && !errors.Is(err, errEventIteratorInterrupted) {
			yield(nil, errors.Wrap(err, "failed to iterate events"))
		}
	}
}

func (db *dbClient) handleError(err error) error {
	var sqlError sqlite3.Error

	if err == nil {
		return err
	}

	if errors.As(err, &sqlError) && sqlError.Code == sqlite3.ErrConstraint {
		switch sqlError.Error() {
		case "onbehalf permission denied":
			err = model.ErrOnBehalfAccessDenied
		case "attestation list update must be linear":
			err = ErrAttestationUpdateRejected
		}
	}

	return err
}

func (db *dbClient) generateEventsCountClause(ctx context.Context, filters ...model.Filter) (sqlQuery string, params map[string]any, err error) {
	if len(filters) > 0 {
		where, params, err := newWhereBuilder().BuildForPrecalculatedCounters(filters...)
		if err == nil {
			return `select coalesce(sum(value), 0) from event_counters where ` + where, params, nil
		} else if !errors.Is(err, errUnsupportedCombination) {
			return "", nil, errors.Wrap(err, "failed to generate events count where clause")
		}
	}

	where, _, _, params, err := db.generateEventsWhereClause(ctx, filters...)
	if err != nil {
		return "", nil, errors.Wrap(err, "failed to generate events where clause")
	}

	return `select count(id) from events e where ` + where, params, nil
}

func (db *dbClient) CountEvents(ctx context.Context, filters ...model.Filter) (count int64, err error) {
	sqlQuery, params, err := db.generateEventsCountClause(ctx, filters...)
	if err != nil {
		return -1, errors.Wrap(err, "failed to generate events where clause")
	}

	stmt, err := db.prepare(ctx, sqlQuery, hashSQL(sqlQuery))
	if err != nil {
		return -1, errors.Wrapf(err, "failed to prepare query sql: %q", sqlQuery)
	}

	err = errors.Wrapf(stmt.GetContext(ctx, &count, params), "failed to query events count sql: %q", sqlQuery)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}

	return count, err
}

func (db *dbClient) CountGroupedEventReactions(ctx context.Context, filters ...model.Filter) (result string, err error) {
	var sb strings.Builder

	where, params, err := newWhereBuilder().BuildForPrecalculatedCounters(filters...)
	if err != nil {
		return "", errors.Wrap(err, "failed to generate events where clause")
	} else if where == "" {
		where = "1=1"
	}

	sb.WriteString(`WITH cte AS (SELECT COALESCE(NULLIF(f.reference_type, ''), '+') AS key, sum(f.value) as val from event_counters f where kind = 7 AND `)
	sb.WriteString(where)
	sb.WriteString(`group by reference_type) SELECT json_group_object(cte.KEY, cte.val) FROM cte`)
	sqlQuery := sb.String()

	stmt, err := db.prepare(ctx, sqlQuery, hashSQL(sqlQuery))
	if err != nil {
		return "", errors.Wrapf(err, "failed to prepare query sql: %q", sqlQuery)
	}

	err = errors.Wrapf(stmt.GetContext(ctx, &result, params), "failed to query event reactions count sql: %q", sqlQuery)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}

	return result, err
}

func (db *dbClient) generateSelectEventsSQL(ctx context.Context, filters model.Filters, systemCreatedAtPivot, limit int64) (sql string, params map[string]any, err error) {
	whereMain, depClause, whereSearch, params, err := db.generateEventsWhereClause(ctx, filters...)
	if err != nil {
		return "", nil, errors.Wrap(err, "failed to generate events where clause")
	}

	var systemCreatedAtFilter string
	if systemCreatedAtPivot != 0 {
		systemCreatedAtFilter = " (e.system_created_at < :system_created_at_pivot) AND "
		params["system_created_at_pivot"] = systemCreatedAtPivot
	}

	var limitQuery string
	if limit > 0 {
		params["mainlimit"] = limit
		limitQuery = " limit :mainlimit"
	}

	const discoverContentCreatorsToFollow = "discover content creators to follow"
	orderBy := " order by e.system_created_at desc"
	if strings.Contains(filters.String(), discoverContentCreatorsToFollow) {
		orderBy = " order by random()"
	}

	searchKeyword := ""
	val, ok := params["search"]
	if ok && val.(string) != "" {
		searchKeyword = val.(string)
	}
	if depClause == "" {
		if searchKeyword != "" {
			sql, err := db.searchWithoutDepsSQL(whereMain, whereSearch, systemCreatedAtFilter, limitQuery)
			if err != nil {
				return "", nil, err
			}

			return sql, params, nil
		}

		return `
			select
				e.kind,
				e.created_at,
				e.system_created_at,
				e.id,
				e.pubkey,
				e.master_pubkey,
				e.sig,
				e.content,
				tags as jtags
			from
				events e
			where ` + systemCreatedAtFilter + `(` + whereMain + `)` + orderBy + limitQuery, params, nil
	}
	if searchKeyword != "" {
		sql, err := db.searchWithDepsSQL(whereMain, depClause, whereSearch, systemCreatedAtFilter, limitQuery)
		if err != nil {
			return "", nil, err
		}

		return sql, params, nil
	}

	return `
with eventsmain as (
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
		e.h_tag,
		tags as jtags
	from
		events e
	where ` + systemCreatedAtFilter + `(` + whereMain + `)
order by
	system_created_at desc
` + limitQuery + `
)
select
	*
from
	eventsmain
` + depClause, params, nil
}

func (db *dbClient) fetchAllKeysOf(ctx context.Context, pubkey string) (keys []string, err error) {
	for ev, err := range db.SelectEvents(ctx,
		// Attention event of an user itself.
		model.Filter{
			Authors: []string{pubkey},
			Kinds:   []int{model.CustomIONKindAttestation},
		},
		// Attention event where user is mentioned.
		model.Filter{
			Kinds: []int{model.CustomIONKindAttestation},
			Tags:  model.TagMap{}.SetLiterals("p", pubkey),
		},
	) {
		if err != nil {
			return nil, errors.Wrapf(db.handleError(err), "failed to fetch all keys of %v", pubkey)
		}

		entries, err := model.ParseAttestationTags(ev.Tags)
		if err != nil {
			return nil, errors.Wrap(err, "failed to parse attestation tags")
		}

		keys = append(keys, ev.PubKey, ev.GetMasterPublicKey())
		for key := range entries {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func (db *dbClient) extendWhereFilters(ctx context.Context, filters ...model.Filter) model.Filters {
	for i := range filters {
		for _, tag := range []string{"a", "Q"} {
			v, ok := filters[i].Tags[tag]
			if !ok {
				continue
			}

			var delegatedTags []*string
			for _, b := range v {
				// `entry` has format `kind:pubkey:d_tag`.
				for _, entry := range b {
					if entry == nil {
						continue
					}

					parts := strings.Split(*entry, ":")
					if len(parts) != 3 {
						continue
					}

					keys, err := db.fetchAllKeysOf(ctx, parts[1])
					if err != nil {
						log.Printf("subkeys fetch failed: %v", err)

						continue
					}

					delegatedTags = append(delegatedTags, entry)
					for _, key := range keys {
						str := strings.Join([]string{parts[0], key, parts[2]}, ":")
						delegatedTags = append(delegatedTags, &str)
					}
				}
			}

			filters[i].Tags.Set(tag)
			for _, entry := range model.DeduplicateSlice(delegatedTags, func(elem *string) string { return *elem }) {
				filters[i].Tags.Append(tag, entry)
			}
		}
	}

	return filters
}

func (db *dbClient) generateEventsWhereClause(ctx context.Context, filters ...model.Filter) (clauseMain, clauseDeps, clauseSearch string, params map[string]any, err error) {
	builder := newWhereBuilder()
	clauseMain, params, err = builder.Build(db.extendWhereFilters(ctx, filters...)...)
	if err != nil {
		return "", "", "", nil, err
	}

	clauseDeps, params, err = builder.BuildDependencies("eventsmain")
	if err != nil {
		return "", "", "", nil, err
	}
	if val, ok := params["search"]; ok && val.(string) != "" {
		builderSearch := newWhereBuilder()
		paramsSearch := map[string]any{}
		cpy := model.Filters{}
		cpy = append(cpy, filters...)
		for ix, filter := range cpy {
			var toAdd []int
			for _, kind := range filter.Kinds {
				if kind == nostr.KindRepost {
					toAdd = append(toAdd, nostr.KindTextNote)
				} else if kind == nostr.KindGenericRepost {
					toAdd = append(toAdd, nostr.KindArticle)
				}
			}
			cpy[ix].Kinds = append(filter.Kinds, toAdd...)
		}
		clauseSearch, paramsSearch, err = builderSearch.WithPrefix("search").Build(db.extendWhereFilters(ctx, cpy...)...)
		if err != nil {
			return "", "", "", nil, err
		}
		for key, val := range paramsSearch {
			params[key] = val
		}
	}

	return clauseMain, clauseDeps, clauseSearch, params, nil
}

func (db *dbClient) generateEventsWhereSearchClause(ctx context.Context, filters ...model.Filter) (clauseMain, clauseDeps, clauseSearch string, params map[string]any, err error) {
	mainBuilder := newWhereBuilder()

	clauseMain, params, err = mainBuilder.Build(db.extendWhereFilters(ctx, filters...)...)
	if err != nil {
		return "", "", "", nil, err
	}
	cpy := filters
	for ix, filter := range cpy {
		var toAdd []int
		for _, kind := range filter.Kinds {
			if kind == nostr.KindRepost {
				toAdd = append(toAdd, nostr.KindTextNote)
			} else if kind == nostr.KindGenericRepost {
				toAdd = append(toAdd, nostr.KindArticle, model.CustomIONKindEditableTextNote)
			}
		}
		cpy[ix].Kinds = append(filter.Kinds, toAdd...)
	}
	clauseDeps, params, err = mainBuilder.BuildDependencies("eventsmain")
	if err != nil {
		return "", "", "", nil, err
	}
	builderSearch := newWhereBuilder()
	paramsSearch := map[string]any{}
	clauseSearch, paramsSearch, err = builderSearch.WithPrefix("search").Build(db.extendWhereFilters(ctx, cpy...)...)
	if err != nil {
		return "", "", "", nil, err
	}
	for key, val := range paramsSearch {
		params[key] = val
	}

	return clauseMain, clauseDeps, clauseSearch, params, nil
}

func (db *dbClient) deleteExpiredEvents(ctx context.Context) error {
	params := map[string]any{}
	it := &eventIterator{
		OneShot: true,
		Map:     nil,
		Fetch: func(pivot int64) (*sqlx.Rows, error) {
			result, err := db.NamedQueryContext(ctx, `delete from events
															where id in (
																select event_id from event_tags
																	where (((event_tag_key = 'expiration')
																		AND cast(event_tag_value1 as integer) <= unixepoch())))
																		returning 
																				kind,
																				created_at,
																				system_created_at,
																				id,
																				pubkey,
																				master_pubkey,
																				sig,
																				content,
																				d_tag,
																				tags as jtags;
			`, params)
			if err != nil {
				err = errors.Wrap(db.handleError(err), "failed to exec delete expired events")
			}
			return result, err
		}}
	events := []*model.Event{}
	err := it.Each(ctx, func(event *model.Event) error {
		events = append(events, event)

		return nil
	})
	if err != nil {
		return errors.Wrap(err, "failed to exec delete expired events")
	}
	if notifyExpiredEvents != nil && len(events) > 0 {
		err = errors.Wrapf(notifyExpiredEvents(ctx, events...), "failed to process notification of expired events")
	}
	return err
}

func (db *dbClient) prepareCommunityDeleteFilters(ctx context.Context, incomingEvent *model.Event) (filters []databaseFilterDelete, err error) {
	var ids []string
	for _, eTag := range incomingEvent.Tags.GetAll([]string{"e"}) {
		if eTag.Key() == "e" {
			ids = append(ids, eTag.Value())
		}
	}
	filters = make([]databaseFilterDelete, 0)
	for ev := range db.SelectEvents(ctx, model.Filter{IDs: ids}) {
		if hTag := ev.GetTag("h"); hTag == nil {
			continue
		}
		filters = append(filters, databaseFilterDelete{
			Author: ev.GetMasterPublicKey(),
			IDs:    []string{ev.GetID()},
		})
	}

	return filters, nil
}
