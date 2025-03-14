// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"

	postgres "github.com/ice-blockchain/subzero/database/query/internal/postgres"
	"github.com/ice-blockchain/subzero/model"
)

const (
	selectDefaultBatchLimit = 100
)

var (
	ErrUnexpectedRowsAffected    = errors.New("unexpected rows affected")
	ErrAttestationUpdateRejected = errors.New("attestation update rejected")

	notifyExpiredEvents func(ctx context.Context, events ...*model.Event) error
)

type (
	databaseEvent struct {
		model.Event
		SystemCreatedAt int64
		ReferenceID     string
		Jtags           string
		SigAlg          string
		KeyAlg          string
		MasterPubKey    string
		Dtag            string
		Htag            string
		ContentMetadata string
		Deleted         bool
	}
	DatabaseEvent struct {
		ID              string          `db:"id"`
		PubKey          string          `db:"pubkey"`
		MasterPubKey    string          `db:"master_pubkey"`
		CreatedAt       model.Timestamp `db:"created_at"`
		SystemCreatedAt model.Timestamp `db:"system_created_at"`
		Kind            int             `db:"kind"`
		Tags            *model.Tags     `db:"tags"`
		DTag            string          `db:"d_tag"`
		HTag            string          `db:"h_tag"`
		Content         string          `db:"content"`
		Sig             string          `db:"sig"`
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
	var deleted bool

	jtags, err := json.Marshal(e.Tags)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal tags")
	}

	sigAlg, keyAlg, err := parseSigKeyAlg(e)
	if err != nil {
		return nil, err
	}

	// Is it a soft delete?
	if len(e.Content) < 1 {
		switch e.Kind {
		case nostr.KindArticle, nostr.KindDraftArticle, model.CustomIONKindEditableTextNote:
			val, err := strconv.ParseInt(e.GetTag("published_at").Value(), 10, 64)
			deleted = err == nil && int64(e.CreatedAt) > val
		}
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
		Deleted:         deleted,
		ContentMetadata: parseContentMetadata(e),
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
		params []any
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
	e.tags
`

	deletedEvents, err := postgres.ExecMany[DatabaseEvent](ctx, db.dbPostgres, stmt, params...)
	if err != nil {
		return 0, nil, errors.Wrap(handleError(err), "failed to exec delete event sql")
	}
	if len(deletedEvents) == 0 {
		return 0, nil, nil
	}

	for _, ev := range deletedEvents {
		var f databaseFilterDelete

		f.Author = ev.PubKey
		event := db.ToEvent(ev)
		switch {
		case event.IsReplaceable():
			f.Events = append(f.Events, databaseEventAddress{Kind: ev.Kind, Pubkey: ev.PubKey})

		case event.IsAddressable():
			f.Events = append(f.Events, databaseEventAddress{Kind: ev.Kind, Pubkey: ev.PubKey, Dtag: ev.Tags.GetD()})

		case event.IsRegular():
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
			return errors.Wrap(handleError(err), "failed to exec select events")
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
	var stmt string
	params := []any{}
	values := []string{}

	idx := 1
	for _, ev := range events {
		params = append(params, ev.Kind, ev.CreatedAt, ev.SystemCreatedAt, ev.ID, ev.PubKey, ev.MasterPubKey, ev.Sig, ev.SigAlg, ev.KeyAlg, ev.Content, ev.Tags, ev.Dtag, ev.Htag, ev.ContentMetadata, ev.Deleted)
		values = append(values, fmt.Sprintf("($%[1]v::integer, $%[2]v::bigint, $%[3]v::bigint, $%[4]v, $%[5]v, $%[6]v, $%[7]v, $%[8]v, $%[9]v, $%[10]v, COALESCE($%[11]v, '[]'::jsonb), $%[12]v, $%[13]v, $%[14]v, $%[15]v::bool)",
			idx, idx+1, idx+2, idx+3, idx+4, idx+5, idx+6, idx+7, idx+8, idx+9, idx+10, idx+11, idx+12, idx+13, idx+14))
		idx += 15
	}

	stmt = `MERGE INTO events AS target
				USING (VALUES 
					` + strings.Join(values, ",") + `
				) AS source (
					kind, created_at, system_created_at, id, pubkey, master_pubkey, sig, sig_alg, key_alg, content, tags, d_tag, h_tag, content_metadata, deleted
				)
				ON (
					target.id = source.id 
					OR (target.master_pubkey = source.master_pubkey AND target.kind = source.kind AND ((10000 <= source.kind AND source.kind < 20000) OR source.kind = 0 OR source.kind = 3))
					OR (target.master_pubkey = source.master_pubkey AND target.kind = source.kind AND target.d_tag = source.d_tag AND (30000 <= source.kind AND source.kind < 40000))
				)
			WHEN MATCHED AND 
				target.master_pubkey = source.master_pubkey 
				AND target.kind = source.kind 
				AND target.d_tag = source.d_tag 
				AND (30000 <= source.kind AND source.kind < 40000) THEN
				UPDATE SET 
					id = source.id,
					created_at = source.created_at,
					system_created_at = source.system_created_at,
					pubkey = source.pubkey,
					sig = source.sig,
					content = source.content,
					tags = source.tags,
					h_tag = source.h_tag,
					content_metadata = source.content_metadata,
					deleted = source.deleted
			WHEN MATCHED AND 
				target.master_pubkey = source.master_pubkey 
				AND target.kind = source.kind 
				AND ((10000 <= source.kind AND source.kind < 20000) OR source.kind = 0 OR source.kind = 3) THEN
				UPDATE SET 
					id = source.id,
					d_tag = source.d_tag,
					pubkey = source.pubkey,
					created_at = source.created_at,
					system_created_at = source.system_created_at,
					content = source.content,
					tags = source.tags
			WHEN MATCHED AND target.id = source.id THEN
				UPDATE SET 
					kind = source.kind,
					master_pubkey = source.master_pubkey,
					d_tag = source.d_tag,
					created_at = source.created_at,
					system_created_at = source.system_created_at,
					pubkey = source.pubkey,
					sig = source.sig,
					content = source.content,
					tags = source.tags
			WHEN NOT MATCHED THEN
				INSERT (
					id, kind, created_at, system_created_at, pubkey, master_pubkey, 
					sig, sig_alg, key_alg, content, tags, d_tag, h_tag, 
					content_metadata, deleted
				)
				VALUES (
					source.id, source.kind, source.created_at, source.system_created_at,
					source.pubkey, source.master_pubkey, source.sig, source.sig_alg,
					source.key_alg, source.content, source.tags, source.d_tag,
					source.h_tag, source.content_metadata, source.deleted
				);`

	_, err := postgres.Exec(ctx, db.dbPostgres, stmt, params...)

	return errors.Wrap(handleError(err), "failed to exec insert event sql")
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

func (db *dbClient) MustSignEvent(event *model.Event) {
	if event.PubKey != "" {
		event.Tags = append(event.Tags, model.Tag{"p", event.PubKey})
	}
	if event.GetMasterPublicKey() != "" && event.GetMasterPublicKey() != event.PubKey {
		event.Tags.AppendUnique(model.Tag{model.CustomIONTagOnBehalfOf, event.GetMasterPublicKey()})
	} else if event.GetTag(model.CustomIONTagOnBehalfOf) == nil {
		pubkey, _ := model.GetPublicKey(db.relayPrivateKey)
		event.Tags = append(event.Tags, model.Tag{model.CustomIONTagOnBehalfOf, pubkey})
	}

	err := event.SignWithAlg(db.relayPrivateKey, model.SignAlgEDDSA, model.KeyAlgCurve25519)
	if err != nil {
		panic(errors.Wrap(err, "failed to sign event"))
	}
}

func (db *dbClient) ToEvent(de *DatabaseEvent) *model.Event {
	masterPubkey := de.MasterPubKey
	if masterPubkey == "" {
		masterPubkey = de.PubKey
	}
	var tags nostr.Tags
	if de.Tags != nil {
		tags = *de.Tags
	}

	ev := &model.Event{
		Event: nostr.Event{
			ID:        de.ID,
			PubKey:    masterPubkey,
			CreatedAt: de.CreatedAt,
			Kind:      de.Kind,
			Tags:      tags,
			Content:   de.Content,
			Sig:       de.Sig,
		},
	}

	if ev.Sig != "" {
		return ev
	}

	switch ev.Kind {
	case model.CustomIONKindRelayListMetadata:
		db.MustSignEvent(ev)

	case model.KindDVMCountResponse:
		var mev model.Event
		pubkey, _ := model.GetPublicKey(db.relayPrivateKey)
		mev.Kind = model.KindJobNostrEventCount
		mev.CreatedAt = de.CreatedAt
		mev.Content = de.DTag
		mev.Tags = append(tags,
			model.Tag{"param", "relay", db.relayURL},
			model.Tag{model.CustomIONTagOnBehalfOf, pubkey},
		)
		db.MustSignEvent(&mev)

		ev.Tags = model.Tags{
			{"request", mev.String()},
			{"e", mev.ID, db.relayURL},
			{"expiration", strconv.FormatInt(time.Now().Add(model.DVMJobResultExpiration).Unix(), 10)},
		}
		db.MustSignEvent(ev)
	}

	return ev
}

func (db *dbClient) SelectEvents(ctx context.Context, filters ...model.Filter) EventIterator {
	batchSizeLimit := int64(selectDefaultBatchLimit)
	hasLimitFilter := len(filters) > 0 && filters[0].Limit > 0
	if hasLimitFilter {
		batchSizeLimit = int64(filters[0].Limit)
	}

	return func(yield func(*model.Event, error) bool) {
		offset := int64(0)
		for {
			sqlQuery, params, err := db.generateSelectEventsSQL(ctx, filters, 0, batchSizeLimit, offset)
			if err != nil {
				if !yield(nil, errors.Wrap(err, "failed to generate select events SQL")) {
					return
				}
				return
			}

			dbEvents, err := postgres.Select[DatabaseEvent](ctx, db.dbPostgres, sqlQuery, params...)
			if err != nil {
				if !yield(nil, errors.Wrap(err, "failed to query events")) {
					return
				}
				return
			}

			var events []*model.Event
			for _, event := range dbEvents {
				ev := db.ToEvent(event)
				if !yield(ev, nil) {
					return
				}
				events = append(events, ev)
			}
			if hasLimitFilter && len(events) >= filters[0].Limit {
				break
			}

			batchSize := int64(len(events))
			if batchSize < batchSizeLimit {
				break
			}
			offset += batchSize
		}
	}
}

func handleError(err error) error {
	if err == nil {
		return err
	}
	if errors.Is(err, postgres.ErrOnBehalfAccessDenied) {
		return errors.Wrapf(model.ErrOnBehalfAccessDenied, "on behalf error")
	}
	if errors.Is(err, postgres.ErrAttestationUpdateRejected) {
		return errors.Wrapf(ErrAttestationUpdateRejected, "attestation update rejected")
	}

	return err
}

func (db *dbClient) generateEventsCountClause(ctx context.Context, filters ...model.Filter) (sqlQuery string, params []any, err error) {
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
	res, err := postgres.Get[int64](ctx, db.dbPostgres, sqlQuery, params...)
	if err != nil {
		errors.Wrapf(err, "failed to query events count sql: %q", sqlQuery)
	}
	count = *res

	return count, err
}

func (db *dbClient) CountGroupedEventReactions(ctx context.Context, filters ...model.Filter) (result string, err error) {
	var sb strings.Builder

	where, params, err := newWhereBuilder().BuildForPrecalculatedCounters(filters...)
	if err != nil {
		return "", errors.Wrap(err, "failed to generate events where clause")
	} else if where == "" {
		where = "TRUE"
	}

	sb.WriteString(`WITH cte AS (SELECT COALESCE(NULLIF(f.reference_type, ''), '+') AS key, SUM(f.value) AS val FROM event_counters f WHERE kind = 7 AND `)
	sb.WriteString(where)
	sb.WriteString(`GROUP BY reference_type) SELECT jsonb_object_agg(cte.key, cte.val) FROM cte`)
	sqlQuery := sb.String()

	res, err := postgres.Get[string](ctx, db.dbPostgres, sqlQuery, params...)
	if err != nil {
		errors.Wrapf(err, "failed to query events count sql: %q", sqlQuery)
	}
	result = *res

	return result, err
}

func (db *dbClient) generateSelectEventsSQL(ctx context.Context, filters model.Filters, systemCreatedAtPivot, limit, offset int64) (sql string, params []any, err error) {
	whereMain, depClause, whereSearch, params, err := db.generateEventsWhereClause(ctx, filters...)
	if err != nil {
		return "", nil, errors.Wrap(err, "failed to generate events where clause")
	}

	var systemCreatedAtFilter string
	if systemCreatedAtPivot != 0 {
		params = append(params, systemCreatedAtPivot)
		systemCreatedAtFilter = " (e.system_created_at < $" + strconv.Itoa(len(params)) + ") AND "
	}

	var limitQuery string
	if limit > 0 {
		params = append(params, limit)
		limitQuery = " limit $" + strconv.Itoa(len(params)) + " offset $" + strconv.Itoa(len(params)+1)
		params = append(params, offset)
	}

	const discoverContentCreatorsToFollow = "discover content creators to follow"
	orderBy := " order by e.system_created_at desc"
	if strings.Contains(filters.String(), discoverContentCreatorsToFollow) {
		orderBy = " order by random()"
	}
	if depClause == "" {
		if whereSearch != "" {
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
				tags
			from
				events e
			where ` + systemCreatedAtFilter + `(` + whereMain + `)` + orderBy + limitQuery, params, nil
	}
	if whereSearch != "" {
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
		tags
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
			return nil, errors.Wrapf(err, "failed to fetch all keys of %v", pubkey)
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

func (db *dbClient) generateEventsWhereClause(ctx context.Context, filters ...model.Filter) (clauseMain, clauseDeps, clauseSearch string, params []any, err error) {
	builder := newWhereBuilder()
	clauseMain, _, err = builder.Build(db.extendWhereFilters(ctx, filters...)...)
	if err != nil {
		return "", "", "", nil, err
	}

	clauseDeps, params, err = builder.BuildDependencies("eventsmain")
	if err != nil {
		return "", "", "", nil, err
	}
	handleSearch := false
	for ix := range filters {
		f := parseNostrFilterText(&databaseFilterSearch{Filter: filters[ix]})
		if f.SearchText != "" {
			handleSearch = true

			break
		}
	}
	if handleSearch {
		builderSearch := newWhereBuilder()
		var paramsSearch []any
		cpy := model.Filters{}
		cpy = append(cpy, filters...)
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
		clauseSearch, paramsSearch, err = builderSearch.WithPrefix("search").Build(db.extendWhereFilters(ctx, cpy...)...)
		if err != nil {
			return "", "", "", nil, err
		}
		params = append(params, paramsSearch...)
	}

	return clauseMain, clauseDeps, clauseSearch, params, nil
}

func (db *dbClient) deleteExpiredEvents(ctx context.Context) error {
	params := []any{}
	events := []*model.Event{}
	sql := `DELETE FROM events
				WHERE id IN (
					SELECT event_id FROM event_tags
						WHERE event_tag_key = 'expiration' 
							AND CAST(event_tag_value1 AS BIGINT) <= EXTRACT(EPOCH FROM CURRENT_TIMESTAMP))
							RETURNING 
									kind,
									created_at,
									system_created_at,
									id,
									pubkey,
									master_pubkey,
									sig,
									content,
									d_tag,
									tags;`
	dbEvents, err := postgres.ExecMany[DatabaseEvent](ctx, db.dbPostgres, sql, params...)
	if err != nil {
		return errors.Wrap(err, "failed to exec delete expired events")
	}
	for _, ev := range dbEvents {
		events = append(events, db.ToEvent(ev))
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
