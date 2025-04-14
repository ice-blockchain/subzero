-- SPDX-License-Identifier: ice License 1.0

-- systemKindQuote        = 1
-- systemKindCommentRoot  = 2
-- systemKindCommentReply = 3

DO
$$BEGIN
   CREATE TEXT SEARCH CONFIGURATION fts ( COPY = pg_catalog.english );
EXCEPTION
   WHEN unique_violation THEN NULL;
END;$$;

CREATE TABLE IF NOT EXISTS events (
    created_at     TIMESTAMP NOT NULL,
    kind           INTEGER   NOT NULL,
    system_kind    INTEGER,
    lookup         tsvector NOT NULL DEFAULT to_tsvector('fts', ''),
    key_alg        TEXT    NOT NULL DEFAULT '',
    content        TEXT    NOT NULL,
    d_tag          TEXT    NOT NULL DEFAULT '',
    h_tag          TEXT    NOT NULL DEFAULT '',
    address        TEXT    NOT NULL GENERATED ALWAYS AS (
                      CASE
                        WHEN (10000 <= kind AND kind < 20000) OR kind = 0 OR kind = 3
                          THEN coalesce(kind, 0) || ':' || coalesce(master_pubkey, pubkey, '') || ':'
                        WHEN 30000 <= kind AND kind < 40000
                          THEN coalesce(kind, 0) || ':' || coalesce(master_pubkey, pubkey, '') || ':' || coalesce(d_tag, '')
                        ELSE id
                      END
                  ) STORED,
    id             TEXT    PRIMARY KEY,
    pubkey         TEXT    NOT NULL,
    master_pubkey  TEXT    NOT NULL,
    sig            TEXT    NOT NULL,
    sig_alg        TEXT    NOT NULL DEFAULT '',
    reference_id   TEXT    DEFAULT NULL REFERENCES events (id) ON UPDATE CASCADE ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
    tags           JSONB   NOT NULL DEFAULT '[]',
    has_images     BOOLEAN NOT NULL DEFAULT FALSE,
    has_videos     BOOLEAN NOT NULL DEFAULT FALSE,
    deleted        BOOLEAN NOT NULL DEFAULT FALSE,
    hidden         BOOLEAN NOT NULL DEFAULT FALSE
);
--------
DO $$ BEGIN
    IF EXISTS (select true from information_schema.columns where table_name = 'events' and column_name = 'lookup' and is_generated = 'ALWAYS') then
        ALTER TABLE events ADD COLUMN lookup2 tsvector NOT NULL DEFAULT to_tsvector('fts', '');
        UPDATE      events SET lookup2 = lookup;
        ALTER TABLE events DROP COLUMN lookup;
        ALTER TABLE events RENAME lookup2 TO lookup;
    END IF;
END $$;
--------
create unique index if not exists replaceable_event_uk on events(master_pubkey, kind)
where (10000 <= kind AND kind < 20000 ) OR kind = 0 OR kind = 3;
--------
create unique index if not exists parameterized_replaceable_event_uk on events(master_pubkey, kind, d_tag)
where 30000 <= kind AND kind < 40000;
--------
create unique index if not exists transferable_replaceable_event_uk on events(h_tag)
where kind = 31750;
--------

-- Where order:
--   id
--   kind
--   pubkey
--   master_pubkey
--   created_at
-- Order by:
--   created_at DESC
CREATE INDEX IF NOT EXISTS idx_events_lookup ON events USING GIN(lookup);
CREATE INDEX IF NOT EXISTS idx_events_created_at ON events(created_at DESC) WHERE hidden = FALSE;
CREATE INDEX IF NOT EXISTS idx_events_id_created_at ON events(id, created_at DESC) WHERE hidden = FALSE;
CREATE INDEX IF NOT EXISTS idx_events_kind_created_at ON events(kind, created_at DESC) WHERE hidden = FALSE;
CREATE INDEX IF NOT EXISTS idx_events_pubkey_created_at ON events(pubkey, created_at DESC) WHERE hidden = FALSE;
CREATE INDEX IF NOT EXISTS idx_events_master_pubkey_created_at ON events(master_pubkey, created_at DESC) WHERE hidden = FALSE;
CREATE INDEX IF NOT EXISTS idx_events_kind_pubkey_created_at ON events(kind, pubkey, created_at DESC) WHERE hidden = FALSE;
CREATE INDEX IF NOT EXISTS idx_events_kind_master_pubkey_created_at ON events(kind, master_pubkey, created_at DESC) WHERE hidden = FALSE;
CREATE INDEX IF NOT EXISTS idx_events_id_kind_created_at ON events(id, kind, created_at DESC) WHERE hidden = FALSE;
CREATE INDEX IF NOT EXISTS idx_events_id_pubkey_created_at ON events(id, pubkey, created_at DESC) WHERE hidden = FALSE;
CREATE INDEX IF NOT EXISTS idx_events_id_master_pubkey_created_at ON events(id, master_pubkey, created_at DESC) WHERE hidden = FALSE;;
CREATE INDEX IF NOT EXISTS idx_events_pubkey_master_pubkey_created_at ON events(pubkey, master_pubkey, created_at DESC) WHERE hidden = FALSE;
CREATE INDEX IF NOT EXISTS idx_events_h_tag_created_at  ON events(h_tag, created_at DESC) WHERE kind = 1753 AND hidden = FALSE;

-- Special index for inserts.
CREATE INDEX IF NOT EXISTS idx_events_reference_id ON events(reference_id);

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
    primary key (event_id, event_tag_key, event_tag_value1)
);
--------
-- Update primary key.
DO $$ BEGIN
    IF NOT EXISTS (
        select
            true
        FROM
            pg_index,
            pg_class,
            pg_attribute,
            pg_namespace
        WHERE
            pg_class.oid = 'event_tags'::regclass
            AND indrelid = pg_class.oid
            AND nspname = 'public'
            AND pg_class.relnamespace = pg_namespace.oid
            AND pg_attribute.attrelid = pg_class.oid
            AND pg_attribute.attnum = any(pg_index.indkey)
            AND pg_attribute.attname = 'event_tag_value3'
            AND indisprimary
    ) THEN
        ALTER TABLE event_tags RENAME CONSTRAINT event_tags_pkey TO event_tags_pkeyold;
        CREATE UNIQUE INDEX event_tags_pkey ON event_tags (event_id, event_tag_key, event_tag_value1, event_tag_value3);
        ALTER TABLE event_tags DROP CONSTRAINT event_tags_pkeyold;
        ALTER TABLE event_tags ADD PRIMARY KEY USING INDEX event_tags_pkey;
    END IF;
END $$;
--------
--- TODO: optimize index size and usage.
create index if not exists idx_event_tags_key_value1                  on event_tags(event_tag_key, event_tag_value1);
create index if not exists idx_event_tags_key_value2                  on event_tags(event_tag_key, event_tag_value2);
create index if not exists idx_event_tags_key_value3                  on event_tags(event_tag_key, event_tag_value3);
create index if not exists idx_event_tags_id_key_value2               on event_tags(event_id, event_tag_key, event_tag_value2);
create index if not exists idx_event_tags_id_key_value1_value2        on event_tags(event_id, event_tag_key, event_tag_value1, event_tag_value2);
create index if not exists idx_event_tags_id_key_value1_value3        on event_tags(event_id, event_tag_key, event_tag_value1, event_tag_value3);
create index if not exists idx_event_tags_id_key_value1_value2_value3 on event_tags(event_id, event_tag_key, event_tag_value1, event_tag_value2, event_tag_value3);
create index if not exists idx_event_tags_key_value1_expiration       on event_tags(event_tag_key, to_timestamp(cast(event_tag_value1 as bigint))) where
    event_tag_key = 'expiration';
--------
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
        event_tag_value5
    )
    SELECT
        NEW.id,
        value->>0,
        COALESCE(value->>1, ''),
        COALESCE(value->>2, ''),
        COALESCE(value->>3, ''),
        COALESCE(value->>4, ''),
        COALESCE(value->>5, '')
    FROM jsonb_array_elements(COALESCE(NEW.tags, '[]'::jsonb)) AS value
    WHERE
        length(value->>0) = 1 OR value->>0 in ('expiration', 'summary', 'name', 'description', 'title', 'poll')
    ON CONFLICT(event_id, event_tag_key, event_tag_value1, event_tag_value3) DO NOTHING;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trigger_events_after_insert_generate_tags
AFTER INSERT ON events
FOR EACH ROW
EXECUTE FUNCTION trigger_events_after_insert_generate_tags();
--------
CREATE OR REPLACE FUNCTION trigger_events_before_update_remove_old_data()
RETURNS TRIGGER AS $$
BEGIN
    DELETE FROM event_tags WHERE event_id IN (NEW.id, OLD.id);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trigger_events_before_update_remove_old_data
BEFORE UPDATE ON events
FOR EACH ROW
WHEN (NEW.tags != OLD.tags OR NEW.id != OLD.id)
EXECUTE FUNCTION trigger_events_before_update_remove_old_data();
--------
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
        event_tag_value5
    )
    SELECT
        NEW.id,
        value->>0,
        COALESCE(value->>1, ''),
        COALESCE(value->>2, ''),
        COALESCE(value->>3, ''),
        COALESCE(value->>4, ''),
        COALESCE(value->>5, '')
    FROM jsonb_array_elements(COALESCE(NEW.tags, '[]'::jsonb)) AS value
    WHERE
        length(value->>0) = 1 OR value->>0 in ('expiration', 'summary', 'name', 'description', 'title', 'poll')
    ON CONFLICT(event_id, event_tag_key, event_tag_value1, event_tag_value3) DO NOTHING;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trigger_events_after_update_generate_tags
AFTER UPDATE ON events
FOR EACH ROW
WHEN (NEW.tags != OLD.tags OR NEW.id != OLD.id)
EXECUTE FUNCTION trigger_events_after_update_generate_tags();
--------
CREATE OR REPLACE FUNCTION raise_repost_error() RETURNS integer
   LANGUAGE plpgsql AS
$$BEGIN
   RAISE EXCEPTION 'repost of deleted post';
   RETURN 42;
END;$$;
--------
CREATE OR REPLACE FUNCTION trigger_events_before_insert_unwind_repost()
RETURNS TRIGGER AS $$
DECLARE
    val integer;
BEGIN
    INSERT INTO events (
        kind,
        created_at,
        id,
        pubkey,
        master_pubkey,
        sig,
        content,
        tags,
        d_tag,
        h_tag,
        hidden
    )
    SELECT
        x.kind AS kind,
        to_timestamp(0) AS created_at,
        x.id AS id,
        x.pubkey AS pubkey,
        COALESCE((SELECT value->>1 FROM jsonb_array_elements(x.tags) AS value WHERE value->>0 = 'b' LIMIT 1), '') AS master_pubkey,
        '' AS sig,
        x.content AS content,
        x.tags AS tags,
        COALESCE((SELECT value->>1 FROM jsonb_array_elements(x.tags) AS value WHERE value->>0 = 'd' LIMIT 1), '') AS d_tag,
        x.id AS h_tag,
        TRUE AS hidden
    FROM
        jsonb_to_record(
            CASE
                WHEN NEW.content != '' AND jsonb_valid(NEW.content) THEN NEW.content::JSONB
                ELSE '{}'::JSONB
            END
        ) AS x(kind int, pubkey TEXT, id TEXT, content TEXT, tags JSONB)
    WHERE NEW.content != '' AND jsonb_valid(NEW.content) AND NEW.content::JSONB ? 'kind'
    ON CONFLICT DO NOTHING;

    select
       raise_repost_error()
    into val
    from
        events x
    where
        jsonb_valid(NEW.content)
        and (x.id = NEW.content::JSONB->>'id' OR x.address = subzero_nostr_get_event_address_json(NEW.content::JSONB))
        and x.deleted = true;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trigger_events_before_insert_unwind_repost
BEFORE INSERT ON events
FOR EACH ROW
WHEN (NEW.kind IN (6, 16))
EXECUTE FUNCTION trigger_events_before_insert_unwind_repost();
--------
CREATE OR REPLACE FUNCTION trigger_events_after_insert_link_repost()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.kind IN (6, 16) AND NEW.content != '' AND jsonb_valid(NEW.content) AND NEW.content::jsonb ? 'id' THEN
        UPDATE events
        SET reference_id = NEW.content::jsonb ->> 'id'
        WHERE
            id = NEW.id
            AND EXISTS (select 1 from events ee where ee.id = NEW.content::jsonb ->> 'id');
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trigger_events_after_insert_link_repost
AFTER INSERT ON events
FOR EACH ROW
WHEN (NEW.kind IN (6, 16))
EXECUTE FUNCTION trigger_events_after_insert_link_repost();
--------
CREATE OR REPLACE FUNCTION trigger_events_after_update_mark_refereces_deleted()
RETURNS TRIGGER AS $$
BEGIN
    delete from events where reference_id in (new.id, old.id) and kind in (6, 16);
    return NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trigger_events_after_update_mark_refereces_deleted
AFTER UPDATE ON events
FOR EACH ROW
WHEN (new.deleted = true AND old.deleted = false AND ((new.tags != old.tags) OR (new.id != old.id)) AND new.kind in (30023, 30024, 30175) AND new.reference_id is null)
EXECUTE FUNCTION trigger_events_after_update_mark_refereces_deleted();
--------
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

CREATE OR REPLACE TRIGGER trigger_events_before_update_check_attestation_list_content
BEFORE UPDATE ON events
FOR EACH ROW
WHEN (NEW.kind = 10100 AND NEW.tags IS DISTINCT FROM OLD.tags)
EXECUTE FUNCTION trigger_events_before_update_check_attestation_list_content();
--------
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

CREATE OR REPLACE TRIGGER trigger_events_before_delete_remove_tags_explicit
BEFORE DELETE ON events
FOR EACH ROW
EXECUTE FUNCTION trigger_events_before_delete_remove_tags_explicit();
--------
CREATE TABLE IF NOT EXISTS replaceable_events_before_update
AS TABLE events;
DO $$ BEGIN
    ALTER TABLE replaceable_events_before_update
        ADD COLUMN IF NOT EXISTS replaced_by_id TEXT NOT NULL DEFAULT '';
    if NOT exists (select constraint_name from information_schema.table_constraints where table_name = 'replaceable_events_before_update' and constraint_type = 'PRIMARY KEY') then
        ALTER TABLE replaceable_events_before_update
            ADD CONSTRAINT replaceable_events_before_update_pkey PRIMARY KEY(id, replaced_by_id);
    end if;
    ALTER TABLE replaceable_events_before_update ALTER COLUMN replaced_by_id DROP DEFAULT;
END $$;
CREATE INDEX IF NOT EXISTS idx_replaceable_events_before_update_ ON replaceable_events_before_update(replaced_by_id);

CREATE OR REPLACE FUNCTION events_store_replaceable_data_before_update()
    RETURNS TRIGGER AS $$
BEGIN
    insert into replaceable_events_before_update (
        created_at,
        kind,
        system_kind,
        lookup,
        key_alg,
        content,
        d_tag,
        h_tag,
        address,
        id,
        pubkey,
        master_pubkey,
        sig,
        sig_alg,
        reference_id,
        tags,
        has_images,
        has_videos,
        deleted,
        hidden,
        replaced_by_id
    )
    values (
            old.created_at,
            old.kind,
            old.system_kind,
            old.lookup,
            old.key_alg,
            old.content,
            old.d_tag,
            old.h_tag,
            old.address,
            old.id,
            old.pubkey,
            old.master_pubkey,
            old.sig,
            old.sig_alg,
            old.reference_id,
            old.tags,
            old.has_images,
            old.has_videos,
            old.deleted,
            old.hidden,
            new.id
           )
    ON CONFLICT DO NOTHING;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
--------
CREATE OR REPLACE TRIGGER trigger_events_store_replaceable_data_before_update
    AFTER UPDATE ON events
    FOR EACH ROW
    WHEN (((10000 <= old.kind AND old.kind < 20000 ) OR old.kind = 0 OR old.kind = 3 OR (30000 <= old.kind AND old.kind < 40000)) AND old.id != new.id)
    EXECUTE FUNCTION events_store_replaceable_data_before_update();
--------
CREATE TABLE IF NOT EXISTS event_counters
(
    kind           INTEGER NOT NULL,
    value          INTEGER NOT NULL DEFAULT 0,
    reference_id   TEXT NOT NULL,
    reference_type TEXT NOT NULL DEFAULT '', -- for kind 7 events, it contains the actual reaction to the event, like `+` or `-`.
    PRIMARY KEY (kind, reference_type, reference_id)
);
--------
create index if not exists idx_event_counters_reference_id on event_counters(reference_id);
--------
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
CREATE OR REPLACE FUNCTION subzero_nostr_onbehalf_is_allowed(
    master_tags jsonb,
    on_behalf_pubkey text,
    kind integer
) RETURNS boolean AS $$
DECLARE
    entries         jsonb;
    entry           jsonb;
    now_ts          timestamp;
    start_ts        timestamp;
    end_ts          timestamp;
BEGIN
    IF kind = 10100 THEN
        RETURN false;
    END IF;
	entries := parse_attestation_tags(master_tags);
    entry := entries -> on_behalf_pubkey;
    IF entry IS NULL OR entry ? 'revoked' THEN
        RETURN false;
    END IF;

    IF kind > 0 THEN
        IF entry ? 'kinds' AND jsonb_array_length(entry->'kinds') > 0 THEN
            IF NOT (entry->'kinds') @> jsonb_build_array(kind) THEN
                RETURN false;
            END IF;
        END IF;
    END IF;

    now_ts := CURRENT_TIMESTAMP::timestamp;
    start_ts := (entry ->> 'start')::timestamp;
    end_ts := (entry ->> 'end')::timestamp;

    IF start_ts IS NULL THEN
        RETURN false;
    END IF;

    IF now_ts <= start_ts THEN
        RETURN false;
    END IF;

    IF end_ts IS NOT NULL AND now_ts >= end_ts THEN
        RETURN false;
    END IF;

    RETURN true;

EXCEPTION WHEN OTHERS THEN
    RETURN false;
END;
$$ LANGUAGE plpgsql;
--------
CREATE OR REPLACE FUNCTION parse_attestation_tags(tags jsonb)
RETURNS jsonb AS $$
DECLARE
    tag_item        jsonb;
    pubkey          text;
    action_str      text;
    parsed_action   text;
    parsed_ts       timestamp;
    parsed_kinds    integer[];
    current_entry   jsonb;
    entries         jsonb := '{}'::jsonb;
BEGIN
    FOR tag_item IN SELECT * FROM jsonb_array_elements(tags) LOOP
        IF jsonb_array_length(tag_item) < 4 OR tag_item->>0 <> 'p' THEN
            CONTINUE;
        END IF;

        pubkey := tag_item->>1;
        action_str := tag_item->>3;

        SELECT a.action, a.ts, a.kinds INTO parsed_action, parsed_ts, parsed_kinds
        FROM parse_attestation_string(action_str) a;

        IF NOT FOUND THEN
            CONTINUE;
        END IF;

        current_entry := coalesce(entries->pubkey, '{}'::jsonb);

        CASE parsed_action
            WHEN 'revoked' THEN
                current_entry := jsonb_set(current_entry, '{revoked}', to_jsonb(parsed_ts));

            WHEN 'active' THEN
                current_entry := jsonb_set(current_entry, '{start}', to_jsonb(parsed_ts));
                current_entry := jsonb_set(current_entry, '{end}', 'null'::jsonb);

                IF parsed_kinds IS NOT NULL THEN
                    current_entry := jsonb_set(current_entry, '{kinds}', to_jsonb(parsed_kinds));
                END IF;

            WHEN 'inactive' THEN
                current_entry := jsonb_set(current_entry, '{end}', to_jsonb(parsed_ts));

            ELSE
                CONTINUE;
        END CASE;

        entries := jsonb_set(entries, ARRAY[pubkey], current_entry);
    END LOOP;

    RETURN entries;
END;
$$ LANGUAGE plpgsql;

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
            NEW.pubkey::text,
            NEW.kind
        ) THEN
            RAISE EXCEPTION 'onbehalf permission denied';
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trigger_events_before_insert_check_onbehalf_permission
BEFORE INSERT OR UPDATE ON events
FOR EACH ROW
WHEN (NEW.master_pubkey != NEW.pubkey AND (NEW.pubkey != '' AND NEW.master_pubkey != ''))
EXECUTE FUNCTION trigger_events_before_insert_check_onbehalf_permission();
--------
CREATE OR REPLACE FUNCTION subzero_nostr_attestation_update_is_allowed(
    old_tags JSONB,
    new_tags JSONB
) RETURNS BOOLEAN AS $$
DECLARE
    old_len INT;
    new_len INT;
    tag JSONB;
    action_str TEXT;
    parsed_action TEXT;
    pubkey TEXT;
    revoked_pubkeys TEXT[] := '{}';
	prefix JSONB;
BEGIN
    old_len := jsonb_array_length(old_tags);
    new_len := jsonb_array_length(new_tags);

    IF new_len < old_len THEN
        RETURN FALSE;
    END IF;

    SELECT jsonb_agg(elem ORDER BY ordinality) INTO prefix
    FROM jsonb_array_elements(new_tags) WITH ORDINALITY AS t(elem, ordinality)
    WHERE ordinality <= old_len;

    IF old_tags <> prefix THEN
        RETURN FALSE;
    END IF;

    WITH old_attestations AS (
        SELECT elem->>1 AS pubkey
        FROM jsonb_array_elements(old_tags) elem
        WHERE
            elem->>0 = 'p' AND
            jsonb_array_length(elem) >= 4 AND
            split_part(elem->>3, ':', 1) = 'revoked'
    )
    SELECT array_agg(old_attestations.pubkey) INTO revoked_pubkeys FROM old_attestations;

    FOR i IN old_len..new_len-1 LOOP
        tag := new_tags->i;

        CONTINUE WHEN tag->>0 <> 'p';

        IF jsonb_array_length(tag) < 4 THEN
            RETURN FALSE;
        END IF;

        action_str := tag->>3;
        BEGIN
            SELECT a.action INTO parsed_action
            FROM parse_attestation_string(action_str) a;
        EXCEPTION WHEN OTHERS THEN
            RETURN FALSE;
        END;

        IF parsed_action IS NULL THEN
            RETURN FALSE;
        END IF;

        pubkey := tag->>1;

        IF pubkey = ANY(revoked_pubkeys) THEN
            RETURN FALSE;
        END IF;

        IF parsed_action = 'revoked' THEN
            revoked_pubkeys := revoked_pubkeys || pubkey;
        END IF;
    END LOOP;

    RETURN TRUE;

EXCEPTION WHEN OTHERS THEN
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql;
--------
CREATE OR REPLACE FUNCTION parse_attestation_string(input_str text)
RETURNS TABLE (
    action text,
    ts timestamp,
    kinds integer[]
) AS $$
DECLARE
    parts              text[];
    first_colon_pos    int;
    ts_str             text;
    ts_end_pos         int;
    unix_time          bigint;
    kinds_str          text;
    kind_element       text;
    temp_kinds         integer[];
BEGIN
    first_colon_pos := position(':' in input_str);
    IF first_colon_pos = 0 THEN
        RETURN;
    END IF;

    action := left(input_str, first_colon_pos - 1);
    ts_str := substring(input_str from first_colon_pos + 1);

    ts_end_pos := position(':' in ts_str);
    IF ts_end_pos = 0 THEN
        ts_end_pos := length(ts_str) + 1;
    END IF;

    BEGIN
        unix_time := substring(ts_str from 1 for ts_end_pos - 1)::bigint;
    EXCEPTION WHEN OTHERS THEN
        RETURN;
    END;

    ts := to_timestamp(unix_time);

    IF ts_end_pos <= length(ts_str) THEN
        kinds_str := substring(ts_str from ts_end_pos + 1);

        IF kinds_str = '' THEN
            RETURN;
        END IF;

        BEGIN
            temp_kinds := array(select unnest(string_to_array(kinds_str, ','))::integer);
        EXCEPTION WHEN OTHERS THEN
            RETURN;
        END;

        kinds := temp_kinds;
    ELSE
        kinds := NULL;
    END IF;

    RETURN NEXT;
END;
$$ LANGUAGE plpgsql;

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
CREATE OR REPLACE FUNCTION subzero_nostr_get_event_address_json(
    eventJSON JSONB
) RETURNS TEXT AS $$
DECLARE
    kind integer;
    pubkey text;
    dtag text;
BEGIN
    kind := (eventJSON->>'kind')::integer;

    SELECT value->>1 INTO dtag
    FROM jsonb_array_elements(eventJSON->'tags') AS value
    WHERE value->>0 = 'd'
    LIMIT 1;

    SELECT value->>1 INTO pubkey
    FROM jsonb_array_elements(eventJSON->'tags') AS value
    WHERE value->>0 = 'b'
    LIMIT 1;

    RETURN subzero_nostr_get_event_address(
        eventJSON->>'id',
        kind,
        COALESCE(pubkey, eventJSON->>'pubkey'),
        dtag
    );
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
--------
CREATE OR REPLACE FUNCTION subzero_nostr_get_event_address_tag(int) RETURNS TEXT AS $$
BEGIN
    RETURN case when ($1 >= 10000 AND $1 < 20000) OR $1 = 0 OR $1 = 3 OR ($1 >= 30000 AND $1 < 40000) then '#a' else '#e' end;
END;
$$ LANGUAGE plpgsql IMMUTABLE;
--------
CREATE TABLE IF NOT EXISTS ranked_events
(
    event_id          text      not null primary key REFERENCES events (id) ON UPDATE CASCADE ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
    event_kind        integer   not null,
    points            integer   not null,
    event_created_at  timestamp not null,
    score             real      not null
);
--------
create index if not exists ranked_events_points_ix           on ranked_events(points) where points <= 0;
create index if not exists ranked_events_score_ix            on ranked_events(score desc);
create index if not exists ranked_events_created_at_score_ix on ranked_events(event_created_at desc, score desc);
--------
CREATE OR REPLACE FUNCTION event_calculate_score(p integer, created_at timestamp) RETURNS REAL AS $$
BEGIN
    RETURN (round((p / power((1 + extract(EPOCH from (CURRENT_TIMESTAMP - least(CURRENT_TIMESTAMP, created_at))) /3600.0), 0.9)), 4));
END;
$$ LANGUAGE plpgsql;
--------
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
            and (NEW.system_kind is null or case
                when NEW.system_kind = 2 then je->>3 = 'root'
                when NEW.system_kind = 3 then false -- ignore replies
                else true
            end)
    )
    insert into ranked_events(event_id, event_kind, event_created_at, points, score)
    select
        e.id,
        e.kind,
        e.created_at,
        case
            when NEW.kind = 7 then 1                                        -- like
            when NEW.kind in (6, 16) then 3                                 -- repost
            when NEW.system_kind is not null and NEW.system_kind = 1 then 4 -- quote
            when NEW.system_kind is not null and NEW.system_kind = 2 then 2 -- top level comment (root)
            else 0
        end,
        event_calculate_score(case
            when NEW.kind = 7 then 1
            when NEW.kind in (6, 16) then 3
            when NEW.system_kind is not null and NEW.system_kind = 1 then 4
            when NEW.system_kind is not null and NEW.system_kind = 2 then 2
            else 0
        end, e.created_at)
    from
        events e
    inner join cte on e.address = cte.event_address
    where
        e.hidden = false
        and e.deleted = false
        and e.created_at > to_timestamp(0)
        and e.kind in (1, 30023, 30175)
        and (NEW.system_kind is null or NEW.system_kind != 3)
    on conflict (event_id) do update
    set
        points = ranked_events.points + excluded.points,
        score  = event_calculate_score(ranked_events.points + excluded.points, excluded.event_created_at);
    return NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trigger_events_after_insert_score_add
AFTER INSERT ON events
FOR EACH ROW
WHEN (NEW.kind in (1, 6, 7, 16, 30023, 30175) and NEW.hidden = false and NEW.deleted = false)
EXECUTE FUNCTION trigger_events_after_insert_score_add();
--------
CREATE OR REPLACE FUNCTION trigger_events_after_delete_score_dec()
RETURNS TRIGGER AS $$
BEGIN
    update ranked_events
    set
        points = points - case
            when OLD.kind = 7 then 1                                        -- like
            when OLD.kind in (6, 16) then 3                                 -- repost
            when OLD.system_kind is not null and OLD.system_kind = 1 then 4 -- quote
            when OLD.system_kind is not null and OLD.system_kind = 2 then 2 -- top level comment (root)
            else 0
        end,
        score = event_calculate_score(points - case
            when OLD.kind = 7 then 1
            when OLD.kind in (6, 16) then 3
            when OLD.system_kind is not null and OLD.system_kind = 1 then 4
            when OLD.system_kind is not null and OLD.system_kind = 2 then 2
            else 0
        end, event_created_at)
    where exists (
        select 1
        from jsonb_array_elements(OLD.tags) je
        where
        je->>0 in ('a', 'e', 'q', 'Q')
        and event_id in (
            select e.id
            from events e
            where e.address = je->>1
            and e.hidden = false
            and e.deleted = false
            and e.created_at > to_timestamp(0)
            and e.kind in (1, 30023, 30175)
            and (OLD.system_kind is null or OLD.system_kind != 3)
            and (OLD.system_kind is null or case
                when OLD.system_kind = 2 then je->>3 = 'root'
                when OLD.system_kind = 3 then false
                else true
            end)
        )
    );
    delete from ranked_events where points <= 0;
    return OLD;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trigger_events_after_delete_score_dec
AFTER DELETE ON events
FOR EACH ROW
WHEN (OLD.kind in (1, 6, 7, 16, 30023, 30175) and OLD.hidden = false and OLD.deleted = false)
EXECUTE FUNCTION trigger_events_after_delete_score_dec();
--------
CREATE OR REPLACE FUNCTION trigger_events_after_update_score_add()
RETURNS TRIGGER AS $$
BEGIN
    update ranked_events
    set
        points = points - case
            when OLD.kind = 7 then 1                                        -- like
            when OLD.kind in (6, 16) then 3                                 -- repost
            when OLD.system_kind is not null and OLD.system_kind = 1 then 4 -- quote
            when OLD.system_kind is not null and OLD.system_kind = 2 then 2 -- top level comment (root)
            else 0
        end,
        score = event_calculate_score(points - case
            when OLD.kind = 7 then 1
            when OLD.kind in (6, 16) then 3
            when OLD.system_kind is not null and OLD.system_kind = 1 then 4
            when OLD.system_kind is not null and OLD.system_kind = 2 then 2
            else 0
        end, event_created_at)
    where exists (
        select 1
        from jsonb_array_elements(OLD.tags) je
        where
        je->>0 in ('a', 'e', 'q', 'Q')
        and event_id in (
            select e.id
            from events e
            where e.address = je->>1
            and e.hidden = false
            and e.created_at > to_timestamp(0)
            and e.kind in (1, 30023, 30175)
            and (OLD.system_kind is null or OLD.system_kind != 3)
            and (OLD.system_kind is null or case
                when OLD.system_kind = 2 then je->>3 = 'root'
                when OLD.system_kind = 3 then false
                else true
            end)
        )
    );
    with cte as (
        select
            je->>1 as event_address
        from
            jsonb_array_elements(NEW.tags) je
        where
            je->>0 in ('a', 'e', 'q', 'Q')
            and (NEW.system_kind is null or case
                when NEW.system_kind = 2 then je->>3 = 'root'
                when NEW.system_kind = 3 then false -- ignore replies
                else true
            end)
    )
    insert into ranked_events(event_id, event_kind, event_created_at, points, score)
    select
        e.id,
        e.kind,
        e.created_at,
        case
            when NEW.kind = 7 then 1                                        -- like
            when NEW.kind in (6, 16) then 3                                 -- repost
            when NEW.system_kind is not null and NEW.system_kind = 1 then 4 -- quote
            when NEW.system_kind is not null and NEW.system_kind = 2 then 2 -- top level comment (root)
            else 0
        end,
        event_calculate_score(case
            when NEW.kind = 7 then 1
            when NEW.kind in (6, 16) then 3
            when NEW.system_kind is not null and NEW.system_kind = 1 then 4
            when NEW.system_kind is not null and NEW.system_kind = 2 then 2
            else 0
        end, e.created_at)
    from
        events e
    inner join cte on e.address = cte.event_address
    where
        e.hidden = false
        and e.deleted = false
        and e.created_at > to_timestamp(0)
        and e.kind in (1, 30023, 30175)
        and NEW.deleted = false
        and (NEW.system_kind is null or NEW.system_kind != 3)
    on conflict (event_id) do update
    set
        points = ranked_events.points + excluded.points,
        score  = event_calculate_score(ranked_events.points + excluded.points, excluded.event_created_at);
    delete from ranked_events where points <= 0;
    return NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trigger_events_after_update_score_add
AFTER UPDATE ON events
FOR EACH ROW
WHEN (((NEW.kind in (1, 6, 7, 16, 30023, 30175)) and (NEW.hidden = false) and (NEW.tags != OLD.tags)) OR (OLD.deleted != NEW.deleted))
EXECUTE FUNCTION trigger_events_after_update_score_add();