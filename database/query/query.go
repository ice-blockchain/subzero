// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"database/sql"
	"log"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/database/query/internal/connector"
	"github.com/ice-blockchain/subzero/model"
)

const (
	replyMarkerIndex = 3 // event_tag_value3.

	maxTagValues = 5
)

var (
	ErrUnexpectedRowsAffected    = errors.New("unexpected rows affected")
	ErrAttestationUpdateRejected = errors.New("attestation update rejected")
	ErrOnBehalfAccessDenied      = model.ErrOnBehalfAccessDenied
	ErrRepostOfDeletedPost       = errors.New("repost of deleted post")
	ErrInvalidEvent              = errors.New("invalid event")
	ErrInvalidRequest            = errors.New("invalid request")
	ErrRaceCondition             = errors.New("race condition")
	ErrReadOnly                  = errors.New("relay-is-read-only: read only")

	notifyExpiredEvents func(ctx context.Context, events ...*model.Event) error
)

type (
	EventIterator = connector.Iterator[*model.Event]

	databaseEvent struct {
		*model.Event
		LookupCreatedAt int64
		TagID           int64
		Expiration      sql.NullInt64
		ReferenceID     sql.NullString
		GiftReceiver    sql.NullString
		Ttags           []string
		SigAlg          string
		KeyAlg          string
		MasterPubKey    string
		Dtag            string
		Htag            string
		AddressValue    string
		Lookup          string
		SaveMergeAction string
		Deleted         bool
		HasImages       bool
		HasVideos       bool
		HasReferences   bool
		IsReply         bool
		IsQuote         bool
		IsRootReply     bool
	}
	databaseRollbackRequest struct {
		databaseBatchRequest
		ReplaceableEvents map[string]bool
	}
)

type databaseBatchRequest struct {
	EventsHash *eventHash
	// Events to store or replace.
	InsertOrReplace []databaseEvent
	// Events to delete.
	Delete []databaseFilterDelete
	// IDs of replaceable events to rollback update
	Rollback map[string]bool
}

func (d *databaseEvent) FromTags(tags model.Tags) {
	// Syntax: "a|e", "<address>", "", "reply|root", "<master_pubkey>".
	var rootOf, replyOf string

	for _, tag := range tags {
		switch tag.Key() {
		case "t":
			if t := tag.Value(); t != "" {
				d.Ttags = append(d.Ttags, t)
			}
		case "p":
			// Order: master public key, empty string, device public key.
			if d.Kind == nostr.KindGiftWrap {
				target := tag.Value() // Master public key.
				if len(tag) >= 3 && tag[3] != "" {
					target = tag[3] // Device public key.
				}
				if target != "" {
					d.GiftReceiver = sql.NullString{Valid: true, String: target}
				}
			}
		case "imeta":
			for i := range len(tag) {
				if strings.HasPrefix(tag[i], "m image/") {
					d.HasImages = true
				} else if strings.HasPrefix(tag[i], "m video/") {
					d.HasVideos = true
				} else if d.HasImages && d.HasVideos {
					break // No need to check further.
				}
			}
		case "expiration":
			deadline, err := nostr.ParseTimestamp(tag.Value())
			if err == nil && deadline > 0 && (!d.Expiration.Valid || deadline.Before(nostr.Timestamp(d.Expiration.Int64))) {
				d.Expiration.Int64 = deadline.Time().UnixNano()
				d.Expiration.Valid = true
			}
		case "a", "e":
			d.HasReferences = true
			if len(tag) > replyMarkerIndex {
				if strings.EqualFold(tag[replyMarkerIndex], model.TagMarkerReply) && replyOf == "" {
					d.IsReply = true
					replyOf = tag.Value()
				} else if strings.EqualFold(tag[replyMarkerIndex], model.TagMarkerRoot) && rootOf == "" {
					rootOf = tag.Value()
				}
			}
		case "q", "Q":
			d.IsQuote = true
		}
	}

	// If it has both `reply` and `root` tags, that points to the same event,
	// then it is a root reply.
	d.IsRootReply = rootOf != "" && replyOf != "" && rootOf == replyOf
}

func toDatabaseEvent(e *model.Event) (*databaseEvent, error) {
	event := databaseEvent{
		Event:        e,
		Ttags:        []string{},
		MasterPubKey: e.GetMasterPublicKey(),
		Dtag:         e.Tags.GetD(),
		Htag:         e.GetHTag(),
	}

	sigAlg, keyAlg, err := parseSigKeyAlg(e)
	if err != nil {
		return nil, err
	}
	event.SigAlg, event.KeyAlg = sigAlg, keyAlg
	event.Lookup = prepareSearchContent(e)
	event.FromTags(e.Tags)

	switch e.Kind {
	case nostr.KindArticle, nostr.KindDraftArticle, model.CustomIONKindEditableTextNote:
		// Is it a soft delete?
		if len(e.Content) < 1 && e.GetTag(model.CustomIONTagRichText) == nil {
			val, err := nostr.ParseTimestamp(e.GetTag("published_at").Value())
			event.Deleted = err == nil && e.CreatedAt.After(val)
		}
	case nostr.KindRepost, nostr.KindGenericRepost:
		var original model.Event
		if err := original.UnmarshalJSON([]byte(e.Content)); err == nil {
			event.Lookup = prepareSearchContent(&original)
			event.FromTags(original.Tags)
		}
	}

	if event.Expiration.Valid {
		event.Expiration.Int64 = nostr.Timestamp(event.Expiration.Int64).Time().UnixNano()
	}

	return &event, nil
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
	return len(req.InsertOrReplace) == 0 && len(req.Delete) == 0 && len(req.Rollback) == 0
}

func (db *dbClient) AcceptEvents(ctx context.Context, events ...*model.Event) (err error) {
	var req databaseBatchRequest
	eventsHash := hashEvents(events...)
	req.EventsHash = &eventsHash
	var ephemeralEmbeddings map[string][]*model.EphemeralEmbeddingEvent
	if ephemeralEmbeddings, err = model.ParseEphemeralEmbeddingEvents(events...); err != nil {
		return errors.Wrapf(err, "malformed embeddings")
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
			if embeddings, hasEmbeddings := ephemeralEmbeddings[events[i].Address()]; hasEmbeddings && eventValidForEphemeralAttestation(events[i]) {
				if err = verifyEphemeralAttestation(embeddings, events[i], &req); err != nil {
					return err
				}
			}
			if err := req.Remove(events[i]); err != nil {
				return err
			}
		} else {
			if embeddings, hasEmbeddings := ephemeralEmbeddings[events[i].Address()]; hasEmbeddings && eventValidForEphemeralAttestation(events[i]) {
				if err = verifyEphemeralAttestation(embeddings, events[i], &req); err != nil {
					return err
				}
			}
			if err := req.Save(events[i]); err != nil {
				return err
			}
		}
	}

	return db.executeBatch(ctx, &req)
}

func (db *dbClient) CommitEvents(ctx context.Context, events ...*model.Event) error {
	eventsHash := hashEvents(events...)
	eventsToRollback, hasEventsToRollback := db.rollbackableEvents.LoadAndDelete(eventsHash)
	if !hasEventsToRollback {
		return nil
	}

	if val := ctx.Value(model.ConsensusReplayCtxKey); val != nil && val.(bool) {
		return nil
	}
	if err := db.deleteCommittedReplaceableEvents(ctx, eventsToRollback.ReplaceableEvents, events...); err != nil {
		return errors.Wrap(err, "failed to delete tmp replaceableEvents")
	}
	return nil
}

func (db *dbClient) RollbackEvents(ctx context.Context, events ...*model.Event) error {
	eventsHash := hashEvents(events...)
	eventsToRollback, hasEventsToRollback := db.rollbackableEvents.Load(eventsHash)
	if !hasEventsToRollback {
		return nil
	}

	err := db.executeBatch(ctx, &databaseBatchRequest{
		InsertOrReplace: eventsToRollback.InsertOrReplace,
		Delete:          eventsToRollback.Delete,
		Rollback:        eventsToRollback.ReplaceableEvents,
	})

	return errors.Wrap(err, "failed to perform rollback")
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
			} else if len(f.Addresses) > 0 {
				filterA, filterQ := model.Filter{
					Tags: model.TagMap{},
				}, model.Filter{
					Tags: model.TagMap{},
				}
				for i := range f.Addresses {
					filterA.Tags.Append("a", &f.Addresses[i])
					filterQ.Tags.Append("Q", &f.Addresses[i])
					genericFilters = append(genericFilters, filterA, filterQ)
				}
			}
		}
		if len(genericFilters) == 0 {
			panic("attempt to delete events without filters")
		}
		where, params, err = builder.BuildSingleWhere(ctx, genericFilters...)
	}
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to generate events where clause")
	}

	stmt := `delete from events as e where ` + where + ` returning
	kind,
	created_at,
	id,
	pubkey,
	sig,
	content,
	tags
`

	deletedEvents, err = connector.ExecNamed[model.Event](ctx, db.db, stmt, params)
	if err != nil {
		return nil, nil, errors.Wrap(handleError(err), "failed to exec delete event sql")
	}
	if len(deletedEvents) == 0 {
		return nil, nil, nil
	}

	for _, ev := range deletedEvents {
		var f databaseFilterDelete

		f.Author = ev.PubKey
		if ev.IsRegular() {
			f.IDs = append(f.IDs, ev.ID)
		} else {
			f.Addresses = append(f.Addresses, ev.Address())
		}
		dependencies = append(dependencies, f)
	}

	return deletedEvents, dependencies, nil
}

func (db *dbClient) deleteCommittedReplaceableEvents(ctx context.Context, replaceableEventsToDelete map[string]bool, events ...*model.Event) error {
	const sqlQuery = `DELETE from replaceable_events_before_update WHERE replaced_by_id = ANY($1)
RETURNING
	kind,
	created_at,
	id,
	pubkey,
	sig,
	content,
	tags`

	if len(replaceableEventsToDelete) == 0 {
		return nil
	}
	replaceableEventsIDs := make([]string, 0, len(replaceableEventsToDelete))
	for evID := range replaceableEventsToDelete {
		replaceableEventsIDs = append(replaceableEventsIDs, evID)
	}

	deleted, err := connector.ExecMany[model.Event](ctx, db.db, sqlQuery, replaceableEventsIDs)
	if err != nil {
		return errors.Wrap(handleError(err), "failed to exec delete committed replaceable events event sql")
	} else if uint64(len(deleted)) != uint64(len(replaceableEventsToDelete)) {
		return errors.Wrapf(ErrUnexpectedRowsAffected, "expected %d rows affected, got %d", len(replaceableEventsToDelete), len(deleted))
	}
	oldEvents := make(map[string]*model.Event)
	for i := range deleted {
		oldEvents[deleted[i].Address()] = deleted[i]
	}
	for i := range events {
		addr := events[i].Address()
		if oldEvent, hasOldEvent := oldEvents[addr]; hasOldEvent {
			events[i].Previous = oldEvent
		}
	}
	return nil
}

func (db *dbClient) deleteEvents(ctx context.Context, filters *databaseBatchRequest, eventsToRollback *databaseRollbackRequest) error {
	deleted, filtersToDelete, err := db.deleteEventsWithDependencies(ctx, true, filters.Delete)
	if err != nil {
		return err
	}
	if filters.EventsHash != nil {
		for _, e := range deleted {
			if err = eventsToRollback.Save(e); err != nil {
				return errors.Wrapf(err, "failed to convert event to dbEvent: %v", e.String())
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
						return errors.Wrapf(err, "failed to convert event to dbEvent: %v", e.String())
					}
				}
			}
			dependencies = append(dependencies, batchDeps...)
		}
		filtersToDelete = dependencies
	}

	return err
}

func (db *dbClient) saveEvents(
	ctx context.Context,
	events []databaseEvent,
	replaceableEventsToRollback map[string]bool,
) (insertedEvents []*databaseEvent, err error) {
	builder := newQueryBuilder()

	replaceableEventsIDs := make([]string, 0, len(replaceableEventsToRollback))
	for evID := range replaceableEventsToRollback {
		replaceableEventsIDs = append(replaceableEventsIDs, evID)
	}

	var isReplay string
	if replay := ctx.Value(model.ConsensusReplayCtxKey); replay != nil && replay.(bool) {
		isReplay = model.ConsensusReplayCtxKey
	}
	builder.PushValue("model", "_consensuskey", model.ConsensusReplayCtxKey)

	fields := []string{
		"kind",
		"created_at",
		"id",
		"address",
		"pubkey",
		"master_pubkey",
		"gift_receiver_pubkey",
		"sig",
		"sig_alg",
		"key_alg",
		"content",
		"tags",
		"t_tags",
		"d_tag",
		"h_tag",
		"deleted",
		"has_images",
		"has_videos",
		"is_reply",
		"is_root_reply",
		"is_quote",
		"has_references",
		"hidden",
		"lookup",
		"expiration",
		"replaced_by_id",
	}

	builder.WriteString(`
WITH replaced AS (
	DELETE FROM replaceable_events_before_update
	WHERE
		replaced_by_id = ANY(:` + builder.PushValue("merge", "replaceableID", replaceableEventsIDs) + `)
	RETURNING `)
	builder.WriteFields(fields...)
	builder.WriteString(`), source_data (`) // `source_data` contains the events to merge.
	builder.WriteFields(fields...)
	builder.WriteString(`) AS (SELECT * from replaced `)
	if len(events) > 0 {
		builder.WriteString(`UNION ALL VALUES `)
	}
	for i := range events {
		name := "merge_event" + strconv.Itoa(i)
		if i > 0 {
			builder.WriteString(",\n")
		}
		builder.WriteValues(name, []queryBuilderValue{ // Keep in sync with `fields`.
			{
				Name:   "kind",
				CastTo: "integer",
				Value:  events[i].Kind,
			},
			{
				Name:   "created_at",
				CastTo: "bigint",
				Value:  events[i].CreatedAt,
			},
			{
				Name:  "id",
				Value: events[i].ID,
			},
			{
				Name:  "address",
				Value: events[i].Address(),
			},
			{
				Name:  "pubkey",
				Value: events[i].PubKey,
			},
			{
				Name:  "master_pubkey",
				Value: events[i].MasterPubKey,
			},
			{
				Name:  "gift_receiver_pubkey",
				Value: events[i].GiftReceiver,
			},
			{
				Name:  "sig",
				Value: events[i].Sig,
			},
			{
				Name:  "sig_alg",
				Value: events[i].SigAlg,
			},
			{
				Name:  "key_alg",
				Value: events[i].KeyAlg,
			},
			{
				Name:  "content",
				Value: events[i].Content,
			},
			{
				Name:   "tags",
				CastTo: "jsonb",
				Value: func() model.Tags {
					if len(events[i].Tags) > 0 {
						return events[i].Tags
					}
					return model.Tags{}
				}(),
			},
			{
				Name:   "t_tags",
				CastTo: "text[]",
				Value:  events[i].Ttags,
			},
			{
				Name:  "d_tag",
				Value: events[i].Dtag,
			},
			{
				Name:  "h_tag",
				Value: events[i].Htag,
			},
			{
				Name:   "deleted",
				CastTo: "bool",
				Value:  events[i].Deleted,
			},
			{
				Name:   "has_images",
				CastTo: "bool",
				Value:  events[i].HasImages,
			},
			{
				Name:   "has_videos",
				CastTo: "bool",
				Value:  events[i].HasVideos,
			},
			{
				Name:   "is_reply",
				CastTo: "bool",
				Value:  events[i].IsReply,
			},
			{
				Name:   "is_root_reply",
				CastTo: "bool",
				Value:  events[i].IsRootReply,
			},
			{
				Name:   "is_quote",
				CastTo: "bool",
				Value:  events[i].IsQuote,
			},
			{
				Name:   "has_references",
				CastTo: "bool",
				Value:  events[i].HasReferences,
			},
			{
				Name:   "hidden",
				CastTo: "bool",
				Value:  false,
			},
			{
				Name:  "lookup",
				Value: events[i].Lookup,
			},
			{
				Name:   "expiration",
				CastTo: "bigint",
				Value:  events[i].Expiration,
			},
			{
				Name:  "replaced_by_id",
				Value: isReplay,
			},
		})
	}
	// `pre_update_data` contains the OLD state of the events that are being replaced.
	builder.WriteString(`),
pre_update_data AS (SELECT t.* FROM source_data sd LEFT JOIN events AS t ON t.address = sd.address),`)
	builder.WriteString(`
update_data AS (
	INSERT INTO events (
		id, kind, created_at,
		pubkey, master_pubkey,
		gift_receiver_pubkey,
		sig, sig_alg, key_alg,
		content,
		tags, t_tags,
		d_tag, h_tag,
		deleted,
		has_images, has_videos,
		is_reply, is_root_reply, is_quote, has_references,
		hidden,
		lookup,
		expiration
	)
	SELECT
		sd.id, sd.kind, sd.created_at,
		sd.pubkey, sd.master_pubkey,
		sd.gift_receiver_pubkey,
		sd.sig, sd.sig_alg, sd.key_alg,
		sd.content,
		sd.tags, sd.t_tags,
		sd.d_tag, sd.h_tag,
		sd.deleted,
		sd.has_images, sd.has_videos,
		sd.is_reply, sd.is_root_reply, sd.is_quote, sd.has_references,
		sd.hidden,
		sd.lookup,
		sd.expiration
	FROM source_data sd
	ON CONFLICT (address) DO UPDATE SET
		id = EXCLUDED.id,
		kind = EXCLUDED.kind,
		created_at = EXCLUDED.created_at,
		pubkey = EXCLUDED.pubkey,
		master_pubkey = EXCLUDED.master_pubkey,
		gift_receiver_pubkey = EXCLUDED.gift_receiver_pubkey,
		sig = EXCLUDED.sig,
		sig_alg = EXCLUDED.sig_alg,
		key_alg = EXCLUDED.key_alg,
		content = EXCLUDED.content,
		tags = EXCLUDED.tags,
		t_tags = EXCLUDED.t_tags,
		d_tag = EXCLUDED.d_tag,
		h_tag = EXCLUDED.h_tag,
		deleted = EXCLUDED.deleted,
		has_images = EXCLUDED.has_images,
		has_videos = EXCLUDED.has_videos,
		is_reply = EXCLUDED.is_reply,
		is_root_reply = EXCLUDED.is_root_reply,
		is_quote = EXCLUDED.is_quote,
		has_references = EXCLUDED.has_references,
		hidden = EXCLUDED.hidden,
		lookup = EXCLUDED.lookup,
		expiration = EXCLUDED.expiration
	RETURNING
		kind,
		created_at,
		id,
		pubkey,
		sig,
		content,
		tags,
		CASE 
			WHEN xmax = 0 THEN 'INSERT'
			ELSE 'UPDATE'
		END as savemergeaction
)`)
	builder.WriteString(`
SELECT
	*
FROM
	update_data AS ud
UNION ALL
SELECT
	pud.kind,
	pud.created_at,
	pud.id,
	pud.pubkey,
	pud.sig,
	pud.content,
	pud.tags,
	'OLD' as savemergeaction
FROM
	pre_update_data pud
WHERE
	pud.id is not null
`)
	return connector.ExecNamedManyWithCustomRetry[databaseEvent](
		ctx,
		db.db,
		func(err error) (doRetry bool) {
			return errors.IsAny(err, connector.ErrDuplicate, connector.ErrExclusionViolation)
		},
		builder.String(),
		builder.Params,
	)
}

func (db *dbClient) executeSave(ctx context.Context, req *databaseBatchRequest) (replaceableEvents map[string]bool, inserted []databaseFilterDelete, err error) {
	if len(req.InsertOrReplace) == 0 && len(req.Rollback) == 0 {
		return map[string]bool{}, []databaseFilterDelete{}, nil
	}
	events, sErr := db.saveEvents(ctx, req.InsertOrReplace, req.Rollback)
	if sErr != nil {
		sErr = errors.Wrap(handleError(sErr), "failed to exec insert event sql")
	}
	replaceableEvents = map[string]bool{}
	oldEvents := make(map[string]*model.Event, len(events))
	for i := range events {
		if events[i].SaveMergeAction == "OLD" {
			oldEvents[events[i].Address()] = events[i].Event
		} else if events[i].IsReplaceable() || events[i].IsAddressable() {
			replaceableEvents[events[i].ID] = events[i].SaveMergeAction == "INSERT"
		}
	}
	for i := range req.InsertOrReplace {
		addr := req.InsertOrReplace[i].Address()
		if oldEvent, hasOldEvent := oldEvents[addr]; hasOldEvent {
			req.InsertOrReplace[i].Previous = oldEvent
		}
	}
	if len(replaceableEvents) > 0 {
		keepOnlyInsertedEvents := func(event *databaseEvent) bool {
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
	if expectedRows, actual := len(req.Rollback), len(events)+len(replaceableEvents); sErr == nil && actual < expectedRows {
		sErr = errors.Wrapf(ErrUnexpectedRowsAffected, "expected %d rows affected, got %d", expectedRows, actual)
	}
	if sErr == nil && req.EventsHash != nil && (len(events)-len(oldEvents)) > 0 {
		for _, ev := range events {
			var f databaseFilterDelete

			f.Author = ev.PubKey
			if ev.IsRegular() {
				f.IDs = append(f.IDs, ev.ID)
			} else {
				f.Addresses = append(f.Addresses, ev.Address())
			}
			inserted = append(inserted, f)
		}
	}
	if replay := ctx.Value(model.ConsensusReplayCtxKey); replay != nil && replay.(bool) {
		replaceableEvents = map[string]bool{}
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
		err = errors.Join(err, sErr)
		if len(req.Delete) > 0 {
			if dErr := db.deleteEvents(ctx, req, &eventsToRollback); dErr != nil {
				err = errors.Join(err, errors.Wrap(dErr, "failed to delete events"))
			}
		}
	} else {
		if len(req.Delete) > 0 {
			if dErr := db.deleteEvents(ctx, req, &eventsToRollback); dErr != nil {
				err = errors.Join(err, errors.Wrap(dErr, "failed to delete events"))
			}
		}
		_, toRollbackDeleteOp, sErr := db.executeSave(ctx, req)
		eventsToRollback.Delete = append(eventsToRollback.Delete, toRollbackDeleteOp...)
		err = errors.Join(err, sErr)
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
		ev := databaseEvent{Event: new(model.Event)}
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
			{"expiration", nostr.Now().Add(model.DVMJobResultExpiration).String()},
		}
		db.MustSignEvent(event)
	}

	return event
}

func (db *dbClient) SelectEvents(ctx context.Context, filters ...model.Filter) EventIterator {
	return func(yield func(*model.Event, error) bool) {
		sqlQuery, params, err := db.generateSelectEventsSQL(ctx, filters...)
		if err != nil {
			yield(nil, errors.Wrap(err, "failed to generate select events SQL"))
			return
		}

		data, err := connector.SelectNamed[databaseEvent](ctx, db.db, sqlQuery, params)
		if err != nil {
			err = handleError(err)
			if errors.Is(err, connector.ErrNotFound) {
				err = nil
			}
			yield(nil, errors.Wrap(err, "failed to select events"))
			return
		}

		for i := range data {
			if !yield(db.eventTransform(data[i]).Event, nil) {
				return
			}
		}
	}
}

func handleError(err error) error {
	var sqlError *connector.Error

	if err == nil {
		return err
	}

	switch {
	case errors.Is(err, connector.ErrException):
		if errors.As(err, &sqlError) {
			switch sqlError.Message {
			case "attestation list update must be linear":
				return ErrAttestationUpdateRejected
			case "onbehalf permission denied":
				return ErrOnBehalfAccessDenied
			case "repost of deleted post":
				return ErrRepostOfDeletedPost
			}
		}
	case errors.Is(err, connector.ErrInvalidData):
		return ErrInvalidEvent
	case errors.Is(err, connector.ErrInternal):
		log.Printf("[DB] error: %v: %s", err, errors.FlattenDetails(err))
		return ErrInvalidRequest
	case errors.IsAny(err, connector.ErrDuplicate, connector.ErrExclusionViolation):
		return ErrRaceCondition
	case errors.Is(err, connector.ErrReadOnly):
		return ErrReadOnly
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
	where, params, err := newQueryBuilder().BuildSingleWhere(ctx, filters...)
	if err != nil {
		return "", nil, errors.Wrap(err, "failed to generate events where clause")
	}

	return `select count(id) from events e where ` + where, params, nil
}

func (db *dbClient) CountEvents(ctx context.Context, filters ...model.Filter) (int64, error) {
	sqlQuery, params, err := db.generateEventsCountClause(ctx, filters...)
	if err != nil {
		return -1, errors.Wrap(err, "failed to generate events where clause")
	}

	count, err := connector.GetNamed[int64](ctx, db.db, sqlQuery, params)
	if err != nil && !errors.Is(err, connector.ErrNotFound) {
		return -1, errors.Wrap(err, "failed to query events count")
	}
	if count == nil {
		return 0, nil
	}
	return *count, nil
}

func (db *dbClient) CountGroupedEventReactions(ctx context.Context, filters ...model.Filter) (string, error) {
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

	result, err := connector.GetNamed[string](ctx, db.db, sb.String(), params)
	if err != nil && !errors.Is(err, connector.ErrNotFound) {
		return "", errors.Wrap(err, "failed to query event reactions")
	}
	if result == nil {
		return "", nil
	}
	return *result, nil
}

func (db *dbClient) generateSelectEventsSQL(ctx context.Context, filter ...model.Filter) (sql string, params map[string]any, err error) {
	filters := db.extendWhereFilters(ctx, filter...)

	return newQueryBuilder().Build(ctx, filters...)
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
		SELECT
			id
		FROM
			events
		WHERE
			expiration <= :cutoff
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
	params := map[string]any{
		"batch_size": batchSize,
		"cutoff":     time.Now().UnixNano(),
	}

	for ctx.Err() == nil {
		events, err := connector.ExecNamed[model.Event](ctx, db.db, stmt, params)
		if err != nil {
			return errors.Wrap(handleError(err), "failed to exec delete expired events")
		}

		if notifyExpiredEvents != nil {
			if notifyErr := notifyExpiredEvents(ctx, events...); notifyErr != nil {
				log.Printf("failed to process notification of expired events: %v", notifyErr)
				// Continue to delete the events even if notification fails.
			}
		}

		if len(events) < batchSize {
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

func verifyEphemeralAttestation(embeddings []*model.EphemeralEmbeddingEvent, event *model.Event, req *databaseBatchRequest) error {
	var ephemeralAttestationEvent *model.Event
	for _, embedding := range embeddings {
		if embedding.ContentEvent.Kind == model.CustomIONKindAttestation && event.GetMasterPublicKey() == embedding.ContentEvent.GetMasterPublicKey() {
			ephemeralAttestationEvent = embedding.ContentEvent
			break
		}
	}
	if ephemeralAttestationEvent != nil {
		allowed, err := model.OnBehalfIsAccessAllowed(ephemeralAttestationEvent.Tags, event.PubKey, event.Kind, nostr.Now())
		if err != nil {
			return errors.Wrapf(err, "failed to parse attestation event")
		}
		if !allowed {
			return model.ErrOnBehalfAccessDenied
		}
		return req.Save(ephemeralAttestationEvent)
	}
	return nil
}

func eventValidForEphemeralAttestation(event *model.Event) bool {
	switch event.Kind {
	case model.CustomIONKindEditableTextNote, nostr.KindTextNote, nostr.KindArticle:
		// reply, quote or mention
		eTag := event.GetTag("e")
		if eTag != nil && eTag.Value() != "" && len(eTag) >= 4 && eTag[3] == model.TagMarkerReply {
			return true
		}
		refTags := []string{"q", "Q", "a", "p"}
		hasRefTags := false
		for _, tagName := range refTags {
			if tag := event.GetTag(tagName); tag != nil && tag.Value() != "" {
				hasRefTags = true
				break
			}
		}
		return hasRefTags
	case nostr.KindFollowList:
		return true
	case nostr.KindReaction:
		return true
	case nostr.KindGenericRepost, nostr.KindRepost:
		return true
	case nostr.KindDeletion:
		if kTag := event.GetTag("k"); kTag != nil {
			kValue, err := strconv.Atoi(kTag.Value())
			if err != nil {
				return false
			}
			switch kValue {
			case model.CustomIONKindEditableTextNote, nostr.KindTextNote, nostr.KindArticle,
				nostr.KindFollowList, nostr.KindReaction, nostr.KindGenericRepost, nostr.KindRepost:
				return true
			default:
				return false
			}
		}
		return false
	default:
		return false
	}
}

func (db *dbClient) queryDatabaseSize(ctx context.Context) (uint64, error) {
	sizePtr, err := connector.Get[uint64](ctx, db.db, `SELECT pg_database_size(current_database())`)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to query database size")
	}
	return *sizePtr, nil
}
