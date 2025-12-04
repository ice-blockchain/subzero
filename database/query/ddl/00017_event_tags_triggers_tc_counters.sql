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
            WHEN e.kind IN (1, 6, 16, 30023, 30175, 31175) AND NEW.event_tag_key IN ('a', 'e') AND NEW.event_tag_value3 = 'reply' THEN NEW.event_tag_value3
            WHEN e.kind IN (1, 6, 16, 30023, 30175, 31175) AND NEW.event_tag_key IN ('q', 'Q') THEN 'quote'
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
        AND e.kind IN (1, 3, 6, 7, 16, 1750, 30023, 30175, 31175)
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
                WHEN e.kind IN (1, 6, 16, 30023, 30175, 31175) AND NEW.event_tag_key IN ('a', 'e') AND NEW.event_tag_value3 != '' THEN
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
CREATE OR REPLACE TRIGGER trigger_events_after_update_mark_refereces_deleted
AFTER UPDATE ON events
FOR EACH ROW
WHEN (new.deleted = true AND old.deleted = false AND ((new.tags != old.tags) OR (new.id != old.id)) AND new.kind in (30023, 30024, 30175, 31175) AND new.reference_id is null)
EXECUTE FUNCTION trigger_events_after_update_mark_refereces_deleted();
--------

---- ranked events and scoring system.
-- ADD.
CREATE OR REPLACE FUNCTION trigger_events_after_insert_score_add()
RETURNS TRIGGER AS $$
BEGIN
    with cte as (
        select
            je->>1 as event_address
        from
            jsonb_array_elements(NEW.tags) je
        where
            je->>0 in ('a', 'e', 'q', 'Q')
            and event_calculate_points(NEW.kind, NEW.is_quote, NEW.is_root_reply) > 0
            and case when NEW.is_root_reply then je->>3 = 'root' else true end
    )
    insert into ranked_events(event_id, event_kind, event_created_at, points, score)
    select
        e.id,
        e.kind,
        e.created_at,
        event_calculate_points(NEW.kind, NEW.is_quote, NEW.is_root_reply),
        event_calculate_score_int(event_calculate_points(NEW.kind, NEW.is_quote, NEW.is_root_reply), e.created_at)
    from
        events e
    inner join cte on e.address = cte.event_address
    where
        e.hidden = false
        and e.deleted = false
        and e.lookup_created_at > 0
        and e.kind in (1, 30023, 30175, 31175)
        and event_calculate_points(NEW.kind, NEW.is_quote, NEW.is_root_reply) > 0
    on conflict (event_id) do update
    set
        points = ranked_events.points + excluded.points,
        score  = event_calculate_score_int(ranked_events.points + excluded.points, excluded.event_created_at);
    return NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trigger_events_after_insert_score_add
AFTER INSERT ON events
FOR EACH ROW
WHEN (NEW.kind in (1, 6, 7, 16, 30023, 30175, 31175) and NEW.hidden = false and NEW.deleted = false)
EXECUTE FUNCTION trigger_events_after_insert_score_add();
--------
-- Decrement.
CREATE OR REPLACE FUNCTION trigger_events_after_delete_score_dec()
RETURNS TRIGGER AS $$
BEGIN
    with affected_events as (
        select
            e.id AS event_id
        from events e
        join lateral jsonb_array_elements(OLD.tags) je on true
        where
            je->>0 IN ('a', 'e', 'q', 'Q')
            and e.address = je->>1
            and e.hidden = false
            and e.deleted = false
            and e.lookup_created_at > 0
            and e.kind IN (1, 30023, 30175, 31175)
            and event_calculate_points(OLD.kind, OLD.is_quote, OLD.is_root_reply) > 0
            and case when OLD.is_root_reply then je->>3 = 'root' else true end
            limit 1
    )
    update ranked_events
    set
        points = points - event_calculate_points(OLD.kind, OLD.is_quote, OLD.is_root_reply),
        score  = event_calculate_score_int(points - event_calculate_points(OLD.kind, OLD.is_quote, OLD.is_root_reply), event_created_at)
    from
        affected_events
    where
        ranked_events.event_id = affected_events.event_id;
    delete from ranked_events where points <= 0;
    return OLD;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trigger_events_after_delete_score_dec
AFTER DELETE ON events
FOR EACH ROW
WHEN (OLD.kind in (1, 6, 7, 16, 30023, 30175, 31175) and OLD.hidden = false and OLD.deleted = false)
EXECUTE FUNCTION trigger_events_after_delete_score_dec();
--------
DROP INDEX IF EXISTS idx_events_verified_lookup_created_at_kind_expiration_is_null;
CREATE INDEX IF NOT EXISTS idx_events_verified_lookup_created_at_kind_expiration_is_nullv2 ON events (verified desc, lookup_created_at DESC, kind, expiration)
    WHERE expiration IS NULL AND hidden = FALSE AND deleted = false and kind in (1, 6, 16, 30175, 30023, 31175);
--------
