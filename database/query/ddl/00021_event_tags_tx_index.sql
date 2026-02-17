-- SPDX-License-Identifier: ice License 1.0

CREATE OR REPLACE FUNCTION is_indexable_tag_key(tag_key TEXT)
RETURNS BOOLEAN
LANGUAGE sql
IMMUTABLE
AS $$
    SELECT length(tag_key) = 1 OR tag_key IN ('summary', 'name', 'description', 'title', 'poll', 'ox', 'token', 'relay', 'tx_type')
$$;
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
        is_indexable_tag_key(value->>0)
    ON CONFLICT(event_id, event_tag_key, event_tag_value1, event_tag_value3) DO NOTHING;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
--------
CREATE OR REPLACE FUNCTION trigger_events_before_update_remove_old_data()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.deleted = TRUE AND OLD.deleted = FALSE THEN
        DELETE
        FROM
            event_tags
        WHERE
            event_id = OLD.id;
        RETURN NEW;
    END IF;

    WITH new_parsed_tags AS (
        SELECT
            value->>0 as event_tag_key,
            COALESCE(value->>1, '') as event_tag_value1,
            COALESCE(value->>2, '') as event_tag_value2,
            COALESCE(value->>3, '') as event_tag_value3,
            COALESCE(value->>4, '') as event_tag_value4,
            COALESCE(value->>5, '') as event_tag_value5
        FROM
            jsonb_array_elements(COALESCE(NEW.tags, '[]'::jsonb)) AS value
        WHERE
            is_indexable_tag_key(value->>0)
    )
    -- Delete old tags that are not present in the new set of tags.
    DELETE
    FROM
        event_tags et
    WHERE
        et.event_id = OLD.id
        AND (et.event_tag_key, et.event_tag_value1, et.event_tag_value3) IN (
            SELECT event_tag_key, event_tag_value1, event_tag_value3
            FROM event_tags
            WHERE event_id = OLD.id

            EXCEPT

            SELECT event_tag_key, event_tag_value1, event_tag_value3
            FROM new_parsed_tags
    );

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
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
        is_indexable_tag_key(value->>0)
    ON CONFLICT(event_id, event_tag_key, event_tag_value1, event_tag_value3) DO NOTHING;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
