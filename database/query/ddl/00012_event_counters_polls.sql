-- SPDX-License-Identifier: ice License 1.0

CREATE OR REPLACE FUNCTION extract_reference_id(tags jsonb)
RETURNS TEXT AS $$
    SELECT
        elem ->> 1
    FROM
        jsonb_array_elements(tags) AS elem
    WHERE
        elem ->> 0 IN ('a', 'e')
        AND jsonb_array_length(elem) >= 2
    LIMIT 1;
$$ LANGUAGE sql IMMUTABLE;

CREATE OR REPLACE FUNCTION trigger_events_after_insert_vote_inc_counter()
RETURNS TRIGGER AS $$
DECLARE
    ref_id TEXT;
BEGIN
    ref_id := extract_reference_id(NEW.tags);

    IF ref_id IS NOT NULL THEN
        INSERT INTO event_counters (kind, value, reference_id, reference_type)
        SELECT
            NEW.kind,
            1,
            ref_id,
            v.value::text
        FROM
            jsonb_array_elements(NEW.content::jsonb) v
        ON CONFLICT (kind, reference_type, reference_id) DO UPDATE
        SET
            value = event_counters.value + 1;
    END IF;

    RETURN NEW;
EXCEPTION
    WHEN OTHERS THEN
        RAISE LOG 'Error in vote increment trigger: %', SQLERRM;
        RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trigger_events_after_insert_vote_inc_counter
AFTER INSERT ON events
FOR EACH ROW
WHEN (NEW.kind = 1754 AND NOT NEW.hidden AND NOT NEW.deleted and jsonb_array_length(NEW.content::jsonb) > 0)
EXECUTE FUNCTION trigger_events_after_insert_vote_inc_counter();
--------
CREATE OR REPLACE FUNCTION trigger_events_after_delete_vote_dec_counter()
RETURNS TRIGGER AS $$
DECLARE
    ref_id TEXT;
BEGIN
    ref_id := extract_reference_id(OLD.tags);

    IF ref_id IS NOT NULL THEN
        UPDATE event_counters
        SET
            value = GREATEST(0, event_counters.value - 1)
        WHERE
            kind = OLD.kind
            AND reference_id = ref_id
            AND reference_type = ANY(
                ARRAY(SELECT v.value::text FROM jsonb_array_elements(OLD.content::jsonb) v)
            );
    END IF;

    RETURN OLD;
EXCEPTION
    WHEN OTHERS THEN
        RAISE LOG 'Error in vote decrement trigger: %', SQLERRM;
        RETURN OLD;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trigger_events_after_delete_vote_dec_counter
AFTER DELETE ON events
FOR EACH ROW
WHEN (OLD.kind = 1754 AND NOT OLD.hidden AND NOT OLD.deleted and jsonb_array_length(OLD.content::jsonb) > 0)
EXECUTE FUNCTION trigger_events_after_delete_vote_dec_counter();
