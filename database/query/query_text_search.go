// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/model"
)

type (
	fts5DeleteRow struct {
		ID string
	}
	fts5SearchRow struct {
		ID      string
		Content string
	}
)

var (
	urlCleanupPattern       = regexp.MustCompile(`(https?://|http://)[\w./]+(?:\?[\w=&]+)?(?:#[\w./]+)?`)
	fts5TextCleanupPattern  = regexp.MustCompile(`\b(npub|nsec|nprofile|nostr:)\w*\b|#\w+|[^\w\s]`)
	fts5SpaceCleanupPattern = regexp.MustCompile(`\s{2,}`)
)

func (db *dbClient) saveFts5Events(ctx context.Context, events []databaseEvent) error {
	var searchEvents []fts5SearchRow
	var deleteEvents []fts5DeleteRow
	for _, ev := range events {
		if content := parseMetadataContent(&ev.Event); content != "" {
			searchEvents = append(searchEvents, fts5SearchRow{
				ID:      ev.ID,
				Content: content,
			})
			deleteEvents = append(deleteEvents, fts5DeleteRow{
				ID: ev.ID,
			})
		}
	}
	if len(searchEvents) == 0 {
		return nil
	}
	_, err := db.deleteFts5EventsSQL(ctx, deleteEvents)
	if err != nil {
		return errors.Wrapf(err, "can't delete already existing fts5 events: %v", events)
	}

	const stmt = `INSERT INTO events_search(event_id, content) VALUES (:id, :content)`

	result, err := db.NamedExecContext(ctx, stmt, searchEvents)
	if err != nil {
		err = errors.Wrap(db.handleError(err), "failed to exec insert fts5 event sql")
	} else if rows, err := result.RowsAffected(); err != nil {
		err = errors.Wrap(err, "failed to get rows affected")
	} else if expected := int64(len(events)); rows < expected {
		err = errors.Wrapf(ErrUnexpectedRowsAffected, "expected %d rows affected, got %d", expected, rows)
	}

	return err
}

func (db *dbClient) deleteFts5Events(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	var toDelete []fts5DeleteRow
	for _, id := range ids {
		toDelete = append(toDelete, fts5DeleteRow{ID: id})
	}
	rows, err := db.deleteFts5EventsSQL(ctx, toDelete)
	if err != nil {
		return errors.Wrapf(err, "can't delete fts5 events for: %v", ids)
	}
	if expected := int64(len(ids)); rows < expected {
		err = errors.Wrapf(ErrUnexpectedRowsAffected, "expected %d rows affected, got %d", expected, rows)
	}

	return nil
}

func (db *dbClient) deleteFts5EventsSQL(ctx context.Context, ids []fts5DeleteRow) (int64, error) {
	const stmt = `DELETE FROM events_search WHERE event_id = :id;`

	result, err := db.NamedExecContext(ctx, stmt, ids)
	if err != nil {
		return 0, errors.Wrap(db.handleError(err), "failed to exec delete fts5 record")
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, errors.Wrap(err, "failed to get rows affected")
	}

	return rows, err
}

func parseMetadataContent(ev *model.Event) string {
	content := ""
	switch ev.Kind {
	case nostr.KindProfileMetadata:
		if ev.Content == "" {
			return ""
		}
		var parsedContent model.ProfileMetadataContent
		if err := json.Unmarshal([]byte(ev.Content), &parsedContent); err != nil {
			return ""
		}
		content = fmt.Sprintf("%v %v", parsedContent.Name, parsedContent.DisplayName)
	case nostr.KindFileMetadata, nostr.KindTextNote, nostr.KindArticle, model.CustomIONKindEditableTextNote:
		content = strings.Join(extractFTS5IMeta(ev.GetTags("imeta")), " ")
	}

	return fts5CleanupText(content)
}

func extractFTS5IMeta(tags []model.Tag) []string {
	var imetaVals []string
	for _, tag := range tags {
		for _, val := range tag {
			if strings.HasPrefix(val, "alt") && val != "" {
				imetaVals = append(imetaVals, strings.TrimPrefix(val, "alt "))
			} else if strings.HasPrefix(val, "summary") && val != "" {
				imetaVals = append(imetaVals, strings.TrimPrefix(val, "summary "))
			}
		}
	}

	return imetaVals
}

func (db *dbClient) searchWithoutDepsSQL(whereMain, whereSearch, systemCreatedAtFilter, limitQuery string) (sql string, err error) {
	var preSearchWhere string
	if whereSearch != "" {
		preSearchWhere = ` (` + whereSearch + `) AND `
	}
	var where string
	if whereMain != "" {
		where = `AND (` + whereMain + `)`
	}
	systemCreatedAtWhere := systemCreatedAtFilter
	if systemCreatedAtFilter == "" {
		systemCreatedAtWhere = " e.system_created_at >= 0 AND "
	}

	sql = `with pre_search as (
			select
				e.id AS id
			from events e
			full outer join events ref
				on ref.reference_id = e.id
			join events_search es
					on es.rowid = e.rid
				where ` + systemCreatedAtWhere + preSearchWhere + ` events_search MATCH :search
		)
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
		from(
			select
				*
			from events e WHERE id IN (select id from pre_search) ` + where + `
			UNION ALL
			select
				*
			from events e WHERE reference_id IS NOT NULL AND reference_id IN(select id from pre_search)
		) e ` + limitQuery + `;`

	return sql, nil
}

func (db *dbClient) searchWithDepsSQL(whereMain, depClause, whereSearch, systemCreatedAtFilter, limitQuery string) (sql string, err error) {
	sql = `with eventsmain as (
				SELECT
					e.*
				FROM events e
				FULL OUTER JOIN events ref
					ON ref.reference_id = e.id
				JOIN events_search es
					ON es.rowid = e.rid
				WHERE ` + systemCreatedAtFilter + `(` + whereSearch + `) AND events_search MATCH :search
				` + limitQuery + `
			)
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
				tags AS jtags
			from
			(
				SELECT
					e.*
				FROM events e
				WHERE e.id IN (SELECT id FROM eventsmain)
				AND (` + whereMain + `)
				UNION ALL
				SELECT
					ev.*
				FROM events ev
				WHERE reference_id IS NOT NULL
				AND reference_id IN (SELECT id FROM eventsmain)
			) e 
			` + depClause + `;`

	return sql, nil
}

func fts5CleanupText(text string) string {
	text = urlCleanupPattern.ReplaceAllString(text, "")
	text = fts5TextCleanupPattern.ReplaceAllString(text, "")
	text = fts5SpaceCleanupPattern.ReplaceAllString(text, " ")

	return strings.Trim(text, " ")
}
