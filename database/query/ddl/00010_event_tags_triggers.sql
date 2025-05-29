-- SPDX-License-Identifier: ice License 1.0

CREATE OR REPLACE FUNCTION trigger_event_tags_after_insert_inc_counter()
RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO event_counters (reference_id, reference_type, kind, value)
    WITH is_community_closed AS (
        SELECT
            community.h_tag,
            CASE
                WHEN EXISTS (
                    SELECT 1
                    FROM events e
                    WHERE e.h_tag = community.h_tag AND e.kind = 1753 AND EXISTS (
                        SELECT 1
                        FROM jsonb_array_elements(e.tags) AS je(value)
                        WHERE value->>0 = 'closed'
                    )
                    ORDER BY e.created_at DESC
                    LIMIT 1
                ) OR EXISTS (
                    SELECT 1
                    FROM jsonb_array_elements(community.tags) AS je(value)
                    WHERE value->>0 = 'closed'
                ) THEN 1
                ELSE 0
            END AS closed_status
        FROM events community
        WHERE community.h_tag = NEW.event_tag_value1 AND community.kind = 31750
    )
    SELECT
        NEW.event_tag_value1,
        CASE
            WHEN e.kind = 1750 AND NEW.event_tag_key = 'h' THEN 'members'
            WHEN e.kind IN (1, 6, 16, 30023, 30175) AND NEW.event_tag_key IN ('a', 'e') AND NEW.event_tag_value3 = 'reply' THEN NEW.event_tag_value3
            WHEN e.kind IN (1, 6, 16, 30023, 30175) AND NEW.event_tag_key IN ('q', 'Q') THEN 'quote'
            WHEN e.kind = 3 AND NEW.event_tag_key = 'p' THEN 'follower'
            WHEN e.kind = 7 THEN e.content -- reaction type
            ELSE ''
        END,
        e.kind,
        1
    FROM
        events e
    LEFT JOIN events community ON community.h_tag = e.h_tag AND community.kind = 31750
    LEFT JOIN is_community_closed c ON c.h_tag = NEW.event_tag_value1
    WHERE
        e.id = NEW.event_id
        and e.deleted = false
        AND e.kind IN (1, 3, 6, 7, 16, 1750, 30023, 30175)
        AND (e.kind = 3 OR NEW.event_tag_key IN ('a', 'Q', 'h') OR EXISTS (SELECT 1 FROM events WHERE id = NEW.event_tag_value1))
        AND (
            CASE
                WHEN e.kind = 7 THEN
                    -- As per NIP25, we want only the value of the last `e` tag here OR `a` tag.
                    NEW.event_tag_value1 = (
                        WITH tags_data(tags) AS (
                            SELECT e.tags
                        )
                        SELECT
                            (array_agg(tag->>1) FILTER (WHERE tag->>0 = 'e'))[array_length(array_agg(tag->>1) FILTER (WHERE tag->>0 = 'e'), 1)] AS last_e_tag
                        FROM
                            tags_data,
                            jsonb_array_elements(tags) AS tag
                    ) OR NEW.event_tag_key = 'a'
                WHEN e.kind IN (1, 6, 16, 30023, 30175) AND NEW.event_tag_key IN ('a', 'e') AND NEW.event_tag_value3 != '' THEN
                    (NEW.event_tag_value3 = 'reply')
                WHEN e.kind = 1750 AND NEW.event_tag_key = 'h' AND NEW.event_tag_value1 = community.h_tag THEN
                    (
                        c.closed_status = 1 AND
                        (
                            EXISTS (
                                SELECT 1
                                FROM jsonb_array_elements(e.tags) AS je(value)
                                WHERE value->>0 = 'authorization'
                            )
                            OR (
                                (e.pubkey = community.pubkey OR e.master_pubkey = community.master_pubkey)
                                AND EXISTS (
                                    SELECT 1
                                    FROM jsonb_array_elements(e.tags) AS je(value)
                                    WHERE value->>0 = 'p' AND (value->>1 = community.pubkey OR value->>0 = community.master_pubkey)
                                )
                            )
                        )
                    )
                    OR c.closed_status = 0
                ELSE TRUE
            END
        )
    ON CONFLICT (kind, reference_type, reference_id) DO UPDATE
    SET value = event_counters.value + 1;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trigger_event_tags_after_insert_inc_counter
AFTER INSERT ON event_tags
FOR EACH ROW
WHEN (NEW.event_tag_key IN ('a', 'q', 'e', 'p', 'Q', 'h') AND NEW.event_tag_value1 != '')
EXECUTE FUNCTION trigger_event_tags_after_insert_inc_counter();
--------
CREATE OR REPLACE FUNCTION trigger_event_tags_after_delete_dec_counter()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE event_counters
    SET value = GREATEST(value - 1, 0)
    FROM events e
    LEFT JOIN events community ON community.h_tag = e.h_tag AND community.kind = 31750
    WHERE
        e.id = OLD.event_id
        AND event_counters.reference_id = OLD.event_tag_value1
        AND event_counters.kind = e.kind
        AND event_counters.reference_type = CASE
            WHEN e.kind IN (1, 6, 16, 30023, 30175) AND OLD.event_tag_key IN ('a', 'e') AND
                 (OLD.event_tag_value3 = 'reply') THEN OLD.event_tag_value3
            WHEN e.kind IN (1, 6, 16, 30023, 30175) AND OLD.event_tag_key IN ('q', 'Q') THEN 'quote'
            WHEN e.kind = 3 AND OLD.event_tag_key = 'p' THEN 'follower'
            WHEN e.kind = 7 THEN e.content
            WHEN e.kind = 1750 AND OLD.event_tag_key = 'h' THEN 'members'
            ELSE ''
        END
        AND (
            CASE
                WHEN e.kind = 7 THEN
                    OLD.event_tag_value1 = (
                         WITH tags_data(tags) AS (
                            SELECT e.tags
                        )
                        SELECT
                            (array_agg(tag->>1) FILTER (WHERE tag->>0 = 'e'))[array_length(array_agg(tag->>1) FILTER (WHERE tag->>0 = 'e'), 1)] AS last_e_tag
                        FROM
                            tags_data,
                            jsonb_array_elements(tags) AS tag
                    ) OR OLD.event_tag_key = 'a'
                WHEN e.kind = 1750 AND OLD.event_tag_key = 'h' AND OLD.event_tag_value1 = community.h_tag THEN
                    EXISTS (
                        SELECT 1
                        FROM jsonb_array_elements(e.tags) AS je(value)
                        WHERE value->>0 = 'p' AND (value->>1 = e.pubkey OR value->>0 = e.master_pubkey)
                    )
                ELSE TRUE
            END
        );

    DELETE FROM event_counters
    WHERE reference_id = OLD.event_tag_value1 AND value = 0;

    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trigger_event_tags_after_delete_dec_counter
AFTER DELETE ON event_tags
FOR EACH ROW
WHEN (OLD.event_tag_key IN ('a', 'q', 'e', 'p', 'Q', 'h') AND OLD.event_tag_value1 != '')
EXECUTE FUNCTION trigger_event_tags_after_delete_dec_counter();
--------
