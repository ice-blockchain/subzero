-- SPDX-License-Identifier: ice License 1.0

CREATE OR REPLACE FUNCTION subzero_get_first_a_tag_value(tags jsonb, expected_kind INTEGER)
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
        tag->>0 = 'a' AND (expected_kind IS NULL OR starts_with(tag->>1, expected_kind::TEXT || ':'))
    ORDER BY
        tag->>1
    LIMIT 1;
$$;
--------
CREATE OR REPLACE FUNCTION subzero_get_tx_amount(tags jsonb, currency TEXT)
RETURNS NUMERIC
LANGUAGE sql
IMMUTABLE
RETURNS NULL ON NULL INPUT
AS $$
    SELECT
        CAST(tag->>1 AS NUMERIC) as tx_amount
    FROM
        jsonb_array_elements(tags) AS tag
    WHERE
        tag->>0 = 'tx_amount'
        AND tag->>2 = currency
    ORDER BY
        tag->>2
    LIMIT 1;
$$;
--------
CREATE TABLE IF NOT EXISTS pn_price_changes (
    id                          BIGSERIAL PRIMARY KEY,
    last_notified_at            BIGINT, -- unix timestamp in seconds.
    last_tc_action_timestamp    BIGINT, -- unix timestamp in seconds.
    cfg_time_window             INTEGER NOT NULL CHECK (cfg_time_window > 0 AND cfg_time_window <= 31622400), -- seconds, max 1 year.
    cfg_delta_percentage        INTEGER NOT NULL CHECK (-100 <= cfg_delta_percentage AND cfg_delta_percentage <= 100 AND cfg_delta_percentage != 0), -- percentage, can be negative for price drop.
    tc_definition_address       TEXT    NOT NULL,  -- Optional.
    user_master_pubkey          TEXT    NOT NULL,
    user_device_pubkey          TEXT    NOT NULL,
    user_device_uuid            TEXT    NOT NULL,
    request_event_id            TEXT    NOT NULL, -- Event ID of the request that created this record. Either DVM or Device registration event.
    last_tc_action_event_id     TEXT,
    request                     JSONB   NOT NULL,
    UNIQUE (tc_definition_address, user_device_pubkey)
) WITH (FILLFACTOR = 70);
------
CREATE INDEX idx_events_kind_1175_a_tag_lookup_created_at_desc ON events(kind, subzero_get_first_a_tag_value(tags, 31175), lookup_created_at DESC)
    WHERE
        kind=1175
        AND hidden=false;
------
