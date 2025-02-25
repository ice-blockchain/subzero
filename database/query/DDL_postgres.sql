-- SPDX-License-Identifier: ice License 1.0

CREATE TABLE IF NOT EXISTS events
(
    id TEXT PRIMARY KEY,
    kind INTEGER NOT NULL,
    created_at BIGINT NOT NULL,
    system_created_at BIGINT NOT NULL,
    pubkey TEXT NOT NULL,
    master_pubkey TEXT NOT NULL,
    sig TEXT NOT NULL,
    sig_alg TEXT NOT NULL DEFAULT '',
    key_alg TEXT NOT NULL DEFAULT '',
    content TEXT NOT NULL,
    content_metadata TEXT NOT NULL DEFAULT '',
    d_tag TEXT NOT NULL DEFAULT '',
    h_tag TEXT NOT NULL DEFAULT '',
    reference_id TEXT DEFAULT NULL REFERENCES events (id) ON UPDATE CASCADE ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
    tags JSONB NOT NULL DEFAULT '[]',
    deleted BOOLEAN NOT NULL default FALSE,
    hidden BOOLEAN NOT NULL DEFAULT FALSE
    -- lookup tsvector NOT NULL -- TODO: 
);
--------
create unique index if not exists replaceable_event_uk on events(master_pubkey, kind)
where (10000 <= kind AND kind < 20000 ) OR kind = 0 OR kind = 3;
--------
create unique index if not exists parameterized_replaceable_event_uk on events(master_pubkey, kind, d_tag)
where 30000 <= kind AND kind < 40000;
--------
drop index if exists uix_events_h_tag;
create unique index if not exists transferable_replaceable_event_uk on events(h_tag)
where kind = 31750;
--------

-- Where order:
--   system_created_at
--   id
--   kind
--   pubkey
--   master_pubkey
--   created_at
-- Order by:
--   system_created_at DESC

CREATE INDEX IF NOT EXISTS idx_events_system_created_at ON events(system_created_at DESC) WHERE hidden = FALSE;
CREATE INDEX IF NOT EXISTS idx_events_kind_system_created_at ON events(kind, system_created_at DESC) WHERE hidden = FALSE;
CREATE INDEX IF NOT EXISTS idx_events_pubkey_system_created_at ON events(pubkey, system_created_at DESC) WHERE hidden = FALSE;
CREATE INDEX IF NOT EXISTS idx_events_master_pubkey_system_created_at ON events(master_pubkey, system_created_at DESC) WHERE hidden = FALSE;
CREATE INDEX IF NOT EXISTS idx_events_kind_pubkey_system_created_at ON events(kind, pubkey, system_created_at DESC) WHERE hidden = FALSE;
CREATE INDEX IF NOT EXISTS idx_events_kind_master_pubkey_system_created_at ON events(kind, master_pubkey, system_created_at DESC) WHERE hidden = FALSE;
CREATE INDEX IF NOT EXISTS idx_events_id_kind_system_created_at ON events(id, kind, system_created_at DESC) WHERE hidden = FALSE;
CREATE INDEX IF NOT EXISTS idx_events_id_created_at_system_created_at ON events(id, created_at DESC, system_created_at DESC) WHERE hidden = FALSE;
CREATE INDEX IF NOT EXISTS idx_events_id_pubkey_system_created_at ON events(id, pubkey, system_created_at DESC) WHERE hidden = FALSE;
CREATE INDEX IF NOT EXISTS idx_events_id_master_pubkey_system_created_at ON events(id, master_pubkey, system_created_at DESC) WHERE hidden = FALSE;
CREATE INDEX IF NOT EXISTS idx_events_id_kind_pubkey_created_at_system_created_at ON events(id, kind, pubkey, created_at DESC, system_created_at DESC) WHERE hidden = FALSE;
CREATE INDEX IF NOT EXISTS idx_events_id_kind_master_pubkey_created_at_system_created_at ON events(id, kind, master_pubkey, created_at DESC, system_created_at DESC) WHERE hidden = FALSE;
CREATE INDEX IF NOT EXISTS idx_events_system_created_at_id_created_at ON events(system_created_at DESC, id, created_at DESC) WHERE hidden = FALSE;
CREATE INDEX IF NOT EXISTS idx_events_reference_id_system_created_at ON events(reference_id, system_created_at DESC) WHERE hidden = FALSE;
CREATE INDEX IF NOT EXISTS idx_events_pubkey_master_pubkey_system_created_at ON events(pubkey, master_pubkey, system_created_at DESC) WHERE hidden = FALSE;
CREATE INDEX IF NOT EXISTS idx_events_h_tag_system_created_at  ON events(h_tag, system_created_at DESC) WHERE kind = 1753 AND hidden = FALSE;

-- Special index for inserts.
CREATE INDEX IF NOT EXISTS idx_events_reference_id ON events(reference_id);

-- TODO: 
-- CREATE EXTENSION IF NOT EXISTS btree_gin;
-- CREATE INDEX IF NOT EXISTS events_lookup_gin_idx ON events USING GIN (lookup);
--------
CREATE TABLE IF NOT EXISTS event_tags
(
    event_id          text not null references events (id) ON UPDATE RESTRICT ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
    event_tag_key     text not null,
    event_tag_value1  text not null DEFAULT '',
    event_tag_value2  text not null DEFAULT '',
    event_tag_value3  text not null DEFAULT '',
    event_tag_value4  text not null DEFAULT '',
    event_tag_value5  text not null DEFAULT '',
    event_tag_value6  text not null DEFAULT '',
    event_tag_value7  text not null DEFAULT '',
    event_tag_value8  text not null DEFAULT '',
    event_tag_value9  text not null DEFAULT '',
    event_tag_value10 text not null DEFAULT '',
    event_tag_value11 text not null DEFAULT '',
    event_tag_value12 text not null DEFAULT '',
    event_tag_value13 text not null DEFAULT '',
    event_tag_value14 text not null DEFAULT '',
    event_tag_value15 text not null DEFAULT '',
    event_tag_value16 text not null DEFAULT '',
    event_tag_value17 text not null DEFAULT '',
    event_tag_value18 text not null DEFAULT '',
    event_tag_value19 text not null DEFAULT '',
    event_tag_value20 text not null DEFAULT '',
    event_tag_value21 text not null DEFAULT '',
    primary key (event_id, event_tag_key, event_tag_value1)
);
--------
--- TODO: optimize index size and usage.
create index if not exists idx_event_tags_key_value1                  on event_tags(event_tag_key, event_tag_value1);
create index if not exists idx_event_tags_key_value1_expiration       on event_tags(event_tag_key, event_tag_value1) where (event_tag_key = 'expiration' and cast(event_tag_value1 as BIGINT) > 0);
create index if not exists idx_event_tags_key_value2                  on event_tags(event_tag_key, event_tag_value2);
create index if not exists idx_event_tags_key_value3                  on event_tags(event_tag_key, event_tag_value3);
create index if not exists idx_event_tags_id_key_value2               on event_tags(event_id, event_tag_key, event_tag_value2);
create index if not exists idx_event_tags_id_key_value1_value2        on event_tags(event_id, event_tag_key, event_tag_value1, event_tag_value2);
create index if not exists idx_event_tags_id_key_value1_value3        on event_tags(event_id, event_tag_key, event_tag_value1, event_tag_value3);
create index if not exists idx_event_tags_id_key_value1_value2_value3 on event_tags(event_id, event_tag_key, event_tag_value1, event_tag_value2, event_tag_value3);
--------
DROP TRIGGER IF EXISTS trigger_events_after_insert_generate_tags ON events;

CREATE OR REPLACE FUNCTION trigger_events_after_insert_generate_tags()
RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO event_tags (
        event_id,
        event_tag_key,
        event_tag_value1,
        event_tag_value2,
        event_tag_value3,
        event_tag_value4,
        event_tag_value5,
        event_tag_value6,
        event_tag_value7,
        event_tag_value8,
        event_tag_value9,
        event_tag_value10,
        event_tag_value11,
        event_tag_value12,
        event_tag_value13,
        event_tag_value14,
        event_tag_value15,
        event_tag_value16,
        event_tag_value17,
        event_tag_value18,
        event_tag_value19,
        event_tag_value20,
        event_tag_value21
    )
    SELECT
        NEW.id,
        value->>0,
        COALESCE(value->>1, ''),
        COALESCE(value->>2, ''),
        COALESCE(value->>3, ''),
        COALESCE(value->>4, ''),
        COALESCE(value->>5, ''),
        COALESCE(value->>6, ''),
        COALESCE(value->>7, ''),
        COALESCE(value->>8, ''),
        COALESCE(value->>9, ''),
        COALESCE(value->>10, ''),
        COALESCE(value->>11, ''),
        COALESCE(value->>12, ''),
        COALESCE(value->>13, ''),
        COALESCE(value->>14, ''),
        COALESCE(value->>15, ''),
        COALESCE(value->>16, ''),
        COALESCE(value->>17, ''),
        COALESCE(value->>18, ''),
        COALESCE(value->>19, ''),
        COALESCE(value->>20, ''),
        COALESCE(value->>21, '')
    FROM jsonb_array_elements(subzero_nostr_tags_reorder(COALESCE(NEW.tags, '[]'::jsonb))) AS value
    WHERE value->>0 IS NOT NULL
    ON CONFLICT(event_id, event_tag_key, event_tag_value1) DO NOTHING;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_events_after_insert_generate_tags
AFTER INSERT ON events
FOR EACH ROW
EXECUTE FUNCTION trigger_events_after_insert_generate_tags();
--------
DROP TRIGGER IF EXISTS trigger_events_before_update_remove_old_data ON events;
CREATE OR REPLACE FUNCTION trigger_events_before_update_remove_old_data()
RETURNS TRIGGER AS $$
BEGIN
    DELETE FROM event_tags WHERE event_id IN (NEW.id, OLD.id);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_events_before_update_remove_old_data
BEFORE UPDATE ON events
FOR EACH ROW
WHEN (NEW.tags != OLD.tags OR NEW.id != OLD.id)
EXECUTE FUNCTION trigger_events_before_update_remove_old_data();
--------
DROP TRIGGER IF EXISTS trigger_events_after_update_generate_tags ON events;
CREATE OR REPLACE FUNCTION trigger_events_after_update_generate_tags()
RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO event_tags (
        event_id,
        event_tag_key,
        event_tag_value1,
        event_tag_value2,
        event_tag_value3,
        event_tag_value4,
        event_tag_value5,
        event_tag_value6,
        event_tag_value7,
        event_tag_value8,
        event_tag_value9,
        event_tag_value10,
        event_tag_value11,
        event_tag_value12,
        event_tag_value13,
        event_tag_value14,
        event_tag_value15,
        event_tag_value16,
        event_tag_value17,
        event_tag_value18,
        event_tag_value19,
        event_tag_value20,
        event_tag_value21
    )
    SELECT
        NEW.id,
        value->>0,
        COALESCE(value->>1, ''),
        COALESCE(value->>2, ''),
        COALESCE(value->>3, ''),
        COALESCE(value->>4, ''),
        COALESCE(value->>5, ''),
        COALESCE(value->>6, ''),
        COALESCE(value->>7, ''),
        COALESCE(value->>8, ''),
        COALESCE(value->>9, ''),
        COALESCE(value->>10, ''),
        COALESCE(value->>11, ''),
        COALESCE(value->>12, ''),
        COALESCE(value->>13, ''),
        COALESCE(value->>14, ''),
        COALESCE(value->>15, ''),
        COALESCE(value->>16, ''),
        COALESCE(value->>17, ''),
        COALESCE(value->>18, ''),
        COALESCE(value->>19, ''),
        COALESCE(value->>20, ''),
        COALESCE(value->>21, '')
    FROM jsonb_array_elements(subzero_nostr_tags_reorder(COALESCE(NEW.tags, '[]'::jsonb))) AS value
    WHERE value->>0 IS NOT NULL
    ON CONFLICT(event_id, event_tag_key, event_tag_value1) DO NOTHING;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_events_after_update_generate_tags
AFTER UPDATE ON events
FOR EACH ROW
WHEN (NEW.tags != OLD.tags OR NEW.id != OLD.id)
EXECUTE FUNCTION trigger_events_after_update_generate_tags();
--------
DROP TRIGGER IF EXISTS trigger_events_before_insert_unwind_repost ON events;
DROP FUNCTION IF EXISTS trigger_events_before_insert_unwind_repost();

CREATE OR REPLACE FUNCTION trigger_events_before_insert_unwind_repost()
RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO events (
        kind,
        created_at,
        system_created_at,
        id,
        pubkey,
        master_pubkey,
        sig,
        content,
        content_metadata,
        tags,
        d_tag,
        h_tag,
        hidden
    )
    SELECT
        x.kind AS kind,
        0 AS created_at,
        0 AS system_created_at,
        x.id AS id,
        '' AS pubkey,
        '' AS master_pubkey,
        '' AS sig,
        x.content AS content,
        COALESCE(x.content_metadata, '') AS content_metadata,
        x.tags AS tags,
        '' AS d_tag,
        x.id AS h_tag,
        TRUE AS hidden
    FROM 
        jsonb_to_record(
            CASE 
                WHEN NEW.content != '' AND jsonb_valid(NEW.content) THEN NEW.content::JSONB
                ELSE '{}'::JSONB 
            END
        ) AS x(kind int, id TEXT, content TEXT, content_metadata TEXT, tags JSONB)
    WHERE NEW.content != '' AND jsonb_valid(NEW.content) AND NEW.content::JSONB ? 'kind'
    ON CONFLICT (id) DO NOTHING;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_events_before_insert_unwind_repost
BEFORE INSERT ON events
FOR EACH ROW
WHEN (NEW.kind IN (6, 16))
EXECUTE FUNCTION trigger_events_before_insert_unwind_repost();
--------
DROP TRIGGER IF EXISTS trigger_events_after_insert_link_repost ON events;
CREATE OR REPLACE FUNCTION trigger_events_after_insert_link_repost()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.kind IN (6, 16) AND NEW.content != '' AND jsonb_valid(NEW.content) AND NEW.content::jsonb ? 'id' THEN
        UPDATE events
        SET reference_id = NEW.content::jsonb ->> 'id'
        WHERE id = NEW.id;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_events_after_insert_link_repost
AFTER INSERT ON events
FOR EACH ROW
WHEN (NEW.kind IN (6, 16))
EXECUTE FUNCTION trigger_events_after_insert_link_repost();
--------
DROP TRIGGER IF EXISTS trigger_events_before_update_check_attestation_list_content ON events;

CREATE OR REPLACE FUNCTION trigger_events_before_update_check_attestation_list_content()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT subzero_nostr_attestation_update_is_allowed(
        COALESCE(OLD.tags, '[]'::jsonb),
        COALESCE(NEW.tags, '[]'::jsonb)
    ) THEN
        RAISE EXCEPTION 'attestation list update must be linear';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_events_before_update_check_attestation_list_content
BEFORE UPDATE ON events
FOR EACH ROW
WHEN (NEW.kind = 10100 AND NEW.tags IS DISTINCT FROM OLD.tags)
EXECUTE FUNCTION trigger_events_before_update_check_attestation_list_content();
--------
DROP TRIGGER IF EXISTS trigger_events_before_delete_remove_tags_explicit ON events;

CREATE OR REPLACE FUNCTION trigger_events_before_delete_remove_tags_explicit()
RETURNS TRIGGER AS $$
BEGIN
    DELETE FROM event_tags
    WHERE event_id = OLD.id;

    DELETE FROM event_counters
    WHERE reference_id = OLD.id;

    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_events_before_delete_remove_tags_explicit
BEFORE DELETE ON events
FOR EACH ROW
EXECUTE FUNCTION trigger_events_before_delete_remove_tags_explicit();
--------
CREATE TABLE IF NOT EXISTS event_counters
(
    reference_id   TEXT NOT NULL,
    reference_type TEXT NOT NULL DEFAULT '', -- for kind 7 events, it contains the actual reaction to the event, like `+` or `-`.
    kind           INTEGER NOT NULL,
    value          INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (kind, reference_type, reference_id)
);
--------
create index if not exists idx_event_counters_reference_id on event_counters(reference_id);
--------
DROP TRIGGER IF EXISTS trigger_event_tags_after_insert_inc_counter ON event_tags;

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
                    ORDER BY e.system_created_at DESC
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
            WHEN e.kind IN (1, 6, 16, 30023, 30175) AND NEW.event_tag_key IN ('a', 'e') AND NEW.event_tag_value3 IN ('reply', 'root') THEN NEW.event_tag_value3
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
                    ((NEW.event_tag_value3 = 'root' AND NEW.event_tag_value5 = '') OR (NEW.event_tag_value3 = 'reply'))
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

CREATE TRIGGER trigger_event_tags_after_insert_inc_counter
AFTER INSERT ON event_tags
FOR EACH ROW
WHEN (NEW.event_tag_key IN ('a', 'q', 'e', 'p', 'Q', 'h') AND NEW.event_tag_value1 != '')
EXECUTE FUNCTION trigger_event_tags_after_insert_inc_counter();
--------
DROP TRIGGER IF EXISTS trigger_event_tags_after_delete_dec_counter ON event_tags;

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
                 ((OLD.event_tag_value3 = 'root' AND OLD.event_tag_value5 = '') OR (OLD.event_tag_value3 = 'reply')) THEN OLD.event_tag_value3
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

CREATE TRIGGER trigger_event_tags_after_delete_dec_counter
AFTER DELETE ON event_tags
FOR EACH ROW
WHEN (OLD.event_tag_key IN ('a', 'q', 'e', 'p', 'Q', 'h') AND OLD.event_tag_value1 != '')
EXECUTE FUNCTION trigger_event_tags_after_delete_dec_counter();

--------
CREATE OR REPLACE FUNCTION subzero_nostr_onbehalf_is_allowed(
    master_tags JSONB,
    on_behalf_pubkey TEXT,
    kind INTEGER,
    now NUMERIC
) RETURNS BOOLEAN AS $$
DECLARE
    entries JSONB;
    entry JSONB;
    now_timestamp TIMESTAMP WITH TIME ZONE := TO_TIMESTAMP(now);
BEGIN
    IF master_tags IS NULL OR master_tags = '[]' THEN
        RETURN FALSE;
    END IF;

    entries := parse_attestation_tags(master_tags);
    entry := entries->on_behalf_pubkey;

    IF entry IS NULL OR entry->>'revoked' IS NOT NULL THEN
        RETURN FALSE;
    END IF;

    IF kind > 0 
        AND jsonb_array_length(entry->'kinds') > 0 
        AND NOT kind = ANY(
            ARRAY(SELECT CAST(value AS INTEGER) FROM jsonb_array_elements_text(entry->'kinds'))
        ) THEN
        RETURN FALSE;
    END IF;

     IF now_timestamp >= (entry->>'start')::TIMESTAMP WITH TIME ZONE AND (
        entry->>'end' IS NULL OR now_timestamp < (entry->>'end')::TIMESTAMP WITH TIME ZONE
    ) THEN
        RETURN TRUE;
    ELSE
        RETURN FALSE;
    END IF;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

CREATE OR REPLACE FUNCTION parse_attestation_tags(tags JSONB)
RETURNS JSONB AS $$
DECLARE
    attestation_tags JSONB := '{}'::JSONB;
    tag JSONB;
    action TEXT;
    ts TIMESTAMP WITH TIME ZONE;
    kinds INTEGER[];
BEGIN
    FOR tag IN SELECT jsonb_array_elements(tags) LOOP
        IF jsonb_array_length(tag) < 4 OR tag->>0 != 'p' THEN
            CONTINUE;
        END IF;

        SELECT * INTO action, ts, kinds FROM parse_attestation_string(tag->>3);

        CASE
            WHEN action = 'revoked' THEN
                attestation_tags := jsonb_set(attestation_tags, ARRAY[CAST(tag->>1 AS TEXT)], jsonb_build_object('revoked', ts));
            WHEN action = 'active' THEN
                attestation_tags := jsonb_set(attestation_tags, ARRAY[CAST(tag->>1 AS TEXT)], jsonb_build_object('start', ts, 'kinds', kinds));
            WHEN action = 'inactive' THEN
                attestation_tags := jsonb_set(attestation_tags, ARRAY[CAST(tag->>1 AS TEXT)], jsonb_build_object('end', ts));
        END CASE;
    END LOOP;

    RETURN attestation_tags;
END;
$$ LANGUAGE plpgsql IMMUTABLE;


CREATE OR REPLACE FUNCTION parse_attestation_string(s TEXT)
RETURNS TABLE (
    action TEXT,
    ts TIMESTAMP WITH TIME ZONE,
    kinds INTEGER[]
) AS $$
DECLARE
    action_end INT;
    ts_str TEXT;
    kinds_tokens TEXT[];
BEGIN
    action_end := position(':' IN s);
    IF action_end = 0 THEN
        RAISE EXCEPTION 'Invalid attestation string format: %', s;
    END IF;

    action := substr(s, 1, action_end - 1);

    ts_str := substr(s, action_end + 1);
    IF position(':' IN ts_str) > 0 THEN
        ts := TO_TIMESTAMP(CAST(substr(ts_str, 1, position(':' IN ts_str) - 1) AS BIGINT));
        kinds_tokens := string_to_array(substr(ts_str, position(':' IN ts_str) + 1), ',');
        kinds := ARRAY(SELECT CAST(kind_token AS INTEGER) FROM unnest(kinds_tokens) AS kind_token WHERE kind_token != '');
    ELSE
        ts := TO_TIMESTAMP(CAST(ts_str AS BIGINT));
        kinds := '{}'::INTEGER[];
    END IF;

    RETURN NEXT;
END;
$$ LANGUAGE plpgsql IMMUTABLE;
--------
CREATE OR REPLACE FUNCTION trigger_events_before_insert_check_onbehalf_permission()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.master_pubkey != NEW.pubkey THEN
        IF NOT subzero_nostr_onbehalf_is_allowed(
            COALESCE((
                SELECT tags 
                FROM events 
                WHERE kind = 10100 AND pubkey = NEW.master_pubkey AND hidden = FALSE
            ), '[]'::JSONB),
            NEW.pubkey,
            NEW.kind,
            EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)
        ) THEN
            RAISE EXCEPTION 'onbehalf permission denied';
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

drop trigger if exists trigger_events_before_insert_check_onbehalf_permission ON events;
CREATE TRIGGER trigger_events_before_insert_check_onbehalf_permission
BEFORE INSERT ON events
FOR EACH ROW
WHEN (NEW.master_pubkey IS DISTINCT FROM NEW.pubkey)
EXECUTE FUNCTION trigger_events_before_insert_check_onbehalf_permission();

--------
CREATE OR REPLACE FUNCTION event_tag_reorder(tag JSONB)
RETURNS JSONB AS $$
DECLARE
    reordered_tag JSONB := '[]';
    pair TEXT;
    key TEXT;
    value TEXT;
    start_index INT := 1;
    tag_length INT;
    current_index INT := 1;
	has_url BOOLEAN := FALSE;
	has_m BOOLEAN := FALSE;
	m_index TEXT := '1';
BEGIN
	IF jsonb_typeof(tag) != 'array' THEN
        RETURN '[]'::JSONB;
    END IF;

	tag_length := jsonb_array_length(tag);
    IF tag IS NULL OR tag_length = 0 THEN
        RETURN '[]'::JSONB;
    END IF;

    reordered_tag := jsonb_build_array(tag->>0);

	FOR i IN start_index .. tag_length - 1 LOOP
        pair := tag->>i;
		IF pair IS NULL THEN
            CONTINUE;
        END IF;

        SELECT split_part(pair, ' ', 1), split_part(pair, ' ', 2) INTO key, value;		
		if LOWER(key) = 'url' THEN
			has_url := TRUE;

            CONTINUE;
		END IF;
		if LOWER(key) = 'm' THEN
			has_m := TRUE;

            CONTINUE;
		END IF;

        reordered_tag := jsonb_insert(reordered_tag, ARRAY[CAST(current_index AS TEXT)], to_jsonb(pair), TRUE);
        current_index = current_index + 1;
	END LOOP;
		
    FOR i IN start_index .. tag_length - 1 LOOP
        pair := tag->>i;
		IF pair IS NULL THEN
            CONTINUE;
        END IF;

        SELECT split_part(pair, ' ', 1), split_part(pair, ' ', 2) INTO key, value;

        CASE
            WHEN LOWER(key) = 'url' THEN
                reordered_tag := jsonb_insert(reordered_tag, ARRAY['0'], to_jsonb(key || ' ' || value), TRUE);
            WHEN LOWER(key) = 'm' THEN
                IF current_index = 1 THEN
					reordered_tag := jsonb_set(
	                    tag,
	                    ARRAY[CAST('1' AS TEXT)],
	                    to_jsonb(CAST('' AS TEXT))
	                );
				END IF;
	   			IF has_url = TRUE AND has_m = TRUE THEN
					m_index := '1'; -- FIXME: can be wrong for some cases
				END IF;
				 
                reordered_tag := jsonb_insert(reordered_tag, ARRAY[m_index], to_jsonb(key || ' ' || value), TRUE);
            ELSE

        END CASE;
    END LOOP;

    RETURN reordered_tag;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

CREATE OR REPLACE FUNCTION subzero_nostr_tags_reorder(tags JSONB)
RETURNS JSONB AS $$
DECLARE
    reordered_tags JSONB := '[]';
	res_tags JSONB := '[]';
    tag JSONB;
    has_reply BOOLEAN := FALSE;
    reply_marker_index INT := 3;
    patch_marker_index INT := 5;
BEGIN
    IF tags IS NULL OR tags = '[]' THEN
        RETURN '[]'::JSONB;
    END IF;

    FOR tag IN SELECT jsonb_array_elements(tags) LOOP
        tag := event_tag_reorder(tag);

        IF (tag->>0)::TEXT IN ('e', 'a') 
           AND jsonb_array_length(tag) > reply_marker_index
           AND LOWER((tag->>reply_marker_index)::TEXT) = 'reply' THEN
            has_reply := TRUE;
        END IF;

		reordered_tags := jsonb_insert(reordered_tags, ARRAY['0'], tag, TRUE);
    END LOOP;

	FOR tag IN SELECT jsonb_array_elements(reordered_tags) LOOP
        IF has_reply
           AND (tag->>0)::TEXT IN ('e', 'a') 
           AND jsonb_array_length(tag) > reply_marker_index 
           AND LOWER((tag->>reply_marker_index)::TEXT) = 'root' THEN
            WHILE jsonb_array_length(tag) < patch_marker_index LOOP
                tag := jsonb_set(
                    tag,
                    ARRAY[CAST(jsonb_array_length(tag) AS TEXT)],
                    to_jsonb(CAST('' AS TEXT))
                );
            END LOOP;

           tag := jsonb_set(
                    to_jsonb(tag),
                    '{6}',
					to_jsonb(CAST('reply_of_root' AS TEXT))
               );
        END IF;

        res_tags := jsonb_insert(res_tags, ARRAY['0'], tag, TRUE);
	END LOOP;

    RETURN res_tags;
END;
$$ LANGUAGE plpgsql IMMUTABLE;
--------
CREATE OR REPLACE FUNCTION subzero_nostr_attestation_update_is_allowed(
    old_tags JSONB,
    new_tags JSONB
) RETURNS BOOLEAN AS $$
DECLARE
    revoked_pubkeys TEXT[] := '{}';
    tag JSONB;
    action TEXT;
    ts BIGINT;
    kinds INTEGER[];
BEGIN
    IF jsonb_array_length(new_tags) < jsonb_array_length(old_tags) THEN
        RETURN FALSE;
    END IF;

    FOREACH tag IN ARRAY jsonb_array_elements(old_tags) LOOP
        IF NOT (tag <@ new_tags) THEN
            RETURN FALSE;
        END IF;

        IF tag->>0 = 'p' AND jsonb_array_length(tag) >= 4 AND split_part(tag->>3, ':', 1) = 'revoked' THEN
            revoked_pubkeys := array_append(revoked_pubkeys, tag->>1);
        END IF;
    END LOOP;

    FOR i IN jsonb_array_length(old_tags) .. jsonb_array_length(new_tags) - 1 LOOP
        tag := new_tags->i;

        IF tag->>0 != 'p' THEN
            CONTINUE;
        END IF;

        IF jsonb_array_length(tag) < 4 THEN
            RAISE WARNING 'Malformed attestation tag: %', tag;
            RETURN FALSE;
        END IF;

        SELECT * INTO action, ts, kinds FROM parse_attestation_string(tag->>3);

        IF tag->>1 = ANY(revoked_pubkeys) THEN
            RAISE WARNING 'Found attestations for revoked pubkey: %', tag->>1;
            RETURN FALSE;
        END IF;

        IF split_part(action, ':', 1) = 'revoked' THEN
            revoked_pubkeys := array_append(revoked_pubkeys, tag->>1);
        END IF;
    END LOOP;

    RETURN TRUE;
END;
$$ LANGUAGE plpgsql IMMUTABLE;
--------
CREATE OR REPLACE FUNCTION parse_attestation_string(s TEXT)
RETURNS TABLE (
    action TEXT,
    ts TIMESTAMP WITH TIME ZONE,
    kinds INTEGER[]
) AS $$
DECLARE
    action_end INT;
    ts_str TEXT;
    kinds_tokens TEXT[];
BEGIN
    action_end := position(':' IN s);
    IF action_end = 0 THEN
        RAISE EXCEPTION 'Missing timestamp in attestation string: %', s;
    END IF;

    action := substr(s, 1, action_end - 1);

    ts_str := substr(s, action_end + 1);
    IF position(':' IN ts_str) > 0 THEN
        ts := TO_TIMESTAMP(CAST(substr(ts_str, 1, position(':' IN ts_str) - 1) AS BIGINT));
    ELSE
        ts := TO_TIMESTAMP(CAST(ts_str AS BIGINT));
    END IF;

    IF position(':' IN ts_str) > 0 THEN
        kinds_tokens := string_to_array(substr(ts_str, position(':' IN ts_str) + 1), ',');
        kinds := ARRAY(SELECT CAST(kind AS INTEGER) FROM unnest(kinds_tokens) AS kind);
    ELSE
        kinds := '{}';
    END IF;

    RETURN NEXT;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

--------
CREATE OR REPLACE FUNCTION subzero_nostr_tag_a_get_kind(tag TEXT)
RETURNS TEXT AS $$
DECLARE
    fields TEXT[];
BEGIN
    fields := string_to_array(tag, ':');

    IF array_length(fields, 1) <= 0 THEN
        RETURN '';
    END IF;

    RETURN fields[1];
END;
$$ LANGUAGE plpgsql IMMUTABLE;
--------
CREATE OR REPLACE FUNCTION subzero_nostr_tag_a_get_pk(tag TEXT)
RETURNS TEXT AS $$
DECLARE
    fields TEXT[];
BEGIN
    fields := string_to_array(tag, ':');

    IF array_length(fields, 1) <= 1 THEN
        RETURN '';
    END IF;

    RETURN fields[2];
END;
$$ LANGUAGE plpgsql IMMUTABLE;

--------
CREATE OR REPLACE FUNCTION subzero_nostr_tag_a_get_dtag(tag TEXT)
RETURNS TEXT AS $$
DECLARE
    fields TEXT[];
BEGIN
    fields := string_to_array(tag, ':');

    IF array_length(fields, 1) <= 2 THEN
        RETURN '';
    END IF;

    RETURN fields[3];
END;
$$ LANGUAGE plpgsql IMMUTABLE;
--------
CREATE OR REPLACE FUNCTION iif(condition BOOLEAN, true_value ANYELEMENT, false_value ANYELEMENT)
RETURNS ANYELEMENT AS $$
BEGIN
    RETURN CASE WHEN condition THEN true_value ELSE false_value END;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

--------
CREATE OR REPLACE FUNCTION subzero_nostr_get_event_address(
    event_id TEXT,
    kind INTEGER,
    master_pubkey TEXT,
    d_tag TEXT
) RETURNS TEXT AS $$
BEGIN
    IF kind >= 30000 AND kind < 40000 THEN
        RETURN kind::TEXT || ':' || master_pubkey || ':' || COALESCE(d_tag, '');

    ELSIF kind = 0 OR kind = 3 OR (kind >= 10000 AND kind < 20000) THEN
        RETURN kind::TEXT || ':' || master_pubkey || ':';

    ELSE
        RETURN event_id;
    END IF;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

--------
CREATE OR REPLACE FUNCTION jsonb_valid(text) RETURNS BOOLEAN AS $$
BEGIN
    RETURN ($1::JSONB IS NOT NULL);
EXCEPTION 
    WHEN invalid_text_representation THEN 
        RETURN FALSE;
END;
$$ LANGUAGE plpgsql IMMUTABLE;