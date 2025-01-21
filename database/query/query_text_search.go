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
		if content := parseFts5Text(&ev.Event); content != "" {
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
		return errors.Wrapf(err, "can't delete already existing fts5 events: %v")
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

func parseFts5Text(ev *model.Event) string {
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
	case nostr.KindTextNote, nostr.KindArticle:
		content = fmt.Sprintf("%v %v", ev.Content, strings.Join(extractFTS5IMeta(ev.GetTags("imeta")), " "))
	case nostr.KindGenericRepost:
		if ev.Content == "" {
			return ""
		}
		var parsedEvent model.Event
		if err := json.Unmarshal([]byte(ev.Content), &parsedEvent); err != nil {
			return ""
		}
		if parsedEvent.Kind != nostr.KindArticle {
			return ""
		}
		content = fmt.Sprintf("%v %v", parsedEvent.Content, strings.Join(extractFTS5IMeta(parsedEvent.GetTags("imeta")), " "))
	case nostr.KindFileMetadata:
		content = fmt.Sprintf("%v %v", ev.Content, strings.Join(extractFTS5IMeta(ev.GetTags("imeta")), " "))
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

func searchWithoutDepsSQL(systemCreatedAtFilter, whereMain, limitQuery string) (sql string) {
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
		from events e
			join events_search es
				on es.event_id = e.id
		where (` + systemCreatedAtFilter + `(` + whereMain + `)) AND
			es.content MATCH :search
		ORDER BY rank, e.system_created_at desc
		` + limitQuery
}

func searchWithDepsSQL(systemCreatedAtFilter, whereMain, limitQuery, depClause string) (sql string) {
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
				from events e
				where ` + systemCreatedAtFilter + `(` + whereMain + `)
			` + limitQuery + `
			)
			select
				*
			from
			(
				select
					ev.kind,
					ev.created_at,
					ev.system_created_at,
					ev.id,
					ev.pubkey,
					ev.master_pubkey,
					ev.sig,
					ev.content,
					ev.d_tag,
					ev.h_tag,
					tags as jtags
				from events ev
					join events_search es
						on es.event_id = ev.id
				WHERE ev.id IN (
					select id from (
						select
							*
						from
							eventsmain
						` + depClause + `
					)
				) AND es.content MATCH :search
				ORDER BY rank, ev.system_created_at desc
			)`
}

func fts5CleanupText(text string) string {
	text = urlCleanupPattern.ReplaceAllString(text, "")
	text = fts5TextCleanupPattern.ReplaceAllString(text, "")
	text = fts5SpaceCleanupPattern.ReplaceAllString(text, " ")

	return strings.Trim(text, " ")
}
