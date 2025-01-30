// SPDX-License-Identifier: ice License 1.0

package query

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

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
		preSearchWhere = ` (` + whereSearch + `) `
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
				where ` + systemCreatedAtWhere + preSearchWhere + `
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
				WHERE ` + systemCreatedAtFilter + `(` + whereSearch + `)
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
