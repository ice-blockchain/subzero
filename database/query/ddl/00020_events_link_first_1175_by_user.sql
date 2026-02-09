-- SPDX-License-Identifier: ice License 1.0

CREATE OR REPLACE FUNCTION trigger_events_before_insert_tc_link_first_buy_action()
RETURNS TRIGGER AS $$
BEGIN
    NEW.first_1175_address := (
        SELECT
            e.id
        FROM
            events e
        INNER JOIN event_tags et ON et.event_id = e.id
        WHERE
            e.kind = 1175
            AND e.hidden = FALSE
            AND e.first_1175_address IS NULL
            AND e.master_pubkey = NEW.master_pubkey
            AND e.t_tags && ARRAY['community_token_position_reset']::TEXT[]
            AND et.event_tag_key IN ('e', 'a')
            AND et.event_tag_value1 = (
                SELECT
                    elem ->> 1 AS tag_value
                FROM
                    JSONB_ARRAY_ELEMENTS(NEW.tags) elem
                WHERE
                    elem ->> 0 = et.event_tag_key
                    AND JSONB_ARRAY_LENGTH(elem) > 1
                LIMIT 1
            )
            AND e.lookup_created_at <= to_timestamp_nano(NEW.created_at)
        ORDER BY
            e.lookup_created_at DESC
        LIMIT 1
    );

    RETURN NEW;
END;
$$ LANGUAGE PLPGSQL;

CREATE OR REPLACE TRIGGER trigger_events_before_insert_tc_link_first_buy_action
BEFORE INSERT ON events
FOR EACH ROW
WHEN (
    NEW.kind = 1175
    AND NEW.first_1175_address IS NULL
    AND (NOT NEW.t_tags && ARRAY['community_token_position_reset']::TEXT[])
    AND NEW.hidden = FALSE
)
EXECUTE FUNCTION trigger_events_before_insert_tc_link_first_buy_action();

DROP   INDEX IF     EXISTS idx_events_kind_master_pubkey_first_1175_address_lookup_created_at;
CREATE INDEX IF NOT EXISTS idx_events_kind_1175_t_reset_master_pubkey_lookup_created_at
    ON events (master_pubkey, lookup_created_at DESC)
    INCLUDE (id)
    WHERE
        kind = 1175
        AND hidden = FALSE
        AND first_1175_address IS NULL
        AND t_tags && ARRAY['community_token_position_reset']::TEXT[];

----- Old data migration to link existing 1175 events to their first reset event.
-- Reset first_1175_address to NULL for all 1175 "reset" events.
UPDATE events
SET
    first_1175_address = NULL
WHERE
    kind = 1175
    AND hidden = FALSE
    AND t_tags && ARRAY['community_token_position_reset']::TEXT[]
    AND first_1175_address IS NOT NULL;

-- For all non-reset 1175 events, link to the matching reset event.
UPDATE events u
SET
    first_1175_address = (
        SELECT
            e.id
        FROM
            events e
        INNER JOIN event_tags et ON et.event_id = e.id
        WHERE
            e.kind = 1175
            AND e.hidden = FALSE
            AND e.first_1175_address IS NULL
            AND (e.master_pubkey = u.master_pubkey)
            AND e.t_tags && ARRAY['community_token_position_reset']::TEXT[]
            AND et.event_tag_key IN ('e', 'a')
            AND et.event_tag_value1 = (
                SELECT
                    elem ->> 1 AS tag_value
                FROM
                    JSONB_ARRAY_ELEMENTS(u.tags) elem
                WHERE
                    elem ->> 0 = et.event_tag_key
                    AND JSONB_ARRAY_LENGTH(elem) > 1
                LIMIT 1
            )
            AND e.lookup_created_at <= u.lookup_created_at
        ORDER BY
            e.lookup_created_at DESC
        LIMIT 1
    )
WHERE
    u.kind = 1175
    AND u.hidden = FALSE
    AND (NOT u.t_tags && ARRAY['community_token_position_reset']::TEXT[])
    AND u.first_1175_address IS NULL;
