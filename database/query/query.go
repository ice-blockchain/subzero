// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/model"
)

const (
	systemKindQuote        = 1
	systemKindCommentRoot  = 2
	systemKindCommentReply = 3

	replyMarkerIndex = 3 // event_tag_value3.

	maxTagValues = 5
)

var (
	ErrUnexpectedRowsAffected    = errors.New("unexpected rows affected")
	ErrAttestationUpdateRejected = errors.New("attestation update rejected")
	ErrOnBehalfAccessDenied      = model.ErrOnBehalfAccessDenied
	ErrRepostOfDeletedPost       = errors.New("repost of deleted post")
	ErrInvalidEvent              = errors.New("invalid event")
	ErrRaceCondition             = errors.New("race condition")

	errEventIteratorInterrupted = errors.New("interrupted")

	notifyExpiredEvents func(ctx context.Context, events ...*model.Event) error
)

type (
	databaseEvent struct {
		model.Event
		SystemKind      sql.NullInt64
		ReferenceID     sql.NullString
		Jtags           string
		SigAlg          string
		KeyAlg          string
		MasterPubKey    string
		Dtag            string
		Htag            string
		AddressValue    string
		Lookup          string
		TagID           int64
		Deleted         bool
		HasImages       bool
		HasVideos       bool
		SaveMergeAction string
	}
	databaseEventAddress struct {
		Kind   int
		Pubkey string
		Dtag   string
	}
	databaseRollbackRequest struct {
		databaseBatchRequest
		ReplaceableEvents map[string]bool
	}
)

type databaseBatchRequest struct {
	EventsHash *string
	// Events to store or replace.
	InsertOrReplace []databaseEvent

	// Events to delete.
	Delete []databaseFilterDelete
}

func detectImagesVideos(tags model.Tags) (images, videos bool) {
	for _, tag := range tags {
		if tag.Key() != "imeta" {
			continue
		}

		for i := range len(tag) {
			if strings.HasPrefix(tag[i], "m image/") {
				images = true
			} else if strings.HasPrefix(tag[i], "m video/") {
				videos = true
			}
		}
	}
	return images, videos
}

func detectSystemKind(tags model.Tags) (int64, bool) {
	// Syntax: "a|e", "<address>", "", "reply|root", "<master_pubkey>".
	var rootOf, replyOf string
	for i := range tags {
		switch tags[i].Key() {
		case "a", "e":
			if len(tags[i]) <= replyMarkerIndex {
				continue
			}
			if strings.EqualFold(tags[i][replyMarkerIndex], "reply") && replyOf == "" {
				replyOf = tags[i].Value()
			} else if strings.EqualFold(tags[i][replyMarkerIndex], "root") && rootOf == "" {
				rootOf = tags[i].Value()
			}
		case "q", "Q":
			return systemKindQuote, true
		}
	}

	// Cover:
	// - has both reply and root tags that points to the same event.
	// - has only root tag.
	if rootOf != "" && (rootOf == replyOf || replyOf == "") {
		return systemKindCommentRoot, true
	} else if replyOf != "" && replyOf != rootOf { // Has reply tag, and optional root tag.
		return systemKindCommentReply, true
	}
	return -1, false
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

	var images, videos bool
	images, videos = detectImagesVideos(e.Tags)

	var lookup string
	if e.Kind == nostr.KindRepost || e.Kind == nostr.KindGenericRepost {
		var original model.Event
		if err := original.UnmarshalJSON([]byte(e.Content)); err == nil {
			images, videos = detectImagesVideos(original.Tags)
		}
		lookup = prepareSearchContent(&original)
	} else {
		lookup = prepareSearchContent(e)
	}

	var systemKind sql.NullInt64
	systemKind.Int64, systemKind.Valid = detectSystemKind(e.Tags)

	return &databaseEvent{
		Event:        *e,
		MasterPubKey: e.GetMasterPublicKey(),
		SystemKind:   systemKind,
		Jtags:        string(jtags),
		SigAlg:       sigAlg,
		KeyAlg:       keyAlg,
		Dtag:         e.Tags.GetD(),
		Htag:         e.GetHTag(),
		Lookup:       lookup,
		Deleted:      deleted,
		HasImages:    images,
		HasVideos:    videos,
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
	eventsHash := hashEvents(events...)
	req.EventsHash = &eventsHash
	if _, accepted := db.rollbackableEvents.Load(eventsHash); accepted {
		return nil
	}
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

func (db *dbClient) RollbackEvents(ctx context.Context, events ...*model.Event) error {
	eventsHash := hashEvents(events...)
	if eventsToRollback, hasEventsToRollback := db.rollbackableEvents.Load(eventsHash); !hasEventsToRollback {
		return nil
	} else {
		if err := db.executeBatch(ctx, &databaseBatchRequest{
			InsertOrReplace: eventsToRollback.InsertOrReplace,
			Delete:          eventsToRollback.Delete,
		}); err != nil {
			return errors.Wrap(err, "failed to perform rollback")
		}
		return db.rollbackReplaceableEvents(ctx, eventsToRollback.ReplaceableEvents)
	}
}

func parseSigKeyAlg(event *model.Event) (sigAlg, keyAlg string, err error) {
	sAlg, kAlg, _, err := event.ExtractSignature()
	if err != nil {
		return "", "", errors.Wrap(err, "failed to extract signature")
	}

	return string(sAlg), string(kAlg), nil
}

func (db *dbClient) deleteEventsWithDependencies(ctx context.Context, doAccessCheck bool, filters []databaseFilterDelete) (deletedEvents []*model.Event, dependencies []databaseFilterDelete, err error) {
	var (
		where  string
		params map[string]any
	)

	builder := newQueryBuilder()
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
		where, params, err = builder.BuildSingleWhere(genericFilters...)
	}
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to generate events where clause")
	}

	stmt := `delete from events as e where ` + where + ` returning
	kind,
	created_at,
	id,
	pubkey,
	master_pubkey,
	sig,
	content,
	d_tag,
	h_tag,
	tags
`
	for ev, err := range db.newReadEventIterator(ctx, stmt, params) {
		if err != nil {
			return nil, nil, errors.Wrap(handleError(err), "failed to exec delete event sql")
		}
		deletedEvents = append(deletedEvents, ev)
	}
	if len(deletedEvents) == 0 {
		return nil, nil, nil
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

	return deletedEvents, dependencies, nil
}

func (db *dbClient) deleteEvents(ctx context.Context, filters *databaseBatchRequest, eventsToRollback *databaseRollbackRequest) error {
	deleted, filtersToDelete, err := db.deleteEventsWithDependencies(ctx, true, filters.Delete)
	if err != nil {
		return err
	}
	if err == nil && filters.EventsHash != nil {
		for _, e := range deleted {
			if err = eventsToRollback.Save(e); err != nil {
				break
			}
		}
	}
	for len(filtersToDelete) > 0 && err == nil {
		var dependencies []databaseFilterDelete
		for _, batch := range model.SplitBatch(filtersToDelete, 100) {
			batchDelete, batchDeps, err := db.deleteEventsWithDependencies(ctx, false, batch)
			if err != nil {
				break
			}
			if filters.EventsHash != nil {
				for _, e := range batchDelete {
					if err = eventsToRollback.Save(e); err != nil {
						break
					}
				}
			}
			dependencies = append(dependencies, batchDeps...)
		}
		filtersToDelete = dependencies
	}

	return err
}

func (db *dbClient) rollbackReplaceableEvents(ctx context.Context, replacedEvents map[string]bool) error {
	if len(replacedEvents) == 0 {
		return nil
	}
	eventIDs := make([]string, 0, len(replacedEvents))
	for evID := range replacedEvents {
		eventIDs = append(eventIDs, evID)
	}
	var stmt = `
		WITH replaced AS (DELETE FROM replaceable_events_before_update
						WHERE replaced_by_id = ANY($1)
						RETURNING kind, system_kind, created_at, id, pubkey, master_pubkey, sig, sig_alg, key_alg, content, tags, d_tag, h_tag,
						    deleted, has_images, has_videos, lookup ,replaced_by_id)
		MERGE INTO events AS target
				USING (SELECT * FROM replaced) AS source (
					kind, system_kind, created_at, id, pubkey, master_pubkey, sig, sig_alg, key_alg, content, tags, d_tag, h_tag, deleted,
					has_images, has_videos, lookup
				)
				ON (
					target.id = source.replaced_by_id 
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
					system_kind = source.system_kind,
					created_at = source.created_at,
					pubkey = source.pubkey,
					sig = source.sig,
					sig_alg = source.sig_alg,
					key_alg = source.key_alg,
					content = source.content,
					tags = source.tags,
					h_tag = source.h_tag,
					deleted = source.deleted,
					has_images = source.has_images,
					has_videos = source.has_videos,
				    lookup = source.lookup
			WHEN MATCHED AND
				target.master_pubkey = source.master_pubkey
				AND target.kind = source.kind
				AND ((10000 <= source.kind AND source.kind < 20000) OR source.kind = 0 OR source.kind = 3) THEN
				UPDATE SET
					id = source.id,
					system_kind = source.system_kind,
					d_tag = source.d_tag,
					pubkey = source.pubkey,
					created_at = source.created_at,
				    sig = source.sig,
					sig_alg = source.sig_alg,
					key_alg = source.key_alg,
					content = source.content,
					tags = source.tags,
					has_images = source.has_images,
					has_videos = source.has_videos,
				    lookup = source.lookup
			WHEN MATCHED AND target.id = source.id THEN
				UPDATE SET
					kind = source.kind,
					system_kind = source.system_kind,
					master_pubkey = source.master_pubkey,
					d_tag = source.d_tag,
					created_at = source.created_at,
					pubkey = source.pubkey,
					sig = source.sig,
					sig_alg = source.sig_alg,
					key_alg = source.key_alg,
					content = source.content,
					tags = source.tags,
					has_images = source.has_images,
					has_videos = source.has_videos,
				    lookup = source.lookup
			WHEN MATCHED AND target.id = source.replaced_by_id THEN
				UPDATE SET 
					kind = source.kind,
					system_kind = source.system_kind,
					master_pubkey = source.master_pubkey,
					d_tag = source.d_tag,
					created_at = source.created_at,
					pubkey = source.pubkey,
					sig = source.sig,
					sig_alg = source.sig_alg,
					key_alg = source.key_alg,
					content = source.content,
					tags = source.tags,
					has_images = source.has_images,
					has_videos = source.has_videos,
					lookup = source.lookup
				;`

	result, err := db.ExecContext(ctx, stmt, eventIDs)
	if err != nil {
		err = errors.Wrap(handleError(err), "failed to exec rollback replaceable events sql")
	} else if rows, err := result.RowsAffected(); err != nil {
		err = errors.Wrap(err, "failed to get rows affected")
	} else if expected := int64(len(eventIDs)); rows < expected {
		err = errors.Wrapf(ErrUnexpectedRowsAffected, "expected %d rows affected, got %d", expected, rows)
	}
	return err
}

func (db *dbClient) saveEvents(ctx context.Context, events []databaseEvent) *eventIterator {
	var stmt string
	params := []any{}
	values := []string{}

	idx := 1
	for _, ev := range events {
		params = append(params, ev.Kind, ev.SystemKind, ev.CreatedAt,
			ev.ID, ev.PubKey, ev.MasterPubKey, ev.Sig, ev.SigAlg, ev.KeyAlg, ev.Content,
			ev.Tags, ev.Dtag, ev.Htag, ev.Deleted, ev.HasImages, ev.HasVideos,
			ev.Lookup,
		)
		values = append(values, fmt.Sprintf(
			`($%[1]v::integer, $%[2]v::integer, to_timestamp($%[3]v::bigint),
			$%[4]v, $%[5]v, $%[6]v, $%[7]v, $%[8]v, $%[9]v, $%[10]v,
			COALESCE($%[11]v, '[]'::jsonb), $%[12]v, $%[13]v,
			$%[14]v::bool, $%[15]v::bool, $%[16]v::bool, to_tsvector($%[17]v::text))`,
			idx, idx+1, idx+2,
			idx+3, idx+4, idx+5, idx+6, idx+7, idx+8, idx+9, idx+10, idx+11, idx+12, idx+13, idx+14, idx+15, idx+16,
		))
		idx += 17
	}

	stmt = `MERGE INTO events AS target
				USING (VALUES
					` + strings.Join(values, ",") + `
				) AS source (
					kind, system_kind, created_at, id, pubkey, master_pubkey, sig, sig_alg, key_alg, content, tags, d_tag, h_tag, deleted,
					has_images,
					has_videos,
					lookup
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
					system_kind = source.system_kind,
					created_at = source.created_at,
					pubkey = source.pubkey,
					sig = source.sig,
					sig_alg = source.sig_alg,
					key_alg = source.key_alg,
					content = source.content,
					tags = source.tags,
					h_tag = source.h_tag,
					lookup = source.lookup,
					deleted = source.deleted,
					has_images = source.has_images,
					has_videos = source.has_videos
			WHEN MATCHED AND
				target.master_pubkey = source.master_pubkey
				AND target.kind = source.kind
				AND ((10000 <= source.kind AND source.kind < 20000) OR source.kind = 0 OR source.kind = 3) THEN
				UPDATE SET
					id = source.id,
					system_kind = source.system_kind,
					d_tag = source.d_tag,
					sig = source.sig,
					sig_alg = source.sig_alg,
					key_alg = source.key_alg,
					pubkey = source.pubkey,
					created_at = source.created_at,
					content = source.content,
					lookup = source.lookup,
					tags = source.tags,
					has_images = source.has_images,
					has_videos = source.has_videos
			WHEN MATCHED AND target.id = source.id THEN
				UPDATE SET
					kind = source.kind,
					system_kind = source.system_kind,
					master_pubkey = source.master_pubkey,
					d_tag = source.d_tag,
					created_at = source.created_at,
					pubkey = source.pubkey,
					sig = source.sig,
					sig_alg = source.sig_alg,
					key_alg = source.key_alg,
					lookup = source.lookup,
					content = source.content,
					tags = source.tags,
					has_images = source.has_images,
					has_videos = source.has_videos
			WHEN NOT MATCHED THEN
				INSERT (
					id, kind, system_kind, created_at, pubkey, master_pubkey,
					sig, sig_alg, key_alg, content, tags, d_tag, h_tag,
					deleted,
					has_images,
					has_videos,
					lookup
				)
				VALUES (
					source.id, source.kind, source.system_kind, source.created_at,
					source.pubkey, source.master_pubkey, source.sig, source.sig_alg,
					source.key_alg, source.content, source.tags, source.d_tag,
					source.h_tag, source.deleted,
					source.has_images, source.has_videos,
					source.lookup
				)
				RETURNING
					target.kind,
					target.created_at,
					target.id,
					target.pubkey,
					target.master_pubkey,
					target.sig,
					target.content,
					target.d_tag,
					target.h_tag,
					target.lookup,
					target.tags,
					merge_action() as savemergeaction;`

	it := &eventIterator{
		Map: nil,
		Fetch: func() (*sqlx.Rows, error) {
			result, err := db.QueryxContext(ctx, stmt, params...)
			if err != nil {
				err = errors.Wrap(handleError(err), "failed to exec insert event sql")
				if errors.Is(err, ErrRaceCondition) {
					result, err = db.QueryxContext(ctx, stmt, params...)
					if err != nil {
						err = errors.Wrap(handleError(err), "failed to exec insert event sql")
					}
				}
			}
			return result, err
		}}

	return it
}

func (db *dbClient) executeSave(ctx context.Context, req *databaseBatchRequest) (replaceableEvents map[string]bool, inserted []databaseFilterDelete, err error) {
	if len(req.InsertOrReplace) == 0 {
		return map[string]bool{}, []databaseFilterDelete{}, nil
	}
	insertedEvents := db.saveEvents(ctx, req.InsertOrReplace)
	events := []*model.Event{}
	replaceableEvents = map[string]bool{}
	sErr := insertedEvents.Each(ctx, func(dbEvent *databaseEvent) error {
		if dbEvent.Event.IsReplaceable() || nostr.IsAddressableKind(dbEvent.Event.Kind) {
			replaceableEvents[dbEvent.ID] = dbEvent.SaveMergeAction == "INSERT"
		}
		events = append(events, &dbEvent.Event)

		return nil
	})
	if sErr != nil {
		sErr = errors.Wrap(handleError(sErr), "failed to exec insert event sql")
	}
	if len(replaceableEvents) > 0 {
		keepOnlyInsertedEvents := func(event *model.Event) bool {
			if insert, wasUpdatedReplaceableEvent := replaceableEvents[event.ID]; wasUpdatedReplaceableEvent {
				if !insert {
					return true
				}
				delete(replaceableEvents, event.ID)
				return false
			}
			return false
		}
		events = slices.DeleteFunc(events, keepOnlyInsertedEvents)
	}
	if actual := len(events) + len(replaceableEvents); sErr == nil && actual != len(req.InsertOrReplace) {
		sErr = errors.Wrapf(ErrUnexpectedRowsAffected, "expected %d rows affected, got %d", len(req.InsertOrReplace), actual)
	}
	if sErr == nil && req.EventsHash != nil && len(events) > 0 {
		for _, ev := range events {
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
			inserted = append(inserted, f)
		}
	}
	return replaceableEvents, inserted, errors.Wrap(sErr, "failed to save events")
}

func (db *dbClient) executeBatch(ctx context.Context, req *databaseBatchRequest) (err error) {
	if req.Empty() {
		return nil
	}
	var eventsToRollback databaseRollbackRequest
	if req.EventsHash != nil {
		replacedEvents, toRollbackDeleteOp, sErr := db.executeSave(ctx, req)
		eventsToRollback.Delete = append(eventsToRollback.Delete, toRollbackDeleteOp...)
		eventsToRollback.ReplaceableEvents = replacedEvents
		err = errors.Join(err, errors.Wrap(sErr, "failed to save events"))
		if dErr := db.deleteEvents(ctx, req, &eventsToRollback); dErr != nil {
			err = errors.Join(err, errors.Wrap(dErr, "failed to delete events"))
		}
	} else {
		if dErr := db.deleteEvents(ctx, req, &eventsToRollback); dErr != nil {
			err = errors.Join(err, errors.Wrap(dErr, "failed to delete events"))
		}
		_, toRollbackDeleteOp, sErr := db.executeSave(ctx, req)
		eventsToRollback.Delete = append(eventsToRollback.Delete, toRollbackDeleteOp...)
		err = errors.Join(err, errors.Wrap(sErr, "failed to save events"))
	}
	if err == nil && (!eventsToRollback.Empty() || len(eventsToRollback.ReplaceableEvents) > 0) {
		db.rollbackableEvents.Store(*req.EventsHash, &eventsToRollback)
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
	it := &eventIterator{
		Map: db.eventTransform,
		Fetch: func() (*sqlx.Rows, error) {
			sqlQuery, params, err := db.generateSelectEventsSQL(ctx, filters...)
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

			return rows, err
		},
	}

	return func(yield func(*model.Event, error) bool) {
		err := it.Each(ctx, func(event *databaseEvent) error {
			if !yield(&event.Event, nil) {
				return errEventIteratorInterrupted
			}

			return nil
		})

		if err != nil && !errors.Is(err, errEventIteratorInterrupted) {
			yield(nil, errors.Wrap(err, "failed to iterate events"))
		}
	}
}

func handleError(err error) error {
	var sqlError *pgconn.PgError

	if err == nil {
		return err
	}

	if errors.As(err, &sqlError) {
		if sqlError.SQLState() == "P0001" {
			if sqlError.Message == "attestation list update must be linear" {
				return ErrAttestationUpdateRejected
			}
			if sqlError.Message == "onbehalf permission denied" {
				return ErrOnBehalfAccessDenied
			}
			if sqlError.Message == "repost of deleted post" {
				return ErrRepostOfDeletedPost
			}
		} else if sqlError.SQLState() == "22021" {
			return ErrInvalidEvent
		} else if sqlError.SQLState() == "23505" && sqlError.ConstraintName == "events_pkey" {
			return ErrRaceCondition
		}
	}

	return err
}

func (db *dbClient) generateEventsCountClause(ctx context.Context, filters ...model.Filter) (sqlQuery string, params map[string]any, err error) {
	if len(filters) > 0 {
		where, params, err := newQueryBuilder().BuildForPrecalculatedCounters(filters...)
		if err == nil {
			return `select coalesce(sum(value), 0) from event_counters where ` + where, params, nil
		} else if !errors.Is(err, errUnsupportedCombination) {
			return "", nil, errors.Wrap(err, "failed to generate events count where clause")
		}
	}

	filters = db.extendWhereFilters(ctx, filters...)
	where, params, err := newQueryBuilder().BuildSingleWhere(filters...)
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

	where, params, err := newQueryBuilder().BuildForPrecalculatedCounters(filters...)
	if err != nil {
		return "", errors.Wrap(err, "failed to generate events where clause")
	} else if where == "" {
		where = "true"
	}

	sb.WriteString(`WITH cte AS (SELECT COALESCE(NULLIF(f.reference_type, ''), '+') AS key, sum(f.value) as val from event_counters f where kind = 7 AND `)
	sb.WriteString(where)
	sb.WriteString(`group by reference_type) SELECT jsonb_object_agg(cte.KEY, cte.val) FROM cte`)
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

func (db *dbClient) generateSelectEventsSQL(ctx context.Context, filter ...model.Filter) (sql string, params map[string]any, err error) {
	filters := db.extendWhereFilters(ctx, filter...)

	return newQueryBuilder().Build(filters...)
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
			return nil, errors.Wrapf(handleError(err), "failed to fetch all keys of %v", pubkey)
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
	var addressableTags = []string{"a", "Q"}
	for i := range filters {
		for _, tag := range addressableTags {
			v, ok := filters[i].Tags[tag]
			if !ok {
				continue
			}

			var subkeyFilters []model.TagValues
			for _, b := range v {
				if len(b) == 0 || b[0] == nil || *b[0] == "" {
					continue
				}

				// Format: `kind:pubkey:d_tag`.
				parts := strings.Split(*b[0], ":")
				if len(parts) != 3 {
					continue
				}

				keys, err := db.fetchAllKeysOf(ctx, parts[1])
				if err != nil {
					log.Printf("subkeys fetch failed: %v", err)

					continue
				}

				for _, subkey := range keys {
					n := slices.Clone(b)
					address := strings.Join([]string{parts[0], subkey, parts[2]}, ":")
					n[0] = &address
					subkeyFilters = append(subkeyFilters, n)
				}
			}

			if len(subkeyFilters) == 0 {
				continue
			}

			filters[i].Tags.Set(tag)
			for _, v := range subkeyFilters {
				filters[i].Tags.Append(tag, v...)
			}
		}
	}

	return filters
}

func (db *dbClient) deleteExpiredEvents(ctx context.Context) (err error) {
	const batchSize = 1000
	const stmt = `
	WITH expired_events AS (
		SELECT e.id
		FROM event_tags et
		INNER JOIN events e ON e.id = et.event_id 
		WHERE
			et.event_tag_key = 'expiration'
		AND to_timestamp(cast(et.event_tag_value1 as bigint)) <= CURRENT_TIMESTAMP
		ORDER BY et.id ASC
		LIMIT :batch_size
	)
	DELETE FROM events
	WHERE id IN (SELECT id FROM expired_events)
	RETURNING
		kind,
		created_at,
		id,
		pubkey,
		sig,
		content,
		tags`
	params := map[string]any{"batch_size": batchSize}

	for ctx.Err() == nil {
		var deleted int
		it := db.newReadEventIterator(ctx, stmt, params)
		for event, iterErr := range it {
			if iterErr != nil {
				return errors.Wrap(iterErr, "failed to exec delete expired events")
			}

			if notifyExpiredEvents != nil {
				if notifyErr := notifyExpiredEvents(ctx, event); notifyErr != nil {
					log.Printf("failed to process notification of expired events: %v", notifyErr)
					// Continue to delete the events even if notification fails.
				}
			}

			deleted++
		}

		if deleted < batchSize {
			break
		}
	}

	return nil
}

func (db *dbClient) prepareCommunityDeleteFilters(ctx context.Context, incomingEvent *model.Event) (filters []databaseFilterDelete, err error) {
	selectFilter := model.Filter{
		Tags: model.TagMap{}.Set(model.CustomIONTagCommunity),
	}
	for _, tag := range incomingEvent.GetTags("e") {
		selectFilter.IDs = append(selectFilter.IDs, tag.Value())
	}
	if len(selectFilter.IDs) == 0 {
		// Nothing to delete.
		return nil, nil
	}

	filters = make([]databaseFilterDelete, 0)
	for ev, err := range db.SelectEvents(ctx, selectFilter) {
		if err != nil {
			return nil, errors.Wrap(handleError(err), "failed to select community events for deletion")
		}
		filters = append(filters, databaseFilterDelete{
			Author: ev.GetMasterPublicKey(),
			IDs:    []string{ev.ID},
		})
	}

	return filters, nil
}
