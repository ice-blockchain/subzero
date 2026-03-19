// SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/rs/zerolog/log"

	"github.com/ice-blockchain/subzero/database/query/internal/connector"
)

type (
	TokenActivityData struct {
		Definition string
		FirstBuy   string
		Counter    uint64
	}
)

func (client *dbClient) CollectTokenActivityCandidates(ctx context.Context, now time.Time, startID, limit uint64) (defCandidates []string, lastID uint64, err error) {
	type record struct {
		ID      uint64
		Address string
	}
	const query = `
WITH to_claim AS (
	SELECT
		id
	FROM
		pn_token_activity
	WHERE
		window_end_at < :now
		AND action_counter >= cfg_count_threshold
		AND (last_processed_at IS NULL OR last_processed_at < window_end_at)
		AND (last_notified_at IS NULL OR last_notified_at < window_start_at)
		AND id > :startID
	ORDER BY id ASC
	LIMIT :limit
	FOR UPDATE SKIP LOCKED
)
UPDATE
	pn_token_activity p
SET
	last_processed_at = :now
FROM
	to_claim c
WHERE
	p.id = c.id
RETURNING
	p.id,
	p.tc_definition_address as address
`

	rows, err := connector.ExecNamed[record](ctx, client.db, query, map[string]any{
		"now":     now.Unix(),
		"startID": startID,
		"limit":   limit,
	})
	if err != nil {
		return nil, startID, errors.Wrap(err, "failed to collect token activity candidates")
	} else if len(rows) == 0 {
		return nil, startID, nil
	}

	defCandidates = make([]string, 0, len(rows))
	lastID = startID
	for _, row := range rows {
		defCandidates = append(defCandidates, row.Address)
		lastID = row.ID
	}

	log.Trace().
		Str("context", "DB").
		Time("now", now).
		Int("count", len(defCandidates)).
		Uint64("last_id", lastID).
		Msg("collected token activity candidates")

	return defCandidates, lastID, nil
}

func (client *dbClient) FetchAndUpdateTokenActivityNotification(ctx context.Context, now time.Time, tokens []string) ([]*TokenActivityData, error) {
	const query = `
WITH current_tokens AS (
	SELECT
		id,
		tc_definition_address,
		tc_first_action_address,
		window_start_at,
		action_counter
	FROM
		pn_token_activity
	WHERE
		tc_definition_address = ANY(:tokens)
		AND window_end_at < :now
		AND action_counter >= cfg_count_threshold
		AND (last_notified_at IS NULL OR (:now-last_notified_at) > cfg_time_window)
	FOR UPDATE SKIP LOCKED
),
updated_data AS (
	UPDATE pn_token_activity
	SET
		last_notified_at = :now,
		action_counter   = 0
	FROM current_tokens
	WHERE
		pn_token_activity.id = current_tokens.id
	RETURNING
		pn_token_activity.id,
		pn_token_activity.tc_definition_address,
		pn_token_activity.tc_first_action_address,
		current_tokens.action_counter
)
SELECT
	jsonb_build_object(
		'id', def.id,
		'pubkey', def.pubkey,
		'created_at', def.created_at,
		'kind', def.kind,
		'tags', def.tags,
		'content', def.content,
		'sig', def.sig
	) AS definition,
	jsonb_build_object(
		'id', act.id,
		'pubkey', act.pubkey,
		'created_at', act.created_at,
		'kind', act.kind,
		'tags', act.tags,
		'content', act.content,
		'sig', act.sig
	) AS firstbuy,
	action_counter as counter
FROM
	updated_data ud
INNER JOIN events def ON def.address = ud.tc_definition_address
INNER JOIN events act ON act.address = ud.tc_first_action_address
`

	rows, err := connector.ExecNamed[TokenActivityData](ctx, client.db, query, map[string]any{
		"now":    now.Unix(),
		"tokens": tokens,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch and update token activity notification")
	}
	return rows, nil
}
