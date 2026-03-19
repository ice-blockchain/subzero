-- SPDX-License-Identifier: ice License 1.0

CREATE TABLE IF NOT EXISTS pn_token_activity (
    id                          BIGSERIAL PRIMARY KEY,
    last_notified_at            BIGINT,           -- unix timestamp in seconds.
    last_processed_at           BIGINT,           -- unix timestamp in seconds.
    window_start_at             BIGINT  NOT NULL, -- unix timestamp in seconds.
    window_end_at               BIGINT  NOT NULL, -- unix timestamp in seconds.
    action_counter              BIGINT  NOT NULL CHECK (action_counter >= 0),
    cfg_time_window             INTEGER NOT NULL CHECK (cfg_time_window > 0 AND cfg_time_window <= 31622400), -- seconds.
    cfg_count_threshold         INTEGER NOT NULL CHECK (cfg_count_threshold > 0),
    tc_definition_address       TEXT    NOT NULL REFERENCES events (address) ON UPDATE CASCADE ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
    tc_first_action_address     TEXT    NOT NULL REFERENCES events (address) ON UPDATE CASCADE ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
    UNIQUE (tc_definition_address)
) WITH (FILLFACTOR = 70);
------
CREATE OR REPLACE FUNCTION subzero_get_pn_token_activity_config()
RETURNS TABLE (
    cfg_time_window INTEGER,
    cfg_count_threshold INTEGER
)
LANGUAGE sql
IMMUTABLE
AS $$
    SELECT
        86400 AS cfg_time_window,
        30 AS cfg_count_threshold;
$$;
------
CREATE OR REPLACE FUNCTION subzero_get_tx_type(tags jsonb)
RETURNS TEXT
LANGUAGE sql
IMMUTABLE
RETURNS NULL ON NULL INPUT
AS $$
    SELECT
        tag->>1
    FROM
        jsonb_array_elements(tags) AS tag
    WHERE
        tag->>0 = 'tx_type'
    ORDER BY
        tag->>1
    LIMIT 1;
$$;
------
CREATE OR REPLACE FUNCTION trigger_events_after_insert_track_token_activity()
RETURNS TRIGGER AS $$
DECLARE
    token_address               TEXT;
BEGIN
    token_address := subzero_get_first_a_tag_value(NEW.tags, 31175);
    IF token_address = '' OR token_address IS NULL THEN
        RETURN NEW;
    END IF;

    MERGE INTO pn_token_activity AS target
    USING (
        SELECT
            token_address AS tc_definition_address,
            COALESCE(
                (
                    SELECT
                        e.address
                    FROM
                        events e
                    WHERE
                        e.kind = 1175
                        AND e.hidden = FALSE
                        AND e.deleted = FALSE
                        AND subzero_get_first_a_tag_value(e.tags, 31175) = token_address
                        AND subzero_get_tx_type(e.tags) = 'buy'
                    ORDER BY
                        e.lookup_created_at ASC
                    LIMIT 1
                ),
                NEW.address
            ) AS tc_first_action_address,
            to_timestamp_seconds(NEW.created_at) AS event_created_at,
            cfg.cfg_time_window,
            cfg.cfg_count_threshold
        FROM subzero_get_pn_token_activity_config() cfg
    ) AS source
    ON (target.tc_definition_address = source.tc_definition_address)
    WHEN MATCHED AND source.event_created_at < target.window_start_at THEN
        -- Event is outside the current window (too old).
        DO NOTHING
    WHEN MATCHED
        AND (target.last_notified_at IS NULL OR target.last_notified_at < target.window_start_at)
        AND (source.event_created_at >= target.window_end_at)
        AND (target.action_counter >= target.cfg_count_threshold)
    THEN
        -- Keep counting while pending, with some buffer after window end to avoid racing with the notification process.
        UPDATE SET
            window_end_at = (source.event_created_at / 180) * 180,
            action_counter = target.action_counter + 1
    WHEN MATCHED AND source.event_created_at >= target.window_end_at THEN
        -- Event is outside the current window, and no pending notification. Start a new window.
        UPDATE SET
            window_start_at = source.event_created_at,
            window_end_at   = source.event_created_at + target.cfg_time_window,
            action_counter  = 1
    WHEN MATCHED THEN
        -- Event is within the current window, and no pending notification. Just count.
        UPDATE SET
            action_counter = target.action_counter + 1
    WHEN NOT MATCHED AND EXISTS (
        SELECT 1
        FROM events e
        WHERE e.address = source.tc_definition_address
    ) THEN
        -- New record for a token with activity, but no existing window. Start a new window.
        INSERT (
            window_start_at,
            window_end_at,
            action_counter,
            cfg_time_window,
            cfg_count_threshold,
            tc_definition_address,
            tc_first_action_address
        ) VALUES (
            source.event_created_at,
            source.event_created_at + source.cfg_time_window,
            1,
            source.cfg_time_window,
            source.cfg_count_threshold,
            source.tc_definition_address,
            source.tc_first_action_address
        );

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
------
CREATE OR REPLACE TRIGGER trigger_events_after_insert_track_token_activity
AFTER INSERT ON events
FOR EACH ROW
WHEN (NEW.kind = 1175 AND NEW.hidden = FALSE AND NEW.deleted = FALSE AND subzero_get_tx_type(NEW.tags) = 'buy')
EXECUTE FUNCTION trigger_events_after_insert_track_token_activity();
