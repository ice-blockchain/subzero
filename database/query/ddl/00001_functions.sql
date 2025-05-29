-- SPDX-License-Identifier: ice License 1.0

CREATE OR REPLACE FUNCTION to_timestamp_seconds(unix_time bigint)
RETURNS bigint AS $$
DECLARE
    digits integer;
BEGIN
    digits := length(unix_time::text);

    RETURN CASE
        WHEN digits <= 10 THEN unix_time                  -- already in seconds
        WHEN digits <= 13 THEN unix_time / 1000           -- milliseconds to seconds
        WHEN digits <= 16 THEN unix_time / 1000000        -- microseconds to seconds
        ELSE unix_time / 1000000000                       -- nanoseconds to seconds
    END;
END;
$$ LANGUAGE plpgsql IMMUTABLE;
--------
CREATE OR REPLACE FUNCTION to_timestamp_nano(unix_time bigint)
RETURNS bigint AS $$
DECLARE
    digits integer;
BEGIN
    digits := length(unix_time::text);

    RETURN CASE
        WHEN digits <= 10 THEN unix_time * 1000000000    -- seconds to nanoseconds.
        WHEN digits <= 13 THEN unix_time * 1000000       -- milliseconds to nanoseconds.
        WHEN digits <= 16 THEN unix_time * 1000          -- microseconds to nanoseconds.
        ELSE unix_time                                   -- already in nanoseconds.
    END;
END;
$$ LANGUAGE plpgsql IMMUTABLE;
--------
CREATE OR REPLACE FUNCTION get_current_timestamp_nano()
RETURNS bigint AS $$
BEGIN
    RETURN (EXTRACT(EPOCH FROM CURRENT_TIMESTAMP) * 1000000000)::bigint;
END;
$$ LANGUAGE plpgsql;
--------
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
$$ LANGUAGE plpgsql IMMUTABLE;
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
$$ LANGUAGE plpgsql IMMUTABLE;
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

    SELECT
        jsonb_agg(elem ORDER BY ordinality) INTO prefix
    FROM
        jsonb_array_elements(new_tags) WITH ORDINALITY AS t(elem, ordinality)
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
CREATE OR REPLACE FUNCTION event_calculate_score(p integer, created_at bigint) RETURNS REAL AS $$
DECLARE
    created_at_timestamp timestamp;
BEGIN
    created_at_timestamp := to_timestamp(to_timestamp_seconds(created_at));
    RETURN (round((p / power((1 + extract(EPOCH from (CURRENT_TIMESTAMP - least(CURRENT_TIMESTAMP, created_at_timestamp))) /3600.0), 0.9)), 4));
END;
$$ LANGUAGE plpgsql;
--------
CREATE OR REPLACE FUNCTION raise_repost_error() RETURNS integer AS $$
BEGIN
   RAISE EXCEPTION 'repost of deleted post';
   RETURN 42;
END;
$$ LANGUAGE plpgsql;
